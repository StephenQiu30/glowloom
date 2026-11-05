package operation_test

import (
	"context"
	"errors"
	"testing"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// The task handlers run actual PostgreSQL/COS adapters and a fake
// stdio model. This is not a paid provider or real image pricing acceptance.
func TestM1SyncCostAndManualPostgresRetainsExecutedImageReservation(t *testing.T) {
	receiptTestObjects(t)
	database := operationStoreDB(t)
	id, store := seedProviderCallOperation(t, database)
	if err := database.Exec(`UPDATE catalog.provider SET adapter_key='codex' WHERE id=(SELECT m.provider_id FROM operation.operation o JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id JOIN catalog.model_profile m ON m.id=v.model_profile_id WHERE o.id=?::uuid)`, id.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE catalog.model_profile_version SET provider_model_id='fixture-model',queue='agent.codex',supports_query=false,supports_cancel=false WHERE id=(SELECT model_profile_version_id FROM operation.operation WHERE id=?::uuid)`, id.String()).Error; err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadWorkflowOperation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	businessPinOperation(t, database, application.ProviderDispatchIdentity{OperationID: id, ProjectID: loaded.Operation.ProjectID})
	service := application.NewRoutedProviderStageService(staging.NewRoutedFactory(businessStorageRegistry(t), pgmedia.NewStorageRoutes(database)), store)
	finalizer := app.NewOperationFinalizer(database, store, pgbilling.NewStore(database))
	launcher := &codexTestLauncher{t: t, behavior: "success"}
	adapter, err := codex.New(store, launcher, service, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	h := map[string]taskruntime.Handler{}
	operationflow.RegisterFlow(h, store, finalizer, service)
	operationflow.RegisterCodex(h, store, codexSubmitFunc(func(ctx context.Context, input operationflow.ProviderSubmitInput) (operationflow.ProviderSubmitOutput, error) {
		identity := application.ProviderDispatchIdentity{ProjectID: uuid.MustParse(input.ProjectID), OperationID: uuid.MustParse(input.OperationID), Action: "submit", Attempt: input.Attempt, RequestKey: input.ProviderRequestKey, ModelProfileVersionID: uuid.MustParse(input.ModelProfileVersionID), PriceRuleVersionID: uuid.MustParse(input.PriceRuleVersionID)}
		result, err := adapter.Submit(ctx, application.ProviderImageInput{Identity: identity, Model: input.ProviderModelID, Prompt: "synthetic image fixture"})
		return operationflow.ProviderSubmitOutput{Outcome: operationflow.ProviderSubmitOutcome(result.Outcome), Receipt: result.Receipt}, err
	}))
	job := taskruntime.Job{BusinessID: id.String()}
	for n := 0; n < 7; n++ {
		_, err = h["operation.run"](t.Context(), job)
		if err != nil {
			t.Fatal(err)
		}
		_, err = h["operation.codex"](t.Context(), job)
		if err != nil {
			t.Fatal(err)
		}
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

type codexSubmitFunc func(context.Context, operationflow.ProviderSubmitInput) (operationflow.ProviderSubmitOutput, error)

func (f codexSubmitFunc) Submit(ctx context.Context, in operationflow.ProviderSubmitInput) (operationflow.ProviderSubmitOutput, error) {
	return f(ctx, in)
}
