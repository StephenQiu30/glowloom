package http

import (
	"strconv"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// SourceRecoveryHandler exposes permanent explicit recovery of unpublished writes.
type SourceRecoveryHandler struct{ service *application.SourceRecovery }

// NewSourceRecoveryHandler injects the shared synchronous I/O owner's recovery use case.
func NewSourceRecoveryHandler(service *application.SourceRecovery) *SourceRecoveryHandler {
	return &SourceRecoveryHandler{service: service}
}

// Register adds only the four actual intent query/control endpoints.
func (h *SourceRecoveryHandler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/script-source-writes", h.List)
	g.GET("/projects/:pid/script-source-writes/:wid", h.Get)
	g.POST("/projects/:pid/script-source-writes/:wid/cancel", h.Cancel)
	g.POST("/projects/:pid/script-source-writes/:wid/reconcile", h.Reconcile)
}

// SourceControlRequest binds a new explicit command to the visible intent revision.
type SourceControlRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
}

func pageQuery(c *gin.Context, cursor string) (int64, int, bool) {
	before, err := strconv.ParseInt(c.DefaultQuery(cursor, "0"), 10, 64)
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limitErr != nil || before < 0 || limit < 1 || limit > 100 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return 0, 0, false
	}
	return before, limit, true
}

// List supports authorized refresh recovery without exposing the original command body.
// @Summary 未完成正文保存的安全分页
// @Tags script
// @ID listScriptSourceWrites
// @Produce json
// @Param pid path string true "项目UUID"
// @Param after query integer false "分页位置"
// @Param limit query integer false "1..100"
// @Success 200 {object} application.SourceWritePage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-source-writes [get]
func (h *SourceRecoveryHandler) List(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	after, limit, ok := pageQuery(c, "after")
	if !ok {
		return
	}
	result, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), project, after, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// Get reports actual current proof and cancellation state, never a guessed completion.
// @Summary 正文保存意图状态
// @Tags script
// @ID getScriptSourceWrite
// @Produce json
// @Param pid path string true "项目UUID"
// @Param wid path string true "保存意图UUID"
// @Success 200 {object} application.SourceWriteIntent
// @Failure 404 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-source-writes/{wid} [get]
func (h *SourceRecoveryHandler) Get(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "wid")
	if !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// Cancel accepts explicit cancellation; 202 does not mean I/O or cleanup completed.
// @Summary 明确取消未发布正文保存
// @Tags script
// @ID cancelScriptSourceWrite
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param wid path string true "保存意图UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID"
// @Param body body SourceControlRequest true "保存意图修订"
// @Success 202 {object} application.SourceControlReceipt
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-source-writes/{wid}/cancel [post]
func (h *SourceRecoveryHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Reconcile only verifies the original keys/digests and cannot publish someone else's body.
// @Summary 核验原正文保存对象与停止证据
// @Tags script
// @ID reconcileScriptSourceWrite
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param wid path string true "保存意图UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID"
// @Param body body SourceControlRequest true "保存意图修订"
// @Success 202 {object} application.SourceControlReceipt
// @Failure 403 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-source-writes/{wid}/reconcile [post]
func (h *SourceRecoveryHandler) Reconcile(c *gin.Context) { h.control(c, "reconcile") }
func (h *SourceRecoveryHandler) control(c *gin.Context, action string) {
	var request SourceControlRequest
	if !readJSON(c, &request) {
		return
	}
	base, ok := commandInput(c, action, request.ExpectedRevision, nil)
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "wid")
	if !ok {
		return
	}
	result, err := h.service.Control(c.Request.Context(), identityhttp.Principal(c), application.SourceControlCommand{ProjectID: base.ProjectID, IntentID: id, Key: base.Key, RequestID: base.RequestID, Action: action, ExpectedRevision: base.ExpectedRevision})
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(202, result)
}
