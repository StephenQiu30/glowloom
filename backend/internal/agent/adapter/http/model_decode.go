package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// decodeModel reserves a larger, still bounded internal transcript envelope.
// Public JSON and all tool/control inputs retain the ordinary 1MiB limit.
func decodeModel(c *gin.Context, out any) bool {
	return decodeBounded(c, out, domain.MaxModelRequestBytes)
}

func decodeConversation(c *gin.Context, out any) bool { return decodeBounded(c, out, 2<<20) }

func decodeBounded(c *gin.Context, out any, maxBytes int64) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		httpapi.WriteProblem(c, 415, "unsupported_media_type", nil)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpapi.WriteProblem(c, 413, "request_too_large", nil)
		} else {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
		}
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return false
	}
	return true
}
