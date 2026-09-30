// Package httpapi provides the public JSON transport protocol and request security.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Problem is the public RFC 9457 error contract; internal errors never become detail.
type Problem struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    int            `json:"status"`
	Code      string         `json:"code"`
	Detail    string         `json:"detail"`
	RequestID string         `json:"request_id"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// WriteProblem aborts a request with the stable error and request identifier.
func WriteProblem(c *gin.Context, status int, code string, meta map[string]any) {
	problem := Problem{Type: "https://lanverse.local/errors/" + code, Title: http.StatusText(status), Status: status, Code: code, Detail: http.StatusText(status), RequestID: c.GetString("request_id"), Meta: meta}
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(status, problem)
}

// Middleware owns request IDs, public panic recovery and write Origin/key checks.
func Middleware(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if _, err := uuid.Parse(id); err != nil {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-Id", id)
		defer func() {
			if recover() != nil && !c.Writer.Written() {
				WriteProblem(c, http.StatusInternalServerError, "internal_error", nil)
			}
		}()
		if c.FullPath() == "" {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch || c.Request.Method == http.MethodDelete {
			if c.GetHeader("Origin") != origin || origin == "" {
				WriteProblem(c, 403, "origin_forbidden", nil)
				return
			}
			key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
			if err != nil || key == uuid.Nil {
				WriteProblem(c, 422, "invalid_idempotency_key", nil)
				return
			}
		}
		c.Next()
	}
}

// Decode accepts exactly one JSON object, unknown fields are rejected and bodies are bounded.
func Decode(c *gin.Context, out any) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		WriteProblem(c, 415, "unsupported_media_type", nil)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteProblem(c, 413, "request_too_large", nil)
		} else {
			WriteProblem(c, 422, "invalid_request", nil)
		}
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		WriteProblem(c, 422, "invalid_request", nil)
		return false
	}
	return true
}
