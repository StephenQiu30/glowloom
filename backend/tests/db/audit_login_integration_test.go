package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgaudit "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/postgres"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// This test uses a real restricted LOGIN account rather than SET ROLE on the
// migration owner's connection. The two DSNs must name one disposable,
// migrated database; all inserted records are rolled back.
func TestAuditRestrictedLoginCanAppendAndReadButCannotRewrite(t *testing.T) {
	ownerDSN := os.Getenv("LV_TEST_DB_DSN")
	loginDSN := os.Getenv("LV_TEST_AUDIT_LOGIN_DB_DSN")
	if loginDSN == "" {
		t.Skip("set LV_TEST_DB_DSN and LV_TEST_AUDIT_LOGIN_DB_DSN for one disposable migrated PostgreSQL database")
	}
	if ownerDSN == "" {
		t.Fatal("LV_TEST_DB_DSN is required with LV_TEST_AUDIT_LOGIN_DB_DSN")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	ownerConn, err := db.Open(ctx, ownerDSN, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open migration-owner connection: %v", err)
	}
	defer func() { _ = ownerConn.Close() }()
	loginConn, err := db.Open(ctx, loginDSN, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open restricted-login connection: %v", err)
	}
	defer func() { _ = loginConn.Close() }()

	type databaseIdentity struct {
		Database    string
		Role        string
		SessionRole string
		Superuser   bool
		CanLogin    bool
	}
	readIdentity := func(conn *db.Connection) databaseIdentity {
		t.Helper()
		var identity databaseIdentity
		if err := conn.DB.WithContext(ctx).Raw(`
			SELECT current_database() AS database, current_user AS role,
			       session_user AS session_role, r.rolsuper AS superuser,
			       r.rolcanlogin AS can_login
			FROM pg_roles AS r WHERE r.rolname = current_user
		`).Scan(&identity).Error; err != nil {
			t.Fatalf("read database login identity: %v", err)
		}
		return identity
	}
	owner := readIdentity(ownerConn)
	login := readIdentity(loginConn)
	if owner.Database == "" || owner.Database != login.Database ||
		owner.Database == "postgres" || owner.Database == "template0" || owner.Database == "template1" {
		t.Fatal("owner and restricted login must use the same disposable business database")
	}
	if !login.CanLogin || login.Superuser || login.Role == "" || login.Role != login.SessionRole || login.Role == owner.Role {
		t.Fatalf("application identity is not an independent restricted LOGIN: role=%q session=%q superuser=%t can_login=%t",
			login.Role, login.SessionRole, login.Superuser, login.CanLogin)
	}

	var auditOwner string
	if err := ownerConn.DB.WithContext(ctx).Raw(`
		SELECT pg_get_userbyid(relowner) FROM pg_class
		WHERE oid = 'audit.audit_log'::regclass
	`).Scan(&auditOwner).Error; err != nil {
		t.Fatalf("read audit parent owner: %v", err)
	}
	if auditOwner != owner.Role || auditOwner == login.Role {
		t.Fatalf("audit parent owner = %q, want migration owner %q distinct from application login", auditOwner, owner.Role)
	}
	var inheritedAppRole, inheritedOwnerRole bool
	if err := loginConn.DB.WithContext(ctx).Raw(`
		SELECT pg_has_role(current_user, 'lanverse_app', 'member') AS inherited_app_role,
		       pg_has_role(current_user, ?::name, 'member') AS inherited_owner_role
	`, auditOwner).Row().Scan(&inheritedAppRole, &inheritedOwnerRole); err != nil {
		t.Fatalf("check restricted-login role memberships: %v", err)
	}
	if !inheritedAppRole || inheritedOwnerRole {
		t.Fatalf("application role membership: lanverse_app=%t audit_owner=%t; want true/false",
			inheritedAppRole, inheritedOwnerRole)
	}

	var canUseSchema, canInsert, canSelect, canUpdate, canDelete, canTruncate bool
	if err := loginConn.DB.WithContext(ctx).Raw(`
		SELECT has_schema_privilege('audit', 'USAGE'),
		       has_table_privilege('audit.audit_log', 'INSERT'),
		       has_table_privilege('audit.audit_log', 'SELECT'),
		       has_table_privilege('audit.audit_log', 'UPDATE'),
		       has_table_privilege('audit.audit_log', 'DELETE'),
		       has_table_privilege('audit.audit_log', 'TRUNCATE')
	`).Row().Scan(&canUseSchema, &canInsert, &canSelect, &canUpdate, &canDelete, &canTruncate); err != nil {
		t.Fatalf("read restricted-login audit privileges: %v", err)
	}
	if !canUseSchema || !canInsert || !canSelect || canUpdate || canDelete || canTruncate {
		t.Fatalf("audit privileges: schema=%t insert=%t select=%t update=%t delete=%t truncate=%t; want only USAGE, INSERT, SELECT",
			canUseSchema, canInsert, canSelect, canUpdate, canDelete, canTruncate)
	}

	tx := loginConn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin restricted-login transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	store := pgaudit.NewStore(tx)
	record := domain.Record{
		ID: uuid.New(), OrgID: uuid.New(), ActorKind: "system",
		Action: "acl.checked", ObjectType: "audit", CreateTime: time.Now().UTC(),
	}
	record.ObjectID = record.ID.String()
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("append audit record through real application login: %v", err)
	}
	got, err := store.Get(ctx, record.OrgID, record.ID)
	if err != nil || got.ID != record.ID || got.OrgID != record.OrgID {
		t.Fatalf("read inserted audit record = %+v, error = %v", got, err)
	}
	page, err := store.List(ctx, record.OrgID, domain.Filter{Limit: 10, Action: record.Action})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != record.ID {
		t.Fatalf("list inserted audit record = %+v, error = %v", page, err)
	}
	if _, err := store.Get(ctx, uuid.New(), record.ID); !errors.Is(err, pgaudit.ErrNotFound) {
		t.Fatalf("cross-organization read = %v, want not found", err)
	}
	actorID := uuid.New()
	if err := tx.Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Audit Administrator', 'admin', 'unused-test-hash', false)
	`, actorID.String(), record.OrgID.String(), actorID.String()).Error; err != nil {
		t.Fatalf("insert administrator through restricted login: %v", err)
	}
	read := auditapp.NewReadQuery(store)
	adminPage, err := read.List(ctx, actorID, record.OrgID, domain.Filter{Limit: 10})
	if err != nil || len(adminPage.Items) != 1 || adminPage.Items[0].ID != record.ID {
		t.Fatalf("restricted-login administrator page = %+v, error = %v", adminPage, err)
	}
	if _, err := read.Get(ctx, actorID, uuid.New(), record.ID); !errors.Is(err, auditapp.ErrForbidden) {
		t.Fatalf("cross-organization administrator read = %v, want forbidden", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid`, actorID.String()).Error; err != nil {
		t.Fatalf("revoke administrator through restricted login: %v", err)
	}
	if _, err := read.List(ctx, actorID, record.OrgID, domain.Filter{Limit: 10}); !errors.Is(err, auditapp.ErrForbidden) {
		t.Fatalf("revoked administrator read = %v, want forbidden", err)
	}
	var actualPartition string
	if err := tx.Raw("SELECT tableoid::regclass::text FROM audit.audit_log WHERE id = ?::uuid", record.ID.String()).Scan(&actualPartition).Error; err != nil {
		t.Fatalf("read audit partition through parent: %v", err)
	}
	if want := "audit.audit_log_" + record.CreateTime.Format("200601"); actualPartition != want {
		t.Fatalf("audit partition = %q, want %q", actualPartition, want)
	}

	for _, check := range []struct {
		statement string
		args      []any
	}{
		{"UPDATE audit.audit_log SET after = '{}'::jsonb WHERE id = ?::uuid", []any{record.ID.String()}},
		{"DELETE FROM audit.audit_log WHERE id = ?::uuid", []any{record.ID.String()}},
		{"TRUNCATE audit.audit_log", nil},
		{"ALTER TABLE audit.audit_log ADD COLUMN audit_login_probe text", nil},
	} {
		assertRestrictedAuditMutationDenied(ctx, t, tx, check.statement, check.args...)
	}
}

func assertRestrictedAuditMutationDenied(ctx context.Context, t *testing.T, tx *gorm.DB, statement string, args ...any) {
	t.Helper()
	if err := tx.WithContext(ctx).Exec("SAVEPOINT audit_login_probe").Error; err != nil {
		t.Fatalf("create audit permission savepoint: %v", err)
	}
	execErr := tx.WithContext(ctx).Exec(statement, args...).Error
	if err := tx.WithContext(ctx).Exec("ROLLBACK TO SAVEPOINT audit_login_probe").Error; err != nil {
		t.Fatalf("rollback denied audit mutation: %v", err)
	}
	if err := tx.WithContext(ctx).Exec("RELEASE SAVEPOINT audit_login_probe").Error; err != nil {
		t.Fatalf("release audit permission savepoint: %v", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(execErr, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("%q error = %v, want permission denied SQLSTATE 42501", statement, execErr)
	}
}
