package http

import (
	"errors"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DraftHandler struct{ store application.DraftStore }

func NewDraftHandler(store application.DraftStore) *DraftHandler { return &DraftHandler{store} }
func (h *DraftHandler) Register(g *gin.RouterGroup) {
	g.POST("/projects/:pid/prompt-drafts", h.Create)
	g.PUT("/projects/:pid/prompt-drafts/:id", h.SaveDraft)
	g.GET("/projects/:pid/prompt-drafts/:id", h.ReadDraft)
	g.GET("/projects/:pid/prompt-drafts", h.ListDrafts)
}

type DraftRequest struct {
	ExpectedRevision int64        `json:"expected_revision"`
	Draft            domain.Draft `json:"draft"`
}

// Create commits a new private project draft.
// @Summary 创建完整提示词草稿
// @ID createPromptDraft
// @Tags promptEditor
// @Accept json
// @Produce json
// @Param pid path string true "项目"
// @Param Idempotency-Key header string true "UUID"
// @Param body body DraftRequest true "完整草稿"
// @Success 200 {object} application.DraftSnapshot
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/prompt-drafts [post]
func (h *DraftHandler) Create(c *gin.Context) { h.write(c, true) }

// SaveDraft atomically replaces a revision after idempotency replay.
// @Summary 条件保存完整提示词草稿
// @ID savePromptDraft
// @Tags promptEditor
// @Accept json
// @Produce json
// @Param pid path string true "项目"
// @Param id path string true "草稿"
// @Param Idempotency-Key header string true "UUID"
// @Param body body DraftRequest true "完整草稿及基线"
// @Success 200 {object} application.DraftSnapshot
// @Failure 409 {object} httpapi.Problem
// @Router /api/projects/{pid}/prompt-drafts/{id} [put]
func (h *DraftHandler) SaveDraft(c *gin.Context) { h.write(c, false) }
func (h *DraftHandler) write(c *gin.Context, create bool) {
	var body DraftRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	p, e1 := uuid.Parse(c.Param("pid"))
	key, e2 := uuid.Parse(c.GetHeader("Idempotency-Key"))
	id := uuid.Nil
	if !create {
		var err error
		id, err = uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			writeDraftError(c, domain.ErrInvalidDraft)
			return
		}
	}
	if e1 != nil || e2 != nil || key == uuid.Nil || (create && body.ExpectedRevision != 0) || (!create && body.ExpectedRevision < 1) {
		writeDraftError(c, domain.ErrInvalidDraft)
		return
	}
	out, err := h.store.SaveDraft(c.Request.Context(), identityhttp.Principal(c), application.DraftWrite{ProjectID: p, DraftID: id, ExpectedRevision: body.ExpectedRevision, Key: key, Draft: body.Draft})
	if err != nil {
		writeDraftError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, out)
}

// ReadDraft returns only the current actor's draft.
// @Summary 读取完整提示词草稿
// @ID readPromptDraft
// @Tags promptEditor
// @Produce json
// @Param pid path string true "项目"
// @Param id path string true "草稿"
// @Success 200 {object} application.DraftSnapshot
// @Failure 404 {object} httpapi.Problem
// @Router /api/projects/{pid}/prompt-drafts/{id} [get]
func (h *DraftHandler) ReadDraft(c *gin.Context) {
	p, e1 := uuid.Parse(c.Param("pid"))
	id, e2 := uuid.Parse(c.Param("id"))
	if e1 != nil || e2 != nil {
		writeDraftError(c, domain.ErrInvalidDraft)
		return
	}
	out, err := h.store.ReadDraft(c.Request.Context(), identityhttp.Principal(c), p, id)
	if err != nil {
		writeDraftError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, out)
}

// ListDrafts discovers private drafts without browser state.
// @Summary 查找项目中自己的草稿
// @ID listPromptDrafts
// @Tags promptEditor
// @Produce json
// @Param pid path string true "项目"
// @Param cursor query string false "游标"
// @Success 200 {object} application.DraftPage
// @Failure 403 {object} httpapi.Problem
// @Router /api/projects/{pid}/prompt-drafts [get]
func (h *DraftHandler) ListDrafts(c *gin.Context) {
	p, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		writeDraftError(c, domain.ErrInvalidDraft)
		return
	}
	out, err := h.store.ListDrafts(c.Request.Context(), identityhttp.Principal(c), p, c.Query("cursor"))
	if err != nil {
		writeDraftError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, out)
}
func writeDraftError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrDraftNotFound), errors.Is(err, workspaceapp.ErrProjectNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, domain.ErrLabelConflict):
		httpapi.WriteProblem(c, 409, "label_allocation_conflict", nil)
	case errors.Is(err, domain.ErrUnsupportedSchema):
		httpapi.WriteProblem(c, 422, "unsupported_schema", nil)
	case errors.Is(err, domain.ErrReferenceUnavailable):
		httpapi.WriteProblem(c, 422, "reference_unavailable", nil)
	case errors.Is(err, domain.ErrInvalidDraft):
		httpapi.WriteProblem(c, 422, "invalid_prompt_draft", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
