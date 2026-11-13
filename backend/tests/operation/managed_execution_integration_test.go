package operation_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

type fixtureUnsealer struct{}

func (fixtureUnsealer) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"api_key":"fixture-only-not-real"}`), nil
}

type fixtureGeneration struct{ calls int }

func (f *fixtureGeneration) Submit(context.Context, application.GenerationRequest, string) (application.GenerationResult, error) {
	f.calls++
	return application.GenerationResult{Outcome: "completed", State: "succeeded", Text: "fixture generated content", Usage: json.RawMessage(`{"prompt_tokens":4,"completion_tokens":8}`)}, nil
}
func (f *fixtureGeneration) Query(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	panic("text must not query")
}
func (f *fixtureGeneration) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	panic("cancel unsupported")
}

func TestManagedTextDispatchRecoveryAndSettlement(t *testing.T) {
	database := operationStoreDB(t)
	fixture := seedUnverifiedManagedFixture(t, database)
	i, store := fixture.identity, fixture.store
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET current_config_version_id=? WHERE id=?`, fixture.config, fixture.provider).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
	seedDispatchModeEvidence(t, database, fixture)
	require.NoError(t, database.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','fixture prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	network := &fixtureGeneration{}
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil)
	progress, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.EqualValues(t, "succeeded", progress.State)
	// Repeating an activity delivery reads its durable manifest; it never resends.
	replay, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, progress, replay)
	require.Equal(t, 1, network.calls)
	require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSucceeded})
	require.NoError(t, err)
	_, err = store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSucceeded}, To: domain.StatusIngesting})
	require.NoError(t, err)
	require.NoError(t, store.IngestManagedText(t.Context(), i))
	require.NoError(t, store.IngestManagedText(t.Context(), i))
	cost, err := store.LoadProviderCost(t.Context(), i.OperationID)
	require.NoError(t, err)
	require.Equal(t, int64(10), cost.ActualCostMicros)
	var actor identityapp.Principal
	require.NoError(t, database.Raw(`SELECT actor.id,actor.org_id,actor.role FROM identity."user" actor JOIN workspace.project p ON p.org_id=actor.org_id WHERE p.id=?`, i.ProjectID).Scan(&actor).Error)
	actor = identityfixture.Session(t, database, actor)
	actor.PlatformRole = "platform_admin"
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
	actor = identityfixture.Session(t, database, actor)
	require.NoError(t, database.Exec(`INSERT INTO operation.provider_trial(operation_id,actor_id,max_charge_micros,supplier_max_micros,supplier_currency) VALUES(?,?,100,100,'USD')`, i.OperationID, actor.ID).Error)
	finalizer := app.NewOperationFinalizer(database, store, billingpg.NewStore(database))
	finish := operationflow.SettlementInput{OperationID: i.OperationID.String(), From: domain.StatusIngesting, To: domain.StatusCompleted, ActualCostMicros: 10}
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
	require.NoError(t, finalizer.FinalizeOperation(t.Context(), finish))
	var count int64
	require.NoError(t, database.Raw(`SELECT count(*) FROM billing.ledger_entry WHERE operation_id=? AND entry_type='settle'`, i.OperationID).Scan(&count).Error)
	require.Equal(t, int64(1), count)
	// Exercise the evidence consumer using isolated fake transport. This fixture
	// does not assert that a live supplier or account has been accepted.
	var model struct {
		ID       uuid.UUID
		Revision int64
	}
	require.NoError(t, database.Raw(`SELECT m.id,m.revision FROM catalog.model_profile m JOIN catalog.model_profile_version v ON v.model_profile_id=m.id WHERE v.id=?`, i.ModelProfileVersionID).Scan(&model).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET active_release_id=NULL,status='disabled' WHERE id=?`, model.ID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_release SET status='draft' WHERE id=?`, fixture.release).Error)
	releases := catalogapp.NewModelReleases(app.NewCatalogStore(database))
	listed, err := releases.List(t.Context(), actor, model.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, []string{"text_to_text"}, listed[0].ValidatedModes)
	activated, err := releases.Activate(t.Context(), actor, fixture.release, catalogapp.ActivateReleaseInput{ExpectedRevision: listed[0].Revision, ExpectedModelRevision: model.Revision, RequestID: uuid.New()})
	require.NoError(t, err)
	require.Equal(t, "active", activated.Status)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET active_release_id=NULL,status='disabled' WHERE id=?`, model.ID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_release SET status='retired' WHERE id=?`, fixture.release).Error)
	restored, err := releases.Activate(t.Context(), actor, fixture.release, catalogapp.ActivateReleaseInput{ExpectedRevision: activated.Revision, ExpectedModelRevision: model.Revision + 1, RequestID: uuid.New()})
	require.NoError(t, err)
	require.Equal(t, "active", restored.Status)
}

// This transport fixture supplies supplier usage; persistence and normalization
// use the real managed activity and PostgreSQL completion path.
type cachedUsageGeneration struct{ calls int }

func (f *cachedUsageGeneration) Submit(context.Context, application.GenerationRequest, string) (application.GenerationResult, error) {
	f.calls++
	return application.GenerationResult{Outcome: "completed", State: "succeeded", Text: "cached reply", Usage: json.RawMessage(`{"prompt_tokens":4,"cached_input_tokens":2,"completion_tokens":8}`)}, nil
}
func (*cachedUsageGeneration) Query(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	panic("text must not query")
}
func (*cachedUsageGeneration) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	panic("cancel unsupported")
}

func TestManagedCachedTokensReachFrozenSettlement(t *testing.T) {
	database := operationStoreDB(t)
	fixture := seedUnverifiedManagedFixture(t, database)
	i, store := fixture.identity, fixture.store
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"input_micros_per_1k":1000,"cached_input_micros_per_1k":100,"output_micros_per_1k":2000}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
	seedDispatchModeEvidence(t, database, fixture)
	require.NoError(t, database.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','cached prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	network := &cachedUsageGeneration{}
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil)
	progress, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.EqualValues(t, "succeeded", progress.State)
	require.NoError(t, store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "completed", Managed: true}))
	cost, err := store.LoadProviderCost(t.Context(), i.OperationID)
	require.NoError(t, err)
	require.Equal(t, int64(19), cost.ActualCostMicros)
	_, err = worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, 1, network.calls)
}

type videoTokenGeneration struct{}

func (*videoTokenGeneration) Submit(context.Context, application.GenerationRequest, string) (application.GenerationResult, error) {
	return application.GenerationResult{Outcome: "accepted", State: "pending", TaskID: "fixture-video"}, nil
}
func (*videoTokenGeneration) Query(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	return application.GenerationResult{State: "succeeded", TaskID: "fixture-video", URLs: []string{"https://fixture.invalid/video.mp4"}, Usage: json.RawMessage(`{"completion_tokens":37}`)}, nil
}
func (*videoTokenGeneration) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	panic("cancel unsupported")
}
func (*videoTokenGeneration) Read(context.Context, catalogdomain.ProviderConfig, string, string, string) ([]byte, int64, error) {
	return []byte("synthetic video bytes; no media acceptance"), 99999, nil
}

func TestManagedVideoUsesSupplierTokensInsteadOfArtifactDuration(t *testing.T) {
	database := operationStoreDB(t)
	f := seedUnverifiedManagedFixture(t, database)
	i := f.identity
	require.NoError(t, database.Exec(`UPDATE catalog.provider SET adapter_key='volcengine_ark',region='domestic' WHERE id=?`, f.provider).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.provider_config_version SET config=(SELECT payload->'config' FROM catalog.provider_preset_version WHERE id='6da07a12-0000-4000-8000-000000000001') WHERE id=?`, f.config).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='video.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text2video'],supports_query=true,provider_model_id='doubao-seedance-2-0-mini-260615',queue='provider',moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	procurement := uuid.New()
	require.NoError(t, database.Exec(`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from)
		SELECT ?,model_profile_id,2,'per_1k_tokens','{"input_micros_per_1k":0,"output_micros_per_1k":500}','CNY',effective_from
		FROM catalog.price_rule_version WHERE id=?`, procurement, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',
		rule=jsonb_build_object('input_micros_per_1k',0,'output_micros_per_1k',1000,'procurement_price_version_id',?::text)
		WHERE id=?`, procurement, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='video.generate',mode='text2video',params='{"duration_ms":4000,"resolution":"480p","aspect_ratio":"16:9"}' WHERE id=?`, i.OperationID).Error)
	seedDispatchModeEvidence(t, database, f)
	require.NoError(t, database.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','video prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, f.store)
	network := &videoTokenGeneration{}
	worker := managed.NewActivities(f.store, fixtureUnsealer{}, network, results, network)
	progress, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.EqualValues(t, "accepted", progress.State)
	taskID := "fixture-video"
	require.NoError(t, f.store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{ProviderTaskID: &taskID, OperationID: i.OperationID, Action: "submit", Attempt: 1, Outcome: "ok", State: "accepted"}))
	_, err = f.store.TransitionWorkflowOperation(t.Context(), application.TransitionInput{OperationID: i.OperationID, From: []domain.Status{domain.StatusSubmitting}, To: domain.StatusSubmitted, ProviderTaskID: &taskID})
	require.NoError(t, err)
	require.NoError(t, f.store.BeginProviderCall(t.Context(), application.BeginProviderCallInput{OperationID: i.OperationID, Action: "query", Attempt: 1, ProviderTaskID: &taskID}))
	progress, err = worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "query"})
	require.NoError(t, err)
	require.EqualValues(t, "succeeded", progress.State)
	require.NoError(t, f.store.CompleteProviderCall(t.Context(), application.CompleteProviderCallInput{ProviderTaskID: &taskID, OperationID: i.OperationID, Action: "query", Attempt: 1, Outcome: "ok", State: "succeeded", Managed: true}))
	cost, err := f.store.LoadProviderCost(t.Context(), i.OperationID)
	require.NoError(t, err)
	require.Equal(t, int64(37), cost.ActualCostMicros)
	var supplierMicros int64
	require.NoError(t, database.Raw(`SELECT procurement_cost_micros FROM operation.provider_call
		WHERE operation_id=? AND action='query'`, i.OperationID).Row().Scan(&supplierMicros))
	require.Equal(t, int64(19), supplierMicros)
}
