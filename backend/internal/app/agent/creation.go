package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbible "github.com/StephenQiu30/glowloom-server/backend/creation/bible/adapter/postgres"
	bible "github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"
	bibledomain "github.com/StephenQiu30/glowloom-server/backend/creation/bible/domain"
	pgscript "github.com/StephenQiu30/glowloom-server/backend/creation/script/adapter/postgres"
	script "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/execution/operation/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
)

// commandSchema derives nested business argument fields from the consumed Go
// command. Trusted scope and idempotency fields are excluded at the boundary.
func commandSchema(value any) string {
	schema := argumentSchema(reflect.TypeOf(value), 0)
	if properties, ok := schema["properties"].(map[string]any); ok {
		for _, field := range []string{"project_id", "key", "request_id"} {
			delete(properties, field)
		}
	}
	raw, _ := json.Marshal(schema)
	return string(raw)
}
func argumentSchema(t reflect.Type, depth int) map[string]any {
	if t.Kind() == reflect.Pointer {
		return argumentSchema(t.Elem(), depth)
	}
	if t == reflect.TypeOf(uuid.UUID{}) {
		return map[string]any{"type": "string", "format": "uuid"}
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		// Raw JSON carries primitive, array or object operation values. The owning
		// application validates the concrete closed field policy after decoding.
		return map[string]any{}
	}
	if depth > 12 {
		return map[string]any{"type": "object"}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Array, reflect.Slice:
		return map[string]any{"type": "array", "items": argumentSchema(t.Elem(), depth+1)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": argumentSchema(t.Elem(), depth+1)}
	case reflect.Struct:
		properties := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" || !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			properties[name] = argumentSchema(f.Type, depth+1)
		}
		return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	default:
		return map[string]any{}
	}
}
func bibleService(tx *gorm.DB) *bible.Service {
	return bible.NewService(pgbible.NewStore(tx, pgbible.Factories{Access: func(inner *gorm.DB) bible.ProjectAccess {
		return biblewire.ProjectAccess(scopewire.ProvideWorkerProjectContentAccess(inner))
	}, Scopes: func(inner *gorm.DB) bible.ScriptScopes {
		return bibleScopes{pgscript.NewBibleScopes(inner, scriptwire.ProjectAccess(scopewire.ProvideWorkerProjectContentAccess(inner)))}
	}, Results: func(inner *gorm.DB) bible.GeneratedResults { return extractedResults{inner} }}), time.Now)
}

type extractedResults struct{ tx *gorm.DB }

func (r extractedResults) read(ctx context.Context, a identity.Principal, pid, operationID, outputID uuid.UUID, out any) (bibledomain.ResultSource, error) {
	task, err := gatewayTask(ctx, r.tx, a, pid, operationID)
	if err != nil {
		return bibledomain.ResultSource{}, err
	}
	if task.Status != "completed" {
		return bibledomain.ResultSource{}, domain.ErrBlocked
	}
	for _, output := range task.Outputs {
		if output.ID != outputID || output.Text == nil || output.ModerationStatus != "passed" && output.ModerationStatus != "skipped" {
			continue
		}
		if err = decode(json.RawMessage(*output.Text), out); err != nil {
			return bibledomain.ResultSource{}, err
		}
		var hash string
		if err := r.tx.Raw("SELECT input_hash FROM ai.quote WHERE id=?", operationID).Row().Scan(&hash); err != nil {
			return bibledomain.ResultSource{}, err
		}
		return bibledomain.ResultSource{OperationID: operationID, OutputID: outputID, OutputSHA256: domain.RawHash([]byte(*output.Text)), InputSHA256: hash}, nil
	}
	return bibledomain.ResultSource{}, domain.ErrNotFound
}
func (r extractedResults) Character(ctx context.Context, a bible.Actor, pid, op, output uuid.UUID) (bible.CharacterInput, bibledomain.ResultSource, error) {
	var content bible.CharacterInput
	source, err := r.read(ctx, biblewire.IdentityActor(a), pid, op, output, &content)
	return content, source, err
}
func (r extractedResults) Location(ctx context.Context, a bible.Actor, pid, op, output uuid.UUID) (bibledomain.LocationContent, bibledomain.ResultSource, error) {
	var content bibledomain.LocationContent
	source, err := r.read(ctx, biblewire.IdentityActor(a), pid, op, output, &content)
	return content, source, err
}
func (r extractedResults) Prop(ctx context.Context, a bible.Actor, pid, op, output uuid.UUID) (bibledomain.PropContent, bibledomain.ResultSource, error) {
	var content bibledomain.PropContent
	source, err := r.read(ctx, biblewire.IdentityActor(a), pid, op, output, &content)
	return content, source, err
}

type quoteInput struct {
	ModelKey    string                        `json:"model_key"`
	Capability  string                        `json:"capability"`
	Mode        string                        `json:"mode"`
	Prompt      string                        `json:"prompt"`
	Params      json.RawMessage               `json:"params"`
	OutputCount int32                         `json:"output_count"`
	Source      *operationdomain.CanvasSource `json:"source,omitempty"`
}

func creationTools() []toolBinding {
	return []toolBinding{
		tool("operation_propose", "提交原真实报价的生成提案并立即停止整棵运行树的新模型/写工具，等待用户批准", "write", `{"type":"object","properties":{"operation_id":{"type":"string","format":"uuid"},"expected_canvas_revision":{"type":"integer","minimum":0}},"required":["operation_id"],"additionalProperties":false}`, nil),

		tool("operation_confirm", "仅确认当前已批准提案中的原正式报价；同事务复核grant、画布/价格、预算并持久生成任务", "write", `{"type":"object","properties":{"operation_id":{"type":"string","format":"uuid"}},"required":["operation_id"],"additionalProperties":false}`, nil),

		tool("bible_change", "创建、修改、确认人物/场景/道具，或采用真实生成结果；版本由正式业务命令校验", "write", commandSchema(bible.Command{}), func(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, raw json.RawMessage) (any, error) {
			var in bible.Command
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			if in.ProjectID != uuid.Nil || in.Key != uuid.Nil || in.RequestID != uuid.Nil {
				return nil, domain.ErrInvalid
			}
			in.ProjectID = s.ProjectID
			in.Key = key
			in.RequestID = key
			return bibleService(tx).Change(ctx, biblewire.Actor(app.Principal(s)), in)
		}),
		tool("script_adopt", "按剧本/分集双版本采用正式剧本版本", "write", commandSchema(script.AdoptCommand{}), func(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, raw json.RawMessage) (any, error) {
			var in script.AdoptCommand
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			if in.ProjectID != uuid.Nil || in.Key != uuid.Nil || in.RequestID != uuid.Nil {
				return nil, domain.ErrInvalid
			}
			in.ProjectID = s.ProjectID
			in.Key = key
			in.RequestID = key
			store := pgscript.NewSourceStore(tx, func(inner *gorm.DB) script.ProjectAccess {
				return scriptwire.ProjectAccess(scopewire.ProvideWorkerProjectContentAccess(inner))
			})
			return script.NewAdoptService(store, time.Now).Adopt(ctx, scriptwire.Actor(app.Principal(s)), in)
		}),
		tool("script_confirm_split", "确认完整分集边界与映射；保留既有正式剧集身份", "write", commandSchema(script.SplitCommand{}), func(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, raw json.RawMessage) (any, error) {
			var in script.SplitCommand
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			if in.ProjectID != uuid.Nil || in.Key != uuid.Nil || in.RequestID != uuid.Nil {
				return nil, domain.ErrInvalid
			}
			in.ProjectID = s.ProjectID
			in.Key = key
			in.RequestID = key
			store := pgscript.NewSourceStore(tx, func(inner *gorm.DB) script.ProjectAccess {
				return scriptwire.ProjectAccess(scopewire.ProvideWorkerProjectContentAccess(inner))
			})
			return script.NewEpisodeService(store, nil, time.Now).ConfirmSplit(ctx, scriptwire.Actor(app.Principal(s)), in)
		}),
		tool("operation_quote", "建立统一网关冻结报价，不确认费用；随后提交提案并取得用户批准", "write", commandSchema(quoteInput{}), nil),
	}
}

// proposalValidator uses live owning application facts at approval and again
// immediately before confirmation; no model-supplied price or revision is trusted.
type proposalValidator struct {
	tx      *gorm.DB
	gateway *aigateway.Runtime
}

func (v proposalValidator) ValidateProposal(ctx context.Context, s domain.Scope, p domain.ProposalInput) error {
	var delivery shotDeliveryCandidate
	if json.Unmarshal(p.Candidate, &delivery) == nil && delivery.Kind == shotDeliveryCandidateKind {
		if p.BudgetMicros != 0 || p.PriceVersion != shotDeliveryPriceVersion || p.CanvasRevision != delivery.Input.ExpectedRevision {
			return domain.ErrConflict
		}
		task, asset, err := loadShotCandidate(ctx, v.tx, s, delivery.Input)
		if err != nil {
			return err
		}
		if !delivery.matches(task, asset) {
			return domain.ErrConflict
		}
		return nil
	}
	var candidate struct {
		OperationID uuid.UUID `json:"operation_id"`
	}
	if json.Unmarshal(p.Candidate, &candidate) != nil || candidate.OperationID == uuid.Nil {
		return domain.ErrInvalid
	}
	if v.gateway == nil {
		return domain.ErrUnavailable
	}
	quoted, version, err := newGatewayQuote(ctx, v.tx, s, candidate.OperationID)
	if err != nil {
		return err
	}
	if quoted.QuoteMicros > p.BudgetMicros || p.PriceVersion != version.String() {
		return domain.ErrConflict
	}
	revision, err := gatewayQuoteRevision(ctx, v.tx, s, candidate.OperationID)
	if err != nil {
		return err
	}
	if revision != p.CanvasRevision {
		return domain.ErrConflict
	}
	var available int64
	if err := v.tx.Raw("SELECT COALESCE((SELECT available FROM ai.account WHERE scope_id=?),0)", s.ScopeID).Row().Scan(&available); err != nil {
		return err
	}
	if p.BudgetMicros/100000 > available {
		return domain.ErrBudget
	}

	return nil
}

var _ app.DecisionValidator = proposalValidator{}

type bibleScopes struct{ reader *pgscript.BibleScopes }

func (s bibleScopes) ValidateScopes(ctx context.Context, actor bible.Actor, pid uuid.UUID, scopes []bibledomain.LookScope) error {
	requested := make([]script.BibleLookScope, len(scopes))
	for i, v := range scopes {
		requested[i] = script.BibleLookScope{EpisodeID: v.EpisodeID, SceneKey: v.SceneKey}
	}
	return s.reader.ValidateLookScopes(ctx, scriptwire.Actor(biblewire.IdentityActor(actor)), pid, requested)
}

func (b *Boundary) confirmOperation(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, in domain.CallInput) (domain.CallResult, error) {
	var requested struct {
		OperationID uuid.UUID `json:"operation_id"`
	}
	if err := decode(in.Input, &requested); err != nil {
		return domain.CallResult{}, err
	}
	proposal, err := b.Store.ApprovedProposalInTransaction(tx, in.Grant)
	if err != nil {
		return domain.CallResult{}, err
	}
	var candidate struct {
		OperationID uuid.UUID `json:"operation_id"`
	}
	if json.Unmarshal(proposal.Candidate, &candidate) != nil || candidate.OperationID != requested.OperationID {
		return domain.CallResult{}, domain.ErrBlocked
	}
	if err = (proposalValidator{tx: tx, gateway: b.gateway}).ValidateProposal(ctx, s, proposal); err != nil {
		return domain.CallResult{}, err
	}
	quoted, _, err := newGatewayQuote(ctx, tx, s, requested.OperationID)
	if err != nil {
		return domain.CallResult{}, err
	}
	if quoted.QuoteMicros > in.MaxMicros {
		return domain.CallResult{}, domain.ErrBudget
	}
	creation, err := b.gatewayCreation(tx, in.Grant.RootRunID)
	if err != nil {
		return domain.CallResult{}, err
	}
	record, err := creation.Confirm(ctx, app.Principal(s), s.ProjectID, requested.OperationID, key.String())
	if err != nil {
		return domain.CallResult{}, err
	}
	if err = tx.Exec("INSERT INTO ai.agent_intent(receipt_id,request_id,root_run_id) VALUES(?,?,?)", key, record.ID, in.Grant.RootRunID).Error; err != nil {
		return domain.CallResult{}, err
	}
	raw, err := json.Marshal(map[string]any{"operation_id": record.ID, "reservation_id": record.ID, "budget_revision": 1})
	return domain.CallResult{Status: domain.CallStatusCompleted, Result: raw}, err
}

func (b *Boundary) proposeOperation(ctx context.Context, tx *gorm.DB, s domain.Scope, key uuid.UUID, in domain.CallInput) (domain.CallResult, error) {
	var requested struct {
		OperationID            uuid.UUID `json:"operation_id"`
		ExpectedCanvasRevision *int64    `json:"expected_canvas_revision,omitempty"`
	}
	if err := decode(in.Input, &requested); err != nil {
		return domain.CallResult{}, err
	}
	quoted, version, err := newGatewayQuote(ctx, tx, s, requested.OperationID)
	if err != nil {
		return domain.CallResult{}, err
	}
	revision, err := gatewayQuoteRevision(ctx, tx, s, requested.OperationID)
	if err != nil {
		return domain.CallResult{}, err
	}
	if requested.ExpectedCanvasRevision != nil && *requested.ExpectedCanvasRevision != revision {
		return domain.CallResult{}, domain.ErrConflict
	}
	candidate, err := json.Marshal(map[string]any{"operation_id": quoted.OperationID, "quote_micros": quoted.QuoteMicros, "quote_detail": quoted.QuoteDetail, "price_version": version, "model_version": version, "canvas_id": s.CanvasID, "canvas_revision": revision, "expires_at": quoted.ExpiresAt})
	if err != nil {
		return domain.CallResult{}, err
	}
	proposal, err := b.Store.ProposeInTransaction(tx, domain.ProposalInput{Grant: in.Grant, RequestID: uuid.NewSHA1(key, []byte("proposal")), CandidateHash: domain.Hash(candidate), CanvasRevision: revision, PriceVersion: version.String(), BudgetMicros: quoted.QuoteMicros, Candidate: candidate})
	if err != nil {
		return domain.CallResult{}, err
	}
	result, err := json.Marshal(map[string]any{"proposal_id": proposal.ProposalID, "root_run_id": proposal.RootRunID, "candidate_hash": proposal.CandidateHash, "authorization_revision": proposal.AuthorizationRevision, "candidate": proposal.Candidate, "canvas_revision": proposal.CanvasRevision, "price_version": proposal.PriceVersion, "quote_micros": quoted.QuoteMicros, "budget_micros": proposal.BudgetMicros, "control": map[string]bool{"terminate": true}})
	return domain.CallResult{Status: domain.CallStatusCompleted, Result: result}, err
}
