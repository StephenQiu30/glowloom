// Package httpapi provides the public JSON transport protocol and request security.
package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

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

// Middleware owns request IDs, public panic recovery and write Origin/CSRF/key checks.
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
		if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch || c.Request.Method == http.MethodDelete {
			if c.GetHeader("Origin") != origin || origin == "" {
				WriteProblem(c, 403, "origin_forbidden", nil)
				return
			}
			if c.Request.URL.Path != "/api/auth/login" {
				session, _ := c.Cookie("lv_session")
				token, _ := c.Cookie("lv_csrf")
				if session == "" {
					WriteProblem(c, 401, "unauthenticated", nil)
					return
				}
				if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(c.GetHeader("X-CSRF-Token"))) != 1 || subtle.ConstantTimeCompare([]byte(token), []byte(CSRFToken(session))) != 1 {
					WriteProblem(c, 403, "csrf_invalid", nil)
					return
				}
				key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
				if err != nil || key == uuid.Nil {
					WriteProblem(c, 422, "invalid_idempotency_key", nil)
					return
				}
			}
		}
		c.Next()
	}
}

// CSRFToken binds the readable double-submit cookie to its opaque session.
func CSRFToken(session string) string {
	hash := sha256.Sum256([]byte("lanverse/csrf/" + session))
	return hex.EncodeToString(hash[:])
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

// Token returns only the session cookie; Authorization headers are not accepted.
func Token(c *gin.Context) string {
	token, _ := c.Cookie("lv_session")
	return strings.TrimSpace(token)
}
