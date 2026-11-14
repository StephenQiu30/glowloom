package operation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	op "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

func TestCommercialWalletQuoteConfirmationAndMockedSettlement(t *testing.T) {
	db := operationStoreDB(t)
	actor, project := operationStoreProject(t, db)
	seed, model, provider := seedUnfundedQuoteCatalog(t, db, project, time.Now().UTC())
	price, version := *seed.PriceRuleVersionID, *seed.ModelProfileVersionID
	procurement, config, release := uuid.New(), uuid.New(), uuid.New()
	modelKey := "quote-model-" + model.String()
	require.NoError(t, db.Exec(`UPDATE billing.budget SET limit_micros=500000 WHERE project_id=?`, project).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.provider SET adapter_key='deepseek',status='active' WHERE id=?`, provider).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=?`, model).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile_version SET provider_model_id='deepseek-flash',
		modes=ARRAY['text_to_text'],supports_query=false,limits='{"max_outputs":1}',queue='provider',moderation='provider'
		WHERE id=?`, version).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":120000}' WHERE id=?`, price).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from)
		SELECT ?,model_profile_id,2,'per_request','{"base_micros":50000}','CNY',effective_from
		FROM catalog.price_rule_version WHERE id=?`, procurement, price).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.provider_config_version(id,provider_id,preset_version_id,version_no,config,content_hash)
		SELECT ?,?,v.id,1,v.payload->'config',v.content_hash FROM catalog.provider_preset_version v
		JOIN catalog.provider_preset p ON p.id=v.preset_id WHERE p.slug='deepseek' ORDER BY v.version_no DESC LIMIT 1`, config, provider).Error)
	require.NoError(t, db.Exec(`INSERT INTO catalog.model_release(id,model_id,model_version_id,price_version_id,config_version_id,upstream_cost,status)
		VALUES(?,?,?,?,?,'{}','active')`, release, model, version, price, config).Error)
	require.NoError(t, db.Exec(`UPDATE catalog.model_profile SET active_release_id=? WHERE id=?`, release, model).Error)

	// This wallet test owns synthetic catalog acceptance; the real projection is
	// exercised by catalog availability integration tests. Keep its intentional
	// pre-review tariffs available to test the independent wallet rejection.
	store := operationStore(db).WithExecutionAvailability(func(*gorm.DB) op.ExecutionAvailability {
		return approvedWalletFixtureExecution{version: version, release: release}
	})
	input := op.CreateFreeQuoteInput{ProjectID: project, RequestID: uuid.NewString(), FreeQuoteItemInput: op.FreeQuoteItemInput{
		ModelKey: modelKey, Capability: "text.generate", Mode: "text_to_text", Prompt: "测试分镜", OutputCount: 1,
	}}

	unfunded, err := store.CreateFreeQuote(t.Context(), actor, input)
	require.NoError(t, err)
	require.False(t, unfunded.Confirmable, "project budget must not replace wallet credits")
	require.Contains(t, string(unfunded.QuoteDetail), "credits_insufficient")
	_, err = store.ConfirmSingleQuote(t.Context(), actor, op.ConfirmSingleQuoteInput{
		ProjectID: project, OperationID: unfunded.OperationID, RequestID: uuid.NewString(),
	})
	require.ErrorIs(t, err, billingdomain.ErrWalletInsufficient)
	input.RequestID = uuid.NewString()
	var scope, lot uuid.UUID
	require.NoError(t, db.Raw(`SELECT billing.bind_project_wallet(?)`, project).Row().Scan(&scope))
	require.NoError(t, db.Raw(`SELECT billing.issue_promotional_credit(?,?,210000,now()+interval '1 hour',?,?)`,
		scope, uuid.New(), modelKey, "isolated model wallet test").Row().Scan(&lot))
	blocked, err := store.CreateFreeQuote(t.Context(), actor, input)
	require.NoError(t, err)
	require.False(t, blocked.Confirmable)
	require.Contains(t, string(blocked.QuoteDetail), "commercial_price_unavailable")
	_, err = store.ConfirmSingleQuote(t.Context(), actor, op.ConfirmSingleQuoteInput{
		ProjectID: project, OperationID: blocked.OperationID, RequestID: uuid.NewString(),
	})
	require.ErrorIs(t, err, billingdomain.ErrWalletPriceUnavailable)

	// The dedicated release test verifies published pairing. This fixture
	// supplies those already-reviewed immutable versions to the runtime.
	require.NoError(t, db.Exec(`UPDATE catalog.price_rule_version SET rule=jsonb_set(rule,'{procurement_price_version_id}',to_jsonb(?::text)) WHERE id=?`,
		procurement, price).Error)
	batch, err := store.CreateBatchFreeQuote(t.Context(), actor, op.CreateBatchFreeQuoteInput{
		ProjectID: project, RequestID: uuid.NewString(), Items: []op.FreeQuoteItemInput{input.FreeQuoteItemInput, input.FreeQuoteItemInput},
	})
	require.NoError(t, err)
	require.NotNil(t, batch.BatchID)
	require.EqualValues(t, 400_000, batch.TotalMicros)
	require.False(t, batch.Confirmable)
	_, err = store.ConfirmBatchQuote(t.Context(), actor, op.ConfirmBatchQuoteInput{
		ProjectID: project, BatchID: *batch.BatchID, RequestID: uuid.NewString(),
	})
	require.ErrorIs(t, err, billingdomain.ErrWalletInsufficient)
	var reservations int
	require.NoError(t, db.Raw(`SELECT count(*) FROM billing.reservation WHERE project_id=?`, project).Row().Scan(&reservations))
	require.Zero(t, reservations, "the affordable first item must roll back as well")
	input.RequestID = uuid.NewString()
	quote, err := store.CreateFreeQuote(t.Context(), actor, input)
	require.NoError(t, err)
	require.True(t, quote.Confirmable)
	var detail op.QuoteCostDetail
	require.NoError(t, json.Unmarshal(quote.QuoteDetail, &detail))
	require.Equal(t, "2", detail.MaxChargeCredits)
	require.Equal(t, "120000", detail.ExactQuoteMicros)
	require.True(t, detail.WholeCredits)
	require.EqualValues(t, 200_000, quote.QuoteMicros)
	confirmed, err := store.ConfirmSingleQuote(t.Context(), actor, op.ConfirmSingleQuoteInput{
		ProjectID: project, OperationID: quote.OperationID, RequestID: uuid.NewString(),
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, confirmed.ReservationID)
	id := quote.OperationID
	var key string
	require.NoError(t, db.Raw(`SELECT provider_request_key FROM operation.operation WHERE id=?`, id).Row().Scan(&key))
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusConfirmed}, To: domain.StatusSubmitting, ProviderRequestKey: &key})
	require.NoError(t, err)
	require.NoError(t, store.BeginProviderCall(t.Context(), op.BeginProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, DispatchRequired: true}))
	_, err = store.LoadProviderCost(t.Context(), id)
	require.ErrorIs(t, err, op.ErrProviderCostUnknown)
	wallet, err := billingpg.NewStore(db).ReadWallet(t.Context(), scope, true, nil)
	require.NoError(t, err)
	require.Equal(t, "2", wallet.HeldCredits)

	identity := op.ProviderDispatchIdentity{OperationID: id, ProjectID: project, ModelProfileVersionID: version,
		PriceRuleVersionID: price, Action: "submit", Attempt: 1, RequestKey: key}
	var credential uuid.UUID
	require.NoError(t, db.Raw(`SELECT id FROM catalog.provider_credential WHERE provider_id=? AND status='active'`, provider).Row().Scan(&credential))
	seedDispatchModeEvidence(t, db, managedFixture{identity: identity, release: release, credential: credential})

	network := &fixtureGeneration{}
	results := op.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil)
	progress, err := worker.Step(t.Context(), op.ManagedStep{OperationID: id, Action: "submit"})
	require.NoError(t, err)
	require.EqualValues(t, "succeeded", progress.State)
	require.NoError(t, store.CompleteProviderCall(t.Context(), op.CompleteProviderCallInput{
		OperationID: id, Attempt: 1, Action: "submit", State: "completed", Outcome: "ok", Managed: true,
	}))
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), op.TransitionInput{OperationID: id, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	require.NoError(t, store.IngestManagedText(t.Context(), identity))
	f := app.NewOperationFinalizer(db, store, billingpg.NewStore(db))
	finish := operationflow.SettlementInput{OperationID: id.String(), From: domain.StatusIngesting, To: domain.StatusCompleted, ActualCostMicros: 120_000}
	require.NoError(t, f.FinalizeOperation(t.Context(), finish))
	require.NoError(t, f.FinalizeOperation(t.Context(), finish))
	wallet, err = billingpg.NewStore(db).ReadWallet(t.Context(), scope, true, nil)
	require.NoError(t, err)
	require.Equal(t, "0", wallet.HeldCredits)
	require.Equal(t, "2", wallet.RedeemedToday)
	require.Equal(t, "0.1", wallet.RestrictedCredits)
	require.Equal(t, 1, network.calls)
	var supplierMicros int64
	require.NoError(t, db.Raw(`SELECT procurement_cost_micros FROM operation.provider_call
		WHERE operation_id=? AND action='submit'`, id).Row().Scan(&supplierMicros))
	require.Equal(t, int64(50_000), supplierMicros)
}

type approvedWalletFixtureExecution struct{ version, release uuid.UUID }

func (r approvedWalletFixtureExecution) AvailableForNewExecution(_ context.Context, version, release uuid.UUID, mode string) (bool, error) {
	return version == r.version && release == r.release && mode == "text_to_text", nil
}
