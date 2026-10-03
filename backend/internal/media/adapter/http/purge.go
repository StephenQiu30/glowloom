package http

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

// PurgeHandler exposes durable private-object purges, never object keys.
type PurgeHandler struct {
	repo application.PurgeRepository
}

// NewPurgeHandler injects the media owning command and query boundary.
func NewPurgeHandler(repo application.PurgeRepository) *PurgeHandler {
	return &PurgeHandler{repo: repo}
}

// Register adds real purge admission, tracking and explicit control actions.
func (h *PurgeHandler) Register(group *gin.RouterGroup) {
	group.POST("/media/library/purges", h.Create)
	group.GET("/media/library/purges", h.List)
	group.GET("/media/library/purges/:job_id", h.Get)
	group.POST("/media/library/purges/:job_id/cancel", h.Cancel)
	group.POST("/media/library/purges/:job_id/reconcile", h.Reconcile)
}

// PurgeControlRequest binds an explicit action to the observed job revision.
type PurgeControlRequest struct {
	Revision int64 `json:"revision"`
}

// PurgePage contains only current creator-scoped results and bounded paging.
type PurgePage struct {
	CurrentActorID uuid.UUID         `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID         `json:"current_org_id"`
	Items          []domain.PurgeJob `json:"items"`
	Page           int               `json:"page"`
	PageSize       int               `json:"page_size"`
}

func purgeKey(c *gin.Context) (uuid.UUID, bool) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		writePurgeError(c, domain.ErrInvalidLibrary)
		return uuid.Nil, false
	}
	return key, true
}

// Create freezes exact scoped ownership after explicit permanent confirmation.
// @Summary 明确确认后永久清理回收素材
// @ID createMediaPurge
// @Tags media
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body application.PurgeInput true "闭合scope、全部条目CAS和明确永久删除确认"
// @Success 202 {object} domain.PurgeJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/purges [post]
func (h *PurgeHandler) Create(c *gin.Context) {
	if h == nil || h.repo == nil {
		writePurgeError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writePurgeError(c, domain.ErrInvalidLibrary)
		return
	}
	var input application.PurgeInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := purgeKey(c)
	if !ok {
		return
	}
	input.Key = key
	job, err := h.repo.CreatePurge(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		writePurgeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, job)
}

// List returns the current creator's authorized purge history.
// @Summary 永久清理记录分页
// @ID listMediaPurges
// @Tags media
// @Produce json
// @Param scope query string false "personal/project；默认personal"
// @Param project_id query string false "project scope必要UUID"
// @Param page query int false "1..10000；默认1"
// @Param page_size query int false "1..100；默认20"
// @Success 200 {object} PurgePage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/purges [get]
func (h *PurgeHandler) List(c *gin.Context) {
	if h == nil || h.repo == nil {
		writePurgeError(c, application.ErrUnavailable)
		return
	}
	query := c.Request.URL.Query()
	for key := range query {
		if key != "scope" && key != "project_id" && key != "page" && key != "page_size" {
			writePurgeError(c, domain.ErrInvalidLibrary)
			return
		}
	}
	scope, err := libraryScope(query)
	if err != nil {
		writePurgeError(c, err)
		return
	}
	page, size := 1, 20
	for key, destination := range map[string]*int{"page": &page, "page_size": &size} {
		if query.Has(key) {
			value, err := strconv.Atoi(query.Get(key))
			if err != nil {
				writePurgeError(c, domain.ErrInvalidLibrary)
				return
			}
			*destination = value
		}
	}
	actor := identityhttp.Principal(c)
	items, err := h.repo.ListPurges(c.Request.Context(), actor, scope, page, size)
	if err != nil {
		writePurgeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, PurgePage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Items: items, Page: page, PageSize: size})
}

// Get returns current actual outcomes without private file paths or private source content.
// @Summary 永久清理状态与逐行结果
// @ID getMediaPurge
// @Tags media
// @Produce json
// @Param job_id path string true "清理UUID"
// @Success 200 {object} domain.PurgeJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/purges/{job_id} [get]
func (h *PurgeHandler) Get(c *gin.Context) {
	if h == nil || h.repo == nil {
		writePurgeError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writePurgeError(c, domain.ErrInvalidLibrary)
		return
	}
	id, ok := httpapi.PathUUID(c, "job_id")
	if !ok {
		return
	}
	job, err := h.repo.GetPurge(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writePurgeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, job)
}

func (h *PurgeHandler) control(c *gin.Context, action string) {
	if h == nil || h.repo == nil {
		writePurgeError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writePurgeError(c, domain.ErrInvalidLibrary)
		return
	}
	id, ok := httpapi.PathUUID(c, "job_id")
	if !ok {
		return
	}
	key, ok := purgeKey(c)
	if !ok {
		return
	}
	var request PurgeControlRequest
	if !httpapi.Decode(c, &request) {
		return
	}
	job, err := h.repo.ControlPurge(c.Request.Context(), identityhttp.Principal(c), id, key, request.Revision, action)
	if err != nil {
		writePurgeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, job)
}

// Cancel requests cleanup; it reports cancelled only after actual owned object cessation.
// @Summary 取消永久清理
// @ID cancelMediaPurge
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "清理UUID"
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body PurgeControlRequest true "已观察revision"
// @Success 202 {object} domain.PurgeJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/purges/{job_id}/cancel [post]
func (h *PurgeHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Reconcile preserves original removal intents and independent cancellation purpose.
// @Summary 对账永久清理未知结果
// @ID reconcileMediaPurge
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "清理UUID"
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body PurgeControlRequest true "已观察revision"
// @Success 202 {object} domain.PurgeJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/purges/{job_id}/reconcile [post]
func (h *PurgeHandler) Reconcile(c *gin.Context) { h.control(c, "reconcile") }

func writePurgeError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrPurgeConflict) {
		httpapi.WriteProblem(c, 409, "media_purge_conflict", nil)
		return
	}
	if errors.Is(err, application.ErrInvalidQuery) {
		err = domain.ErrInvalidLibrary
	}
	writeLibraryError(c, err)
}
