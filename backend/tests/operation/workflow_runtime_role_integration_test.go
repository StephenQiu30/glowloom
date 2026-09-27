package operation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestWorkflowTransitionAndSettlementUnderRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	_, operationID, _, _ := operationStoreRows(t, database, actor, projectID)
	if err := database.Exec(`UPDATE billing.budget SET reserved_micros = 10 WHERE project_id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("seed budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros, operation_id)
		VALUES (?::uuid, ?::uuid, 'reserve', 10, ?::uuid)
	`, uuid.NewString(), projectID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("seed reserve ledger: %v", err)
	}
	requestKey := "operation/" + operationID.String()
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return fmt.Errorf("assume runtime role: %w", err)
		}
		operationStore := pgoperation.NewStore(tx)
		if _, err := operationStore.TransitionWorkflowOperation(t.Context(), application.TransitionInput{
			OperationID: operationID, From: []domain.Status{domain.StatusConfirmed},
			To: domain.StatusSubmitting, ProviderRequestKey: &requestKey,
		}); err != nil {
			return fmt.Errorf("transition as runtime role: %w", err)
		}
		finalize := application.FinalizeInput{
			OperationID: operationID, From: []domain.Status{domain.StatusSubmitting},
			To: domain.StatusFailed, FailureCode: "provider_rejected", Retryable: new(false),
		}
		if _, err := operationStore.PrepareFinalizationInTransaction(t.Context(), tx, finalize); err != nil {
			return fmt.Errorf("prepare finalization as runtime role: %w", err)
		}
		result, err := pgbilling.NewStore(tx).SettleInTransaction(t.Context(), tx, billingapp.SettleInput{
			ProjectID: projectID, OperationID: operationID, ActualCostMicros: 7,
			Capability: "image.generate", OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			return fmt.Errorf("settle as runtime role: %w", err)
		}
		finalize.SettledMicros = result.ChargeMicros
		if err := operationStore.FinalizeInTransaction(t.Context(), tx, finalize); err != nil {
			return fmt.Errorf("finalize as runtime role: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status        string
		SettledMicros int64
	}
	if err := database.Raw(`
		SELECT status, settled_micros FROM operation.operation WHERE id = ?::uuid
	`, operationID.String()).Scan(&result).Error; err != nil || result.Status != "failed" || result.SettledMicros != 7 {
		t.Fatalf("runtime settlement = %+v: %v", result, err)
	}
}
