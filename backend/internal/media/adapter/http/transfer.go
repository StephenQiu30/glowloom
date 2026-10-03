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

// TransferHandler exposes durable independent media transfers, never object keys.
type TransferHandler struct {
	repo application.TransferRepository
}

// NewTransferHandler injects the media owning command and query boundary.
func NewTransferHandler(repo application.TransferRepository) *TransferHandler {
	return &TransferHandler{repo: repo}
}

// Register adds real transfer admission, tracking and explicit control actions.
func (h *TransferHandler) Register(group *gin.RouterGroup) {
	group.POST("/media/library/transfers", h.Create)
	group.GET("/media/library/transfers", h.List)
	group.GET("/media/library/transfers/:job_id", h.Get)
	group.POST("/media/library/transfers/:job_id/cancel", h.Cancel)
	group.POST("/media/library/transfers/:job_id/retry", h.Retry)
	group.POST("/media/library/transfers/:job_id/reconcile", h.Reconcile)
}

// TransferControlRequest binds an explicit action to the observed job revision.
type TransferControlRequest struct {
	Revision int64 `json:"revision"`
}

// TransferPage contains only current creator-scoped results and bounded paging.
type TransferPage struct {
	CurrentActorID uuid.UUID            `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID            `json:"current_org_id"`
	Items          []domain.TransferJob `json:"items"`
	Page           int                  `json:"page"`
	PageSize       int                  `json:"page_size"`
}

func transferKey(c *gin.Context) (uuid.UUID, bool) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		writeTransferError(c, domain.ErrInvalidLibrary)
		return uuid.Nil, false
	}
	return key, true
}

// Create freezes same-owner selected items and independent target identities.
// @Summary 在个人库与项目库间独立转移素材
// @ID createMediaTransfer
// @Tags media
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body application.TransferInput true "闭合双scope、全部条目CAS和目标目录"
// @Success 202 {object} domain.TransferJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/transfers [post]
func (h *TransferHandler) Create(c *gin.Context) {
	if h == nil || h.repo == nil {
		writeTransferError(c, application.ErrUnavailable)
		return
	}
	var input application.TransferInput
	if !httpapi.Decode(c, &input) {
		return
	}
	key, ok := transferKey(c)
	if !ok {
		return
	}
	input.Key = key
	job, err := h.repo.CreateTransfer(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		writeTransferError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, job)
}

// List returns the current creator's authorized transfer history.
// @Summary 素材转移记录分页
// @ID listMediaTransfers
// @Tags media
// @Produce json
// @Param scope query string false "personal/project；默认personal"
// @Param project_id query string false "project scope必要UUID"
// @Param page query int false "1..10000；默认1"
// @Param page_size query int false "1..100；默认20"
// @Success 200 {object} TransferPage
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/transfers [get]
func (h *TransferHandler) List(c *gin.Context) {
	if h == nil || h.repo == nil {
		writeTransferError(c, application.ErrUnavailable)
		return
	}
	query := c.Request.URL.Query()
	for key := range query {
		if key != "scope" && key != "project_id" && key != "page" && key != "page_size" {
			writeTransferError(c, domain.ErrInvalidLibrary)
			return
		}
	}
	scope, err := libraryScope(query)
	if err != nil {
		writeTransferError(c, err)
		return
	}
	page, size := 1, 20
	for key, destination := range map[string]*int{"page": &page, "page_size": &size} {
		if query.Has(key) {
			value, err := strconv.Atoi(query.Get(key))
			if err != nil {
				writeTransferError(c, domain.ErrInvalidLibrary)
				return
			}
			*destination = value
		}
	}
	actor := identityhttp.Principal(c)
	items, err := h.repo.ListTransfers(c.Request.Context(), actor, scope, page, size)
	if err != nil {
		writeTransferError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, TransferPage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, Items: items, Page: page, PageSize: size})
}

// Get returns current actual outcomes without private file paths or source notes.
// @Summary 素材转移状态与逐行结果
// @ID getMediaTransfer
// @Tags media
// @Produce json
// @Param job_id path string true "转移UUID"
// @Success 200 {object} domain.TransferJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/transfers/{job_id} [get]
func (h *TransferHandler) Get(c *gin.Context) {
	if h == nil || h.repo == nil {
		writeTransferError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeTransferError(c, domain.ErrInvalidLibrary)
		return
	}
	id, ok := httpapi.PathUUID(c, "job_id")
	if !ok {
		return
	}
	job, err := h.repo.GetTransfer(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeTransferError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, job)
}

func (h *TransferHandler) control(c *gin.Context, action string) {
	if h == nil || h.repo == nil {
		writeTransferError(c, application.ErrUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeTransferError(c, domain.ErrInvalidLibrary)
		return
	}
	id, ok := httpapi.PathUUID(c, "job_id")
	if !ok {
		return
	}
	key, ok := transferKey(c)
	if !ok {
		return
	}
	var request TransferControlRequest
	if !httpapi.Decode(c, &request) {
		return
	}
	job, err := h.repo.ControlTransfer(c.Request.Context(), identityhttp.Principal(c), id, key, request.Revision, action)
	if err != nil {
		writeTransferError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(202, job)
}

// Cancel requests cleanup; it reports cancelled only after actual owned object cessation.
// @Summary 取消素材转移
// @ID cancelMediaTransfer
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "转移UUID"
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body TransferControlRequest true "已观察revision"
// @Success 202 {object} domain.TransferJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/transfers/{job_id}/cancel [post]
func (h *TransferHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Retry starts only explicitly observed failures whose objects are proven cleaned.
// @Summary 重试已清理失败行
// @ID retryMediaTransfer
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "转移UUID"
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body TransferControlRequest true "已观察revision"
// @Success 202 {object} domain.TransferJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/transfers/{job_id}/retry [post]
func (h *TransferHandler) Retry(c *gin.Context) { h.control(c, "retry") }

// Reconcile preserves target identities and the independent cancellation intent.
// @Summary 对账素材转移未知结果
// @ID reconcileMediaTransfer
// @Tags media
// @Accept json
// @Produce json
// @Param job_id path string true "转移UUID"
// @Param Idempotency-Key header string true "永久UUID幂等键"
// @Param body body TransferControlRequest true "已观察revision"
// @Success 202 {object} domain.TransferJob
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/media/library/transfers/{job_id}/reconcile [post]
func (h *TransferHandler) Reconcile(c *gin.Context) { h.control(c, "reconcile") }

func writeTransferError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrTransferConflict) {
		httpapi.WriteProblem(c, 409, "media_transfer_conflict", nil)
		return
	}
	if errors.Is(err, application.ErrInvalidQuery) {
		err = domain.ErrInvalidLibrary
	}
	writeLibraryError(c, err)
}
