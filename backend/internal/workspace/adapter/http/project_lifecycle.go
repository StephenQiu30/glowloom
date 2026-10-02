package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectLifecycleHandler exposes reversible project changes without copying content.
type ProjectLifecycleHandler struct{ service *application.ProjectLifecycle }

// NewProjectLifecycleHandler injects authorized settings and lifecycle use cases.
func NewProjectLifecycleHandler(service *application.ProjectLifecycle) *ProjectLifecycleHandler {
	return &ProjectLifecycleHandler{service: service}
}

// Register installs detail, mutable settings, and reversible lifecycle endpoints.
func (h *ProjectLifecycleHandler) Register(group *gin.RouterGroup) {
	group.GET("/projects/:pid", h.Read)
	group.PATCH("/projects/:pid", h.Update)
	group.POST("/projects/:pid/archive", h.Archive)
	group.POST("/projects/:pid/unarchive", h.Unarchive)
	group.DELETE("/projects/:pid", h.Delete)
	group.POST("/projects/:pid/restore", h.Restore)
}

// ProjectDetailResponse contains safe settings and actual persisted recovery facts.
type ProjectDetailResponse struct {
	CoverAssetID        *uuid.UUID        `json:"cover_asset_id" extensions:"x-nullable"`
	CoverUnavailable    bool              `json:"cover_unavailable"`
	ID                  uuid.UUID         `json:"id"`
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	AspectRatio         string            `json:"aspect_ratio"`
	StyleType           string            `json:"style_type"`
	StyleSubtype        string            `json:"style_subtype,omitempty"`
	StylePresetID       *uuid.UUID        `json:"style_preset_id" extensions:"x-nullable"`
	Resolution          string            `json:"resolution"`
	AllowOverseasModels bool              `json:"allow_overseas_models"`
	DefaultModels       map[string]string `json:"default_models" swaggertype:"object,string"`
	Status              string            `json:"status"`
	IsDelete            bool              `json:"is_delete"`
	ArchivedAt          *time.Time        `json:"archived_at" extensions:"x-nullable"`
	DeleteTime          *time.Time        `json:"delete_time" extensions:"x-nullable"`
	PurgeAfter          *time.Time        `json:"purge_after" extensions:"x-nullable"`
	Revision            int64             `json:"revision"`
	CreateTime          time.Time         `json:"create_time"`
	UpdateTime          time.Time         `json:"update_time"`
}

// ProjectUpdateRequest contains only settings mutable after creation.
type ProjectUpdateRequest struct {
	CoverAssetID        *uuid.UUID `json:"cover_asset_id,omitempty" extensions:"x-nullable"`
	SetCover            bool       `json:"-" swaggerignore:"true"`
	ExpectedRevision    *int64     `json:"expected_revision" binding:"required"`
	Name                *string    `json:"name,omitempty" minLength:"1" maxLength:"50"`
	Description         *string    `json:"description,omitempty"`
	StylePresetID       *uuid.UUID `json:"style_preset_id,omitempty"`
	AllowOverseasModels *bool      `json:"allow_overseas_models,omitempty"`
}

// UnmarshalJSON preserves invalid UTF-8 rejection instead of encoding/json's
// replacement behavior, while keeping the same closed-field transport contract.
func (r *ProjectUpdateRequest) UnmarshalJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return errors.New("project update contains invalid UTF-8")
	}
	type fields ProjectUpdateRequest
	var decoded fields
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("project update contains trailing JSON")
	}
	var presence map[string]json.RawMessage
	if err := json.Unmarshal(raw, &presence); err != nil {
		return err
	}
	_, decoded.SetCover = presence["cover_asset_id"]
	*r = ProjectUpdateRequest(decoded)
	return nil
}

// ProjectTransitionRequest applies a lifecycle action at its observed revision.
type ProjectTransitionRequest struct {
	ExpectedRevision *int64 `json:"expected_revision" binding:"required"`
}

// Read returns the current organization's undeleted project settings.
// @Summary 项目详情
// @Tags projects
// @ID getProject
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {object} ProjectDetailResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid} [get]
func (h *ProjectLifecycleHandler) Read(c *gin.Context) {
	id, err := uuid.Parse(c.Param("pid"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	snapshot, err := h.service.Get(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeLifecycleError(c, err)
		return
	}
	writeProjectSnapshot(c, snapshot)
}

// Update changes mutable project settings with a durable request key.
// @Summary 修改项目名称与设置
// @Tags projects
// @ID updateProject
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectUpdateRequest true "项目修订与可变设置；零值预设UUID清除预设"
// @Success 200 {object} ProjectDetailResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid} [patch]
func (h *ProjectLifecycleHandler) Update(c *gin.Context) {
	var body ProjectUpdateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	h.change(c, "patch", application.UpdateProjectInput{SetCover: body.SetCover, CoverAssetID: body.CoverAssetID, ExpectedRevision: *body.ExpectedRevision, Name: body.Name, Description: body.Description, StylePresetID: body.StylePresetID, AllowOverseasModels: body.AllowOverseasModels})
}

// Archive makes a project read only after owning modules prove no inflight work.
// @Summary 归档项目
// @Tags projects
// @ID archiveProject
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "项目修订"
// @Success 200 {object} ProjectDetailResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/archive [post]
func (h *ProjectLifecycleHandler) Archive(c *gin.Context) { h.transition(c, "archive") }

// Unarchive resumes an undeleted archived project.
// @Summary 取消项目归档
// @Tags projects
// @ID unarchiveProject
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "项目修订"
// @Success 200 {object} ProjectDetailResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/unarchive [post]
func (h *ProjectLifecycleHandler) Unarchive(c *gin.Context) { h.transition(c, "unarchive") }

// Delete places a project in the thirty-day recycle bin without deleting its contents.
// @Summary 项目移入回收站
// @Tags projects
// @ID deleteProject
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "项目修订"
// @Success 200 {object} ProjectDetailResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid} [delete]
func (h *ProjectLifecycleHandler) Delete(c *gin.Context) { h.transition(c, "delete") }

// Restore recovers the prior active or archived state before the persisted deadline.
// @Summary 恢复回收项目
// @Tags projects
// @ID restoreProject
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "项目修订"
// @Success 200 {object} ProjectDetailResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/restore [post]
func (h *ProjectLifecycleHandler) Restore(c *gin.Context) { h.transition(c, "restore") }

func (h *ProjectLifecycleHandler) transition(c *gin.Context, action string) {
	var body ProjectTransitionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	h.change(c, action, application.UpdateProjectInput{ExpectedRevision: *body.ExpectedRevision})
}

func (h *ProjectLifecycleHandler) change(c *gin.Context, action string, patch application.UpdateProjectInput) {
	id, idErr := uuid.Parse(c.Param("pid"))
	key, keyErr := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if idErr != nil || id == uuid.Nil || keyErr != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	patch.ProjectID = id
	patch.RequestID = c.GetString("request_id")
	result, err := h.service.Change(c.Request.Context(), identityhttp.Principal(c), application.ProjectChangeInput{Action: action, IdempotencyKey: key, Patch: patch})
	if err != nil {
		writeLifecycleError(c, err)
		return
	}
	writeProjectSnapshot(c, result)
}

func writeProjectSnapshot(c *gin.Context, s application.ProjectSnapshot) {
	p := s.Project
	var preset *uuid.UUID
	if p.StylePresetID != uuid.Nil {
		id := p.StylePresetID
		preset = &id
	}
	response := ProjectDetailResponse{CoverAssetID: p.CoverAssetID, CoverUnavailable: s.CoverUnavailable, ID: p.ID, Name: p.Name, Description: p.Description, AspectRatio: p.AspectRatio, StyleType: p.StyleType, StyleSubtype: p.StyleSubtype, StylePresetID: preset, Resolution: p.Resolution, AllowOverseasModels: p.AllowOverseasModels, DefaultModels: s.DefaultModels, Status: p.Status, IsDelete: p.IsDelete, ArchivedAt: p.ArchivedAt, DeleteTime: p.DeleteTime, PurgeAfter: p.PurgeAfter, Revision: p.Revision, CreateTime: p.CreateTime, UpdateTime: p.UpdateTime}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, response)
}

func writeLifecycleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrProjectNotFound), errors.Is(err, application.ErrStylePresetNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, domain.ErrProjectRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, application.ErrProjectCoverUnavailable):
		httpapi.WriteProblem(c, 409, "cover_unavailable", nil)
	case errors.Is(err, domain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "state_conflict", nil)
	case errors.Is(err, domain.ErrProjectHasInflightOperations):
		httpapi.WriteProblem(c, 409, "inflight_work", nil)
	case errors.Is(err, domain.ErrProjectRestoreExpired):
		httpapi.WriteProblem(c, 409, "restore_expired", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_key_reused", nil)
	case errors.Is(err, application.ErrInvalidProjectChange), errors.Is(err, domain.ErrInvalidProject), errors.Is(err, application.ErrStylePresetMismatch):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
