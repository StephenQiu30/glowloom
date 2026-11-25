package http

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// AuthenticateSocket rechecks original cookies and CSRF without recording user
// activity or writing HTTP headers after the connection has been upgraded.
func (h *EmailAuthHandler) AuthenticateSocket(ctx context.Context, r *http.Request, csrf string) (application.Principal, error) {
	c := &gin.Context{Request: r.Clone(ctx)}
	result, err := h.authenticateRequest(c, application.NoActivity)
	if err != nil {
		return application.Principal{}, err
	}
	if h.origin == "" || r.Header.Get("Origin") != h.origin || csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(result.CSRFToken)) != 1 {
		return application.Principal{}, application.ErrForbidden
	}
	h.setPrincipal(c, result)
	return Principal(c), nil
}
