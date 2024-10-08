package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"praas/pkg/config"
	"praas/pkg/control_plane"
	"praas/pkg/logging"
)

const (
	reqTimeout = time.Minute
)

var (
	unknownHTTPMethodError = errors.New("bad request: unknown HTTP Method")
)

func NewControlPlaneServer(ctx context.Context) *http.Server {
	cfg := config.GetPraasConfig(ctx)
	appHandler := control_plane.NewApplicationHandler(ctx)
	processHandler := control_plane.NewProcessHandler(ctx)
	invokeHandler := control_plane.NewInvokeHandler(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/application", requestHandler(ctx, appHandler))
	mux.HandleFunc("/process", requestHandler(ctx, processHandler))
	mux.HandleFunc("/invoke", requestHandler(ctx, invokeHandler))

	return &http.Server{
		Addr:    cfg.ServePort,
		Handler: mux,
	}
}

func requestHandler(ctx context.Context, handler control_plane.K8APIObjectHandler) func(
	http.ResponseWriter,
	*http.Request,
) {
	logger := logging.FromContext(ctx)
	return func(writer http.ResponseWriter, httpRequest *http.Request) {
		// Setup context for this request
		reqCtx, cancel := context.WithTimeout(context.Background(), reqTimeout)
		defer cancel()

		var req Request
		if httpRequest.ContentLength > 0 {
			decoder := json.NewDecoder(httpRequest.Body)
			decoder.DisallowUnknownFields()
			err := decoder.Decode(&req)
			if err != nil {
				handleServerError(writer, fmt.Errorf("failed to decode httpRequest (wrong format?)"))
				return
			}
		}

		// Handle request
		var response Response
		var err error
		var data any
		switch httpRequest.Method {
		case http.MethodGet:
			data, err = handler.Get(reqCtx, httpRequest.URL.Query())
		case http.MethodPost:
			data, err = handler.Post(reqCtx, req)
		case http.MethodDelete:
			data, err = handler.Delete(reqCtx, req)
		default:
			err = unknownHTTPMethodError
		}
		if err != nil {
			response.Success = false
			response.Reason = err.Error()
		} else {
			response.Success = true
			response.Data = data
		}

		// Send response
		bytes, err := json.Marshal(response)
		if err != nil {
			logger.Errorw("Failed to marshal response", zap.Error(err))
			handleServerError(writer, fmt.Errorf("internal server error"))
			return
		}
		writer.Write(bytes)
	}
}

func handleServerError(writer http.ResponseWriter, err error) {
	writer.WriteHeader(500)

	var response Response
	response.Success = false
	response.Reason = err.Error()
	bytes, err := json.Marshal(response)
	if err != nil {
		writer.Write([]byte("internal server error (" + err.Error() + ")"))
	}
	writer.Write(bytes)
}
