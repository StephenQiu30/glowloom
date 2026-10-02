package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyHandler exposes frozen content copying and explicit recovery controls.
type ProjectCopyHandler struct {
	service *application.ProjectCopyService
}

// NewProjectCopyHandler injects authorized whole-copy use cases.
func NewProjectCopyHandler(service *application.ProjectCopyService) *ProjectCopyHandler {
	return &ProjectCopyHandler{service: service}
}

// Register installs admission, bounded recovery reads and three durable controls.
func (h *ProjectCopyHandler) Register(g *gin.RouterGroup) {
	g.POST("/projects/:pid/copies", h.Create)
	g.GET("/projects/:pid/copies", h.List)
	g.GET("/project-copies/:id", h.Get)
	g.POST("/project-copies/:id/cancel", h.Cancel)
	g.POST("/project-copies/:id/retry", h.Retry)
	g.POST("/project-copies/:id/reconcile", h.Reconcile)
}

// ProjectCopyRequest binds the source revision, target name and optional personal placement CAS.
type ProjectCopyRequest struct {
	ExpectedRevision *int64                       `json:"expected_revision" binding:"required"`
	TargetName       string                       `json:"target_name" minLength:"1" maxLength:"50"`
	Placement        *ProjectCopyPlacementRequest `json:"placement,omitempty"`
}

// UnmarshalJSON rejects replacement of invalid UTF-8 and undeclared transport fields.
func (r *ProjectCopyRequest) UnmarshalJSON(body []byte) error {
	if !utf8.Valid(body) {
		return domain.ErrInvalidProjectCopy
	}
	type fields ProjectCopyRequest
	var value fields
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&value) != nil || d.Decode(new(any)) != io.EOF {
		return domain.ErrInvalidProjectCopy
	}
	var present map[string]json.RawMessage
	if json.Unmarshal(body, &present) != nil || present == nil {
		return domain.ErrInvalidProjectCopy
	}
	if raw, ok := present["placement"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return domain.ErrInvalidProjectCopy
	}
	*r = ProjectCopyRequest(value)
	return nil
}

// ProjectCopyResponse carries safe progress facts without keys, workers or snapshots.
type ProjectCopyResponse struct {
	SourceRevision          int64                      `json:"source_revision"`
	ID                      uuid.UUID                  `json:"id"`
	SourceProjectID         uuid.UUID                  `json:"source_project_id"`
	TargetProjectID         uuid.UUID                  `json:"target_project_id"`
	TargetName              string                     `json:"target_name"`
	Status                  string                     `json:"status" enums:"queued,running,failed,cancel_requested,cancelled,succeeded"`
	Stage                   string                     `json:"stage" enums:"media,bible,script,canvases,finalizing,cleanup,complete"`
	Revision                int64                      `json:"revision"`
	Attempt                 int64                      `json:"attempt"`
	Documents               int                        `json:"documents"`
	Assets                  int                        `json:"assets"`
	Renditions              int                        `json:"renditions"`
	CompletedDocuments      int                        `json:"completed_documents"`
	CompletedAssets         int                        `json:"completed_assets"`
	CompletedRenditions     int                        `json:"completed_renditions"`
	Retryable               bool                       `json:"retryable"`
	NeedsReconciliation     bool                       `json:"needs_reconciliation"`
	ReconciliationRequested bool                       `json:"reconciliation_requested"`
	ExecutionUnconfirmed    bool                       `json:"execution_unconfirmed"`
	CancellationRequested   bool                       `json:"cancellation_requested"`
	FailureCode             string                     `json:"failure_code,omitempty"`
	Script                  *ProjectCopyScriptProgress `json:"script,omitempty"`
	Bible                   *ProjectCopyBibleProgress  `json:"bible,omitempty"`
}

// ProjectCopyScriptCounts exposes only complete owning history/object cardinalities.
type ProjectCopyScriptCounts struct {
	Sources            int `json:"sources"`
	Versions           int `json:"versions"`
	VersionSources     int `json:"version_sources"`
	ProjectStates      int `json:"project_states"`
	VersionHeads       int `json:"version_heads"`
	SplitSets          int `json:"split_sets"`
	SplitConfirmations int `json:"split_confirmations"`
	Episodes           int `json:"episodes"`
	Structures         int `json:"structures"`
	Scenes             int `json:"scenes"`
	DialogueLines      int `json:"dialogue_lines"`
	ActionLines        int `json:"action_lines"`
	Objects            int `json:"objects"`
}

// ProjectCopyScriptProgress distinguishes admitted history from a completed receipt.
type ProjectCopyScriptProgress struct {
	Counts          ProjectCopyScriptCounts  `json:"counts"`
	CompletedCounts *ProjectCopyScriptCounts `json:"completed_counts,omitempty"`
}

func scriptCounts(c domain.ProjectCopyScriptCounts) ProjectCopyScriptCounts {
	return ProjectCopyScriptCounts{Sources: c.Sources, Versions: c.Versions, VersionSources: c.VersionSources, ProjectStates: c.ProjectStates, VersionHeads: c.VersionHeads, SplitSets: c.SplitSets, SplitConfirmations: c.SplitConfirmations, Episodes: c.Episodes, Structures: c.Structures, Scenes: c.Scenes, DialogueLines: c.DialogueLines, ActionLines: c.ActionLines, Objects: c.Objects}
}

// ProjectCopyBibleCounts exposes every complete historical set without private content.
type ProjectCopyBibleCounts struct {
	Characters             int `json:"characters"`
	CharacterVersions      int `json:"character_versions"`
	CharacterConfirmations int `json:"character_confirmations"`
	Locations              int `json:"locations"`
	LocationVersions       int `json:"location_versions"`
	LocationConfirmations  int `json:"location_confirmations"`
	Props                  int `json:"props"`
	PropVersions           int `json:"prop_versions"`
	PropConfirmations      int `json:"prop_confirmations"`
	Looks                  int `json:"looks"`
	LookVersions           int `json:"look_versions"`
	References             int `json:"references"`
	Voices                 int `json:"voices"`
	Redirects              int `json:"redirects"`
	Splits                 int `json:"splits"`
}

// ProjectCopyBibleProgress separates admission evidence from registered completion.
type ProjectCopyBibleProgress struct {
	Counts          ProjectCopyBibleCounts  `json:"counts"`
	CompletedCounts *ProjectCopyBibleCounts `json:"completed_counts,omitempty"`
}

func bibleCounts(c domain.ProjectCopyBibleCounts) ProjectCopyBibleCounts {
	return ProjectCopyBibleCounts{Characters: c.Characters, CharacterVersions: c.CharacterVersions, CharacterConfirmations: c.CharacterConfirmations, Locations: c.Locations, LocationVersions: c.LocationVersions, LocationConfirmations: c.LocationConfirmations, Props: c.Props, PropVersions: c.PropVersions, PropConfirmations: c.PropConfirmations, Looks: c.Looks, LookVersions: c.LookVersions, References: c.References, Voices: c.Voices, Redirects: c.Redirects, Splits: c.Splits}
}

// ProjectCopyListResponse permits recovery after an unknown 202 response or refresh.
type ProjectCopyListResponse struct {
	CurrentActorID uuid.UUID             `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID             `json:"current_org_id"`
	Copies         []ProjectCopyResponse `json:"copies"`
	NextCursor     *string               `json:"next_cursor" extensions:"x-nullable"`
}

func copyResponse(j domain.ProjectCopyJob) ProjectCopyResponse {
	r := ProjectCopyResponse{ID: j.ID, SourceProjectID: j.SourceProjectID, SourceRevision: j.SourceRevision, TargetProjectID: j.TargetProjectID, TargetName: j.TargetName, Status: j.Status, Stage: j.Stage, Revision: j.Revision, Attempt: j.Attempt, Documents: j.Manifest.Documents, Assets: j.Manifest.Assets, Renditions: j.Manifest.Renditions, Retryable: j.Retryable, NeedsReconciliation: j.NeedsReconciliation, ReconciliationRequested: j.ReconciliationRequested, ExecutionUnconfirmed: j.ExecutionUnconfirmed, CancellationRequested: j.CancellationRequested, FailureCode: j.FailureCode}
	if j.MediaReceipt != nil {
		r.CompletedAssets = j.MediaReceipt.PrimaryCount
		r.CompletedRenditions = j.MediaReceipt.SecondaryCount
	}
	if j.CanvasReceipt != nil {
		r.CompletedDocuments = j.CanvasReceipt.PrimaryCount
	}
	if j.Manifest.Script != nil {
		r.Script = &ProjectCopyScriptProgress{Counts: scriptCounts(j.Manifest.Script.Counts)}
		if j.ScriptReceipt != nil {
			counts := scriptCounts(j.ScriptReceipt.Counts)
			r.Script.CompletedCounts = &counts
		}
	}
	if j.Manifest.Bible != nil {
		r.Bible = &ProjectCopyBibleProgress{Counts: bibleCounts(j.Manifest.Bible.Counts)}
		if j.BibleReceipt != nil {
			counts := bibleCounts(j.BibleReceipt.Counts)
			r.Bible.CompletedCounts = &counts
		}
	}
	return r
}
func copyID(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return uuid.Nil, false
	}
	return id, true
}
func writeCopyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrProjectNotFound), errors.Is(err, application.ErrStylePresetNotFound), errors.Is(err, domain.ErrProjectFolderNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, domain.ErrProjectRevisionConflict), errors.Is(err, domain.ErrProjectFolderRevisionConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, domain.ErrProjectCopyStateConflict), errors.Is(err, domain.ErrProjectCopyWorkerConflict), errors.Is(err, domain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "state_conflict", nil)
	case errors.Is(err, domain.ErrProjectHasInflightOperations):
		httpapi.WriteProblem(c, 409, "inflight_work", nil)
	case errors.Is(err, canvasdomain.ErrUnsupportedProjectCopy):
		httpapi.WriteProblem(c, 409, "unsupported_copy_content", nil)
	case errors.Is(err, mediaapp.ErrProjectCopyConsentUnavailable):
		httpapi.WriteProblem(c, 409, "consent_context_unavailable", nil)
	case errors.Is(err, mediaapp.ErrProjectCopyMediaUnavailable):
		httpapi.WriteProblem(c, 409, "copy_media_unavailable", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 422, "idempotency_key_reused", nil)
	case errors.Is(err, domain.ErrInvalidProjectCopy), errors.Is(err, canvasdomain.ErrInvalidProjectCopy):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}

// Create accepts a copy without claiming the target is published.
// @Summary 创建完整项目内容副本
// @Tags projectCopies
// @ID createProjectCopy
// @Accept json
// @Produce json
// @Param pid path string true "源项目UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectCopyRequest true "冻结修订与目标名称"
// @Success 202 {object} ProjectCopyResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/copies [post]
func (h *ProjectCopyHandler) Create(c *gin.Context) {
	id, ok := copyID(c, "pid")
	if !ok {
		return
	}
	var body ProjectCopyRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	input := application.ProjectCopyInput{SourceProjectID: id, ExpectedRevision: *body.ExpectedRevision, TargetName: body.TargetName, IdempotencyKey: key, RequestID: c.GetString("request_id")}
	if body.Placement != nil {
		expect := body.Placement.expectation()
		input.Placement = &expect
	}
	job, err := h.service.Create(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		writeCopyError(c, err)
		return
	}
	c.JSON(202, copyResponse(job))
}

// Get returns actual phase receipts without exposing frozen content.
// @Summary 读取项目复制状态
// @Tags projectCopies
// @ID getProjectCopy
// @Produce json
// @Param id path string true "复制任务UUID"
// @Success 200 {object} ProjectCopyResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-copies/{id} [get]
func (h *ProjectCopyHandler) Get(c *gin.Context) {
	id, ok := copyID(c, "id")
	if !ok {
		return
	}
	job, err := h.service.Get(c.Request.Context(), identityhttp.Principal(c), id)
	if err != nil {
		writeCopyError(c, err)
		return
	}
	c.JSON(200, copyResponse(job))
}

// List restores this source's tasks through a bounded immutable keyset page.
// @Summary 分页恢复项目复制任务
// @Tags projectCopies
// @ID listProjectCopies
// @Produce json
// @Param pid path string true "源项目UUID"
// @Param limit query int false "1至100，默认50"
// @Param cursor query string false "上一页next_cursor"
// @Success 200 {object} ProjectCopyListResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/copies [get]
func (h *ProjectCopyHandler) List(c *gin.Context) {
	id, ok := copyID(c, "pid")
	if !ok {
		return
	}
	input := application.ProjectCopyListInput{SourceProjectID: id}
	if value := c.Query("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Limit = limit
	}
	if value := c.Query("cursor"); value != "" {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil || len(raw) > 512 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		var cursor application.ProjectCopyCursor
		if d.Decode(&cursor) != nil || d.Decode(new(any)) != io.EOF {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = &cursor
	}
	page, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		writeCopyError(c, err)
		return
	}
	principal := identityhttp.Principal(c)
	result := ProjectCopyListResponse{CurrentActorID: principal.ID, CurrentOrgID: principal.OrgID, Copies: make([]ProjectCopyResponse, 0, len(page.Jobs))}
	for _, job := range page.Jobs {
		result.Copies = append(result.Copies, copyResponse(job))
	}
	if page.Next != nil {
		raw, err := json.Marshal(page.Next)
		if err != nil {
			writeCopyError(c, err)
			return
		}
		value := base64.RawURLEncoding.EncodeToString(raw)
		result.NextCursor = &value
	}
	c.JSON(200, result)
}
func (h *ProjectCopyHandler) control(c *gin.Context, action string) {
	id, ok := copyID(c, "id")
	if !ok {
		return
	}
	var body ProjectTransitionRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if body.ExpectedRevision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_idempotency_key", nil)
		return
	}
	job, err := h.service.Change(c.Request.Context(), identityhttp.Principal(c), id, action, *body.ExpectedRevision, key, c.GetString("request_id"))
	if err != nil {
		writeCopyError(c, err)
		return
	}
	c.JSON(202, copyResponse(job))
}

// Cancel records intent and waits for verified owned cleanup.
// @Summary 请求取消项目复制
// @Tags projectCopies
// @ID cancelProjectCopy
// @Accept json
// @Produce json
// @Param id path string true "复制任务UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "任务修订"
// @Success 202 {object} ProjectCopyResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-copies/{id}/cancel [post]
func (h *ProjectCopyHandler) Cancel(c *gin.Context) { h.control(c, "cancel") }

// Retry resumes only a known stopped failure using its original manifest.
// @Summary 重试已知失败的项目复制
// @Tags projectCopies
// @ID retryProjectCopy
// @Accept json
// @Produce json
// @Param id path string true "复制任务UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "任务修订"
// @Success 202 {object} ProjectCopyResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-copies/{id}/retry [post]
func (h *ProjectCopyHandler) Retry(c *gin.Context) { h.control(c, "retry") }

// Reconcile authorizes exact-key readback after actual execution cessation.
// @Summary 核验原项目复制的未知结果
// @Tags projectCopies
// @ID reconcileProjectCopy
// @Accept json
// @Produce json
// @Param id path string true "复制任务UUID"
// @Param Idempotency-Key header string true "UUID幂等键"
// @Param body body ProjectTransitionRequest true "任务修订"
// @Success 202 {object} ProjectCopyResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/project-copies/{id}/reconcile [post]
func (h *ProjectCopyHandler) Reconcile(c *gin.Context) { h.control(c, "reconcile") }
