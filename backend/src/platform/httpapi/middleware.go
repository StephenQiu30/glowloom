package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type requestIDKey struct{}

func RequestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return value
	}
	return r.Header.Get("X-Request-Id")
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(r.Context(), "http panic recovered", "request_id", RequestID(r))
				WriteError(w, r, NewError(StatusInternalServerError, CodeInternalError, "服务暂时不可用", "稍后重试；如问题持续请提供 request_id"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		WriteError(w, r, NewError(StatusBadRequest, CodeInvalidJSON, "请求体不是有效的当前 JSON 合同", "按照 OpenAPI 请求模型重新提交"))
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		WriteError(w, r, NewError(StatusBadRequest, CodeInvalidJSON, "请求体只能包含一个 JSON 文档", "删除请求体中的多余内容后重试"))
		return false
	}
	return true
}
