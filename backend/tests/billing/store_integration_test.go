package billing_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func billingDB(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_BILLING_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_BILLING_DB_DSN to an isolated PostgreSQL database with migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open billing test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return ctx, conn.DB.WithContext(ctx)
}

func billingProject(t *testing.T, database *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	orgID, userID, projectID := uuid.New(), uuid.New(), uuid.New()
	if err := database.Exec(`INSERT INTO workspace.organization (id, name) VALUES (?::uuid, ?)`, orgID.String(), "billing-"+orgID.String()).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Budget Producer', 'producer', 'test-hash', false)
	`, userID.String(), orgID.String(), "billing-"+userID.String()).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO workspace.project (id, org_id, name, aspect_ratio, style_type)
		VALUES (?::uuid, ?::uuid, '预算测试', '16:9', 'realistic')
	`, projectID.String(), orgID.String()).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	return identityapp.Principal{ID: userID, OrgID: orgID, Role: identitydomain.RoleProducer}, projectID
}

func TestBillingStoreScopesBudgetAndLedgerToLiveProject(t *testing.T) {
	_, database := billingDB(t)
	actor, projectID := billingProject(t, database)
	otherActor, otherProjectID := billingProject(t, database)
	budgetID, entryID := uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 0)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create zero budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros)
		VALUES (?::uuid, ?::uuid, 0)
	`, uuid.NewString(), otherProjectID.String()).Error; err != nil {
		t.Fatalf("create other project budget: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros, create_by)
		VALUES (?::uuid, ?::uuid, 'budget_change', 500, ?::uuid)
	`, entryID.String(), projectID.String(), actor.ID.String()).Error; err != nil {
		t.Fatalf("create ledger entry: %v", err)
	}
	store := pgbilling.NewStore(database)
	budget, err := store.FindBudget(t.Context(), actor, projectID)
	if err != nil || budget.ID != budgetID || budget.ProjectID != projectID || budget.LimitMicros != 0 {
		t.Fatalf("own budget = %+v: %v", budget, err)
	}
	entry, err := store.FindLedgerEntry(t.Context(), actor, projectID, entryID)
	if err != nil || entry.ID != entryID || entry.ProjectID != projectID || entry.AmountMicros != 500 || entry.CreateBy == nil || *entry.CreateBy != actor.ID {
		t.Fatalf("own ledger entry = %+v: %v", entry, err)
	}
	for _, scoped := range []struct {
		name      string
		actor     identityapp.Principal
		projectID uuid.UUID
	}{
		{"other organization", otherActor, projectID},
		{"wrong project ID", actor, otherProjectID},
	} {
		t.Run(scoped.name, func(t *testing.T) {
			if _, err := store.FindBudget(t.Context(), scoped.actor, scoped.projectID); !errors.Is(err, pgbilling.ErrNotFound) {
				t.Fatalf("budget visibility error = %v", err)
			}
			if _, err := store.FindLedgerEntry(t.Context(), scoped.actor, scoped.projectID, entryID); !errors.Is(err, pgbilling.ErrNotFound) {
				t.Fatalf("ledger visibility error = %v", err)
			}
		})
	}
	forged := actor
	forged.ID = uuid.New()
	if _, err := store.FindBudget(t.Context(), forged, projectID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("forged actor budget error = %v", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET is_delete = true, delete_time = now(), purge_after = now() + interval '30 days' WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("mark project deleted: %v", err)
	}
	if _, err := store.FindBudget(t.Context(), actor, projectID); !errors.Is(err, pgbilling.ErrNotFound) {
		t.Fatalf("deleted project budget error = %v", err)
	}
	if _, err := store.FindLedgerEntry(t.Context(), actor, projectID, entryID); !errors.Is(err, pgbilling.ErrNotFound) {
		t.Fatalf("deleted project ledger error = %v", err)
	}
}

func TestLedgerMigrationEnforcesAppendOnlyAndApplicationPrivileges(t *testing.T) {
	ctx, database := billingDB(t)
	entryID := uuid.New()
	if err := database.Exec(`
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros)
		VALUES (?::uuid, ?::uuid, 'budget_change', -100)
	`, entryID.String(), uuid.NewString()).Error; err != nil {
		t.Fatalf("insert signed budget change: %v", err)
	}
	if err := database.Exec(`UPDATE billing.ledger_entry SET amount_micros = 0 WHERE id = ?::uuid`, entryID.String()).Error; err == nil {
		t.Fatal("ledger amount was mutable")
	}
	if err := database.Exec(`DELETE FROM billing.ledger_entry WHERE id = ?::uuid`, entryID.String()).Error; err == nil {
		t.Fatal("ledger entry was physically deletable")
	}
	if err := database.Exec(`UPDATE billing.ledger_entry SET is_delete = true WHERE id = ?::uuid`, entryID.String()).Error; err != nil {
		t.Fatalf("soft delete for project purge: %v", err)
	}
	if err := database.Exec(`UPDATE billing.ledger_entry SET is_delete = false WHERE id = ?::uuid`, entryID.String()).Error; err == nil {
		t.Fatal("soft-deleted ledger entry was restored")
	}

	pool, err := database.DB()
	if err != nil {
		t.Fatalf("get database pool: %v", err)
	}
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin ACL transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE lanverse_app"); err != nil {
		t.Fatalf("assume application role: %v", err)
	}
	var canInsert, canSelect, canUpdateAmount, canUpdateDelete, canDelete bool
	if err := tx.QueryRowContext(ctx, `
		SELECT has_table_privilege('billing.ledger_entry', 'INSERT'),
		       has_table_privilege('billing.ledger_entry', 'SELECT'),
		       has_column_privilege('billing.ledger_entry', 'amount_micros', 'UPDATE'),
		       has_column_privilege('billing.ledger_entry', 'is_delete', 'UPDATE'),
		       has_table_privilege('billing.ledger_entry', 'DELETE')
	`).Scan(&canInsert, &canSelect, &canUpdateAmount, &canUpdateDelete, &canDelete); err != nil {
		t.Fatalf("read ledger application privileges: %v", err)
	}
	if !canInsert || !canSelect || canUpdateAmount || !canUpdateDelete || canDelete {
		t.Fatalf("ledger privileges = insert %t select %t update_amount %t update_delete %t delete %t", canInsert, canSelect, canUpdateAmount, canUpdateDelete, canDelete)
	}
	appID := uuid.New()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO billing.ledger_entry (id, project_id, entry_type, amount_micros)
		VALUES ($1::uuid, $2::uuid, 'budget_change', 20)
	`, appID.String(), uuid.NewString()); err != nil {
		t.Fatalf("application insert ledger: %v", err)
	}
	assertLedgerPermissionDenied(t, ctx, tx, `UPDATE billing.ledger_entry SET amount_micros = 30 WHERE id = $1::uuid`, appID.String())
	assertLedgerPermissionDenied(t, ctx, tx, `DELETE FROM billing.ledger_entry WHERE id = $1::uuid`, appID.String())
}

func assertLedgerPermissionDenied(t *testing.T, ctx context.Context, tx *sql.Tx, statement string, id string) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, "SAVEPOINT ledger_acl_probe"); err != nil {
		t.Fatalf("create ACL savepoint: %v", err)
	}
	_, execErr := tx.ExecContext(ctx, statement, id)
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT ledger_acl_probe"); err != nil {
		t.Fatalf("rollback ACL attempt: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT ledger_acl_probe"); err != nil {
		t.Fatalf("release ACL savepoint: %v", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(execErr, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("statement error = %v, want permission denied", execErr)
	}
}
