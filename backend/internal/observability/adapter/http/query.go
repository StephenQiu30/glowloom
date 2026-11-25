package http

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// Handler exposes the authenticated, read-only platform diagnostic projection.
type Handler struct {
	query *application.Query
	clock func() time.Time
}

// NewHandler injects the diagnostic use cases and filter clock.
func NewHandler(query *application.Query, clock func() time.Time) *Handler {
	return &Handler{query, clock}
}

// Register requires the existing email-session middleware; the use case rechecks DB authority.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/platform/diagnostics/overview", h.overview)
	group.GET("/platform/diagnostics/logs", h.logs)
	group.GET("/platform/diagnostics/logs/:id", h.detail)
}
func problem(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, domain.ErrInvalidFilter):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, domain.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, identity.ErrSessionInvalid):
		httpapi.WriteProblem(c, 401, "session_invalid", nil)
	case errors.Is(err, identity.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
	return true
}

// @Summary getDiagnosticsOverview
// @ID getDiagnosticsOverview
// @Tags diagnostics
// @Produce json
// @Param from query string false "Inclusive UTC RFC3339 time, default last 24h"
// @Param to query string false "Exclusive UTC RFC3339 time, maximum range 7d"
// @Param service query string false "Connected service: backend"
// @Param level query string false "info, warn or error"
// @Param request_id query string false "Exact request UUID"
// @Param trace_id query string false "Exact lowercase trace ID"
// @Success 200 {object} domain.Overview
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/platform/diagnostics/overview [get]
func (h *Handler) overview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := domain.ParseFilter(c.Request.URL.RawQuery, h.clock())
	if err != nil || c.Request.URL.Query().Has("limit") || c.Request.URL.Query().Has("cursor") {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	out, err := h.query.Overview(c.Request.Context(), identityhttp.Principal(c), f)
	if !problem(c, err) {
		c.JSON(200, out)
	}
}

// @Summary listDiagnosticLogs
// @ID listDiagnosticLogs
// @Tags diagnostics
// @Produce json
// @Param from query string false "Inclusive UTC RFC3339 time, default last 24h"
// @Param to query string false "Exclusive UTC RFC3339 time, maximum range 7d"
// @Param service query string false "Connected service: backend"
// @Param level query string false "info, warn or error"
// @Param request_id query string false "Exact request UUID"
// @Param trace_id query string false "Exact lowercase trace ID"
// @Param limit query int false "Page size 1..100, default 50"
// @Param cursor query string false "Signed actor/filter/snapshot cursor; expires after 10 minutes"
// @Success 200 {object} domain.Page
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/platform/diagnostics/logs [get]
func (h *Handler) logs(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, err := domain.ParseFilter(c.Request.URL.RawQuery, h.clock())
	if problem(c, err) {
		return
	}
	out, err := h.query.List(c.Request.Context(), identityhttp.Principal(c), f)
	if !problem(c, err) {
		c.JSON(200, out)
	}
}

// @Summary getDiagnosticLog
// @ID getDiagnosticLog
// @Tags diagnostics
// @Produce json
// @Param id path string true "Opaque diagnostic record UUID"
// @Success 200 {object} domain.Detail
// @Failure 401 {object} httpapi.Problem
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/platform/diagnostics/logs/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil || id.String() != c.Param("id") || c.Request.URL.RawQuery != "" {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	out, err := h.query.Detail(c.Request.Context(), identityhttp.Principal(c), id)
	if !problem(c, err) {
		c.JSON(200, out)
	}
}
