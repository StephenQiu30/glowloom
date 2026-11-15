// Package httpprojects adapts the actual Creation project source to its public API.
package httpprojects

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// NativeProjectResolver extracts the original credential/CSRF and selected logical
// scope after the public Cookie/Origin policy. It supplies no actor or role.
// Scope selection remains an untrusted resource reference until source policy.
type NativeProjectResolver func(*gin.Context) (workspacesource.Credential, uuid.UUID, error)

// NativeProjectHandler exposes the actual source-preparation create application.
// Its group must carry the original Cookie/Origin/CSRF transport policy.
type NativeProjectHandler struct {
	create  *workspacesource.NativeProjectCreation
	resolve NativeProjectResolver
}

// NewNativeProjectHandler refuses an absent native use case or credential boundary.
func NewNativeProjectHandler(create *workspacesource.NativeProjectCreation, resolve NativeProjectResolver) (*NativeProjectHandler, error) {
	if create == nil || resolve == nil {
		return nil, workspacesource.ErrProjectDependencyUnavailable
	}
	return &NativeProjectHandler{create, resolve}, nil
}

// Register retains the two existing public mutation paths and their 201 shape.
// Navigation/read routes remain separately owned by their actual query handlers.
func (h *NativeProjectHandler) Register(group *gin.RouterGroup) {
	group.POST("/projects", h.Create)
	group.POST("/independent-canvases", h.CreateIndependentCanvas)
}

// Create returns success only after the original budget and all writer ACKs.
func (h *NativeProjectHandler) Create(c *gin.Context) { h.execute(c, false) }

// CreateIndependentCanvas uses the same actual source protocol and stable canvas.
func (h *NativeProjectHandler) CreateIndependentCanvas(c *gin.Context) { h.execute(c, true) }

func (h *NativeProjectHandler) execute(c *gin.Context, independent bool) {
	var body CreateProjectRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	if h == nil || h.create == nil || h.resolve == nil {
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
		return
	}
	credential, scope, err := h.resolve(c)
	if c.IsAborted() {
		return
	}
	if err != nil {
		WriteProjectProblem(c, err)
		return
	}
	input := workspacesource.CreateProjectInput{IndependentCanvas: independent, StyleKey: body.StyleKey, Name: body.Name, Description: body.Description, AspectRatio: body.AspectRatio, StyleType: body.StyleType, StyleSubtype: body.StyleSubtype, RequestID: c.GetString("request_id")}
	if body.StylePresetID != nil {
		if *body.StylePresetID == uuid.Nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.StylePresetID = *body.StylePresetID
	}
	created, err := h.create.Execute(c.Request.Context(), credential, scope, key, input)
	if err != nil {
		WriteProjectProblem(c, err)
		return
	}
	c.JSON(201, created)
}

// WriteProjectProblem preserves public credential, scope and mutation problem codes.
func WriteProjectProblem(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspacesource.ErrSourceCSRFInvalid):
		httpapi.WriteProblem(c, 403, "csrf_invalid", nil)
	case errors.Is(err, workspacesource.ErrSourceAccessExpired):
		httpapi.WriteProblem(c, 401, "access_expired", nil)
	case errors.Is(err, workspacesource.ErrSourceSessionInvalid):
		httpapi.WriteProblem(c, 401, "session_invalid", nil)
	case errors.Is(err, workspacesource.ErrSourceForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, workspacesource.ErrProjectPreparationConflict), errors.Is(err, writefence.ErrConflict):
		httpapi.WriteProblem(c, 409, "idempotency_key_reused", nil)
	case errors.Is(err, workspacesource.ErrScopeNotFound), errors.Is(err, workspacesource.ErrProjectNotFound), errors.Is(err, workspacesource.ErrStylePresetNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, workspacesource.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_key_reused", nil)
	case errors.Is(err, workspacesource.ErrStylePresetMismatch), errors.Is(err, workspacesource.ErrInvalidCreateProject):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
