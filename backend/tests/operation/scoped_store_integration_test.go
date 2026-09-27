package operation_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func operationStoreDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_OPERATION_STORE_DB_DSN to an isolated migrated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open operation store database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB.WithContext(ctx)
}

func operationStoreProject(t *testing.T, database *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	orgID, userID, projectID := uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)`,
		orgID.String(), "operation-"+orgID.String()).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Operation Producer', 'producer', 'test-hash', false)
	`, userID.String(), orgID.String(), "operation-"+userID.String()).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO workspace.project (id, org_id, name, aspect_ratio, style_type)
		VALUES (?::uuid, ?::uuid, '操作测试', '16:9', 'realistic')
	`, projectID.String(), orgID.String()).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 100)
	`, uuid.NewString(), projectID.String()).Error; err != nil {
		t.Fatalf("create project budget: %v", err)
	}
	return identityapp.Principal{ID: userID, OrgID: orgID, Role: identitydomain.RoleProducer}, projectID
}

func operationStoreRows(t *testing.T, database *gorm.DB, actor identityapp.Principal, projectID uuid.UUID) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	batchID, operationID, inputID, reservationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO operation.batch
		  (id, project_id, kind, scope, status, total_count, quote_total_micros)
		VALUES (?::uuid, ?::uuid, 'mixed', '{}'::jsonb, 'confirmed', 1, 10)
	`, batchID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create confirmed batch: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, batch_id, target_type, capability, mode,
		   model_profile_version_id, price_rule_version_id, input_hash, origin, status, quote_micros,
		   quote_expires_at, region, confirmed_at, confirmed_by)
		VALUES (?::uuid, ?::uuid, ?::uuid, 'free', 'image.generate', 'text_to_image',
		        ?::uuid, ?::uuid, ?, 'canvas', 'confirmed', 10,
		        now() + interval '15 minutes', 'domestic', now(), ?::uuid)
	`, operationID.String(), projectID.String(), batchID.String(),
		uuid.NewString(), uuid.NewString(),
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		actor.ID.String()).Error; err != nil {
		t.Fatalf("create confirmed operation: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation_input
		  (id, operation_id, seq_no, role, ref_type, text_value)
		VALUES (?::uuid, ?::uuid, 1, 'prompt', 'text', 'input snapshot')
	`, inputID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("create frozen input: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.reservation
		  (id, project_id, operation_id, amount_micros, status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 10, 'held')
	`, reservationID.String(), projectID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	if err := database.Exec(`
		UPDATE operation.operation SET reservation_id = ?::uuid WHERE id = ?::uuid
	`, reservationID.String(), operationID.String()).Error; err != nil {
		t.Fatalf("link confirmed operation reservation: %v", err)
	}
	return batchID, operationID, inputID, reservationID
}

func TestOperationStoresReturnProjectScopedSnapshots(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	batchID, operationID, inputID, reservationID := operationStoreRows(t, database, actor, projectID)
	store := pgoperation.NewStore(database)
	batch, err := store.FindBatch(t.Context(), actor, projectID, batchID)
	if err != nil || batch.ID != batchID || batch.ProjectID != projectID || batch.TotalCount != 1 || batch.QuoteTotalMicros != 10 {
		t.Fatalf("scoped batch = %+v: %v", batch, err)
	}
	operation, err := store.FindOperation(t.Context(), actor, projectID, operationID)
	if err != nil || operation.ID != operationID || operation.ProjectID != projectID ||
		operation.BatchID == nil || *operation.BatchID != batchID || operation.QuoteMicros == nil || *operation.QuoteMicros != 10 {
		t.Fatalf("scoped operation = %+v: %v", operation, err)
	}
	inputs, err := store.FindOperationInputs(t.Context(), actor, projectID, operationID)
	if err != nil || len(inputs) != 1 || inputs[0].ID != inputID || inputs[0].OperationID != operationID || inputs[0].SeqNo != 1 {
		t.Fatalf("scoped frozen inputs = %+v: %v", inputs, err)
	}
	reservation, err := pgbilling.NewStore(database).FindReservation(t.Context(), actor, projectID, reservationID)
	if err != nil || reservation.ID != reservationID || reservation.ProjectID != projectID ||
		reservation.OperationID != operationID || reservation.AmountMicros != 10 {
		t.Fatalf("scoped reservation = %+v: %v", reservation, err)
	}
}

func TestOperationStoresHideOtherProjectsAndRevokedActors(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	batchID, operationID, _, reservationID := operationStoreRows(t, database, actor, projectID)
	otherActor, otherProjectID := operationStoreProject(t, database)
	store := pgoperation.NewStore(database)
	billingStore := pgbilling.NewStore(database)
	for _, tc := range []struct {
		name      string
		actor     identityapp.Principal
		projectID uuid.UUID
	}{
		{"other organization", otherActor, projectID},
		{"wrong project", actor, otherProjectID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.FindBatch(t.Context(), tc.actor, tc.projectID, batchID); !errors.Is(err, pgoperation.ErrNotFound) {
				t.Fatalf("batch visibility: %v", err)
			}
			if _, err := store.FindOperation(t.Context(), tc.actor, tc.projectID, operationID); !errors.Is(err, pgoperation.ErrNotFound) {
				t.Fatalf("operation visibility: %v", err)
			}
			if _, err := store.FindOperationInputs(t.Context(), tc.actor, tc.projectID, operationID); !errors.Is(err, pgoperation.ErrNotFound) {
				t.Fatalf("input visibility: %v", err)
			}
			if _, err := billingStore.FindReservation(t.Context(), tc.actor, tc.projectID, reservationID); !errors.Is(err, pgbilling.ErrNotFound) {
				t.Fatalf("reservation visibility: %v", err)
			}
		})
	}
	forged := actor
	forged.ID = uuid.New()
	if _, err := store.FindOperation(t.Context(), forged, projectID, operationID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("forged actor operation: %v", err)
	}
	if _, err := billingStore.FindReservation(t.Context(), forged, projectID, reservationID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("forged actor reservation: %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := store.FindOperation(t.Context(), actor, projectID, operationID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor operation: %v", err)
	}
	if _, err := billingStore.FindReservation(t.Context(), actor, projectID, reservationID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor reservation: %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("restore actor: %v", err)
	}
	if err := database.Exec(`
		UPDATE workspace.project SET is_delete = true, delete_time = now(), purge_after = now() + interval '30 days'
		WHERE id = ?::uuid
	`, projectID.String()).Error; err != nil {
		t.Fatalf("soft delete project: %v", err)
	}
	if _, err := store.FindBatch(t.Context(), actor, projectID, batchID); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("deleted project batch: %v", err)
	}
	if _, err := store.FindOperationInputs(t.Context(), actor, projectID, operationID); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("deleted project input: %v", err)
	}
	if _, err := billingStore.FindReservation(t.Context(), actor, projectID, reservationID); !errors.Is(err, pgbilling.ErrNotFound) {
		t.Fatalf("deleted project reservation: %v", err)
	}
}
