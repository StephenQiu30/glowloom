package operation_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

func TestConfirmationWritesUnderRuntimeRole(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	item := quotedSnapshotItem(projectID, 20)
	batchID := uuid.New()
	item.Operation.BatchID = &batchID
	batch := domain.Batch{
		ID: batchID, ProjectID: projectID, Kind: "mixed",
		Scope: json.RawMessage(`{}`), Status: domain.BatchStatusQuoted,
		TotalCount: 1, QuoteTotalMicros: 20,
	}
	if err := pgoperation.NewStore(database).CreateQuoteSnapshot(t.Context(), actor, &batch, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatalf("seed quoted batch: %v", err)
	}
	reservationID, ledgerID := uuid.New(), uuid.New()
	rollback := errors.New("rollback confirmation ACL probe")
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return fmt.Errorf("assume runtime role: %w", err)
		}
		if err := tx.Exec(`
			INSERT INTO billing.reservation (id, project_id, operation_id, amount_micros, status)
			VALUES (?::uuid, ?::uuid, ?::uuid, 20, 'held')
		`, reservationID.String(), projectID.String(), item.Operation.ID.String()).Error; err != nil {
			return fmt.Errorf("insert held reservation: %w", err)
		}
		if err := tx.Exec(`
			UPDATE billing.budget SET reserved_micros = reserved_micros + 20, revision = revision + 1
			WHERE project_id = ?::uuid
		`, projectID.String()).Error; err != nil {
			return fmt.Errorf("reserve project budget: %w", err)
		}
		if err := tx.Exec(`
			INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros, operation_id)
			VALUES (?::uuid, ?::uuid, 'reserve', 20, ?::uuid)
		`, ledgerID.String(), projectID.String(), item.Operation.ID.String()).Error; err != nil {
			return fmt.Errorf("insert reserve ledger entry: %w", err)
		}
		updated := tx.Exec(`
			UPDATE operation.operation
			SET status = 'confirmed', reservation_id = ?::uuid, confirmed_at = ?, confirmed_by = ?::uuid,
			    provider_request_key = ?, workflow_id = ?
			WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'
		`, reservationID.String(), time.Now().UTC(), actor.ID.String(),
			"lv-"+item.Operation.ID.String(), "operation/"+item.Operation.ID.String(),
			item.Operation.ID.String(), projectID.String())
		if updated.Error != nil {
			return fmt.Errorf("confirm quoted operation: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("confirm quoted operation: updated %d rows, want 1", updated.RowsAffected)
		}
		updated = tx.Exec(`
			UPDATE operation.batch
			SET status = 'confirmed', total_count = 1, quote_total_micros = 20, update_time = now()
			WHERE id = ?::uuid AND project_id = ?::uuid AND status = 'quoted'
		`, batchID.String(), projectID.String())
		if updated.Error != nil {
			return fmt.Errorf("confirm quoted batch: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("confirm quoted batch: updated %d rows, want 1", updated.RowsAffected)
		}
		var privileges struct {
			CanAlterQuote      bool
			CanDeleteHeldFunds bool
		}
		if err := tx.Raw(`
			SELECT has_column_privilege(current_user, 'operation.operation', 'quote_micros', 'UPDATE') AS can_alter_quote,
			       has_table_privilege(current_user, 'billing.reservation', 'DELETE') AS can_delete_held_funds
		`).Scan(&privileges).Error; err != nil {
			return fmt.Errorf("inspect confirmation role privileges: %w", err)
		}
		if privileges.CanAlterQuote || privileges.CanDeleteHeldFunds {
			return fmt.Errorf("confirmation role has unrelated write privileges: %+v", privileges)
		}
		requestID := uuid.New()
		if err := tx.Exec(`
			INSERT INTO operation.confirmation_request
			  (org_id, actor_id, request_id, fingerprint, outcome)
			VALUES (?::uuid, ?::uuid, ?::uuid, decode(repeat('ab', 32), 'hex'), '{"result":{}}'::jsonb)
		`, actor.OrgID.String(), actor.ID.String(), requestID.String()).Error; err != nil {
			return fmt.Errorf("append confirmation result as runtime role: %w", err)
		}
		var visible bool
		if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM operation.confirmation_request
			WHERE org_id = ?::uuid AND actor_id = ?::uuid AND request_id = ?::uuid)`,
			actor.OrgID.String(), actor.ID.String(), requestID.String()).Scan(&visible).Error; err != nil {
			return fmt.Errorf("read confirmation result as runtime role: %w", err)
		}
		if !visible {
			return errors.New("confirmation result is not visible to runtime role")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("confirmation writes as runtime role: %v", err)
	}
}
