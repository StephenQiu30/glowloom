package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

func (h *Handler) save(c *gin.Context, id uuid.UUID) {
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	key, ok := commandKey(c)
	if !ok {
		return
	}
	var input application.SaveInput
	if !httpapi.Decode(c, &input) {
		return
	}
	if h == nil || h.commands == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	detail, err := h.commands.Save(c.Request.Context(), currentActor(c), id, key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, detail)
}

// Create publishes complete settings in one permanent command.
// @Summary 新建主体
// @ID createSubject
// @Tags subjects
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "永久UUID命令键"
// @Param body body application.SaveInput true "完整设置；创建expected_revision为0"
// @Success 200 {object} application.Detail
// @Failure 401,403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/subjects [post]
func (h *Handler) Create(c *gin.Context) { h.save(c, uuid.Nil) }

// Update atomically publishes the next immutable version under shared content CAS.
// @Summary 更新主体设置
// @ID updateSubject
// @Tags subjects
// @Accept json
// @Produce json
// @Param id path string true "主体UUID"
// @Param Idempotency-Key header string true "永久UUID命令键"
// @Param body body application.SaveInput true "完整设置及已观察revision"
// @Success 200 {object} application.Detail
// @Failure 401,403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/subjects/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if ok {
		h.save(c, id)
	}
}

// Delete prevents future references without destroying retained versions or pins.
// @Summary 删除主体（不提供恢复）
// @ID deleteSubject
// @Tags subjects
// @Accept json
// @Produce json
// @Param id path string true "主体UUID"
// @Param Idempotency-Key header string true "永久UUID命令键"
// @Param body body application.RevisionInput true "已观察revision"
// @Success 200 {object} application.ItemResult
// @Failure 401,403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/subjects/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	key, ok := commandKey(c)
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	var input application.RevisionInput
	if !httpapi.Decode(c, &input) {
		return
	}
	if h == nil || h.commands == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	result, err := h.commands.Delete(c.Request.Context(), currentActor(c), id, key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}

// Favorite changes only the current actor's preference.
// @Summary 收藏或取消收藏主体
// @ID setSubjectFavorite
// @Tags subjects
// @Accept json
// @Produce json
// @Param id path string true "主体UUID"
// @Param Idempotency-Key header string true "永久UUID命令键"
// @Param body body application.FavoriteInput true "收藏目标状态和已观察内容revision"
// @Success 200 {object} application.ItemResult
// @Failure 401,403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/subjects/{id}/favorite [put]
func (h *Handler) Favorite(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	key, ok := commandKey(c)
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	var request struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		Favorite         *bool  `json:"favorite"`
	}
	if !httpapi.Decode(c, &request) {
		return
	}
	if request.ExpectedRevision == nil || request.Favorite == nil {
		writeError(c, domain.ErrInvalid)
		return
	}
	input := application.FavoriteInput{ExpectedRevision: *request.ExpectedRevision, Favorite: *request.Favorite}
	if h == nil || h.commands == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	result, err := h.commands.Favorite(c.Request.Context(), currentActor(c), id, key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}

// Batch returns each accepted, missing or stale selected item separately.
// @Summary 批量删除或收藏主体
// @ID batchSubjects
// @Tags subjects
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "永久UUID命令键"
// @Param body body application.BatchInput true "delete/favorite/unfavorite及1..120个ID+revision"
// @Success 200 {object} application.BatchResult
// @Failure 401,403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/subjects/batch [post]
func (h *Handler) Batch(c *gin.Context) {
	key, ok := commandKey(c)
	if !ok {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		writeError(c, domain.ErrInvalid)
		return
	}
	var input application.BatchInput
	if !httpapi.Decode(c, &input) {
		return
	}
	if h == nil || h.commands == nil {
		writeError(c, application.ErrUnavailable)
		return
	}
	result, err := h.commands.Batch(c.Request.Context(), currentActor(c), key, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
