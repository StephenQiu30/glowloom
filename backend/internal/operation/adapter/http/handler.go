// Package http exposes project-authorized quotes, tasks, and durable workflow controls.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	billingdomain "github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	promptapp "github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
)

// Dependencies are the commands and queries consumed by the public workbench.
type Dependencies struct {
	FreeQuote    *application.CreateFreeQuoteCommand
	BatchQuote   *application.CreateBatchFreeQuoteCommand
	Confirm      *application.ConfirmSingleQuoteCommand
	ConfirmBatch *application.ConfirmBatchQuoteCommand
	Query        *application.PublicQuery
	Control      *application.WorkflowControlCommand
}

// Handler binds the workbench to one durable operation system.
type Handler struct{ dependencies Dependencies }

// NewHandler injects commands and query adapters from the composition root.
func NewHandler(dependencies Dependencies) *Handler { return &Handler{dependencies: dependencies} }

// Register installs project-scoped task and quote routes.
func (h *Handler) Register(group *gin.RouterGroup) {
	group.POST("/projects/:pid/free-operations", h.FreeQuote)
	group.POST("/projects/:pid/quotes", h.Quotes)
	group.GET("/projects/:pid/operations", h.List)
	group.GET("/operations/:id", h.Task)
	group.POST("/operations/:id/confirm", h.Confirm)
	group.POST("/operations/:id/cancel", h.Cancel)
	group.GET("/batches/:id", h.Batch)
	group.POST("/batches/:id/confirm", h.ConfirmBatch)
	group.POST("/batches/:id/cancel", h.CancelBatch)
	group.POST("/batches/:id/resume", h.ResumeBatch)
}

// QuoteMediaInput preserves ordered, project-owned reference roles.
type QuoteMediaInput struct {
	Role         string    `json:"role"`
	MediaAssetID uuid.UUID `json:"media_asset_id"`
}

// QuoteItemRequest contains user input; model and price versions freeze server-side.
type QuoteItemRequest struct {
	ModelKey        string                     `json:"model_key"`
	Capability      string                     `json:"capability"`
	Mode            string                     `json:"mode"`
	Prompt          string                     `json:"prompt"`
	Params          json.RawMessage            `json:"params" swaggertype:"object"`
	OutputCount     int32                      `json:"output_count"`
	ForceRegenerate bool                       `json:"force_regenerate"`
	MediaInputs     []QuoteMediaInput          `json:"media_inputs"`
	Source          *domain.CanvasSource       `json:"source,omitempty" extensions:"x-nullable"`
	PromptTemplate  *promptapp.TemplateRequest `json:"prompt_template,omitempty"`
}

func (i QuoteItemRequest) input() application.FreeQuoteItemInput {
	media := make([]application.FreeQuoteMediaInput, 0, len(i.MediaInputs))
	for _, item := range i.MediaInputs {
		media = append(media, application.FreeQuoteMediaInput{Role: item.Role, MediaAssetID: item.MediaAssetID})
	}
	return application.FreeQuoteItemInput{ModelKey: i.ModelKey, Capability: i.Capability, Mode: i.Mode, Prompt: i.Prompt, Params: i.Params, OutputCount: i.OutputCount, ForceRegenerate: i.ForceRegenerate, MediaInputs: media, Source: i.Source, PromptTemplate: i.PromptTemplate}
}

// QuoteResponse is an immutable estimate, not an execution result.
type QuoteResponse struct {
	OperationID       uuid.UUID                 `json:"operation_id"`
	QuoteMicros       int64                     `json:"quote_micros"`
	QuoteDetail       json.RawMessage           `json:"quote_detail" swaggertype:"object"`
	AvailableMicros   int64                     `json:"available_micros"`
	ExpiresAt         time.Time                 `json:"expires_at"`
	ReusedFromID      *uuid.UUID                `json:"reused_from_id" extensions:"x-nullable"`
	Confirmable       bool                      `json:"confirmable"`
	PromptPreparation *domain.PromptPreparation `json:"prompt_preparation,omitempty"`
	FinalPrompt       string                    `json:"final_prompt,omitempty"`
}

// QuotesRequest contains one or more independent free-generation items.
type QuotesRequest struct {
	Items []QuoteItemRequest `json:"items"`
}

// FreeQuote creates an estimate without starting a paid request.
// @Summary 自由生成报价
// @ID createFreeQuote
// @Tags operations
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body QuoteItemRequest true "生成输入"
// @Success 201 {object} QuoteResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/free-operations [post]
func (h *Handler) FreeQuote(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var body QuoteItemRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.dependencies.FreeQuote.Execute(c.Request.Context(), identityhttp.Principal(c), application.CreateFreeQuoteInput{ProjectID: project, RequestID: c.GetHeader("Idempotency-Key"), FreeQuoteItemInput: body.input()})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(201, quoteResponse(result))
}

func quoteResponse(result application.CreateFreeQuoteResult) QuoteResponse {
	return QuoteResponse{OperationID: result.OperationID, QuoteMicros: result.QuoteMicros, QuoteDetail: result.QuoteDetail, AvailableMicros: result.AvailableMicros, ExpiresAt: result.ExpiresAt, ReusedFromID: result.ReusedFromID, Confirmable: result.Confirmable, PromptPreparation: result.PromptPreparation, FinalPrompt: result.FinalPrompt}
}

// Quotes creates a bounded batch, preserving invalid-item results in request order.
// @Summary 单项或批量自由生成报价
// @ID createGenerationQuotes
// @Tags operations
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body QuotesRequest true "1..150个生成输入"
// @Success 201 {object} application.CreateBatchFreeQuoteResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/quotes [post]
func (h *Handler) Quotes(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	var body QuotesRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	if len(body.Items) < 1 || len(body.Items) > 150 {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	if len(body.Items) == 1 {
		result, err := h.dependencies.FreeQuote.Execute(c.Request.Context(), identityhttp.Principal(c), application.CreateFreeQuoteInput{ProjectID: project, RequestID: c.GetHeader("Idempotency-Key"), FreeQuoteItemInput: body.Items[0].input()})
		if err != nil {
			writeError(c, err)
			return
		}
		item := body.Items[0]
		c.JSON(201, application.CreateBatchFreeQuoteResult{ExpiresAt: result.ExpiresAt, Items: []application.BatchFreeQuoteItemResult{{OperationID: &result.OperationID, ModelKey: item.ModelKey, Mode: item.Mode, QuoteMicros: &result.QuoteMicros, QuoteDetail: result.QuoteDetail, ReusedFromID: result.ReusedFromID, PromptPreparation: result.PromptPreparation, FinalPrompt: result.FinalPrompt}}, TotalMicros: result.QuoteMicros, AvailableMicros: result.AvailableMicros, Confirmable: result.Confirmable})
		return
	}
	items := make([]application.FreeQuoteItemInput, 0, len(body.Items))
	for _, item := range body.Items {
		items = append(items, item.input())
	}
	result, err := h.dependencies.BatchQuote.Execute(c.Request.Context(), identityhttp.Principal(c), application.CreateBatchFreeQuoteInput{ProjectID: project, RequestID: c.GetHeader("Idempotency-Key"), Items: items})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(201, result)
}

// ProjectRequest binds a task mutation to its current project.
type ProjectRequest struct {
	ProjectID uuid.UUID `json:"project_id"`
}

// ConfirmResponse exposes only the committed reservation and balance.
type ConfirmResponse struct {
	ReservationID   uuid.UUID `json:"reservation_id"`
	AvailableMicros int64     `json:"available_micros"`
	BudgetRevision  int64     `json:"budget_revision"`
}

// Confirm explicitly authorizes the already frozen single quote.
// @Summary 确认单项报价并预留预算
// @ID confirmOperationQuote
// @Tags operations
// @Accept json
// @Produce json
// @Param id path string true "Operation UUID"
// @Param Idempotency-Key header string true "确认请求UUID"
// @Param body body ProjectRequest true "项目"
// @Success 200 {object} ConfirmResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body ProjectRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.dependencies.Confirm.Execute(c.Request.Context(), identityhttp.Principal(c), application.ConfirmSingleQuoteInput{ProjectID: body.ProjectID, OperationID: id, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, ConfirmResponse{ReservationID: result.ReservationID, AvailableMicros: result.AvailableMicros, BudgetRevision: result.BudgetRevision})
}

// ConfirmBatchRequest contains only project scope and explicitly excluded quotes.
type ConfirmBatchRequest struct {
	ProjectID           uuid.UUID   `json:"project_id"`
	ExcludeOperationIDs []uuid.UUID `json:"exclude_operation_ids"`
}

// ConfirmBatchItem reports expired or confirmed members without hiding their reason.
type ConfirmBatchItem struct {
	OperationID uuid.UUID     `json:"operation_id"`
	Status      domain.Status `json:"status"`
	Reasons     []string      `json:"reasons"`
}

// ConfirmBatchResponse contains the result committed by the batch confirmation.
type ConfirmBatchResponse struct {
	Items            []ConfirmBatchItem `json:"items"`
	ConfirmedCount   int32              `json:"confirmed_count"`
	QuoteTotalMicros int64              `json:"quote_total_micros"`
	AvailableMicros  int64              `json:"available_micros"`
	BudgetRevision   int64              `json:"budget_revision"`
}

// ConfirmBatch authorizes only the explicitly selected, still current quotes.
// @Summary 确认批量报价
// @ID confirmBatchQuote
// @Tags operations
// @Accept json
// @Produce json
// @Param id path string true "Batch UUID"
// @Param Idempotency-Key header string true "确认请求UUID"
// @Param body body ConfirmBatchRequest true "确认范围"
// @Success 200 {object} ConfirmBatchResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/batches/{id}/confirm [post]
func (h *Handler) ConfirmBatch(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body ConfirmBatchRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.dependencies.ConfirmBatch.Execute(c.Request.Context(), identityhttp.Principal(c), application.ConfirmBatchQuoteInput{ProjectID: body.ProjectID, BatchID: id, ExcludeOperationIDs: body.ExcludeOperationIDs, RequestID: c.GetHeader("Idempotency-Key")})
	response := ConfirmBatchResponse{Items: make([]ConfirmBatchItem, 0, len(result.Items)), ConfirmedCount: result.ConfirmedCount, QuoteTotalMicros: result.QuoteTotalMicros, AvailableMicros: result.AvailableMicros, BudgetRevision: result.BudgetRevision}
	for _, item := range result.Items {
		reasons := item.Reasons
		if reasons == nil {
			reasons = []string{}
		}
		response.Items = append(response.Items, ConfirmBatchItem{OperationID: item.OperationID, Status: item.Status, Reasons: reasons})
	}
	if err != nil {
		if errors.Is(err, application.ErrQuoteStale) {
			httpapi.WriteProblem(c, 409, "quote_stale", map[string]any{"items": response.Items})
			return
		}
		writeError(c, err)
		return
	}
	c.JSON(200, response)
}

// TaskPageResponse is a project-bound task navigation page.
type TaskPageResponse struct {
	Items      []application.TaskSummary `json:"items"`
	NextCursor *string                   `json:"next_cursor" extensions:"x-nullable"`
}
type taskCursor struct {
	ID         uuid.UUID `json:"id"`
	CreateTime time.Time `json:"create_time"`
	Binding    string    `json:"binding"`
}

// List restores real task state independently of browser-local editor state.
// @Summary 项目任务中心
// @ID listProjectOperations
// @Tags operations
// @Produce json
// @Param pid path string true "项目UUID"
// @Param status query string false "任务状态"
// @Param capability query string false "能力"
// @Param model_key query string false "模型"
// @Param origin query string false "来源"
// @Param canvas_id query string false "冻结来源画布UUID"
// @Param node_id query string false "冻结来源节点UUID，需canvas_id"
// @Param row_id query string false "冻结来源行UUID，需node_id"
// @Param limit query integer false "1..200，默认50"
// @Param cursor query string false "项目与筛选绑定游标"
// @Success 200 {object} TaskPageResponse
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/projects/{pid}/operations [get]
func (h *Handler) List(c *gin.Context) {
	project, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return
	}
	input := application.ListTasksInput{ProjectID: project, Status: c.Query("status"), Capability: c.Query("capability"), ModelKey: c.Query("model_key"), Origin: c.Query("origin"), Limit: 50}
	for _, field := range []struct {
		name  string
		value **uuid.UUID
	}{{"canvas_id", &input.CanvasID}, {"node_id", &input.NodeID}, {"row_id", &input.RowID}} {
		if raw := c.Query(field.name); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil || id == uuid.Nil {
				httpapi.WriteProblem(c, 422, "invalid_request", nil)
				return
			}
			*field.value = &id
		}
	}
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.Limit = value
	}
	actor := identityhttp.Principal(c)
	encoded, err := json.Marshal(struct {
		Actor, Org uuid.UUID
		Input      application.ListTasksInput
	}{actor.ID, actor.OrgID, input})
	if err != nil {
		writeError(c, err)
		return
	}
	digest := sha256.Sum256(encoded)
	binding := hex.EncodeToString(digest[:])
	if raw := c.Query("cursor"); raw != "" {
		if len(raw) > 1024 {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		var cur taskCursor
		body, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(body) > 768 || json.Unmarshal(body, &cur) != nil || cur.ID == uuid.Nil || cur.CreateTime.IsZero() || cur.Binding != binding {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return
		}
		input.After = &application.TaskCursor{ID: cur.ID, CreateTime: cur.CreateTime}
	}
	page, err := h.dependencies.Query.List(c.Request.Context(), actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response := TaskPageResponse{Items: page.Items}
	if response.Items == nil {
		response.Items = []application.TaskSummary{}
	}
	if page.Next != nil {
		body, err := json.Marshal(taskCursor{ID: page.Next.ID, CreateTime: page.Next.CreateTime, Binding: binding})
		if err != nil {
			writeError(c, err)
			return
		}
		value := base64.RawURLEncoding.EncodeToString(body)
		response.NextCursor = &value
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, response)
}

// Task returns frozen input, safe lifecycle history and output asset identities.
// @Summary 任务详情与候选
// @ID getOperation
// @Tags operations
// @Produce json
// @Param id path string true "Operation UUID"
// @Param project_id query string true "项目UUID"
// @Success 200 {object} application.TaskDetail
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/{id} [get]
func (h *Handler) Task(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	project, ok := queryProject(c)
	if !ok {
		return
	}
	result, err := h.dependencies.Query.Task(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Batch returns parent progress and the persisted bounded child list.
// @Summary 批次汇总与逐项任务
// @ID getOperationBatch
// @Tags operations
// @Produce json
// @Param id path string true "Batch UUID"
// @Param project_id query string true "项目UUID"
// @Success 200 {object} application.BatchDetail
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/batches/{id} [get]
func (h *Handler) Batch(c *gin.Context) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	project, ok := queryProject(c)
	if !ok {
		return
	}
	result, err := h.dependencies.Query.Batch(c.Request.Context(), identityhttp.Principal(c), project, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, result)
}

// Cancel persists cancellation intent without claiming the provider stopped.
// @Summary 请求取消任务
// @ID cancelOperation
// @Tags operations
// @Accept json
// @Produce json
// @Param id path string true "Operation UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body ProjectRequest true "项目"
// @Success 202 {object} application.WorkflowControlResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/operations/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) { h.control(c, "operation", "cancel") }

// CancelBatch prevents further child admission after durable signal delivery.
// @Summary 请求取消批次
// @ID cancelOperationBatch
// @Tags operations
// @Accept json
// @Produce json
// @Param id path string true "Batch UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body ProjectRequest true "项目"
// @Success 202 {object} application.WorkflowControlResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/batches/{id}/cancel [post]
func (h *Handler) CancelBatch(c *gin.Context) { h.control(c, "batch", "cancel") }

// ResumeBatch requests continuation of a paused, uncancelled batch.
// @Summary 请求继续暂停批次
// @ID resumeOperationBatch
// @Tags operations
// @Accept json
// @Produce json
// @Param id path string true "Batch UUID"
// @Param Idempotency-Key header string true "请求UUID"
// @Param body body ProjectRequest true "项目"
// @Success 202 {object} application.WorkflowControlResult
// @Failure 403 {object} httpapi.Problem
// @Failure 404 {object} httpapi.Problem
// @Failure 409 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Failure 503 {object} httpapi.Problem
// @Router /api/batches/{id}/resume [post]
func (h *Handler) ResumeBatch(c *gin.Context) { h.control(c, "batch", "resume") }

func (h *Handler) control(c *gin.Context, kind, action string) {
	id, ok := httpapi.PathUUID(c, "id")
	if !ok {
		return
	}
	var body ProjectRequest
	if !httpapi.Decode(c, &body) {
		return
	}
	result, err := h.dependencies.Control.Execute(c.Request.Context(), identityhttp.Principal(c), application.WorkflowControlInput{ProjectID: body.ProjectID, TargetID: id, TargetType: kind, Action: action, RequestID: c.GetHeader("Idempotency-Key")})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(202, result)
}

func queryProject(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Query("project_id"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return uuid.Nil, false
	}
	return id, true
}
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, promptapp.ErrTemplateInvalid):
		httpapi.WriteProblem(c, 422, "template_invalid", nil)
	case errors.Is(err, promptapp.ErrTemplateChanged):
		httpapi.WriteProblem(c, 409, "template_changed", nil)
	case errors.Is(err, promptapp.ErrTemplateContextUnavailable):
		httpapi.WriteProblem(c, 409, "context_unavailable", nil)
	case errors.Is(err, application.ErrQuoteSourceStale):
		httpapi.WriteProblem(c, 409, "canvas_revision_conflict", nil)
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrPublicNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrQuoteKeyReused), errors.Is(err, application.ErrConfirmationKeyReused), errors.Is(err, application.ErrWorkflowControlKeyReused):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrQuoteStale), errors.Is(err, domain.ErrQuoteExpired):
		httpapi.WriteProblem(c, 409, "quote_stale", nil)
	case errors.Is(err, billingdomain.ErrBudgetInsufficient):
		httpapi.WriteProblem(c, 409, "budget_insufficient", nil)
	case errors.Is(err, application.ErrFreeQuoteModelMissing), errors.Is(err, application.ErrWorkflowModelUnavailable):
		httpapi.WriteProblem(c, 409, "model_unavailable", nil)
	case errors.Is(err, application.ErrFreeQuoteInputNotReady), errors.Is(err, application.ErrWorkflowInputNotReady):
		httpapi.WriteProblem(c, 409, "input_not_ready", nil)
	case errors.Is(err, application.ErrWorkflowControlConflict), errors.Is(err, domain.ErrQuoteNotConfirmable):
		httpapi.WriteProblem(c, 409, "state_conflict", nil)
	case errors.Is(err, application.ErrConfirmationTargetUnavailable):
		httpapi.WriteProblem(c, 409, "target_unavailable", nil)
	case errors.Is(err, application.ErrInvalidFreeQuote), errors.Is(err, application.ErrInvalidBatchFreeQuote), errors.Is(err, application.ErrInvalidConfirmSingleQuote), errors.Is(err, application.ErrInvalidConfirmBatchQuote), errors.Is(err, application.ErrInvalidPublicQuery), errors.Is(err, application.ErrInvalidWorkflowControl):
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
	default:
		httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
	}
}
