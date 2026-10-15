package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// SubmissionHandler exposes private submission discovery and explicit resolution.
type SubmissionHandler struct{ store application.EditorSubmissions }

// NewSubmissionHandler injects the durable submission owner.
func NewSubmissionHandler(store application.EditorSubmissions) *SubmissionHandler {
	return &SubmissionHandler{store}
}

// Register installs authenticated submission routes.
func (h *SubmissionHandler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/prompt-submissions/current", h.Current)
	g.POST("/projects/:pid/prompt-submissions/:id/resolve", h.Resolve)
}

// ResolveSubmissionRequest names the expected slot generation and user decision.
type ResolveSubmissionRequest struct {
	ExpectedEpoch int64  `json:"expected_epoch"`
	Action        string `json:"action"`
}

// Current discovers unresolved intent without local browser storage.
// @Summary 查找当前未决编辑器提交
// @ID currentPromptSubmission
// @Tags promptEditor
// @Produce json
// @Param pid path string true "项目"
// @Param owner_kind query string true "prompt_draft 或 canvas_input"
// @Param draft_id query string false "草稿"
// @Param canvas_id query string false "画布"
// @Param node_id query string false "节点"
// @Success 200 {object} application.SubmissionCurrent
// @Failure 403 {object} httpapi.Problem
// @Router /api/projects/{pid}/prompt-submissions/current [get]
func (h *SubmissionHandler) Current(c *gin.Context) {
	p, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	source := promptdomain.InputSource{Kind: c.Query("owner_kind")}
	if source.Kind == "prompt_draft" {
		source.DraftID, _ = uuid.Parse(c.Query("draft_id"))
		source.Revision = 1
	} else {
		source.CanvasID, _ = uuid.Parse(c.Query("canvas_id"))
		source.NodeID, _ = uuid.Parse(c.Query("node_id"))
		source.CanvasRevision = 1
		source.InputRevision = 1
	}
	out, err := h.store.CurrentSubmission(c.Request.Context(), identityhttp.Principal(c), p, source)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, out)
}

// Resolve closes only the selected epoch, leaving accepted executions intact.
// @Summary 知晓接受结果或放弃未确认报价
// @ID resolvePromptSubmission
// @Tags promptEditor
// @Accept json
// @Produce json
// @Param pid path string true "项目"
// @Param id path string true "提交"
// @Param Idempotency-Key header string true "UUID"
// @Param body body ResolveSubmissionRequest true "动作和代次"
// @Success 200 {object} application.SubmissionCurrent
// @Failure 409 {object} httpapi.Problem
// @Router /api/projects/{pid}/prompt-submissions/{id}/resolve [post]
func (h *SubmissionHandler) Resolve(c *gin.Context) {
	p, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body ResolveSubmissionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		writeError(c, application.ErrInvalidFreeQuote)
		return
	}
	out, err := h.store.ResolveSubmission(c.Request.Context(), identityhttp.Principal(c), application.SubmissionResolution{ProjectID: p, ID: id, Key: key, ExpectedEpoch: body.ExpectedEpoch, Action: body.Action})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("ETag", strconv.FormatInt(out.Epoch, 10))
	c.JSON(200, out)
}
