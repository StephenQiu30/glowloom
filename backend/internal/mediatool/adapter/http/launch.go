package http

import (
	"net/http"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/gin-gonic/gin"
)

// RegisterLaunch installs a capability-only native download, without weakening
// any session-protected export route. It accepts no identity or arbitrary URL.
func (h *Handler) RegisterLaunch(g *gin.RouterGroup) { g.GET("/media-export-launch", h.Launch) }
func (h *Handler) Launch(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	query := c.Request.URL.Query()
	if len(query) != 1 || len(query["token"]) != 1 {
		httpapi.WriteProblem(c, 404, "not_found", nil)
		return
	}
	body, e := h.query.Launch(c.Request.Context(), query.Get("token"))
	if e != nil {
		fail(c, e)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}
