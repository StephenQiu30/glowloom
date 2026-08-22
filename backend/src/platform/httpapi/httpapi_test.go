package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorUsesStableEnvelopeAndRequestID(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, NewError(StatusConflict, CodeConflict, "当前资源已被更新", "重新读取当前版本后重试"))
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/example", strings.NewReader(`{}`))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"conflict"`) || !strings.Contains(recorder.Body.String(), `"request_id"`) {
		t.Fatalf("response = %s", recorder.Body.String())
	}
}

func TestDecodeJSONRejectsUnknownAndTrailingDocuments(t *testing.T) {
	for _, body := range []string{`{"unexpected":true}`, `{"name":"ok"}{"extra":true}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/example", strings.NewReader(body))
		if DecodeJSON(recorder, request, &struct {
			Name string `json:"name"`
		}{}, 1<<20) {
			t.Fatalf("DecodeJSON accepted invalid body %s", body)
		}
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"invalid_json"`) {
			t.Fatalf("response = %s", recorder.Body.String())
		}
	}
}

func TestRecoverMiddlewareHidesPanic(t *testing.T) {
	handler := RecoverMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(errors.New("secret stack")) }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "secret stack") {
		t.Fatalf("panic response = %s", recorder.Body.String())
	}
}
