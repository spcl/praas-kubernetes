package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	failedError = errors.New("request unexpectedly failed")
	emptyError  = errors.New("no pods received")
)

type Response map[string]any

type PraasClient interface {
	CreateApp() (int, error)
	DeleteApp(int) error
	GetScaleForApp(int, bool) (int, error)
	GetPodsForApp(int) ([]string, error)
	CreateProcess(int) (string, string, error)
	CreateProcesses(int, int) ([]string, []string, error)
	SwapProcessIn(int, string) (string, error)
}

type Clients struct {
	HTTP      *http.Client
	HTTPPraas *RestPraasClient
	Praas     PraasClient
	Proxy     ProxyClient
	Kube      kubernetes.Interface
}

func NewTestClients(kubeConfig *rest.Config) *Clients {
	kubeClient := kubernetes.NewForConfigOrDie(kubeConfig)
	ctx := context.Background()

	c := kubeClient.RESTClient()
	c.Get().Namespace("default").Resource("pods").Name("")

	var namespace string
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, "knative-serving", metav1.GetOptions{})
	if err != nil {
		namespace = "praas"
	} else if ns == nil {
		namespace = "praas"
	} else {
		namespace = "knative-serving"
	}

	svc, err := kubeClient.CoreV1().Services(namespace).Get(ctx, "praas-controller", metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	pathBase := svc.Status.LoadBalancer.Ingress[0].Hostname
	if pathBase == "" {
		pathBase = svc.Status.LoadBalancer.Ingress[0].IP
	}
	controlPlaneAddr := pathBase + ":8080"
	fmt.Println("Created praas client with address: " + controlPlaneAddr)
	httpClient := http.DefaultClient
	restClient := &RestPraasClient{addr: controlPlaneAddr, client: httpClient}
	return &Clients{
		Kube:      kubeClient,
		HTTP:      httpClient,
		HTTPPraas: restClient,
		Praas:     newPraasClient(restClient, kubeClient),
		Proxy:     newProxyClient(kubeClient),
	}
}

type PraasHTTPRequest struct {
	client  *http.Client
	address string
	Method  string
	body    string
}

type RestPraasClient struct {
	addr   string
	client *http.Client
}

func (r *RestPraasClient) App() *PraasHTTPRequest {
	return &PraasHTTPRequest{client: r.client, address: "http://" + r.addr + "/application"}
}

func (r *RestPraasClient) Process() *PraasHTTPRequest {
	return &PraasHTTPRequest{client: r.client, address: "http://" + r.addr + "/process"}
}

func (r *PraasHTTPRequest) Create() *PraasHTTPRequest {
	r.Method = http.MethodPost
	return r
}

func (r *PraasHTTPRequest) Delete() *PraasHTTPRequest {
	r.Method = http.MethodDelete
	return r
}

func (r PraasHTTPRequest) Do(body string) (Response, error) {
	return sendRequest(r.client, r.address, r.Method, body)
}

func sendRequest(client *http.Client, address, method, body string) (Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, address, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	var response map[string]any
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	err = json.Unmarshal(buf.Bytes(), &response)
	if err != nil {
		return nil, fmt.Errorf("response decode error: %s (\"%s\")", err.Error(), buf.String())
	}

	return response, nil
}

type praasImpl struct {
	restClient *RestPraasClient
	kube       kubernetes.Interface
}

var _ PraasClient = (*praasImpl)(nil)

func newPraasClient(c *RestPraasClient, k kubernetes.Interface) PraasClient {
	return &praasImpl{c, k}
}

func (p *praasImpl) GetPodsForApp(pid int) ([]string, error) {
	pods, err := p.listPods(pid, true)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names, nil
}

func (p *praasImpl) CreateApp() (int, error) {
	response, err := p.restClient.App().Create().Do("{}")
	if err != nil {
		return -1, err
	}

	// Make sure we get a success response
	if !isSuccess(response) {
		return -1, fmt.Errorf("failed to create new process. See response: %v", response)
	}

	return getAppID(response)
}

func (p *praasImpl) DeleteApp(pid int) error {
	body := fmt.Sprintf(`{"app-id":%d}`, pid)
	response, err := p.restClient.App().Delete().Do(body)
	if err != nil {
		return err
	}

	if !isSuccess(response) {
		return fmt.Errorf("failed to delete app %d. See response: %v", pid, response)
	}

	return nil
}

func (p *praasImpl) CreateProcess(pid int) (string, string, error) {
	body := fmt.Sprintf(`{"app-id":%d,"processes":[{}]}`, pid)
	response, err := p.restClient.Process().Create().Do(body)
	if err != nil {
		return "", "", err
	}

	if !isSuccess(response) {
		return "", "", fmt.Errorf("failed to create new session. See response: %v", response)
	}

	id, err := getPID(response)
	if err != nil {
		panic(err)
	}

	pod, err := getPod(response)
	if err != nil {
		panic(err)
	}
	if pod == "" {
		panic("Empty pod name in response")
	}

	return id, pod, nil
}

func (p *praasImpl) CreateProcesses(appID, count int) ([]string, []string, error) {
	var pids []string
	var podNames []string
	processesStr := ""
	for i := 0; i < count; i++ {
		processesStr += "{}"
		if i != count-1 {
			processesStr += ","
		}
	}
	body := fmt.Sprintf(`{"app-id":%d,"processes":[%s]}`, appID, processesStr)
	response, err := p.restClient.Process().Create().Do(body)
	if err != nil {
		return pids, podNames, err
	}

	if !isSuccess(response) {
		return pids, podNames, fmt.Errorf("failed to create new session. See response: %v", response)
	}

	pidData, err := GetArrayDataField[string](response, "processes", "pid")
	if err != nil {
		return pids, podNames, err
	}
	podData, err := GetArrayDataField[string](response, "processes", "pod_name")
	if err != nil {
		return pids, podNames, err
	}

	for i := 0; i < count; i++ {
		pids = append(pids, pidData[i])
		podNames = append(podNames, podData[i])
	}

	return pids, podNames, nil
}

func (p *praasImpl) SwapProcessIn(appID int, pid string) (string, error) {
	body := fmt.Sprintf(`{"app-id":%d,"processes":[{"pid":"%s"}]}`, appID, pid)
	response, err := p.restClient.Process().Create().Do(body)
	if err != nil {
		return "", err
	}
	fmt.Println(response)

	if !isSuccess(response) {
		return "", fmt.Errorf("failed to swap in session. See response: %v", response)
	}

	pod, err := getPod(response)
	if err != nil {
		return "", err
	}

	return pod, nil
}

func getSingleArrayElem(response Response, field string) (string, error) {
	elems, err := GetArrayDataField[string](response, "processes", field)
	if err != nil {
		return "", err
	}
	if len(elems) != 1 {
		return "", fmt.Errorf("unexpected length (%d) for field: %s", len(elems), field)
	}
	return elems[0], nil
}

func getPod(response Response) (string, error) {
	return getSingleArrayElem(response, "pod_name")
}

func isSuccess(response Response) bool {
	result, err := GetField[bool](response, "success")
	if err != nil {
		panic(err)
	}
	return result
}

func getAppID(response Response) (int, error) {
	fPid, err := GetDataField[float64](response, "app-id")
	if err != nil {
		return -1, err
	}
	return int(fPid), nil
}

func getPID(response Response) (string, error) {
	return getSingleArrayElem(response, "pid")
}

func (p *praasImpl) GetScaleForApp(AppID int, real bool) (int, error) {
	pods, err := p.listPods(AppID, real)
	if err != nil {
		if errors.Is(err, emptyError) {
			return 0, nil
		}
		return -1, err
	}

	return len(pods), nil
}

func (p *praasImpl) listPods(pid int, filter bool) ([]v1.Pod, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	selector := fmt.Sprintf("praas.process/app-id=%d", pid)
	podList, err := p.kube.CoreV1().Pods("default").List(
		ctx,
		metav1.ListOptions{LabelSelector: selector},
	)

	if err != nil {
		return nil, err
	}

	if podList == nil || podList.Items == nil {
		return nil, emptyError
	}

	if !filter {
		return podList.Items, nil
	}

	// Filter out pods that are not running or will be deleted
	var runningPods []v1.Pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == v1.PodRunning && pod.DeletionTimestamp == nil {
			runningPods = append(runningPods, pod)
		}
	}

	return runningPods, nil
}

type ProxyClient interface {
	SendRequest(podName, endpoint string) (string, error)
}

type proxyImpl struct {
	client     rest.Interface
	apiSrvAddr string
}

var _ ProxyClient = (*proxyImpl)(nil)

func newProxyClient(kube *kubernetes.Clientset) ProxyClient {
	return &proxyImpl{
		client:     kube.RESTClient(),
		apiSrvAddr: "https://127.0.0.1:33883",
	}
}

func (p *proxyImpl) SendRequest(podName, endpoint string) (string, error) {
	p.waitForPodReady(podName)
	req := p.client.
		Get().
		Prefix("api", "v1").
		Namespace("default").
		Resource("pods").
		Name(podName).
		SubResource("proxy").
		Suffix(endpoint)
	// fmt.Println("Send request to: ", req.URL())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resBytes, err := req.Do(ctx).Raw()
	return string(resBytes), err
}

func (p *proxyImpl) waitForPodReady(podName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	deadline := time.Now().Add(5 * time.Second)
	isReady := false
	var pod v1.Pod
	for !isReady && time.Now().Before(deadline) {
		err := p.client.
			Get().
			Prefix("api", "v1").
			Namespace("default").
			Resource("pods").
			Name(podName).
			Do(ctx).
			Into(&pod)
		if err == nil && pod.Status.PodIP != "" && pod.Status.ContainerStatuses[0].Ready {
			isReady = true
		} else if !(errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)) {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if !isReady {
		fmt.Printf("Deadline reached waiting for pod: %s\n", podName)
	}
}
