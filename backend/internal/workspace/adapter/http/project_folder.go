package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectFolderHandler exposes the actor's flat library and guarded recycling.
type ProjectFolderHandler struct{ service *application.ProjectFolders }

// NewProjectFolderHandler injects authorized directory commands and queries.
func NewProjectFolderHandler(service *application.ProjectFolders) *ProjectFolderHandler {
	return &ProjectFolderHandler{service: service}
}

// Register installs directory management separately from project content settings.
func (h *ProjectFolderHandler) Register(group *gin.RouterGroup) {
	group.GET("/project-folders", h.List)
	group.POST("/project-folders", h.Create)
	group.PATCH("/project-folders/:folder_id", h.Update)
	group.DELETE("/project-folders/:folder_id", h.Recycle)
	group.PUT("/projects/:pid/folder", h.Move)
}

// ProjectFolderResponse omits actor, organization and private storage facts.
type ProjectFolderResponse struct {
	ID               uuid.UUID           `json:"id"`
	Name             string              `json:"name"`
	Cover            *domain.FolderCover `json:"cover" extensions:"x-nullable"`
	CoverUnavailable bool                `json:"cover_unavailable"`
	ProjectCount     *int64              `json:"project_count,omitempty" extensions:"x-nullable"`
	Revision         int64               `json:"revision"`
	IsDelete         bool                `json:"is_delete"`
	DeleteTime       *time.Time          `json:"delete_time" extensions:"x-nullable"`
	CreateTime       time.Time           `json:"create_time"`
	UpdateTime       time.Time           `json:"update_time"`
}

// ProjectFolderListResponse contains a bounded directory page.
type ProjectFolderListResponse struct {
	CurrentActorID uuid.UUID               `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID               `json:"current_org_id"`
	Items          []ProjectFolderResponse `json:"items"`
	NextCursor     *string                 `json:"next_cursor" extensions:"x-nullable"`
}

// ProjectFolderPlacementResponse uses a separate navigation revision.
type ProjectFolderPlacementResponse struct {
	ProjectID uuid.UUID  `json:"project_id"`
	FolderID  *uuid.UUID `json:"folder_id" extensions:"x-nullable"`
	Revision  int64      `json:"revision"`
}

// ProjectFolderChangeResponse is the safe first response retained by the command.
type ProjectFolderChangeResponse struct {
	Folder             *ProjectFolderResponse          `json:"folder,omitempty"`
	Placement          *ProjectFolderPlacementResponse `json:"placement,omitempty"`
	RecycledProjectIDs []uuid.UUID                     `json:"recycled_project_ids,omitempty"`
}

// ProjectFolderCreateRequest only accepts a name and an eligible formal image.
type ProjectFolderCreateRequest struct {
	Name  string              `json:"name" minLength:"1" maxLength:"160"`
	Cover *domain.FolderCover `json:"cover,omitempty"`
}

// ProjectFolderUpdateRequest distinguishes removing a cover from omitting it.
type ProjectFolderUpdateRequest struct {
	ExpectedRevision *int64              `json:"expected_revision"`
	Name             *string             `json:"name,omitempty" minLength:"1" maxLength:"160"`
	Cover            *domain.FolderCover `json:"cover,omitempty" extensions:"x-nullable"`
	SetCover         bool                `json:"-"`
}

// ProjectFolderMoveRequest explicitly names root or an observed destination folder.
type ProjectFolderMoveRequest struct {
	ExpectedProjectRevision   *int64     `json:"expected_project_revision"`
	ExpectedPlacementRevision *int64     `json:"expected_placement_revision"`
	ExpectedFolderRevision    int64      `json:"expected_folder_revision,omitempty"`
	FolderID                  *uuid.UUID `json:"folder_id" extensions:"x-nullable"`
	HasFolder                 bool       `json:"-"`
}

func decodeFolderJSON(raw []byte, out any) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, domain.ErrInvalidProjectFolder
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, domain.ErrInvalidProjectFolder
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, domain.ErrInvalidProjectFolder
	}
	return fields, nil
}

// UnmarshalJSON rejects unsafe names and preserves explicit-null cover deletion.
func (r *ProjectFolderUpdateRequest) UnmarshalJSON(raw []byte) error {
	type plain ProjectFolderUpdateRequest
	var decoded plain
	fields, err := decodeFolderJSON(raw, &decoded)
	if err != nil {
		return err
	}
	_, decoded.SetCover = fields["cover"]
	if value, present := fields["name"]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return domain.ErrInvalidProjectFolder
	}
	*r = ProjectFolderUpdateRequest(decoded)
	return nil
}

// UnmarshalJSON requires the destination even when its intended value is null.
func (r *ProjectFolderMoveRequest) UnmarshalJSON(raw []byte) error {
	type plain ProjectFolderMoveRequest
	var decoded plain
	fields, err := decodeFolderJSON(raw, &decoded)
	if err != nil {
		return err
	}
	_, decoded.HasFolder = fields["folder_id"]
	*r = ProjectFolderMoveRequest(decoded)
	return nil
}

// UnmarshalJSON rejects invalid UTF-8 before encoding/json can replace it.
func (r *ProjectFolderCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain ProjectFolderCreateRequest
	var decoded plain
	if _, err := decodeFolderJSON(raw, &decoded); err != nil {
		return err
	}
	*r = ProjectFolderCreateRequest(decoded)
	return nil
}

// List returns current directories and safe eligible cover identities.
// @Summary 项目目录列表
// @Tags projects
// @ID listProjectFolders
// @Produce json
// @Param limit query int false "分页大小，上限200"
// @Param cursor query string false "当前主体目录分页游标"
// @Success 200 {object} ProjectFolderListResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-folders [get]
func (h *ProjectFolderHandler) List(c *gin.Context) {
	in := application.FolderListInput{Limit: 50}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		in.Limit = limit
	}
	actor := identityhttp.Principal(c)
	binding := cursorBinding(actor, "project-folders", in.Limit)
	if raw := c.Query("cursor"); raw != "" {
		if len(raw) > base64.RawURLEncoding.EncodedLen(512) {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		var cur cursor
		body, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(body) > 512 || json.Unmarshal(body, &cur) != nil || cur.ID == uuid.Nil || cur.Time.IsZero() || cur.Binding != binding {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		in.After = &application.ProjectListCursor{ID: cur.ID, UpdateTime: cur.Time}
	}
	page, err := h.service.List(c.Request.Context(), actor, in)
	if err != nil {
		writeFolderError(c, err)
		return
	}
	out := ProjectFolderListResponse{Items: []ProjectFolderResponse{}, CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID}
	for _, item := range page.Items {
		response := folderResponse(item.Folder)
		response.ProjectCount = &item.ProjectCount
		response.CoverUnavailable = item.CoverUnavailable
		out.Items = append(out.Items, response)
	}
	if page.Next != nil {
		body, _ := json.Marshal(cursor{ID: page.Next.ID, Time: page.Next.UpdateTime, Binding: binding})
		next := base64.RawURLEncoding.EncodeToString(body)
		out.NextCursor = &next
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, out)
}

// Create creates one directory without a parent hierarchy.
// @Summary 新建项目目录
// @Tags projects
// @ID createProjectFolder
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectFolderCreateRequest true "目录名称及可选正式封面"
// @Success 200 {object} ProjectFolderChangeResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-folders [post]
func (h *ProjectFolderHandler) Create(c *gin.Context) {
	var body ProjectFolderCreateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	h.change(c, application.FolderChangeInput{Action: "create", Name: &body.Name, SetCover: body.Cover != nil, Cover: body.Cover})
}

// Update changes only the observed directory's name or eligible cover.
// @Summary 修改项目目录
// @Tags projects
// @ID updateProjectFolder
// @Accept json
// @Produce json
// @Param folder_id path string true "目录UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectFolderUpdateRequest true "目录修订及字段，cover=null移除封面"
// @Success 200 {object} ProjectFolderChangeResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-folders/{folder_id} [patch]
func (h *ProjectFolderHandler) Update(c *gin.Context) {
	var body ProjectFolderUpdateRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	h.change(c, application.FolderChangeInput{Action: "patch", ExpectedRevision: *body.ExpectedRevision, Name: body.Name, SetCover: body.SetCover, Cover: body.Cover})
}

// Move changes personal classification without altering project content revision.
// @Summary 移动项目目录位置
// @Tags projects
// @ID moveProjectToFolder
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectFolderMoveRequest true "观察到的项目、放置与目标目录修订，folder_id=null移至根目录"
// @Success 200 {object} ProjectFolderChangeResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/folder [put]
func (h *ProjectFolderHandler) Move(c *gin.Context) {
	var body ProjectFolderMoveRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if !body.HasFolder || body.ExpectedProjectRevision == nil || body.ExpectedPlacementRevision == nil || body.FolderID != nil && *body.FolderID == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	in := application.FolderChangeInput{Action: "move", ExpectedRevision: body.ExpectedFolderRevision, ExpectedProjectRevision: *body.ExpectedProjectRevision, ExpectedPlacementRevision: *body.ExpectedPlacementRevision}
	if body.FolderID != nil {
		in.FolderID = *body.FolderID
	}
	h.change(c, in)
}

// Recycle atomically moves every eligible directory member into the project recycle bin.
// @Summary 回收目录及其中全部项目
// @Tags projects
// @ID recycleProjectFolder
// @Accept json
// @Produce json
// @Param folder_id path string true "目录UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "观察到的目录修订"
// @Success 200 {object} ProjectFolderChangeResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-folders/{folder_id} [delete]
func (h *ProjectFolderHandler) Recycle(c *gin.Context) {
	var body ProjectTransitionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	h.change(c, application.FolderChangeInput{Action: "recycle", ExpectedRevision: *body.ExpectedRevision})
}

func (h *ProjectFolderHandler) change(c *gin.Context, in application.FolderChangeInput) {
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	in.IdempotencyKey = key
	in.RequestID = c.GetString("request_id")
	if in.Action == "move" {
		id, err := uuid.Parse(c.Param("pid"))
		if err != nil || id == uuid.Nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		in.ProjectID = id
	} else if in.Action != "create" {
		id, err := uuid.Parse(c.Param("folder_id"))
		if err != nil || id == uuid.Nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		in.FolderID = id
	}
	out, err := h.service.Change(c.Request.Context(), identityhttp.Principal(c), in)
	if err != nil {
		writeFolderError(c, err)
		return
	}
	response := ProjectFolderChangeResponse{RecycledProjectIDs: out.RecycledProjectIDs}
	if out.Folder != nil {
		f := folderResponse(*out.Folder)
		response.Folder = &f
	}
	if out.Placement != nil {
		response.Placement = &ProjectFolderPlacementResponse{ProjectID: out.Placement.ProjectID, FolderID: out.Placement.FolderID, Revision: out.Placement.Revision}
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, response)
}

func folderResponse(f domain.ProjectFolder) ProjectFolderResponse {
	return ProjectFolderResponse{ID: f.ID, Name: f.Name, Cover: f.Cover, Revision: f.Revision, IsDelete: f.IsDelete, DeleteTime: f.DeleteTime, CreateTime: f.CreateTime, UpdateTime: f.UpdateTime}
}

func writeFolderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, domain.ErrProjectFolderNotFound), errors.Is(err, application.ErrProjectNotFound), errors.Is(err, mediaapp.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, domain.ErrProjectFolderRevisionConflict), errors.Is(err, domain.ErrProjectRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, domain.ErrProjectHasInflightOperations):
		httpapi.WriteProblem(c, 409, "inflight_work", nil)
	case errors.Is(err, domain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "state_conflict", nil)
	case errors.Is(err, domain.ErrProjectFolderCoverUnavailable):
		httpapi.WriteProblem(c, 409, "cover_unavailable", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_key_reused", nil)
	case errors.Is(err, domain.ErrInvalidProjectFolder):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
