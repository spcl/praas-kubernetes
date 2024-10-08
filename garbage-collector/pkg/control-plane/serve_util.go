package control_plane

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"praas/pkg/networking"
)

const (
	requestTimeout = time.Minute
)

func handleServerError(writer http.ResponseWriter, err error) {
	sanitizedErr := strings.ReplaceAll(err.Error(), "\"", "'")
	_, _ = writer.Write([]byte(`{"success": false, "reason": "` + sanitizedErr + `"}`))
}

func handleBadRequest(writer http.ResponseWriter, reason string) {
	writer.WriteHeader(400)
	var resp networking.ResponseBase
	resp.Success = false
	resp.Reason = "Bad Request: " + reason
	bytes, err := json.Marshal(resp)
	if err == nil {
		_, _ = writer.Write(bytes)
	}
}

func unknownMethod(request *http.Request) string {
	return "Unknown HTTP Method: " + request.Method
}
