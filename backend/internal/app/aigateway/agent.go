package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	evidencepg "github.com/StephenQiu30/glowloom-server/backend/creation/evidence/adapter/postgres"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
)

var errProjectMemoryChanged = errors.New("project memory changed")

// AgentModel uses the approved Agent grant as a caller authority, not as a wallet.
func (r *Runtime) AgentModel(ctx context.Context, scope agent.Scope, id uuid.UUID, raw json.RawMessage, maxMicros int64) (agent.CallResult, error) {
	if !scope.Valid() || id == uuid.Nil || maxMicros < 0 {
		return agentFailure("invalid_model_request"), nil
	}
	var input agentInput
	if json.Unmarshal(raw, &input) != nil || input.ModelKey == "" {
		return agentFailure("invalid_model_request"), nil
	}
	actor := identity.Principal{ID: scope.ActorID, OrgID: scope.OrgID, ScopeID: scope.ScopeID}
	var root uuid.UUID
	auth := func(ctx context.Context, tx *gorm.DB, a identity.Principal, sid, pid uuid.UUID, admin bool) error {
		if admin || a.ID != scope.ActorID || sid != scope.ScopeID || pid != scope.ProjectID {
			return domain.ErrForbidden
		}
		if _, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, a.ID, sid, true); err != nil {
			return domain.ErrForbidden
		}
		if err := projectScope(tx, sid, pid); err != nil {
			return err
		}
		if input.ProjectMemoryVersion != "" {
			memories, err := workspacepg.ReadActiveProjectMemories(tx, pid)
			if err != nil {
				return err
			}
			if workspacepg.ProjectMemoryVersion(memories) != input.ProjectMemoryVersion {
				return errProjectMemoryChanged
			}
		}
		var approved struct {
			RootRunID    uuid.UUID
			SessionID    uuid.UUID
			BudgetMicros int64
			Selection    []byte
		}
		scopeRaw, _ := json.Marshal(scope)
		q := tx.Raw(`SELECT a.root_run_id,a.session_id,a.budget_micros,a.model_selection AS selection FROM agent_business.call_receipt c JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id
   JOIN agent_business.execution_grant g ON g.root_run_id=a.root_run_id AND g.submission_id=a.submission_id AND g.owner_epoch=a.owner_epoch AND g.authorization_revision=a.authorization_revision AND g.expires_at>clock_timestamp()
   WHERE c.receipt_id=? AND c.kind='model' AND c.status='in_flight' AND c.input_hash=? AND c.max_micros=? AND c.submission_id=a.submission_id AND a.scope=?::jsonb AND NOT a.barrier AND NOT a.terminal FOR SHARE OF a,g`, id, agent.Hash(raw), maxMicros, string(scopeRaw)).Scan(&approved)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return domain.ErrForbidden
		}
		var selection struct {
			ModelKey  string                     `json:"model_key"`
			ReleaseID string                     `json:"release_id"`
			MaxTokens int                        `json:"max_tokens"`
			Params    map[string]json.RawMessage `json:"params"`
		}
		if json.Unmarshal(approved.Selection, &selection) != nil || selection.ModelKey != input.ModelKey || selection.MaxTokens < input.MaxTokens || selection.ReleaseID != "" && selection.ReleaseID != input.ReleaseID || !sameAgentParams(selection.Params, input.Params) {
			return domain.ErrForbidden
		}
		if err := agentpg.ValidatePinnedSkills(tx, approved.RootRunID, scope); err != nil {
			return domain.ErrForbidden
		}
		if err := evidencepg.VerifyModelReceipts(tx, a.ID, a.OrgID, sid, pid, approved.SessionID, input.ProjectEvidenceReceipts); err != nil {
			return err
		}
		root = approved.RootRunID

		return nil
	}
	store := pg.New(r.DB, actor, auth).WithRunLimit(ApprovedRunLimit)
	// Read the exact published product, independently of the old model catalog.
	var docs []struct{ Document []byte }
	if err := r.DB.WithContext(ctx).Raw("SELECT document FROM ai.model WHERE name=? AND enabled AND (?='' OR id::text=?)", input.ModelKey, input.ReleaseID, input.ReleaseID).Scan(&docs).Error; err != nil {
		return agentFailure("model_unavailable"), err
	}
	if len(docs) != 1 {
		return agentFailure("model_unavailable"), nil
	}
	var model domain.Model
	if json.Unmarshal(docs[0].Document, &model) != nil || model.Validate() != nil || input.ReleaseID != "" && input.ReleaseID != model.ID.String() {
		return agentFailure("model_unavailable"), nil
	}
	native, err := input.native(model.Protocol)
	if err != nil {
		return agentFailure("invalid_model_request"), nil
	}
	// Discover the already-approved root before Prepare; Admit repeats its live check.
	if err := r.DB.WithContext(ctx).Raw("SELECT root_run_id FROM agent_business.call_receipt WHERE receipt_id=?", id).Row().Scan(&root); err != nil {
		return agentFailure("model_unavailable"), err
	}
	call, err := application.Prepare(model, scope.ActorID, scope.ScopeID, scope.ProjectID, root, "agent/"+id.String(), "agent", native)
	if err != nil {
		return agentFailure("invalid_model_request"), nil
	}
	call.ID = id
	if call.Limit > maxMicros/100000 {
		return agentFailure("budget_exceeded"), nil
	}
	record, err := application.New(store, r.Provider).Execute(ctx, call, nil)
	if err != nil {
		var durable int64
		readErr := r.DB.WithContext(context.WithoutCancel(ctx)).Raw("SELECT count(*) FROM ai.request WHERE id=? AND actor_id=? AND scope_id=?", id, actor.ID, actor.ScopeID).Scan(&durable).Error
		if errors.Is(err, domain.ErrUnknown) || readErr != nil || durable == 1 {
			return agent.CallResult{Status: agent.CallStatusUnknown, Result: json.RawMessage(`{"code":"ai_result_unknown"}`)}, nil
		}
		if errors.Is(err, errProjectMemoryChanged) {
			return agentFailure("project_memory_changed"), nil
		}
		if errors.Is(err, evidencepg.ErrStale) {
			return agentFailure("project_evidence_changed"), nil
		}
		return agentFailure("model_admission_failed"), nil
	}
	return agentResult(record, input.ModelKey, model.Protocol)
}

// AgentRecover only reads durable new evidence and settles the original request.
func (r *Runtime) AgentRecover(ctx context.Context, id uuid.UUID) (agent.CallResult, error) {
	var row struct {
		ScopeID  uuid.UUID
		Contract []byte
	}
	q := r.DB.WithContext(ctx).Raw("SELECT scope_id,contract FROM ai.request WHERE id=? AND origin='agent'", id).Scan(&row)
	if q.Error != nil {
		return agent.CallResult{}, q.Error
	}
	if q.RowsAffected != 1 {
		return agent.CallResult{Status: agent.CallStatusUnknown, Result: json.RawMessage(`{"code":"ai_result_unknown"}`)}, nil
	}
	record, err := pg.New(r.DB, identity.Principal{}, nil).Settle(ctx, id)
	if err != nil {
		return agent.CallResult{Status: agent.CallStatusUnknown, Result: json.RawMessage(`{"code":"ai_result_unknown"}`)}, nil
	}
	var model domain.Model
	if json.Unmarshal(row.Contract, &model) != nil {
		return agent.CallResult{}, domain.ErrInvalid
	}
	return agentResult(record, model.Name, model.Protocol)
}
func agentFailure(code string) agent.CallResult {
	raw, _ := json.Marshal(map[string]string{"code": code})
	return agent.CallResult{Status: agent.CallStatusFailed, Result: raw}
}

type agentInput struct {
	ProjectEvidenceReceipts []uuid.UUID       `json:"project_evidence_receipts,omitempty"`
	ProjectMemoryVersion    string            `json:"project_memory_version,omitempty"`
	ModelKey                string            `json:"model_key"`
	ReleaseID               string            `json:"release_id"`
	MaxTokens               int               `json:"max_tokens"`
	Messages                []json.RawMessage `json:"messages"`
	Tools                   []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"tools"`
	Params map[string]json.RawMessage `json:"params"`
}

func (in agentInput) native(protocol string) (json.RawMessage, error) {
	if in.MaxTokens < 1 || len(in.Messages) == 0 {
		return nil, domain.ErrInvalid
	}
	payload := map[string]any{"model": in.ModelKey, "stream": false, "max_tokens": in.MaxTokens}
	for k, v := range in.Params {
		if k != "temperature" && k != "top_p" {
			return nil, domain.ErrInvalid
		}
		payload[k] = v
	}
	var tools []any
	for _, tool := range in.Tools {
		switch protocol {
		case "chat":
			tools = append(tools, map[string]any{"type": "function", "function": tool})
		case "responses":
			tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": tool.Parameters})
		default:
			tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.Parameters})
		}
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if protocol == "chat" {
		payload["messages"] = in.Messages
		return json.Marshal(payload)
	}
	var messages []any
	var system string
	for _, raw := range in.Messages {
		var message struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		}
		if json.Unmarshal(raw, &message) != nil {
			return nil, domain.ErrInvalid
		}
		if message.Role == "system" {
			var s string
			if json.Unmarshal(message.Content, &s) != nil {
				return nil, domain.ErrInvalid
			}
			system += s + "\n"
			continue
		}
		if protocol == "responses" {
			if message.Role == "tool" {
				var out string
				if json.Unmarshal(message.Content, &out) != nil {
					return nil, domain.ErrInvalid
				}
				messages = append(messages, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": out})
				continue
			}
			if len(message.Content) > 0 && string(message.Content) != "null" {
				messages = append(messages, map[string]any{"role": message.Role, "content": message.Content})
			}
			for _, tool := range message.ToolCalls {
				messages = append(messages, map[string]any{"type": "function_call", "call_id": tool.ID, "name": tool.Function.Name, "arguments": tool.Function.Arguments})
			}
		} else {
			var content []any
			if message.Role == "tool" {
				content = append(content, map[string]any{"type": "tool_result", "tool_use_id": message.ToolCallID, "content": message.Content})
				message.Role = "user"
			} else {
				if len(message.Content) > 0 && string(message.Content) != "null" {
					var text string
					if json.Unmarshal(message.Content, &text) != nil {
						return nil, domain.ErrInvalid
					}
					content = append(content, map[string]any{"type": "text", "text": text})
				}
				for _, tool := range message.ToolCalls {
					var args map[string]any
					if json.Unmarshal([]byte(tool.Function.Arguments), &args) != nil {
						return nil, domain.ErrInvalid
					}
					content = append(content, map[string]any{"type": "tool_use", "id": tool.ID, "name": tool.Function.Name, "input": args})
				}
			}
			messages = append(messages, map[string]any{"role": message.Role, "content": content})
		}
	}
	if protocol == "responses" {
		delete(payload, "max_tokens")
		payload["max_output_tokens"] = in.MaxTokens
		payload["input"] = messages
		if system != "" {
			payload["instructions"] = system
		}
	} else {
		payload["messages"] = messages
		if system != "" {
			payload["system"] = system
		}
	}
	return json.Marshal(payload)
}
func agentResult(record domain.Record, model, protocol string) (agent.CallResult, error) {
	if record.State != "settled" || record.Receipt == nil {
		return agent.CallResult{Status: agent.CallStatusUnknown, Result: json.RawMessage(`{"code":"ai_result_unknown"}`)}, nil
	}
	var body struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string                           `json:"id"`
					Function struct{ Name, Arguments string } `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Output []struct {
			Type, Text, Name, Arguments string
			CallID                      string `json:"call_id"`
			Content                     []struct{ Type, Text string }
		} `json:"output"`
		Content []struct {
			Type, Text, Name, ID string
			Input                json.RawMessage `json:"input"`
		} `json:"content"`
		Stop   string `json:"stop_reason"`
		Status string `json:"status"`
	}
	if json.Unmarshal(record.Receipt.Body, &body) != nil {
		return agentFailure("invalid_provider_result"), nil
	}
	text, stop := "", "stop"
	tools := []map[string]any{}
	add := func(id, name, args string) error {
		var object map[string]any
		if id == "" || name == "" || json.Unmarshal([]byte(args), &object) != nil {
			return domain.ErrUnknown
		}
		tools = append(tools, map[string]any{"id": id, "name": name, "arguments": object})
		return nil
	}
	switch protocol {
	case "chat":
		if len(body.Choices) != 1 {
			return agentFailure("invalid_provider_result"), nil
		}
		choice := body.Choices[0]
		text = choice.Message.Content
		if choice.Finish == "length" {
			stop = "length"
		}
		for _, tool := range choice.Message.ToolCalls {
			if err := add(tool.ID, tool.Function.Name, tool.Function.Arguments); err != nil {
				return agent.CallResult{}, err
			}
		}
	case "responses":
		if body.Status == "incomplete" {
			stop = "length"
		}
		for _, item := range body.Output {
			if item.Type == "function_call" {
				if err := add(item.CallID, item.Name, item.Arguments); err != nil {
					return agent.CallResult{}, err
				}
			}
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text += part.Text
				}
			}
		}
	default:
		for _, part := range body.Content {
			if part.Type == "text" {
				text += part.Text
			}
			if part.Type == "tool_use" {
				if err := add(part.ID, part.Name, string(part.Input)); err != nil {
					return agent.CallResult{}, err
				}
			}
		}
		if body.Stop == "max_tokens" {
			stop = "length"
		}
	}
	if len(tools) > 0 {
		stop = "tool_use"
	}
	raw, _ := json.Marshal(map[string]any{"output": text, "tool_calls": tools, "model_key": model, "stop_reason": stop, "usage": map[string]any{"input_tokens": record.Receipt.Usage.Input, "output_tokens": record.Receipt.Usage.Output, "cache_read_tokens": record.Receipt.Usage.CacheRead, "cache_write_tokens": record.Receipt.Usage.CacheWrite}})
	if record.Charged > 92233720368547 {
		return agent.CallResult{}, fmt.Errorf("AI charge overflow")
	}
	return agent.CallResult{Status: agent.CallStatusCompleted, Result: raw, CostMicros: record.Charged * 100000}, nil
}

// ApprovedRunLimit reads the caller's authoritative approval; no credits are issued here.
func ApprovedRunLimit(ctx context.Context, tx *gorm.DB, id uuid.UUID) (int64, error) {
	var amount int64
	if err := tx.WithContext(ctx).Raw("SELECT budget_micros FROM agent_business.run_authorization WHERE root_run_id=? AND NOT terminal AND NOT barrier FOR SHARE", id).Row().Scan(&amount); err != nil {
		return 0, domain.ErrForbidden
	}
	return amount / 100000, nil
}

func sameAgentParams(a, b map[string]json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return agent.ContentHash(a) == agent.ContentHash(b)
}
