package httpprojects

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// NativeClosureHandler preserves the original lifecycle transport while using
// actual source admission, registered owner work and a frozen original response.
type NativeClosureHandler struct {
	service *source.NativeProjectClosure
	resolve NativeProjectResolver
}

// NewNativeClosureHandler requires the original credential/scope resolver.
func NewNativeClosureHandler(service *source.NativeProjectClosure, resolve NativeProjectResolver) (*NativeClosureHandler, error) {
	if service == nil || resolve == nil {
		return nil, writefence.ErrInvalid
	}
	return &NativeClosureHandler{service, resolve}, nil
}

// Register mounts only the implemented close controls, preserving original paths.
func (h *NativeClosureHandler) Register(group *gin.RouterGroup) {
	group.POST("/projects/:pid/archive", func(c *gin.Context) { h.transition(c, "archive") })
	group.DELETE("/projects/:pid", func(c *gin.Context) { h.transition(c, "delete") })
}
func (h *NativeClosureHandler) transition(c *gin.Context, action string) {
	var body struct {
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	project, projectErr := uuid.Parse(c.Param("pid"))
	key, keyErr := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if projectErr != nil || keyErr != nil || project == uuid.Nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
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
	details, err := h.service.ExecuteDetails(c.Request.Context(), credential, scope, source.CloseProjectInput{ProjectID: project, IdempotencyKey: key, ExpectedRevision: *body.ExpectedRevision, Action: action, RequestID: c.GetString("request_id")})
	if err != nil {
		switch {
		case errors.Is(err, source.ErrProjectClosureBusy), errors.Is(err, domain.ErrProjectHasInflightOperations):
			httpapi.WriteProblem(c, 409, "inflight_work", nil)
		case errors.Is(err, domain.ErrProjectRevisionConflict):
			httpapi.WriteProblem(c, 409, "revision_conflict", nil)
		case errors.Is(err, domain.ErrProjectStateConflict):
			httpapi.WriteProblem(c, 409, "state_conflict", nil)
		case errors.Is(err, writefence.ErrInvalid), errors.Is(err, domain.ErrInvalidProject):
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
		case errors.Is(err, source.ErrProjectClosureUnknown):
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
		case errors.Is(err, writefence.ErrFenced), errors.Is(err, writefence.ErrFrozen):
			httpapi.WriteProblem(c, 403, "forbidden", nil)
		default:
			WriteProjectProblem(c, err)
		}
		return
	}
	WriteProjectDetails(c, details)
}
