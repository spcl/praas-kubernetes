package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/websocket"
	"praas/pkg/control_plane/store"
	"praas/pkg/types"
)

var (
	unsupportedMethod = errors.New("unsupported HTTP method")
	negativeFIDErr    = errors.New("function ID cannot be negative")
	functionRunning   = errors.New("function with given ID is already running")
	resultReadError   = errors.New("failed to get result from function")
)

type invokeHandler struct {
	objectHandlerBase
	connections    map[string]*ProcessClient
	processHandler processHandler
}

var _ K8APIObjectHandler = (*invokeHandler)(nil)

func NewInvokeHandler(ctx context.Context) K8APIObjectHandler {
	obj := &invokeHandler{
		objectHandlerBase: buildBase(ctx),
		connections:       make(map[string]*ProcessClient),
		processHandler:    pHandler,
	}
	return obj
}

func (i *invokeHandler) Post(ctx context.Context, input map[string]any) (any, error) {
	i.logger.Info("Recived post request to /invoke")
	ctx = context.Background()
	// Parse input
	var appID types.IDType
	var cpu int
	var functionInput FuncExecCMD
	functionInput.CPU = &cpu
	err := parseData(
		input,
		asID(appIDKey, &appID),
		asInt("id", &functionInput.ID),
		asString("function", &functionInput.Function),
		asStringArray("args", &functionInput.Args),
		asOptionalInt("cpu", functionInput.CPU, 1),
		asOptionalString("mem", &functionInput.Mem, "500MiB"),
	)
	if err != nil {
		i.logger.Errorw("Failed to parse input", zap.Error(err))
		return nil, err
	}
	if functionInput.ID < 0 {
		return nil, negativeFIDErr
	}

	i.logger.Info("Function invocation request:", functionInput)

	// Get a connection to a suitable process
	process, err := i.selectProcess(ctx, appID, *functionInput.CPU, functionInput.Mem)
	if err != nil {
		return nil, err
	}
	conn, err := i.connectToProcess(process.String())
	if err != nil {
		i.logger.Errorw("Failed to connect to process", zap.Error(err))
		return nil, err
	}

	// Invoke function
	err = conn.ExecuteFunction(&functionInput)
	if err != nil {
		i.logger.Errorw("Failed to invoke function", zap.Error(err), zap.Any("input", functionInput))
		return nil, err
	}

	// Listen for result
	result, err := conn.GetResult(functionInput.ID)
	err = i.store.DeAllocFunc(ctx, appID, process.PID)
	if err != nil {
		i.logger.Errorw("Failed to deallocate function from process", zap.Error(err))
	}

	return map[string]any{
		"result": result,
	}, nil
}

func (i *invokeHandler) connectToProcess(name string) (*ProcessClient, error) {
	if conn, exists := i.connections[name]; exists {
		return conn, nil
	}

	// Get IP address
	ip, err := i.processHandler.GetPodIP(name)
	if err != nil {
		return nil, err
	}

	origin, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	origin = "http://" + origin

	clusterAddress := "ws://" + ip + ":8080/data-plane"

	var conn *websocket.Conn
	connected := false
	for !connected {
		conn, err = websocket.Dial(clusterAddress, "", origin)
		if err != nil {
			// logger.Errorw("Could not connect to process", zap.Error(err))
			time.Sleep(100 * time.Millisecond)
		} else {
			connected = true
		}
	}
	if err != nil {
		return nil, err
	}
	conn.MaxPayloadBytes = 1 << 30 // 1 GiB

	client := newProcessClient(
		conn, func() {
			delete(i.connections, name)
		},
	)
	i.connections[name] = client
	return client, nil
}

func (i *invokeHandler) Delete(ctx context.Context, input map[string]any) (any, error) {
	return nil, unsupportedMethod
}

func (i *invokeHandler) Get(ctx context.Context, query url.Values) (any, error) {
	return nil, unsupportedMethod
}

func (i *invokeHandler) selectProcess(
	ctx context.Context,
	appID types.IDType,
	cpu int,
	mem string,
) (types.ProcessRef, error) {
	var pData types.ProcessData
	// Get current processes
	selectedProc, inactiveProcs, err := i.store.GetProcessForFunc(ctx, appID, i.cfg.FuncLimit)
	i.logger.Infof("Selected proc: %s, inactive procs: %v, err: %v", selectedProc, inactiveProcs, err)
	if err != nil {
		if !errors.Is(err, store.NoSuchProcess) {
			return types.InvalidProcess(), err
		}
	} else {
		return types.ProcessRef{AppID: appID, PID: selectedProc}, nil
	}

	// Try to swap an inactive process in
	for _, pid := range inactiveProcs {
		pData.PID = pid
		ref, err := i.processHandler.CreateProcess(ctx, appID, pData)
		if err == nil {
			err = i.store.AllocFunc(ctx, appID, pid)
			if err == nil {
				return ref, nil
			}
		}
	}

	// Could not swap inactive process in, create a new process
	pData.PID = ""
	processRef, err := i.processHandler.CreateProcess(ctx, appID, pData)
	if err != nil {
		return types.InvalidProcess(), err
	}
	err = i.store.AllocFunc(ctx, appID, processRef.PID)
	return processRef, err
}

type ProcessClient struct {
	conn     *websocket.Conn
	messages map[int]chan processResponse
	lock     sync.RWMutex
}

type FuncExecCMD struct {
	ID       int      `json:"id"`
	Function string   `json:"function"`
	Args     []string `json:"args"`
	Mem      string   `json:"mem,omitempty"`
	CPU      *int     `json:"cpu,omitempty"`
}

type processResponse struct {
	Success   bool   `json:"success"`
	ID        *int   `json:"func-id,omitempty"`
	Operation string `json:"operation,omitempty"`
	Result    string `json:"result,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
}

func newProcessClient(conn *websocket.Conn, onDisconnect func()) *ProcessClient {
	messageDirectory := make(map[int]chan processResponse)
	client := &ProcessClient{
		conn:     conn,
		messages: messageDirectory,
	}

	go func() {
		var response processResponse

		for true {
			err := websocket.JSON.Receive(conn, &response)
			if err != nil {
				onDisconnect()
				_ = client.Disconnect()
				return
			}

			fID := -1
			if response.ID != nil {
				fID = *response.ID
			}

			// Is there a listener?
			client.lock.RLock()
			if outChan, exists := messageDirectory[fID]; exists {
				outChan <- response
			}
			client.lock.RUnlock()
		}
	}()

	return client
}

func (r processResponse) String() string {
	msg := fmt.Sprintf("{success: %t", r.Success)
	if r.ID != nil {
		msg += fmt.Sprintf(", func-id: %d", *r.ID)
	}
	if r.Operation != "" {
		msg += fmt.Sprintf(", operation: %s", r.Operation)
	}
	if r.Reason != "" {
		msg += fmt.Sprintf(", reason: %s", r.Reason)
	}
	if r.Result != "" {
		msg += fmt.Sprintf(", result: %s", r.Result)
	}
	if r.Message != "" {
		msg += fmt.Sprintf(", msg: %s", r.Message)
	}
	msg += "}"
	return msg
}

func (d *ProcessClient) Disconnect() error {
	return d.conn.Close()
}

func (d *ProcessClient) ExecuteFunction(cmd *FuncExecCMD) error {
	// Start listening for messages from this function
	d.lock.Lock()
	defer d.lock.Unlock()
	_, exists := d.messages[cmd.ID]
	if exists {
		return functionRunning
	}
	d.messages[cmd.ID] = make(chan processResponse)

	// Start the function
	return websocket.JSON.Send(d.conn, cmd)
}

func (d *ProcessClient) GetResult(id int) (string, error) {
	hasFinished := false
	var response processResponse

	// Get the message channel
	d.lock.RLock()
	messageChan, exists := d.messages[id]
	d.lock.RUnlock()
	if !exists {
		return "", resultReadError
	}

	// Stop listening after this is done
	defer func() {
		d.lock.Lock()
		delete(d.messages, id)
		d.lock.Unlock()
	}()

	// Wait for the finish message
	for !hasFinished {
		response = <-messageChan

		if !response.Success {
			return "", fmt.Errorf("failed to get result for %d, because %s", id, response.Reason)
		}

		hasFinished = response.Operation == "run"
	}

	return response.Result, nil
}
