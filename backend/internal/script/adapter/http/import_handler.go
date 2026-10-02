package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// ImportHandler exposes durable document import acceptance and exact-key controls.
type ImportHandler struct{ service *application.ImportService }

// NewImportHandler injects the real persistent import use cases.
func NewImportHandler(service *application.ImportService) *ImportHandler {
	return &ImportHandler{service: service}
}

// Register installs the complete import refresh and explicit control surface.
func (h *ImportHandler) Register(g *gin.RouterGroup) {
	g.POST("/projects/:pid/script-file-imports", h.Create)
	g.GET("/projects/:pid/script-file-imports", h.List)
	g.GET("/projects/:pid/script-file-imports/:id", h.Get)
	g.POST("/projects/:pid/script-file-imports/:id/cancel", h.Cancel)
	g.POST("/projects/:pid/script-file-imports/:id/retry", h.Retry)
	g.POST("/projects/:pid/script-file-imports/:id/reconcile", h.Reconcile)
}

// FileImportRequest binds ordered formal document IDs to the observed script head.
type FileImportRequest struct {
	ExpectedRevision *int64      `json:"expected_revision"`
	BaseVersionID    *uuid.UUID  `json:"base_version_id,omitempty"`
	AssetIDs         []uuid.UUID `json:"asset_ids"`
	RightsConfirmed  bool        `json:"rights_confirmed"`
}

// FileImportControlRequest observes the import job revision independently of script CAS.
type FileImportControlRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
}

// Create accepts a persistent import without claiming extraction completion.
// @ID createScriptFileImport
// @Tags script
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param Idempotency-Key header string true "Permanent UUID key"
// @Param input body FileImportRequest true "Ordered frozen originals"
// @Success 202 {object} application.ImportJob
// @Failure 403,404,409,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-file-imports [post]
func (h *ImportHandler) Create(c *gin.Context) {
	var request FileImportRequest
	if !readJSON(c, &request) {
		return
	}
	in, ok := commandInput(c, "import", request.ExpectedRevision, request.BaseVersionID)
	if !ok {
		return
	}
	actor := identityhttp.Principal(c)
	if len(request.AssetIDs) == 0 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	out, err := h.service.Create(c.Request.Context(), actor, application.ImportCommand{ProjectID: in.ProjectID, Key: in.Key, RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision, BaseVersionID: in.BaseVersionID, AssetIDs: request.AssetIDs, RightsConfirmed: request.RightsConfirmed})
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(http.StatusAccepted, out)
}

// List supports refresh after unknown admission with only the current caller's scope.
// @ID listScriptFileImports
// @Tags script
// @Produce json
// @Param pid path string true "Project UUID"
// @Param after query integer false "Offset" default(0)
// @Param limit query integer false "Page size" default(50)
// @Success 200 {object} application.ImportPage
// @Failure 403,404,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-file-imports [get]
func (h *ImportHandler) List(c *gin.Context) {
	project, ok := pathID(c, "pid")
	if !ok {
		return
	}
	actor := identityhttp.Principal(c)
	after, err := strconv.ParseInt(c.DefaultQuery("after", "0"), 10, 64)
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	page, err := h.service.List(c.Request.Context(), actor, project, after, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// Get exposes the current actual state and all file outcomes.
// @ID getScriptFileImport
// @Tags script
// @Produce json
// @Param pid path string true "Project UUID"
// @Param id path string true "Import UUID"
// @Success 200 {object} application.ImportJob
// @Failure 403,404,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-file-imports/{id} [get]
func (h *ImportHandler) Get(c *gin.Context) {
	project, ok := pathID(c, "pid")
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	actor := identityhttp.Principal(c)
	job, err := h.service.Get(c.Request.Context(), actor, project, id)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(http.StatusOK, job)
}

// Cancel accepts cancellation of only unpublished work; published sources stay intact.
// @ID cancelScriptFileImport
// @Tags script
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param id path string true "Import UUID"
// @Param Idempotency-Key header string true "Permanent UUID key"
// @Param input body FileImportControlRequest true "Observed job revision"
// @Success 202 {object} application.ImportJob
// @Failure 403,404,409,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-file-imports/{id}/cancel [post]
func (h *ImportHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Retry accepts only failed originals under the last published draft CAS.
// @ID retryScriptFileImport
// @Tags script
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param id path string true "Import UUID"
// @Param Idempotency-Key header string true "Permanent UUID key"
// @Param input body FileImportControlRequest true "Observed job revision"
// @Success 202 {object} application.ImportJob
// @Failure 403,404,409,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-file-imports/{id}/retry [post]
func (h *ImportHandler) Retry(c *gin.Context) { h.control(c, "retry") }

// Reconcile retains the original attempt, originals, keys and object digest facts.
// @ID reconcileScriptFileImport
// @Tags script
// @Accept json
// @Produce json
// @Param pid path string true "Project UUID"
// @Param id path string true "Import UUID"
// @Param Idempotency-Key header string true "Permanent UUID key"
// @Param input body FileImportControlRequest true "Observed job revision"
// @Success 202 {object} application.ImportJob
// @Failure 403,404,409,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-file-imports/{id}/reconcile [post]
func (h *ImportHandler) Reconcile(c *gin.Context) { h.control(c, "reconcile") }

func (h *ImportHandler) control(c *gin.Context, action string) {
	var request FileImportControlRequest
	if !readJSON(c, &request) {
		return
	}
	in, ok := commandInput(c, action, request.ExpectedRevision, nil)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	actor := identityhttp.Principal(c)
	out, err := h.service.Control(c.Request.Context(), actor, application.ImportControl{ProjectID: in.ProjectID, JobID: id, Key: in.Key, RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision, Action: action})
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(http.StatusAccepted, out)
}
