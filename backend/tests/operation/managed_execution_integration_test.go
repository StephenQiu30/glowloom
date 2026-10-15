package operation_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	catalogpg "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
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
	fixture := seedManagedFixture(t, database)
	i, store := fixture.identity, fixture.store
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile SET capability='text.generate' WHERE id=(SELECT model_profile_id FROM catalog.model_profile_version WHERE id=?)`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.model_profile_version SET modes=ARRAY['text_to_text'],queue='provider',supports_cancel=false,supports_query=false,moderation='provider' WHERE id=?`, i.ModelProfileVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":10}' WHERE id=?`, i.PriceRuleVersionID).Error)
	require.NoError(t, database.Exec(`UPDATE operation.operation SET capability='text.generate',mode='text_to_text',params='{}' WHERE id=?`, i.OperationID).Error)
	require.NoError(t, database.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,text_value) VALUES(?,?,0,'prompt','text','fixture prompt') ON CONFLICT(operation_id,seq_no) DO NOTHING`, uuid.New(), i.OperationID).Error)
	results := application.NewManagedResults(&stageTestObjects{entries: map[string]stageTestObject{}}, store)
	network := &fixtureGeneration{}
	worker := managed.NewActivities(store, fixtureUnsealer{}, network, results, nil, nil)
	progress, err := worker.Step(t.Context(), application.ManagedStep{OperationID: i.OperationID, Action: "submit"})
	require.NoError(t, err)
	require.Equal(t, "succeeded", progress.State)
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
	actor.PlatformRole = "platform_admin"
	require.NoError(t, database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, actor.ID).Error)
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
	releases := catalogapp.NewModelReleases(catalogpg.NewStore(database))
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
