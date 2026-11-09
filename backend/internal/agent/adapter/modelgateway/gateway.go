package modelgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	agentpg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	opstore "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// Unsealer opens an encrypted catalog credential for its exact owning identity.
type Unsealer interface {
	Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error)
}

// Gateway binds model requests to Go-owned credentials, provider receipts and billing.
type Gateway struct {
	db         *gorm.DB
	operations *opstore.Store
	billing    *billingpg.Store
	keys       Unsealer
	chat       *ChatClient
}

// New injects catalog, operation, billing and HTTP dependencies for model execution.
func New(db *gorm.DB, operations *opstore.Store, budget *billingpg.Store, keys Unsealer, client *http.Client) *Gateway {
	return &Gateway{db: db, operations: operations, billing: budget, keys: keys, chat: NewChatClient(client)}
}

type release struct {
	ReleaseID, ModelID, VersionID, PriceID              uuid.UUID
	ProviderModelID, ModelKey, Region, Unit, AdapterKey string
	ParamSchema, Rule                                   []byte
	VersionNo                                           int
	Currency, FX                                        string
	EffectiveFrom                                       time.Time
}

func failed(code string) agent.CallResult {
	raw, _ := json.Marshal(map[string]string{"code": code})
	return agent.CallResult{Status: "failed", Result: raw}
}
func pending() agent.CallResult {
	return agent.CallResult{Status: "unknown", Result: json.RawMessage(`{"code":"provider_result_unknown"}`)}
}

// Model binds, reserves and claims the one paid send in the same transaction.
// A receipt can have at most one operation, dispatch and settlement.
func (g *Gateway) Model(ctx context.Context, scope agent.Scope, id uuid.UUID, raw json.RawMessage, maxMicros int64) (agent.CallResult, error) {
	if g == nil || g.keys == nil || g.billing == nil || g.operations == nil || g.db == nil {
		return failed("model_gateway_unavailable"), nil
	}
	in, err := parseChat(raw)
	if err != nil || !scope.Valid() || id == uuid.Nil || maxMicros < 0 {
		return failed("invalid_model_request"), nil
	}
	var r release
	var i op.ProviderDispatchIdentity
	var config catalog.ProviderConfig
	var key string
	err = g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check the live root/segment authority again immediately before committing a dispatch.
		var authority struct {
			RootRunID      uuid.UUID
			ModelSelection []byte
		}
		scopeRaw, _ := json.Marshal(scope)
		auth := tx.Raw(`SELECT a.root_run_id,a.model_selection FROM agent_business.call_receipt c JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id
    JOIN agent_business.execution_grant g ON g.root_run_id=a.root_run_id AND g.submission_id=a.submission_id AND g.owner_epoch=a.owner_epoch AND g.authorization_revision=a.authorization_revision AND g.expires_at>clock_timestamp()
    WHERE c.receipt_id=? AND c.status='in_flight' AND c.kind='model' AND c.submission_id=a.submission_id AND c.input_hash=? AND c.max_micros=? AND a.scope=?::jsonb AND NOT a.barrier AND NOT a.terminal FOR SHARE OF a,g`, id, agent.Hash(raw), maxMicros, string(scopeRaw)).Scan(&authority)
		if auth.Error != nil {
			return auth.Error
		}
		if auth.RowsAffected != 1 {
			return agent.ErrGrant
		}
		if err := agentpg.ValidatePinnedSkills(tx, authority.RootRunID, scope); err != nil {
			return err
		}
		var selection struct {
			ReleaseID uuid.UUID                  `json:"release_id"`
			ModelKey  string                     `json:"model_key"`
			Params    map[string]json.RawMessage `json:"params"`
			MaxTokens int                        `json:"max_tokens"`
		}
		params := in.Params
		if params == nil {
			params = map[string]json.RawMessage{}
		}
		if json.Unmarshal(authority.ModelSelection, &selection) != nil || selection.ModelKey != in.ModelKey || selection.ReleaseID != uuid.Nil && selection.ReleaseID.String() != in.ReleaseID || selection.MaxTokens < in.MaxTokens || selection.Params == nil || agent.ContentHash(selection.Params) != agent.ContentHash(params) {
			return agent.ErrGrant
		}

		// A committed dispatch belongs to its original receipt even if the
		// current catalog changed. Recovery owns its outcome; never admit a
		// second send or turn an unknown supplier result into a new rejection.
		var dispatched bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM operation.operation WHERE id=? AND project_id=? AND input_hash=? AND origin='agent' AND capability='text.generate' AND mode='text_to_text' AND NOT is_delete)`, id, scope.ProjectID, agent.Hash(raw)).Row().Scan(&dispatched); err != nil {
			return err
		}
		if dispatched {
			return agent.ErrConflict
		}

		row := tx.Raw(`SELECT r.id AS release_id,m.id AS model_id,v.id AS version_id,p.id AS price_id,m.model_key,v.provider_model_id,v.param_schema,
   c.adapter_key,c.region,p.unit,p.rule,p.version_no,p.currency,coalesce(p.fx_rate_to_cny::text,'') AS fx,p.effective_from
   FROM catalog.model_profile m JOIN catalog.provider c ON c.id=m.provider_id AND c.status='active' AND NOT c.is_delete
   JOIN catalog.model_release r ON r.id=m.active_release_id AND r.model_id=m.id AND r.status='active'
   JOIN catalog.model_profile_version v ON v.id=r.model_version_id AND v.model_profile_id=m.id AND NOT v.is_delete
   JOIN catalog.price_rule_version p ON p.id=r.price_version_id AND p.model_profile_id=m.id AND NOT p.is_delete
   WHERE m.model_key=? AND m.status='active' AND NOT m.is_delete AND m.capability='text.generate'
   AND p.unit IN ('per_request','per_1k_tokens') AND p.effective_from<=clock_timestamp()
   AND NOT EXISTS (SELECT FROM agent_business.call_receipt past JOIN operation.provider_binding b ON b.operation_id=past.receipt_id
      WHERE past.kind='model' AND past.root_run_id=? AND b.release_id<>r.id)
   FOR SHARE OF c,m,r`, in.ModelKey, authority.RootRunID).Scan(&r)
		if row.Error != nil {
			return row.Error
		}
		if row.RowsAffected != 1 || in.ReleaseID != "" && in.ReleaseID != r.ReleaseID.String() {
			return op.ErrWorkflowModelUnavailable
		}
		store := g.operations.BindTransaction(tx)
		if store == nil {
			return op.ErrWorkflowModelUnavailable
		}
		available, err := store.AvailableForNewExecution(ctx, r.VersionID, r.ReleaseID, "text_to_text")
		if err != nil {
			return err
		}
		if !available {
			return op.ErrWorkflowModelUnavailable
		}
		if !slices.Contains([]string{"openai", "openrouter", "vercel", "deepseek", "byteplus", "volcengine_ark"}, r.AdapterKey) {
			return op.ErrWorkflowModelUnavailable
		}
		// Validate authorization against the original input above, then apply
		// only defaults from this immutable, pinned release's parameter schema.
		requested := make(map[string]json.RawMessage, len(in.Params)+1)
		for name, value := range in.Params {
			requested[name] = value
		}
		requested["max_tokens"], _ = json.Marshal(in.MaxTokens)
		requestedJSON, err := json.Marshal(requested)
		if err != nil {
			return errInput
		}
		normalized, err := op.NormalizeQuoteParams(requestedJSON, r.ParamSchema, "text_to_text")
		if err != nil || catalog.ValidateModelParameters(r.ProviderModelID, normalized) != nil {
			return errInput
		}
		providerParams := make(map[string]json.RawMessage, len(normalized))
		for name, value := range normalized {
			if name == "max_tokens" {
				continue
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return errInput
			}
			providerParams[name] = encoded
		}
		if !validProviderChatParams(r.AdapterKey, providerParams) {
			return errInput
		}
		in.Params = providerParams

		price := catalog.PriceRuleVersion{ID: r.PriceID, ModelID: r.ModelID, VersionNo: r.VersionNo, Unit: catalog.PriceUnit(r.Unit), Rule: r.Rule, Currency: r.Currency, FXRateToCNY: r.FX, EffectiveFrom: r.EffectiveFrom}
		estimate := int64(len(raw))
		quote, costs, err := op.CalculateQuoteDetailed(price, op.PricingInput{QuotedAt: time.Now(), EstimatedInputTokens: estimate, MaxOutputTokens: int64(in.MaxTokens), OutputCount: 1, Mode: "text_to_text"})
		if err != nil {
			return err
		}
		procurement, err := price.ProcurementVersion()
		if err != nil {
			return err
		}
		wholeCredits := procurement != uuid.Nil
		if wholeCredits {
			quote, err = billingdomain.WholeCreditMicros(quote)
			if err != nil {
				return err
			}
			credits := quote / billingdomain.MicrosPerCredit
			costs.WholeCredits = true
			costs.EstimatedCredits = &credits
			costs.ExactCredits = strconv.FormatInt(credits, 10)
			costs.MaxChargeCredits = costs.ExactCredits
		}
		quoteDetail, err := json.Marshal(costs)
		if err != nil {
			return err
		}
		if quote > maxMicros {
			return agent.ErrBudget
		}
		i = op.ProviderDispatchIdentity{OperationID: id, ProjectID: scope.ProjectID, ModelProfileVersionID: r.VersionID, PriceRuleVersionID: r.PriceID, Action: "submit", Attempt: 1, RequestKey: "model/" + id.String()}
		frozenParams := make(map[string]json.RawMessage, len(in.Params)+1)
		for name, value := range in.Params {
			frozenParams[name] = value
		}
		frozenParams["max_tokens"], _ = json.Marshal(in.MaxTokens)
		paramsJSON, err := json.Marshal(frozenParams)
		if err != nil {
			return err
		}
		insert := tx.Exec(`INSERT INTO operation.operation(id,project_id,target_type,capability,mode,model_profile_version_id,price_rule_version_id,params,input_hash,origin,status,region,quote_micros,quote_detail,quote_expires_at,confirmed_at,confirmed_by,provider_request_key)
   VALUES(?,?,'free','text.generate','text_to_text',?,?,?::jsonb,?,'agent','submitting',?,?,?::jsonb,clock_timestamp()+interval '15 minutes',clock_timestamp(),?,?) ON CONFLICT(id) DO NOTHING`, id, scope.ProjectID, r.VersionID, r.PriceID, string(paramsJSON), agent.Hash(raw), r.Region, quote, string(quoteDetail), scope.ActorID, i.RequestKey)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected != 1 {
			return agent.ErrConflict
		}
		if err := tx.Exec(`INSERT INTO operation.provider_binding(operation_id,release_id) VALUES(?,?)`, id, r.ReleaseID).Error; err != nil {
			return err
		}
		reserve, err := g.billing.ReserveInTransaction(ctx, tx, billing.ReserveInput{Actor: scope.Principal(), ActorID: scope.ActorID, ProjectID: scope.ProjectID, OperationID: id, AmountMicros: quote, WholeCredits: wholeCredits, CommercialPriceReady: procurement != uuid.Nil, ModelKey: &r.ModelKey, Region: &r.Region, OccurredAt: time.Now()})
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE operation.operation SET reservation_id=? WHERE id=?`, reserve.ReservationID, id).Error; err != nil {
			return err
		}
		if err := store.BeginProviderCall(ctx, op.BeginProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, DispatchRequired: true}); err != nil {
			return err
		}
		claim, err := store.ClaimProviderDispatch(ctx, i)
		if err != nil {
			return err
		}
		if !claim.Claimed {
			return agent.ErrConflict
		}
		cred, err := store.LoadManagedCredential(ctx, id)
		if err != nil {
			return err
		}
		if json.Unmarshal(cred.Config, &config) != nil || config.Validate() != nil || config.AdapterKey != r.AdapterKey || !slices.Contains(config.Capabilities, "text") {
			return op.ErrWorkflowModelUnavailable
		}
		secret, err := g.keys.Open(cred.KeyID, cred.ProviderID, cred.CredentialID, cred.Ciphertext)
		if err != nil {
			return op.ErrWorkflowModelUnavailable
		}
		defer clear(secret)
		key, err = config.CredentialValue(secret)
		return err
	})
	if errors.Is(err, billingdomain.ErrWalletInsufficient) {
		return failed("credits_insufficient"), nil
	}
	if errors.Is(err, agent.ErrConflict) {
		return pending(), nil
	}
	if err != nil {
		return failed("model_admission_failed"), nil
	}
	reply, err := g.chat.complete(ctx, config, key, r.ProviderModelID, in)
	if err != nil {
		return pending(), nil
	}
	received := time.Now().UTC()
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	result, err := json.Marshal(reply)
	if err != nil {
		return pending(), err
	}
	one := int64(1)
	usage, err := json.Marshal(op.BillableUsage{BillableRequests: &one, InputTokens: &reply.Usage.InputTokens, OutputTokens: &reply.Usage.OutputTokens, CachedInputTokens: reply.Usage.CacheReadTokens})
	if err != nil {
		return pending(), err
	}
	// Preserve known supplier output even if accounting cannot yet price it.
	evidence := op.ManagedResult{Identity: i, State: op.ManagedResultSucceeded, ReceivedAt: &received, Text: string(result), Usage: usage}
	if err := g.operations.SaveManagedResult(saveCtx, evidence); err != nil {
		return pending(), err
	}
	return g.settle(saveCtx, scope, id, r, i, evidence, result)
}

// Recover consults the original durable supplier evidence and never sends a new
// request. It also closes a crash after evidence persistence but before billing.
func (g *Gateway) Recover(ctx context.Context, id uuid.UUID) (agent.CallResult, error) {
	if g == nil || g.db == nil || g.operations == nil || g.billing == nil || id == uuid.Nil {
		return pending(), nil
	}
	var row struct {
		Scope                                                []byte
		ProjectID, ModelProfileVersionID, PriceRuleVersionID uuid.UUID
		ProviderRequestKey, ModelKey, Region                 string
	}
	q := g.db.WithContext(ctx).Raw(`SELECT a.scope,o.project_id,o.model_profile_version_id,o.price_rule_version_id,o.provider_request_key,m.model_key,o.region
 FROM agent_business.call_receipt c JOIN agent_business.run_authorization a ON a.root_run_id=c.root_run_id
 JOIN operation.operation o ON o.id=c.receipt_id JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id
 JOIN catalog.model_profile m ON m.id=v.model_profile_id WHERE c.receipt_id=? AND c.kind='model'`, id).Scan(&row)
	if q.Error != nil {
		return pending(), q.Error
	}
	if q.RowsAffected != 1 {
		return pending(), nil
	}
	var scope agent.Scope
	if json.Unmarshal(row.Scope, &scope) != nil || !scope.Valid() {
		return pending(), agent.ErrInvalid
	}
	i := op.ProviderDispatchIdentity{OperationID: id, ProjectID: row.ProjectID, ModelProfileVersionID: row.ModelProfileVersionID, PriceRuleVersionID: row.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: row.ProviderRequestKey}
	e, err := g.operations.ReadManagedResult(ctx, i)
	if err != nil {
		return pending(), err
	}
	if e == nil || e.State != op.ManagedResultSucceeded {
		return pending(), nil
	}
	var reply Result
	if json.Unmarshal([]byte(e.Text), &reply) != nil || reply.ModelKey != row.ModelKey {
		return pending(), agent.ErrConflict
	}
	return g.settle(ctx, scope, id, release{ModelKey: row.ModelKey, Region: row.Region}, i, *e, json.RawMessage(e.Text))
}

func (g *Gateway) settle(ctx context.Context, scope agent.Scope, id uuid.UUID, r release, _ op.ProviderDispatchIdentity, evidence op.ManagedResult, result json.RawMessage) (agent.CallResult, error) {
	out := agent.CallResult{Status: "completed", Result: result}
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Keep the same root-before-business lock order as admission and call replay.
		if err := tx.Exec(`SELECT a.root_run_id FROM agent_business.run_authorization a JOIN agent_business.call_receipt c ON c.root_run_id=a.root_run_id WHERE c.receipt_id=? FOR UPDATE OF a`, id).Error; err != nil {
			return err
		}
		store := g.operations.BindTransaction(tx)
		var persisted struct {
			Status      operation.Status
			FailureCode string
		}
		if err := tx.Raw(`SELECT status,coalesce(failure_code,'') AS failure_code FROM operation.operation WHERE id=? FOR UPDATE`, id).Scan(&persisted).Error; err != nil {
			return err
		}
		status := persisted.Status
		revoked := status == operation.StatusFailed && persisted.FailureCode == "execution_authority_revoked"
		if status != operation.StatusCompleted && !revoked {
			if err := store.CompleteProviderCall(ctx, op.CompleteProviderCallInput{OperationID: id, Attempt: 1, Action: "submit", State: op.ProviderCallStateCompleted, Outcome: op.ProviderCallOutcomeOK, Managed: true}); err != nil {
				return err
			}
			if err := store.AuthorizeWorker(ctx, id, true); err != nil {
				if !errors.Is(err, op.ErrExecutionAuthorityRevoked) {
					return err
				}
				revoked = true
			}
			if !revoked {
				if status == operation.StatusSubmitting || status == operation.StatusUnknown || status == operation.StatusReconciling || status == operation.StatusManual {
					if _, err := store.TransitionWorkflowOperation(ctx, op.TransitionInput{OperationID: id, From: []operation.Status{status}, To: operation.StatusSucceeded, Reason: "provider_succeeded"}); err != nil {
						return err
					}
					status = operation.StatusSucceeded
				}
				if status == operation.StatusSucceeded {
					if _, err := store.TransitionWorkflowOperation(ctx, op.TransitionInput{OperationID: id, From: []operation.Status{status}, To: operation.StatusIngesting, Reason: "agent_model_receipt"}); err != nil {
						return err
					}
					status = operation.StatusIngesting
				}
			}
		}
		finalization := op.FinalizeInput{OperationID: id, From: []operation.Status{operation.StatusIngesting}, To: operation.StatusCompleted, Reason: "agent_model_receipt"}
		if revoked {
			out = failed("execution_authority_revoked")
			if status.IsTerminal() {
				status = operation.StatusSubmitting
			}
			finalization = op.FinalizeInput{OperationID: id, From: []operation.Status{status}, To: operation.StatusFailed, FailureCode: "execution_authority_revoked", Reason: "execution_authority_revoked"}
		}
		cost, err := store.ProviderCostInTransaction(ctx, tx, id)
		if err != nil {
			return err
		}
		occurred := time.Now().UTC()
		if evidence.ReceivedAt != nil {
			occurred = *evidence.ReceivedAt
		}
		settled, err := g.billing.SettleInTransaction(ctx, tx, billing.SettleInput{ProjectID: scope.ProjectID, OperationID: id, Capability: "text.generate", ModelKey: &r.ModelKey, Region: &r.Region, ActualCostMicros: cost.ActualCostMicros, CapAtReservation: true, OccurredAt: occurred})
		if err != nil {
			return err
		}
		out.CostMicros = settled.ChargeMicros
		finalization.SettledMicros = settled.ChargeMicros
		if err := store.FinalizeInTransaction(ctx, tx, finalization); err != nil {
			return err
		}
		_, err = agentpg.CompleteCall(tx, id, out)
		return err
	})
	if err != nil {
		return pending(), err
	}
	return out, nil
}
