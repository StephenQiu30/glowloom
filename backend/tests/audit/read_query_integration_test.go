package audit_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgaudit "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/postgres"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAuditReadRechecksCurrentAdminWithRealPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUDIT_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_AUDIT_DB_DSN to a disposable database with audit and identity migrations")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	orgID, otherOrg, actorID, outsiderID, projectID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, user := range []struct{ id, org uuid.UUID }{{actorID, orgID}, {outsiderID, otherOrg}} {
		if err := tx.Exec(`
			INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
			VALUES (?::uuid, ?::uuid, ?, 'Administrator', 'admin', 'unused-test-hash', false)
		`, user.id.String(), user.org.String(), user.id.String()).Error; err != nil {
			t.Fatalf("insert administrator: %v", err)
		}
	}
	firstID, secondID, foreignID := uuid.New(), uuid.New(), uuid.New()
	for _, record := range []struct {
		id, org uuid.UUID
		project *uuid.UUID
		action  string
	}{
		{firstID, orgID, &projectID, "user.updated"},
		{secondID, orgID, nil, "auth.logout"},
		{foreignID, otherOrg, &projectID, "user.updated"},
	} {
		var project any
		if record.project != nil {
			project = record.project.String()
		}
		if err := tx.Exec(`
			INSERT INTO audit.audit_log (id, org_id, project_id, actor_kind, action, object_type, object_id)
			VALUES (?::uuid, ?::uuid, ?::uuid, 'user', ?, 'user', ?)
		`, record.id.String(), record.org.String(), project, record.action, record.id.String()).Error; err != nil {
			t.Fatalf("insert audit row: %v", err)
		}
	}
	query := auditapp.NewReadQuery(pgaudit.NewStore(tx))
	assertDenied := func() {
		t.Helper()
		if _, err := query.List(ctx, actorID, orgID, domain.Filter{}); !errors.Is(err, auditapp.ErrForbidden) {
			t.Fatalf("revoked administrator list: %v", err)
		}
		if _, err := query.Get(ctx, actorID, orgID, firstID); !errors.Is(err, auditapp.ErrForbidden) {
			t.Fatalf("revoked administrator detail: %v", err)
		}
	}
	page, err := query.List(ctx, actorID, orgID, domain.Filter{Limit: 1, ProjectID: &projectID})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != firstID || page.Next != nil {
		t.Fatalf("scoped project page = %+v, %v", page, err)
	}
	page, err = query.List(ctx, actorID, orgID, domain.Filter{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Next == nil {
		t.Fatalf("first keyset page = %+v, %v", page, err)
	}
	page, err = query.List(ctx, actorID, orgID, domain.Filter{Limit: 1, Before: page.Next})
	if err != nil || len(page.Items) != 1 || page.Next != nil {
		t.Fatalf("second keyset page = %+v, %v", page, err)
	}
	if _, err := query.Get(ctx, actorID, orgID, foreignID); !errors.Is(err, pgaudit.ErrNotFound) {
		t.Fatalf("cross-organization detail: %v", err)
	}
	if _, err := query.List(ctx, outsiderID, orgID, domain.Filter{}); !errors.Is(err, auditapp.ErrForbidden) {
		t.Fatalf("cross-organization administrator: %v", err)
	}

	updateActor(t, tx, actorID, "role = 'producer'")
	assertDenied()
	updateActor(t, tx, actorID, "role = 'admin', must_change_password = true")
	assertDenied()
	updateActor(t, tx, actorID, "must_change_password = false, status = 'disabled'")
	assertDenied()
	updateActor(t, tx, actorID, "status = 'active', is_delete = true")
	assertDenied()
}

func TestAuditReadHoldsAdminLockAcrossReadTransaction(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUDIT_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_AUDIT_DB_DSN to a disposable database with audit and identity migrations")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	actorID, orgID := uuid.New(), uuid.New()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Administrator', 'admin', 'unused-test-hash', false)
	`, actorID.String(), orgID.String(), actorID.String()).Error; err != nil {
		t.Fatalf("insert administrator: %v", err)
	}
	defer func() {
		if err := conn.DB.Exec(`DELETE FROM identity."user" WHERE id = ?::uuid`, actorID.String()).Error; err != nil {
			t.Errorf("remove administrator fixture: %v", err)
		}
	}()
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	query := auditapp.NewReadQuery(pgaudit.NewStore(tx))
	if _, err := query.List(ctx, actorID, orgID, domain.Filter{}); err != nil {
		t.Fatalf("read with current administrator: %v", err)
	}

	updateCtx, updateCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer updateCancel()
	updateDone := make(chan error, 1)
	go func() {
		updateDone <- conn.DB.WithContext(updateCtx).Exec(`
			UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
		`, actorID.String()).Error
	}()
	if err := <-updateDone; err == nil {
		t.Fatal("revocation committed while audit read transaction held the administrator lock")
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit read transaction: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actorID.String()).Error; err != nil {
		t.Fatalf("revoke administrator after read: %v", err)
	}
	if _, err := auditapp.NewReadQuery(pgaudit.NewStore(conn.DB)).List(ctx, actorID, orgID, domain.Filter{}); !errors.Is(err, auditapp.ErrForbidden) {
		t.Fatalf("read after revocation: %v", err)
	}
}

func updateActor(t *testing.T, tx *gorm.DB, actorID uuid.UUID, update string) {
	t.Helper()
	if err := tx.Exec(`UPDATE identity."user" SET `+update+` WHERE id = ?::uuid`, actorID.String()).Error; err != nil {
		t.Fatalf("change administrator status: %v", err)
	}
}
