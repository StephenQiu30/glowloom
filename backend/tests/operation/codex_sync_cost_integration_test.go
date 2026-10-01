package operation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// The SDK test environment runs actual PostgreSQL/MinIO Activities and a fake
// stdio model. This is not a paid provider or real image pricing acceptance.
func TestM1SyncCostAndManualPostgresRetainsExecutedImageReservation(t *testing.T) {
	objects := receiptTestObjects(t)
	database := operationStoreDB(t)
	id, store := seedProviderCallOperation(t, database)
	if err := database.Exec(`UPDATE catalog.provider SET adapter_key='codex' WHERE id=(SELECT m.provider_id FROM operation.operation o JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id JOIN catalog.model_profile m ON m.id=v.model_profile_id WHERE o.id=?::uuid)`, id.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile_version SET provider_model_id='fixture-model',queue='agent.codex',supports_query=false,supports_cancel=false WHERE id=(SELECT model_profile_version_id FROM operation.operation WHERE id=?::uuid)`, id.String()).Error; err != nil {
		t.Fatal(err)
	}
	service := application.NewProviderStageService(staging.NewObjects(objects), store)
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	flowActivities := operationflow.NewActivitiesWithImageRecovery(store, finalizer, store, service)
	launcher := &codexTestLauncher{t: t, behavior: "success"}
	adapter, err := codex.New(store, launcher, service, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	register := func(name string, fn any) { env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name}) }
	register("flow.LoadOperation", flowActivities.LoadOperation)
	register("flow.CheckConsent", flowActivities.CheckConsent)
	register("flow.Transition", flowActivities.Transition)
	register("flow.BeginProviderCall", flowActivities.BeginProviderCall)
	register("flow.CompleteProviderCall", flowActivities.CompleteProviderCall)
	register("flow.LoadProviderCost", flowActivities.LoadProviderCost)
	register("flow.RecoverProviderImage", flowActivities.RecoverProviderImage)
	register("flow.CheckManualNotExecuted", flowActivities.CheckManualNotExecuted)
	register("flow.SettleOperation", flowActivities.SettleOperation)
	register("provider.submit", func(ctx context.Context, input operationflow.ProviderSubmitInput) (operationflow.ProviderSubmitOutput, error) {
		identity := application.ProviderDispatchIdentity{ProjectID: uuid.MustParse(input.ProjectID), OperationID: uuid.MustParse(input.OperationID), Action: "submit", Attempt: input.Attempt, RequestKey: input.ProviderRequestKey, ModelProfileVersionID: uuid.MustParse(input.ModelProfileVersionID), PriceRuleVersionID: uuid.MustParse(input.PriceRuleVersionID)}
		result, err := adapter.Submit(ctx, application.ProviderImageInput{Identity: identity, Model: input.ProviderModelID, Prompt: "synthetic image fixture"})
		return operationflow.ProviderSubmitOutput{Outcome: operationflow.ProviderSubmitOutcome(result.Outcome), Receipt: result.Receipt}, err
	})
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("resolve_manual", operationflow.ManualResolution{Outcome: "succeeded", ProviderTaskID: "forged-task"})
	}, time.Second)
	env.RegisterDelayedCallback(func() { env.SignalWorkflow("resolve_manual", operationflow.ManualResolution{Outcome: "not_executed"}) }, 2*time.Second)
	env.RegisterDelayedCallback(env.CancelWorkflow, 3*time.Second)
	env.ExecuteWorkflow(operationflow.OperationWorkflow, operationflow.OperationInput{OperationID: id.String()})
	if err := env.GetWorkflowError(); err == nil || !temporal.IsCanceledError(err) {
		t.Fatalf("unpriced image escaped manual state: %v", err)
	}
	var row struct {
		Status                                       string
		Reserved, Settled, OutputCount, ReleaseCount int64
		TaskID                                       *string
	}
	if err := database.Raw(`SELECT o.status,b.reserved_micros AS reserved,b.settled_micros AS settled,
	 (SELECT COUNT(*) FROM operation.operation_output WHERE operation_id=o.id) AS output_count,
	 (SELECT COUNT(*) FROM billing.ledger_entry WHERE operation_id=o.id AND entry_type<>'reserve') AS release_count,
	 (SELECT provider_task_id FROM operation.provider_call WHERE operation_id=o.id AND action='submit' AND attempt=1) AS task_id
	 FROM operation.operation o JOIN billing.budget b ON b.project_id=o.project_id WHERE o.id=?::uuid`, id.String()).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "manual" || row.Reserved != 10 || row.Settled != 0 || row.OutputCount != 0 || row.ReleaseCount != 0 || row.TaskID != nil || launcher.starts != 1 {
		t.Fatalf("unpriced image changed budget, candidate, task identity, or sending count: %+v starts=%d", row, launcher.starts)
	}
	if err := finalizer.FinalizeOperation(t.Context(), operationflow.SettlementInput{OperationID: id.String(), From: domain.StatusManual, To: domain.StatusFailed, FailureCode: "provider_not_executed", ActualCostMicros: 0}); !errors.Is(err, application.ErrManualResolutionUnverified) {
		t.Fatalf("executed image acquired a zero-cost manual release: %v", err)
	}
}
