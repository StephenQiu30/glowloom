package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	gatewaydomain "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
)

// WithGateway installs the only paid generation and funds authority for Agent tools.
func (b *Boundary) WithGateway(runtime *aigateway.Runtime) *Boundary { b.gateway = runtime; return b }
func gatewayWorkerAuthority(ctx context.Context, tx *gorm.DB, a identity.Principal, sid, pid uuid.UUID, admin bool) error {
	if admin || a.ID == uuid.Nil || sid != a.ScopeID || pid == uuid.Nil {
		return gatewaydomain.ErrForbidden
	}
	if _, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, a.ID, sid, true); err != nil {
		return err
	}
	var n int64
	if err := tx.Raw("SELECT count(*) FROM workspace.project WHERE id=? AND owner_scope_id=? AND status='active' AND NOT is_delete", pid, sid).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return gatewaydomain.ErrForbidden
	}
	return nil
}
func (b *Boundary) gatewayCreation(tx *gorm.DB, root uuid.UUID) (aigateway.Creation, error) {
	if b.gateway == nil {
		return aigateway.Creation{}, domain.ErrUnavailable
	}
	runtime := *b.gateway
	runtime.DB = tx
	return aigateway.Creation{Runtime: &runtime, Authorize: gatewayWorkerAuthority, Run: root, Origin: "agent_creation", RunLimit: aigateway.ApprovedRunLimit, Permit: aigateway.AgentPermit}, nil
}
func (b *Boundary) gatewayQuote(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, in domain.CallInput) (domain.CallResult, error) {
	var input quoteInput
	if err := decode(in.Input, &input); err != nil {
		return domain.CallResult{}, err
	}
	if input.Source != nil && s.CanvasID != nil && input.Source.CanvasID != *s.CanvasID {
		return domain.CallResult{}, domain.ErrNotFound
	}
	creation, err := b.gatewayCreation(tx, in.Grant.RootRunID)
	if err != nil {
		return domain.CallResult{}, err
	}
	quote, err := creation.Quote(ctx, app.Principal(s), s.ProjectID, key.String(), operationhttp.QuoteItemRequest{ModelKey: input.ModelKey, Capability: input.Capability, Mode: input.Mode, Prompt: input.Prompt, Params: input.Params, OutputCount: input.OutputCount, Source: input.Source, ForceRegenerate: true})
	if err != nil {
		return domain.CallResult{}, err
	}
	var version uuid.UUID
	if err := tx.Raw("SELECT (contract->'Model'->>'id')::uuid FROM ai.quote WHERE id=?", quote.OperationID).Row().Scan(&version); err != nil {
		return domain.CallResult{}, err
	}
	raw, _ := json.Marshal(map[string]any{"operation_id": quote.OperationID, "quote_micros": quote.QuoteMicros, "quote_detail": quote.QuoteDetail, "expires_at": quote.ExpiresAt, "price_version": version, "model_version": version, "source": input.Source})
	return domain.CallResult{Status: domain.CallStatusCompleted, Result: raw}, nil
}
func newGatewayQuote(_ context.Context, tx *gorm.DB, s domain.Scope, id uuid.UUID) (operation.CreateFreeQuoteResult, uuid.UUID, error) {
	var row struct {
		Presentation, Contract []byte
		State                  string
		ActorID                uuid.UUID
	}
	q := tx.Raw("SELECT c.presentation,c.state,c.actor_id,q.contract FROM ai.creation c JOIN ai.quote q ON q.id=c.id WHERE c.id=? AND c.scope_id=? AND c.project_id=?", id, s.ScopeID, s.ProjectID).Scan(&row)
	if q.Error != nil {
		return operation.CreateFreeQuoteResult{}, uuid.Nil, q.Error
	}
	if q.RowsAffected != 1 || row.ActorID != s.ActorID {
		return operation.CreateFreeQuoteResult{}, uuid.Nil, domain.ErrNotFound
	}
	var out operation.CreateFreeQuoteResult
	var contract struct{ Model struct{ ID uuid.UUID } }
	if json.Unmarshal(row.Presentation, &out) != nil || json.Unmarshal(row.Contract, &contract) != nil {
		return out, uuid.Nil, domain.ErrInvalid
	}
	if row.State != "quoted" || !out.ExpiresAt.After(time.Now()) {
		return out, uuid.Nil, domain.ErrConflict
	}
	return out, contract.Model.ID, nil
}

func gatewayQuoteRevision(ctx context.Context, tx *gorm.DB, s domain.Scope, id uuid.UUID) (int64, error) {
	var row struct{ Source []byte }
	q := tx.WithContext(ctx).Raw("SELECT source FROM ai.creation WHERE id=? AND scope_id=? AND project_id=?", id, s.ScopeID, s.ProjectID).Scan(&row)
	if q.Error != nil {
		return 0, q.Error
	}
	if q.RowsAffected != 1 {
		return 0, domain.ErrNotFound
	}
	var source struct {
		CanvasID uuid.UUID `json:"canvas_id"`
		NodeID   uuid.UUID `json:"node_id"`
		Revision int64     `json:"revision"`
	}
	if json.Unmarshal(row.Source, &source) != nil {
		return 0, domain.ErrInvalid
	}
	if source.CanvasID == uuid.Nil {
		if s.CanvasID == nil {
			return 0, nil
		}
		source.CanvasID = *s.CanvasID
	}
	var revision int64
	if err := tx.Raw("SELECT revision FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE", source.CanvasID, s.ProjectID).Row().Scan(&revision); err != nil {
		return 0, err
	}
	if source.Revision > 0 && source.Revision != revision {
		return 0, domain.ErrConflict
	}
	return revision, nil
}

func gatewayTask(ctx context.Context, tx *gorm.DB, a identity.Principal, p, id uuid.UUID) (operation.TaskDetail, error) {
	return (aigateway.Creation{Runtime: &aigateway.Runtime{DB: tx}, Authorize: gatewayWorkerAuthority}).Task(ctx, a, p, id)
}
