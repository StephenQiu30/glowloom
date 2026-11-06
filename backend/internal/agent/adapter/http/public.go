package http

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/runtimeclient"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// ProjectAccess checks the caller's current access to an Agent project.
type ProjectAccess interface {
	AuthorizeProject(context.Context, identity.Principal, uuid.UUID, bool) error
}

// Runtime sends authenticated requests to the independent Agent runtime.
type Runtime interface {
	Request(context.Context, string, map[string]string, url.Values, any, string) (*stdhttp.Response, error)
}

// PublicHandler serves private Agent conversations under current business authority.
type PublicHandler struct {
	store      app.BusinessStore
	access     ProjectAccess
	runtime    Runtime
	canvas     func(context.Context, identity.Principal, uuid.UUID, uuid.UUID) error
	controls   app.ControlStatusReader
	inputs     app.ConversationInputFreezer
	history    app.ConversationHistoryReader
	unresolved app.UnresolvedConversationReader
	concurrent chan struct{}
}

// NewPublicHandler injects business authority, project access and the runtime client.
func NewPublicHandler(store app.BusinessStore, access ProjectAccess, runtime Runtime) *PublicHandler {
	return &PublicHandler{store: store, access: access, runtime: runtime, concurrent: make(chan struct{}, 64)}
}

// WithCanvasAccess injects current access checks for an optional canvas scope.
func (h *PublicHandler) WithCanvasAccess(check func(context.Context, identity.Principal, uuid.UUID, uuid.UUID) error) *PublicHandler {
	h.canvas = check
	return h
}

// Register installs this handler's HTTP routes.
func (h *PublicHandler) Register(g *gin.RouterGroup) {
	g = g.Group("", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		select {
		case h.concurrent <- struct{}{}:
			defer func() { <-h.concurrent }()
			c.Next()
		default:
			c.AbortWithStatusJSON(503, gin.H{"code": "agent_busy"})
		}
	})
	g.GET("/projects/:pid/agent/sessions", h.List)
	g.POST("/projects/:pid/agent/sessions", h.Create)
	g.GET("/projects/:pid/agent/sessions/:sid", h.Read)
	g.PATCH("/projects/:pid/agent/sessions/:sid", h.Update)
	g.POST("/projects/:pid/agent/sessions/:sid/runs", h.Start)
	g.GET("/projects/:pid/agent/sessions/:sid/runs/:rid", h.Run)
	g.POST("/projects/:pid/agent/sessions/:sid/runs/:rid/commands", h.Command)
	g.GET("/projects/:pid/agent/sessions/:sid/runs/:rid/events", h.Events)
	g.GET("/projects/:pid/agent/sessions/:sid/runs/:rid/commands/current", h.ControlStatus)
	g.GET("/projects/:pid/agent/sessions/:sid/runs", h.Runs)
	g.GET("/projects/:pid/agent/sessions/:sid/submissions/current", h.CurrentSubmission)
	g.GET("/projects/:pid/agent/sessions/:sid/submissions/:requestId", h.Submission)
	g.GET("/projects/:pid/agent/sessions/:sid/runs/:rid/messages", h.Messages)
	g.GET("/projects/:pid/agent/sessions/:sid/runs/:rid/messages/:messageId", h.Message)
}
func (h *PublicHandler) scope(c *gin.Context, write bool) (domain.Scope, bool) {
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return domain.Scope{}, false
	}
	actor := identityhttp.Principal(c)
	if h.access == nil || h.runtime == nil || !(domain.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: pid}).Valid() {
		send(c, nil, domain.ErrUnavailable)
		return domain.Scope{}, false
	}
	if err := h.access.AuthorizeProject(c.Request.Context(), actor, pid, write); err != nil {
		send(c, nil, err)
		return domain.Scope{}, false
	}
	return domain.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: pid}, true
}
func scopeQuery(s domain.Scope) url.Values {
	return url.Values{"actor_id": {s.ActorID.String()}, "project_id": {s.ProjectID.String()}}
}
func canvasString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	v := id.String()
	return &v
}
func (h *PublicHandler) proxy(c *gin.Context, operation string, scope domain.Scope, body any) {
	if h.runtime == nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	params := map[string]string{}
	if sid := c.Param("sid"); sid != "" {
		parsed, err := uuid.Parse(sid)
		if err != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		params["sid"] = parsed.String()
	}
	if rid := c.Param("rid"); rid != "" {
		if _, err := uuid.Parse(rid); err != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		params["rid"] = rid
	}
	if value := c.Param("requestId"); value != "" {
		if _, err := uuid.Parse(value); err != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		params["requestId"] = value
	}
	if value := c.Param("messageId"); value != "" {
		if len(value) > 256 {
			send(c, nil, domain.ErrInvalid)
			return
		}
		params["messageId"] = value
	}
	query := scopeQuery(scope)
	for _, key := range []string{"limit", "cursor", "before", "through"} {
		if value := c.Query(key); value != "" {
			query.Set(key, value)
		}
	}
	response, err := h.runtime.Request(c.Request.Context(), operation, params, query, body, "")
	if err != nil {
		send(c, nil, err)
		return
	}
	// Closing the read-only response cannot change the received result.
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 || !json.Valid(data) {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && (operation == "getAgentSession" || operation == "createAgentSession" || operation == "updateAgentSession") {
		var session runtimeclient.SessionView
		if json.Unmarshal(data, &session) != nil || session.ActorID != scope.ActorID.String() || session.OrgID != scope.OrgID.String() || session.ScopeID != scope.ScopeID.String() || session.ProjectID != scope.ProjectID.String() || (params["sid"] != "" && session.ID != params["sid"]) {
			send(c, nil, domain.ErrNotFound)
			return
		}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && operation == "listAgentSessions" {
		var page struct {
			Items      []runtimeclient.SessionView `json:"items"`
			NextCursor string                      `json:"next_cursor,omitempty"`
		}
		if json.Unmarshal(data, &page) != nil || page.Items == nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		filtered := make([]runtimeclient.SessionView, 0, len(page.Items))
		for _, session := range page.Items {
			if session.ActorID == scope.ActorID.String() && session.OrgID == scope.OrgID.String() && session.ScopeID == scope.ScopeID.String() && session.ProjectID == scope.ProjectID.String() {
				filtered = append(filtered, session)
			}
		}
		page.Items = filtered
		c.JSON(response.StatusCode, page)
		return
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if operation == "getCurrentAgentSubmission" && h.unresolved != nil {
			ids, readErr := h.unresolved.UnresolvedConversationRuns(c.Request.Context(), scope.Principal(), scope.ProjectID, uuid.MustParse(c.Param("sid")))
			if readErr != nil {
				send(c, nil, readErr)
				return
			}
			var result map[string]json.RawMessage
			if json.Unmarshal(data, &result) != nil {
				send(c, nil, domain.ErrUnavailable)
				return
			}
			var prior []uuid.UUID
			_ = json.Unmarshal(result["unresolved_run_ids"], &prior)
			if prior == nil {
				prior = make([]uuid.UUID, 0)
			}
			seen := map[uuid.UUID]bool{}
			for _, id := range prior {
				seen[id] = true
			}
			for _, id := range ids {
				if !seen[id] {
					prior = append(prior, id)
					seen[id] = true
				}
			}
			result["unresolved_run_ids"], _ = json.Marshal(prior)
			data, _ = json.Marshal(result)
		}
		if h.history != nil && (operation == "getAgentSession" || operation == "getAgentRun") {
			var result map[string]json.RawMessage
			if json.Unmarshal(data, &result) != nil {
				send(c, nil, domain.ErrUnavailable)
				return
			}
			run := result
			if operation == "getAgentSession" && len(result["run"]) > 0 && string(result["run"]) != "null" {
				run = map[string]json.RawMessage{}
				if json.Unmarshal(result["run"], &run) != nil {
					send(c, nil, domain.ErrUnavailable)
					return
				}
			}
			if raw := run["input"]; len(raw) > 0 && string(raw) != "null" {
				filtered, readErr := h.history.ReadConversationInput(c.Request.Context(), scope.Principal(), scope.ProjectID, raw)
				if readErr != nil {
					send(c, nil, readErr)
					return
				}
				run["input"] = filtered
				// The internal goal is compiled source context, never a second public copy.
				var view struct {
					PlainText string `json:"plain_text"`
				}
				_ = json.Unmarshal(filtered, &view)
				run["goal"], _ = json.Marshal(view.PlainText)
				if operation == "getAgentSession" {
					result["run"], _ = json.Marshal(run)
				}
				data, _ = json.Marshal(result)
			}
		}
	}
	c.Data(response.StatusCode, "application/json", data)
}

// CreateRequest is public input; identity and scope are supplied by Go.
type CreateRequest struct {
	RequestID uuid.UUID  `json:"request_id"`
	Title     string     `json:"title"`
	CanvasID  *uuid.UUID `json:"canvas_id,omitempty"`
}

// Create opens an actor-private session in the independent Agent service.
// @Summary 新建项目 Agent 私有会话
// @ID createAgentSession
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param body body CreateRequest true "会话"
// @Success 201 {object} runtimeclient.SessionView
// @Router /api/projects/{pid}/agent/sessions [post]
func (h *PublicHandler) Create(c *gin.Context) {
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	var in CreateRequest
	if !httpapi.Decode(c, &in) {
		return
	}
	if in.RequestID == uuid.Nil || strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 240 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	scope.CanvasID = in.CanvasID
	if !h.checkCanvas(c, scope) {
		return
	}
	body := runtimeclient.CreateSessionRequest{ActorID: scope.ActorID.String(), OrgID: scope.OrgID.String(), ScopeID: scope.ScopeID.String(), ProjectID: scope.ProjectID.String(), CanvasID: canvasString(scope.CanvasID), RequestID: in.RequestID.String(), ContentHash: domain.ContentHash(in), Title: in.Title}
	h.proxy(c, "createAgentSession", scope, body)
}
func (h *PublicHandler) checkCanvas(c *gin.Context, scope domain.Scope) bool {
	if scope.CanvasID == nil {
		return true
	}
	if *scope.CanvasID == uuid.Nil || h.canvas == nil {
		send(c, nil, domain.ErrInvalid)
		return false
	}
	if err := h.canvas(c.Request.Context(), identityhttp.Principal(c), scope.ProjectID, *scope.CanvasID); err != nil {
		send(c, nil, err)
		return false
	}
	return true
}

// List reads the current actor's sessions.
// @Summary 当前项目我的 Agent 会话
// @ID listAgentSessions
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param limit query int false "页大小，默认20，最大50"
// @Param cursor query string false "历史游标"
// @Success 200 {object} runtimeclient.SessionPage
// @Router /api/projects/{pid}/agent/sessions [get]
func (h *PublicHandler) List(c *gin.Context) {
	scope, ok := h.scope(c, false)
	if ok {
		h.proxy(c, "listAgentSessions", scope, nil)
	}
}

// Read reads one private session with its consistent snapshot cursor.
// @Summary 读取 Agent 会话
// @ID getAgentSession
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Success 200 {object} runtimeclient.SessionView
// @Router /api/projects/{pid}/agent/sessions/{sid} [get]
func (h *PublicHandler) Read(c *gin.Context) {
	scope, ok := h.scope(c, false)
	if ok {
		h.proxy(c, "getAgentSession", scope, nil)
	}
}

// UpdateSessionRequest describes a single history operation.
type UpdateSessionRequest struct {
	Action string  `json:"action" binding:"required" enums:"pin,unpin,rename,delete"`
	Title  *string `json:"title,omitempty" maxLength:"240"`
}

// Update pins, unpins, renames, or removes an owned conversation from history.
// @Summary 管理 Agent 对话历史
// @ID updateAgentSession
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param input body UpdateSessionRequest true "历史操作"
// @Success 200 {object} runtimeclient.SessionView
// @Router /api/projects/{pid}/agent/sessions/{sid} [patch]
func (h *PublicHandler) Update(c *gin.Context) {
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	var input UpdateSessionRequest
	if c.ShouldBindJSON(&input) != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	switch input.Action {
	case "rename":
		if input.Title == nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
		title := strings.TrimSpace(*input.Title)
		if title == "" || utf8.RuneCountInString(title) > 240 {
			send(c, nil, domain.ErrInvalid)
			return
		}
		input.Title = &title
	case "pin", "unpin", "delete":
		if input.Title != nil {
			send(c, nil, domain.ErrInvalid)
			return
		}
	default:
		send(c, nil, domain.ErrInvalid)
		return
	}
	h.proxy(c, "updateAgentSession", scope, runtimeclient.UpdateSessionRequest{
		ActorID: scope.ActorID.String(), OrgID: scope.OrgID.String(),
		ScopeID: scope.ScopeID.String(), ProjectID: scope.ProjectID.String(),
		Action: input.Action, Title: input.Title,
	})
}

// StartRequest contains the full draft, model manifest and expected execution revision.
type StartRequest struct {
	RequestID                 uuid.UUID       `json:"request_id"`
	ExpectedExecutionRevision int64           `json:"expected_execution_revision"`
	Input                     json.RawMessage `json:"input" swaggertype:"object"`
	QuotedFrom                *app.QuoteRef   `json:"quoted_from,omitempty"`
	RetryOfRunID              *uuid.UUID      `json:"retry_of_run_id,omitempty"`
	Manifest                  json.RawMessage `json:"manifest" swaggertype:"object"`
	BudgetMicros              int64           `json:"budget_micros"`
	CanvasID                  *uuid.UUID      `json:"canvas_id,omitempty"`
}

// Start admits business scope before forwarding an idempotent accepted run.
// @Summary 提交 Agent 创作目标
// @ID createAgentRun
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param body body StartRequest true "创作目标"
// @Success 202 {object} runtimeclient.AcceptedRun
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs [post]
func (h *PublicHandler) Start(c *gin.Context) {
	scope, ok := h.scope(c, true)
	if !ok {
		return
	}
	sid, ok := httpapi.PathUUID(c, "sid")
	if !ok {
		return
	}
	var in StartRequest
	if !decodeConversation(c, &in) {
		return
	}
	var manifest map[string]any
	if in.RequestID == uuid.Nil || in.ExpectedExecutionRevision < 0 || in.ExpectedExecutionRevision > 9007199254740991 || len(in.Input) == 0 || json.Unmarshal(in.Manifest, &manifest) != nil || manifest == nil || in.BudgetMicros <= 0 || in.BudgetMicros > 1_000_000_000_000 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	scope.CanvasID = in.CanvasID
	if !h.checkCanvas(c, scope) {
		return
	}
	// Validate private session ownership at the Agent before creating business admission.
	response, err := h.runtime.Request(c.Request.Context(), "getAgentSession", map[string]string{"sid": sid.String()}, scopeQuery(scope), nil, "")
	if err != nil {
		send(c, nil, err)
		return
	}
	if response.StatusCode != 200 {
		// Closing the read-only response cannot change the received result.
		defer func() { _ = response.Body.Close() }()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if json.Valid(data) {
			c.Data(response.StatusCode, "application/json", data)
		} else {
			send(c, nil, domain.ErrUnavailable)
		}
		return
	}
	sessionBytes, readErr := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	_ = response.Body.Close()
	var session runtimeclient.SessionView
	if readErr != nil || len(sessionBytes) > 8<<20 || json.Unmarshal(sessionBytes, &session) != nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	if session.ID != sid.String() || session.ActorID != scope.ActorID.String() || session.OrgID != scope.OrgID.String() || session.ScopeID != scope.ScopeID.String() || session.ProjectID != scope.ProjectID.String() {
		send(c, nil, domain.ErrNotFound)
		return
	}
	if session.CanvasID != nil {
		canvas, err := uuid.Parse(*session.CanvasID)
		if err != nil {
			send(c, nil, domain.ErrUnavailable)
			return
		}
		if in.CanvasID != nil && *in.CanvasID != canvas {
			send(c, nil, domain.ErrConflict)
			return
		}
		scope.CanvasID = &canvas
	} else if in.CanvasID != nil {
		send(c, nil, domain.ErrConflict)
		return
	}
	if !h.checkCanvas(c, scope) {
		return
	}
	var pins struct {
		Skills []domain.SkillPin `json:"skills"`
	}
	if json.Unmarshal(in.Manifest, &pins) != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	var selection struct {
		ModelKey  string          `json:"model_key"`
		Params    json.RawMessage `json:"params"`
		MaxTokens int             `json:"max_tokens"`
	}
	if json.Unmarshal(in.Manifest, &selection) != nil || strings.TrimSpace(selection.ModelKey) == "" {
		send(c, nil, domain.ErrInvalid)
		return
	}
	if selection.MaxTokens == 0 {
		selection.MaxTokens = 8192
	}
	if selection.MaxTokens < 1 || selection.MaxTokens > 65536 {
		send(c, nil, domain.ErrInvalid)
		return
	}
	if len(selection.Params) == 0 {
		selection.Params = json.RawMessage(`{}`)
	}
	var modelParams map[string]json.RawMessage
	if json.Unmarshal(selection.Params, &modelParams) != nil || modelParams == nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	if h.inputs == nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	frozen, model, err := h.inputs.Prepare(c.Request.Context(), scope.Principal(), scope.ProjectID, sid, in.Input, in.QuotedFrom, selection.ModelKey)
	if err != nil {
		send(c, nil, err)
		return
	}
	if in.RetryOfRunID != nil {
		if release, ok := manifest["release_id"].(string); !ok || release != model.ReleaseID.String() {
			send(c, nil, domain.ErrConflict)
			return
		}
	}
	manifest["release_id"] = model.ReleaseID.String()
	if model.ContextWindow > 0 {
		manifest["context_window"] = model.ContextWindow
	}
	in.Manifest, err = json.Marshal(manifest)
	if err != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	modelSelection, marshalErr := json.Marshal(struct {
		ModelKey  string          `json:"model_key"`
		Params    json.RawMessage `json:"params"`
		MaxTokens int             `json:"max_tokens"`
		ReleaseID uuid.UUID       `json:"release_id"`
	}{selection.ModelKey, selection.Params, selection.MaxTokens, model.ReleaseID})
	if marshalErr != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	rid := domain.RootRunID(sid, in.RequestID)
	hash := domain.ContentHash(in)
	if err = h.store.AdmitRun(c.Request.Context(), domain.Admission{RootRunID: rid, SessionID: sid, SubmissionID: domain.SubmissionID(rid), RequestID: in.RequestID, ContentHash: hash, Scope: scope, BudgetMicros: in.BudgetMicros, SkillPins: pins.Skills, ModelSelection: modelSelection}); err != nil {
		send(c, nil, err)
		return
	}
	goal := frozen.Prompt
	if strings.TrimSpace(goal) == "" {
		goal = "请根据已提供的参考素材创作。"
	}
	body := runtimeclient.CreateRunRequest{ActorID: scope.ActorID.String(), OrgID: scope.OrgID.String(), ScopeID: scope.ScopeID.String(), ProjectID: scope.ProjectID.String(), CanvasID: canvasString(scope.CanvasID), RequestID: in.RequestID.String(), ContentHash: hash, ExpectedExecutionRevision: in.ExpectedExecutionRevision, Goal: goal, Manifest: in.Manifest}
	encoded, err := json.Marshal(frozen)
	if err != nil || json.Unmarshal(encoded, &body.Input) != nil {
		send(c, nil, domain.ErrInvalid)
		return
	}
	if in.RetryOfRunID != nil {
		value := in.RetryOfRunID.String()
		body.RetryOfRunID = &value
	}
	h.proxy(c, "createAgentRun", scope, body)
}
func (h *PublicHandler) authority(c *gin.Context) (domain.Authority, bool) {
	if h.store == nil {
		send(c, nil, domain.ErrUnavailable)
		return domain.Authority{}, false
	}
	pid, ok := httpapi.PathUUID(c, "pid")
	if !ok {
		return domain.Authority{}, false
	}
	rid, ok := httpapi.PathUUID(c, "rid")
	if !ok {
		return domain.Authority{}, false
	}
	sid, ok := httpapi.PathUUID(c, "sid")
	if !ok {
		return domain.Authority{}, false
	}
	a, err := h.store.ReadAuthority(c.Request.Context(), identityhttp.Principal(c), pid, rid)
	if err == nil && a.SessionID != sid {
		err = domain.ErrNotFound
	}
	if err != nil {
		send(c, nil, err)
		return a, false
	}
	return a, true
}

// Run reads Agent-owned execution state.
// @Summary 读取 Agent 执行快照
// @ID getAgentRun
// @Tags Agent
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param rid path string true "运行UUID"
// @Success 200 {object} runtimeclient.RunSnapshot
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs/{rid} [get]
func (h *PublicHandler) Run(c *gin.Context) {
	a, ok := h.authority(c)
	if ok {
		h.proxy(c, "getAgentRun", a.Scope, nil)
	}
}

// CommandReceipt exposes the durable receipt for an Agent control request.
type CommandReceipt struct {
	Decision       domain.Decision `json:"decision"`
	DeliveryStatus string          `json:"delivery_status"`
}

// Command durably fences business actions before reliable control delivery.
// @Summary 提交 Agent 控制决定
// @ID commandAgentRun
// @Tags Agent
// @Accept json
// @Produce json
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param rid path string true "运行UUID"
// @Param body body domain.DecisionInput true "控制决定"
// @Success 200 {object} CommandReceipt
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs/{rid}/commands [post]
func (h *PublicHandler) Command(c *gin.Context) {
	a, ok := h.authority(c)
	if !ok {
		return
	}
	var in domain.DecisionInput
	if !httpapi.Decode(c, &in) {
		return
	}
	out, err := h.store.Decide(c.Request.Context(), identityhttp.Principal(c), a.Scope.ProjectID, a.RootRunID, in)
	send(c, CommandReceipt{Decision: out, DeliveryStatus: "pending"}, err)
}

// Events proxies durable SSE and rechecks permission before each event frame.
// @Summary 订阅 Agent 已提交事件
// @ID subscribeAgentEvents
// @Tags Agent
// @Produce text/event-stream
// @Param pid path string true "项目UUID"
// @Param sid path string true "会话UUID"
// @Param rid path string true "运行UUID"
// @Success 200 {string} string
// @Router /api/projects/{pid}/agent/sessions/{sid}/runs/{rid}/events [get]
func (h *PublicHandler) Events(c *gin.Context) {
	a, ok := h.authority(c)
	if !ok {
		return
	}
	if h.runtime == nil {
		send(c, nil, domain.ErrUnavailable)
		return
	}
	response, err := h.runtime.Request(c.Request.Context(), "subscribeAgentEvents", map[string]string{"rid": a.RootRunID.String()}, scopeQuery(a.Scope), nil, c.GetHeader("Last-Event-ID"))
	if err != nil {
		send(c, nil, err)
		return
	}
	// Closing the read-only response cannot change the received result.
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if json.Valid(data) {
			c.Data(response.StatusCode, "application/json", data)
		} else {
			send(c, nil, domain.ErrUnavailable)
		}
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	var frame strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if frame.Len()+len(line)+1 > 1<<20 {
			return
		}
		frame.WriteString(line)
		frame.WriteByte('\n')
		if line != "" {
			continue
		}
		if _, err := h.store.ReadAuthority(c.Request.Context(), identityhttp.Principal(c), a.Scope.ProjectID, a.RootRunID); err != nil {
			return
		}
		if _, err = fmt.Fprint(c.Writer, frame.String()); err != nil {
			return
		}
		c.Writer.Flush()
		frame.Reset()
	}
}
