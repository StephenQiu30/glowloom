// Package http exposes authorized script editor contracts through the public REST API.
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// SourceHandler serves persistent source editing and selected private bodies.
type SourceHandler struct{ service *application.SourceService }

// NewSourceHandler injects the actual object/persistence source use case.
func NewSourceHandler(service *application.SourceService) *SourceHandler {
	return &SourceHandler{service: service}
}

// Register uses non-colon command paths compatible with Gin's parameter router.
func (h *SourceHandler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/script-workspace", h.Workspace)
	g.GET("/projects/:pid/script-sources", h.List)
	g.GET("/projects/:pid/script-sources/:lineage", h.Get)
	g.POST("/projects/:pid/script-sources", h.Create)
	g.PUT("/projects/:pid/script-sources/:lineage", h.Update)
	g.DELETE("/projects/:pid/script-sources/:lineage", h.Delete)
	g.POST("/projects/:pid/script-sources/import", h.Import)
	g.POST("/projects/:pid/script-sources/reorder", h.Reorder)
}

// SourceWriteRequest has explicit CAS and one complete rich or legacy HTML source.
type SourceWriteRequest struct {
	ExpectedRevision *int64                  `json:"expected_revision"`
	BaseVersionID    *uuid.UUID              `json:"base_version_id,omitempty"`
	RightsConfirmed  bool                    `json:"rights_confirmed"`
	Kind             string                  `json:"source_kind"`
	Title            string                  `json:"title"`
	Status           string                  `json:"status"`
	Document         *domain.RichDocument    `json:"document,omitempty"`
	OriginalHTML     *string                 `json:"original_html,omitempty"`
	Provenance       domain.SourceProvenance `json:"provenance"`
}

// SourceImportItem is an atomic chapter item with its preserved source-system labels.
type SourceImportItem struct {
	Kind         string                  `json:"source_kind"`
	Title        string                  `json:"title"`
	Status       string                  `json:"status"`
	Document     *domain.RichDocument    `json:"document,omitempty"`
	OriginalHTML *string                 `json:"original_html,omitempty"`
	Provenance   domain.SourceProvenance `json:"provenance"`
}

// SourceImportRequest accepts 1..2500 chapters as one complete atomic command.
type SourceImportRequest struct {
	ExpectedRevision *int64             `json:"expected_revision"`
	BaseVersionID    *uuid.UUID         `json:"base_version_id,omitempty"`
	RightsConfirmed  bool               `json:"rights_confirmed"`
	Sources          []SourceImportItem `json:"sources"`
}

// SourceReorderRequest binds the full current lineage set to its observed version.
type SourceReorderRequest struct {
	ExpectedRevision *int64      `json:"expected_revision"`
	BaseVersionID    *uuid.UUID  `json:"base_version_id,omitempty"`
	Order            []uuid.UUID `json:"source_lineage_ids"`
}

// SourceDeleteRequest only removes a lineage from a new draft, preserving history.
type SourceDeleteRequest struct {
	ExpectedRevision *int64     `json:"expected_revision"`
	BaseVersionID    *uuid.UUID `json:"base_version_id,omitempty"`
}

func readJSON(c *gin.Context, out any) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		httpapi.WriteProblem(c, 415, "unsupported_media_type", nil)
		return false
	}
	reader := http.MaxBytesReader(c.Writer, c.Request.Body, domain.MaxHTTPBytes)
	data, err := io.ReadAll(reader)
	if err != nil {
		httpapi.WriteProblem(c, 413, "input_too_large", nil)
		return false
	}
	if err := domain.ValidateJSON(data); err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return false
	}
	return true
}
func commandInput(c *gin.Context, action string, revision *int64, version *uuid.UUID) (application.SourceCommand, bool) {
	if revision == nil || *revision < 0 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return application.SourceCommand{}, false
	}
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return application.SourceCommand{}, false
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	request, requestErr := uuid.Parse(c.GetString("request_id"))
	if err != nil || requestErr != nil || key == uuid.Nil || request == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return application.SourceCommand{}, false
	}
	return application.SourceCommand{ProjectID: project, Key: key, RequestID: request, Action: action, ExpectedRevision: *revision, BaseVersionID: version}, true
}
func sourceInput(source SourceImportItem) (application.SourceInput, error) {
	if source.Document == nil && source.OriginalHTML == nil || source.Document != nil && source.OriginalHTML != nil {
		return application.SourceInput{}, domain.ErrInvalidDocument
	}
	var doc domain.RichDocument
	if source.OriginalHTML != nil {
		parsed, err := extract.HTML(*source.OriginalHTML)
		if err != nil {
			return application.SourceInput{}, err
		}
		doc = parsed
	} else {
		data, err := json.Marshal(source.Document)
		if err != nil {
			return application.SourceInput{}, err
		}
		parsed, err := domain.DecodeRichDocument(data)
		if err != nil {
			return application.SourceInput{}, err
		}
		doc = parsed
	}
	return application.SourceInput{Kind: source.Kind, Title: source.Title, Status: source.Status, Document: doc, OriginalHTML: source.OriginalHTML, Provenance: source.Provenance}, nil
}
func scriptProblem(c *gin.Context, err error) {
	var meta map[string]any
	var impact *application.ImpactError
	if errors.As(err, &impact) {
		meta = map[string]any{"affected_episodes": impact.Affected}
	}
	switch {
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrNotFound), errors.Is(err, workspaceapp.ErrProjectNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrConfirmationRequired):
		httpapi.WriteProblem(c, 409, "confirmation_required", meta)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrConflict), errors.Is(err, workspacedomain.ErrProjectRevisionConflict):
		httpapi.WriteProblem(c, 409, "stale_revision", nil)
	case errors.Is(err, workspacedomain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "readonly_project", nil)
	case errors.Is(err, domain.ErrInvalidDocument), errors.Is(err, domain.ErrInvalidSource), errors.Is(err, domain.ErrInvalidSpan), errors.Is(err, domain.ErrInvalidStructure):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	case errors.Is(err, application.ErrNeedsReconciliation):
		httpapi.WriteProblem(c, 503, "needs_reconciliation", nil)
	default:
		httpapi.WriteProblem(c, 503, "context_unavailable", meta)
	}
}
func versionQuery(c *gin.Context) (*uuid.UUID, bool) {
	if c.Query("version_id") == "" {
		return nil, true
	}
	id, err := uuid.Parse(c.Query("version_id"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return nil, false
	}
	return &id, true
}

// Workspace returns current draft/adopted heads without inserting an empty head.
// @Summary 剧本工作区头与当前主体
// @Tags script
// @ID getScriptWorkspace
// @Produce json
// @Param pid path string true "项目UUID"
// @Success 200 {object} application.WorkspaceView
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-workspace [get]
func (h *SourceHandler) Workspace(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	result, err := h.service.Workspace(c.Request.Context(), identityhttp.Principal(c), pid)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// List returns bounded source summaries; rich bodies are fetched by Get only.
// @Summary 来源章节有序分页
// @Tags script
// @ID listScriptSources
// @Produce json
// @Param pid path string true "项目UUID"
// @Param version_id query string false "不可变历史版本UUID"
// @Param after query int false "下一个来源位置，须与返回version_id绑定"
// @Param limit query int false "1..100"
// @Success 200 {object} application.SourcePage
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources [get]
func (h *SourceHandler) List(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	version, ok := versionQuery(c)
	if !ok {
		return
	}
	after, limit := 0, 50
	var err error
	if raw := c.Query("after"); raw != "" {
		after, err = strconv.Atoi(raw)
		if err != nil {
			scriptProblem(c, domain.ErrInvalidSource)
			return
		}
	}
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			scriptProblem(c, domain.ErrInvalidSource)
			return
		}
	}
	if after > 0 && version == nil {
		scriptProblem(c, domain.ErrInvalidSource)
		return
	}
	result, err := h.service.Sources(c.Request.Context(), identityhttp.Principal(c), pid, version, after, limit)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// Get returns the selected rich/private text from a stable source lineage.
// @Summary 来源富文本及规范原文
// @Tags script
// @ID getScriptSource
// @Produce json
// @Param pid path string true "项目UUID"
// @Param lineage path string true "稳定来源lineageUUID"
// @Param version_id query string false "历史版本UUID"
// @Success 200 {object} application.SourceDetail
// @Failure 404 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources/{lineage} [get]
func (h *SourceHandler) Get(c *gin.Context) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	lineage, ok := httpapi.PathUUID(c, "lineage")
	if !ok {
		return
	}
	version, ok := versionQuery(c)
	if !ok {
		return
	}
	result, err := h.service.Source(c.Request.Context(), identityhttp.Principal(c), pid, lineage, version)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

func (h *SourceHandler) writeOne(c *gin.Context, action string) {
	var request SourceWriteRequest
	if !readJSON(c, &request) {
		return
	}
	input, ok := commandInput(c, action, request.ExpectedRevision, request.BaseVersionID)
	if !ok {
		return
	}
	if action == "update" {
		lineage, ok := httpapi.PathUUID(c, "lineage")
		if !ok {
			return
		}
		input.LineageID = &lineage
	}
	source, err := sourceInput(SourceImportItem{Kind: request.Kind, Title: request.Title, Status: request.Status, Document: request.Document, OriginalHTML: request.OriginalHTML, Provenance: request.Provenance})
	if err != nil {
		scriptProblem(c, err)
		return
	}
	input.RightsConfirmed = request.RightsConfirmed
	input.Sources = []application.SourceInput{source}
	result, err := h.service.Write(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// Create preserves one new complete source and an immutable draft version.
// @Summary 新增剧本来源
// @Tags script
// @ID createScriptSource
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID，缺省由服务端生成"
// @Param body body SourceWriteRequest true "完整来源与版权确认"
// @Success 200 {object} application.SourceReceipt
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources [post]
func (h *SourceHandler) Create(c *gin.Context) { h.writeOne(c, "create") }

// Update adds an immutable snapshot under the unchanged source lineage.
// @Summary 保存来源富文本新版本
// @Tags script
// @ID updateScriptSource
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param lineage path string true "稳定来源UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID，缺省由服务端生成"
// @Param body body SourceWriteRequest true "完整来源及CAS"
// @Success 200 {object} application.SourceReceipt
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources/{lineage} [put]
func (h *SourceHandler) Update(c *gin.Context) { h.writeOne(c, "update") }

// Delete preserves all old snapshots while removing this lineage from a new draft.
// @Summary 从新草稿移除来源
// @Tags script
// @ID deleteScriptSource
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param lineage path string true "稳定来源UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID，缺省由服务端生成"
// @Param body body SourceDeleteRequest true "当前版本CAS"
// @Success 200 {object} application.SourceReceipt
// @Failure 409 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources/{lineage} [delete]
func (h *SourceHandler) Delete(c *gin.Context) {
	var r SourceDeleteRequest
	if !readJSON(c, &r) {
		return
	}
	input, ok := commandInput(c, "delete", r.ExpectedRevision, r.BaseVersionID)
	if !ok {
		return
	}
	lineage, ok := httpapi.PathUUID(c, "lineage")
	if !ok {
		return
	}
	input.LineageID = &lineage
	result, err := h.service.Write(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// Import validates every chapter before freezing the whole atomic write.
// @Summary 原子导入完整章节批次
// @Tags script
// @ID importScriptSources
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID，缺省由服务端生成"
// @Param body body SourceImportRequest true "1..2500来源"
// @Success 200 {object} application.SourceReceipt
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources/import [post]
func (h *SourceHandler) Import(c *gin.Context) {
	var r SourceImportRequest
	if !readJSON(c, &r) {
		return
	}
	input, ok := commandInput(c, "import", r.ExpectedRevision, r.BaseVersionID)
	if !ok {
		return
	}
	if len(r.Sources) < 1 || len(r.Sources) > domain.MaxChapterImport {
		scriptProblem(c, domain.ErrInvalidSource)
		return
	}
	input.RightsConfirmed = r.RightsConfirmed
	for _, item := range r.Sources {
		source, err := sourceInput(item)
		if err != nil {
			scriptProblem(c, err)
			return
		}
		input.Sources = append(input.Sources, source)
	}
	result, err := h.service.Write(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}

// Reorder requires all current source lineage IDs exactly once.
// @Summary 完整来源集合重排
// @Tags script
// @ID reorderScriptSources
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "UUID"
// @Param X-Request-Id header string false "追踪UUID，缺省由服务端生成"
// @Param body body SourceReorderRequest true "完整来源UUID顺序"
// @Success 200 {object} application.SourceReceipt
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/projects/{pid}/script-sources/reorder [post]
func (h *SourceHandler) Reorder(c *gin.Context) {
	var r SourceReorderRequest
	if !readJSON(c, &r) {
		return
	}
	input, ok := commandInput(c, "reorder", r.ExpectedRevision, r.BaseVersionID)
	if !ok {
		return
	}
	input.Order = r.Order
	result, err := h.service.Write(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		scriptProblem(c, err)
		return
	}
	c.JSON(200, result)
}
