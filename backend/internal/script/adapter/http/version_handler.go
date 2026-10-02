package http

import (
	"strconv"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// VersionHandler exposes selected-version reads and actual confirmed-version adoption.
type VersionHandler struct {
	history *application.HistoryService
	adopt   *application.AdoptService
}

// NewVersionHandler injects authorized immutable reads and formal adoption.
func NewVersionHandler(history *application.HistoryService, adopt *application.AdoptService) *VersionHandler {
	return &VersionHandler{history: history, adopt: adopt}
}

// Register adds only implemented version/confirmation endpoints.
func (h *VersionHandler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/script-versions/:vid/source-text", h.Text)
	g.POST("/projects/:pid/script-versions/:vid/adopt", h.Adopt)
	g.GET("/projects/:pid/script-versions/:vid/split-confirmations", h.Confirmations)
	g.GET("/projects/:pid/script-versions/:vid/split-confirmations/:cid", h.Confirmation)
}

// AdoptVersionRequest binds the visible current heads to explicit impact consent.
type AdoptVersionRequest struct {
	ExpectedRevision      *int64 `json:"expected_revision"`
	ExpectedSplitRevision *int64 `json:"expected_split_revision"`
	AckInvalidate         bool   `json:"ack_invalidate"`
}

// Text reads bounded canonical source text before formal episode identities exist.
// @Summary 候选与历史版本的规范正文片段
// @Tags script
// @ID getScriptVersionSourceText
// @Produce json
// @Param pid path string true "项目UUID"
// @Param vid path string true "不可变剧本版本UUID"
// @Param from query integer true "包含起点，Unicode scalar"
// @Param to query integer true "不含终点，最多65536scalar"
// @Success 200 {object} application.VersionText
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-versions/{vid}/source-text [get]
func (h *VersionHandler) Text(c *gin.Context) {
	pid, ok := pathID(c, "pid")
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid")
	if !ok {
		return
	}
	from, err := strconv.Atoi(c.Query("from"))
	to, toErr := strconv.Atoi(c.Query("to"))
	if err != nil || toErr != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	out, err := h.history.VersionText(c.Request.Context(), identityhttp.Principal(c), pid, vid, from, to)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// Adopt publishes only confirmed content through current CAS and real impact ownership.
// @Summary 采纳已确认分集的正式剧本版本
// @Tags script
// @ID adoptScriptVersion
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param vid path string true "不可变剧本版本UUID"
// @Param Idempotency-Key header string true "永久UUID请求键"
// @Param body body AdoptVersionRequest true "头修订及影响确认"
// @Success 200 {object} application.AdoptReceipt
// @Failure 409 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-versions/{vid}/adopt [post]
func (h *VersionHandler) Adopt(c *gin.Context) {
	var input AdoptVersionRequest
	if !readJSON(c, &input) {
		return
	}
	command, ok := commandInput(c, "adopt", input.ExpectedRevision, nil)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid")
	if !ok {
		return
	}
	if input.ExpectedSplitRevision == nil {
		scriptProblem(c, domain.ErrInvalidSpan)
		return
	}
	out, err := h.adopt.Adopt(c.Request.Context(), identityhttp.Principal(c), application.AdoptCommand{ProjectID: command.ProjectID, VersionID: vid, Key: command.Key, RequestID: command.RequestID, ExpectedRevision: command.ExpectedRevision, ExpectedSplitRevision: *input.ExpectedSplitRevision, AckInvalidate: input.AckInvalidate})
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// Confirmations pages complete past confirmation metadata.
// @Summary 分集确认的不可变历史
// @Tags script
// @ID listScriptSplitConfirmations
// @Produce json
// @Param pid path string true "项目UUID"
// @Param vid path string true "不可变剧本版本UUID"
// @Param before_revision query integer false "严格早于确认修订"
// @Param limit query integer false "1..100"
// @Success 200 {object} application.ConfirmationPage
// @Router /api/projects/{pid}/script-versions/{vid}/split-confirmations [get]
func (h *VersionHandler) Confirmations(c *gin.Context) {
	pid, ok := pathID(c, "pid")
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid")
	if !ok {
		return
	}
	before, limit, ok := pageQuery(c, "before_revision")
	if !ok {
		return
	}
	out, err := h.history.Confirmations(c.Request.Context(), identityhttp.Principal(c), pid, vid, before, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}

// Confirmation returns the exact recorded whole partition and identity mapping.
// @Summary 指定分集确认的完整边界与身份结果
// @Tags script
// @ID getScriptSplitConfirmation
// @Produce json
// @Param pid path string true "项目UUID"
// @Param vid path string true "不可变剧本版本UUID"
// @Param cid path string true "确认UUID"
// @Success 200 {object} domain.SplitConfirmation
// @Router /api/projects/{pid}/script-versions/{vid}/split-confirmations/{cid} [get]
func (h *VersionHandler) Confirmation(c *gin.Context) {
	pid, ok := pathID(c, "pid")
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid")
	if !ok {
		return
	}
	cid, ok := pathID(c, "cid")
	if !ok {
		return
	}
	out, err := h.history.Confirmation(c.Request.Context(), identityhttp.Principal(c), pid, vid, cid)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, out)
}
