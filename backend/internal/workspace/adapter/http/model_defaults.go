package http

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ModelDefaultsHandler adapts the project's canonical defaults without granting catalog management.
type ModelDefaultsHandler struct {
	service *application.ModelDefaultsService
}

// NewModelDefaultsHandler injects project-scoped read and update use cases.
func NewModelDefaultsHandler(service *application.ModelDefaultsService) *ModelDefaultsHandler {
	return &ModelDefaultsHandler{service: service}
}

// Register installs project-default reads and guarded writes in the existing API group.
func (h *ModelDefaultsHandler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid/model-defaults", h.Read)
	group.PATCH("/projects/:pid/model-defaults", h.Save)
}

// ModelDefaultsRequest replaces the project map relative to its observed revision.
type ModelDefaultsRequest struct {
	ExpectedRevision *int64            `json:"expected_revision" binding:"required"`
	DefaultModels    map[string]string `json:"default_models" binding:"required" swaggertype:"object,string"`
}

// Read returns the same defaults used when a generation has no explicit model.
// @Summary 项目默认模型
// @ID getProjectModelDefaults
// @Tags settings
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {object} application.ModelDefaults
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/model-defaults [get]
func (h *ModelDefaultsHandler) Read(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("pid"))
	if err != nil || projectID == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	result, err := h.service.Read(c.Request.Context(), identityhttp.Principal(c), projectID)
	if err != nil {
		writeDefaultsError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Save updates defaults and advances the project revision once per durable request.
// @Summary 保存项目默认模型
// @ID saveProjectModelDefaults
// @Tags settings
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ModelDefaultsRequest true "项目默认模型与读取时修订"
// @Success 200 {object} application.ModelDefaults
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/model-defaults [patch]
func (h *ModelDefaultsHandler) Save(c *gin.Context) {
	var body ModelDefaultsRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	projectID, projectErr := uuid.Parse(c.Param("pid"))
	key, keyErr := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if projectErr != nil || projectID == uuid.Nil || keyErr != nil || key == uuid.Nil || body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	result, err := h.service.Save(c.Request.Context(), identityhttp.Principal(c), application.ModelDefaultsChange{
		ProjectID: projectID, ExpectedRevision: *body.ExpectedRevision, DefaultModels: body.DefaultModels,
		IdempotencyKey: key, RequestID: c.GetString("request_id"),
	})
	if err != nil {
		writeDefaultsError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

func writeDefaultsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrProjectNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, domain.ErrProjectRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, domain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "project_archived", nil)
	case errors.Is(err, application.ErrDefaultModelUnavailable):
		httpapi.WriteProblem(c, 409, "catalog_unavailable", nil)
	case errors.Is(err, application.ErrInvalidModelDefaults):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
