package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Envelope[T any] struct {
	Data  T         `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

// ErrorEnvelope 是 Swagger 注释使用的稳定错误响应模型。
type ErrorEnvelope struct {
	Error *APIError `json:"error"`
}

func WriteData(w http.ResponseWriter, status HTTPStatus, data any) {
	WriteJSON(w, status, Envelope[any]{Data: data})
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := From(err)
	if requestID := RequestID(r); requestID != "" {
		apiErr.RequestID = requestID
	}
	if apiErr.RetryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(apiErr.RetryAfterSeconds))
	}
	WriteJSON(w, apiErr.Status, Envelope[any]{Error: apiErr})
}

func WriteJSON(w http.ResponseWriter, status HTTPStatus, value any) {
	w.Header().Set("Content-Type", "application/json")
	if requestID := w.Header().Get("X-Request-Id"); requestID != "" {
		w.Header().Set("X-Request-Id", requestID)
	}
	w.WriteHeader(status.Int())
	_ = json.NewEncoder(w).Encode(value)
}
