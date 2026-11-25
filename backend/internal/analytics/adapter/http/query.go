// Package http exposes read-only analysis and explicit unavailable capabilities.
package http

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/analytics/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/analytics/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// Handler exposes creation facts and no financial details without finance authority.
type Handler struct {
	query *application.Query
	clock func() time.Time
}

// NewHandler injects the read cases and clock.
func NewHandler(query *application.Query, clock func() time.Time) *Handler {
	return &Handler{query, clock}
}

// Register requires the existing session middleware; database authorization is repeated per read.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/analytics/creation", h.creation)
	group.GET("/analytics/business", h.business)
}
func problem(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, domain.ErrInvalidFilter):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, identity.ErrSessionInvalid):
		httpapi.WriteProblem(c, 401, "session_invalid", nil)
	case errors.Is(err, identity.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
	return true
}

// @Summary getCreationAnalytics
// @ID getCreationAnalytics
// @Tags analytics
// @Produce json
// @Param scope query string false "personal (default) or platform; current platform permission required"
// @Param from query string false "Inclusive RFC3339 accepted-operation time, default last 7d"
// @Param to query string false "Exclusive RFC3339 accepted-operation time, maximum range 31d"
// @Param project_id query string false "Exact accessible project UUID"
// @Param model_id query string false "Immutable model profile version UUID"
// @Param origin query string false "canvas, agent, pipeline, batch or system"
// @Success 200 {object} domain.Creation
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/analytics/creation [get]
func (h *Handler) creation(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := domain.ParseFilter(c.Request.URL.RawQuery, h.clock())
	if problem(c, err) {
		return
	}
	out, err := h.query.Creation(c.Request.Context(), identityhttp.Principal(c), f)
	if !problem(c, err) {
		c.JSON(200, out)
	}
}

// @Summary getBusinessAnalyticsCapability
// @ID getBusinessAnalyticsCapability
// @Tags analytics
// @Produce json
// @Success 200 {object} domain.Business
// @Failure 401 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/analytics/business [get]
func (h *Handler) business(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	out, err := h.query.Business(c.Request.Context(), identityhttp.Principal(c))
	if !problem(c, err) {
		c.JSON(200, out)
	}
}
