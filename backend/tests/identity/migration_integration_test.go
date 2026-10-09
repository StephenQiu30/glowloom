package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestIdentityUserLoginNameScopeAndDefaults(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to a disposable PostgreSQL database with identity migration applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	orgA, orgB := uuid.NewString(), uuid.NewString()
	insert := func(orgID, loginName string) error {
		return conn.DB.WithContext(ctx).Exec(`
			INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash)
			VALUES (?::uuid, ?::uuid, ?, 'Alice', 'producer', 'test-hash')
		`, uuid.NewString(), orgID, loginName).Error
	}
	if err := insert(orgA, "Alice"); err != nil {
		t.Fatalf("insert first account: %v", err)
	}
	var state struct {
		Status             string
		MustChangePassword bool
		SessionEpoch       int64
		Revision           int64
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT status, must_change_password, session_epoch, revision
		FROM identity."user" WHERE org_id = ?::uuid AND login_name = 'alice'
	`, orgA).Scan(&state).Error; err != nil {
		t.Fatalf("read account defaults: %v", err)
	}
	if state.Status != "active" || !state.MustChangePassword || state.SessionEpoch != 1 || state.Revision != 1 {
		t.Fatalf("account defaults = %+v", state)
	}
	var pgErr *pgconn.PgError
	if err := insert(orgA, "ALICE"); !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("case-insensitive duplicate = %v, want unique violation", err)
	}
	if err := insert(orgB, "ALICE"); !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_user_login_global" {
		t.Fatalf("cross-organization duplicate = %v, want global unique violation", err)
	}
}

func TestIdentityStoreCreatesAndScopesAccounts(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to a disposable PostgreSQL database with identity migration applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, otherOrgID := uuid.New(), uuid.New()
	accountID := uuid.New()
	login := "Alice-" + accountID.String()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatalf("hash initial password: %v", err)
	}
	account := domain.User{
		ID: accountID, OrgID: orgID, LoginName: login, DisplayName: "Alice",
		Role: domain.RoleProducer, PasswordHash: hash,
	}
	unsafe := account
	unsafe.ID = uuid.New()
	unsafe.PasswordHash = "initialPassword123"
	if err := store.Create(ctx, unsafe); !errors.Is(err, pgidentity.ErrInvalidUser) {
		t.Fatalf("plaintext password passed to repository = %v, want ErrInvalidUser", err)
	}
	if err := store.Create(ctx, account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	got, err := store.FindByLogin(ctx, orgID, "alice-"+accountID.String())
	if err != nil {
		t.Fatalf("find case-insensitive account: %v", err)
	}
	if got.ID != accountID || got.OrgID != orgID || got.Status != domain.StatusActive ||
		!got.MustChangePassword || got.SessionEpoch != 1 || got.Revision != 1 ||
		!domain.VerifyPassword(got.PasswordHash, "initialPassword123") {
		t.Fatal("created account did not round-trip with secure defaults")
	}
	if _, err := store.FindByLogin(ctx, otherOrgID, login); !errors.Is(err, pgidentity.ErrNotFound) {
		t.Fatalf("cross-organization lookup = %v, want ErrNotFound", err)
	}
	if _, err := store.FindByID(ctx, otherOrgID, accountID); !errors.Is(err, pgidentity.ErrNotFound) {
		t.Fatalf("cross-organization ID lookup = %v, want ErrNotFound", err)
	}
	duplicate := account
	duplicate.ID = uuid.New()
	duplicate.LoginName = "ALICE-" + accountID.String()
	if err := store.Create(ctx, duplicate); !errors.Is(err, pgidentity.ErrLoginExists) {
		t.Fatalf("duplicate login = %v, want ErrLoginExists", err)
	}
	account.ID = uuid.New()
	account.OrgID = otherOrgID
	if err := store.Create(ctx, account); !errors.Is(err, pgidentity.ErrLoginExists) {
		t.Fatalf("cross-organization duplicate = %v, want ErrLoginExists", err)
	}
}

func TestIdentityStoreRejectsStaleAccountRevision(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to a disposable PostgreSQL database with identity migration applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, userID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatalf("hash initial password: %v", err)
	}
	if err := store.Create(ctx, domain.User{
		ID: userID, OrgID: orgID, LoginName: userID.String(), DisplayName: "Alice",
		Role: domain.RoleAdmin, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	first, err := store.FindByID(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("load account: %v", err)
	}
	if first.Revision != 1 {
		t.Fatal("new identity must retain its initial revision")
	}
}

func TestIdentityStoreProtectsLastActiveAdmin(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to a disposable PostgreSQL database with identity migration applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, firstID, secondID := uuid.New(), uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatalf("hash initial password: %v", err)
	}
	createAdmin := func(id uuid.UUID) {
		t.Helper()
		if err := store.Create(ctx, domain.User{
			ID: id, OrgID: orgID, LoginName: id.String(), DisplayName: "Admin",
			Role: domain.RoleAdmin, PasswordHash: hash,
		}); err != nil {
			t.Fatalf("create admin: %v", err)
		}
	}
	createAdmin(firstID)
	if err := store.Disable(ctx, orgID, firstID, 1); !errors.Is(err, domain.ErrLastActiveAdmin) {
		t.Fatalf("disable only admin = %v, want ErrLastActiveAdmin", err)
	}
	createAdmin(secondID)
	if err := store.Disable(ctx, orgID, firstID, 1); err != nil {
		t.Fatalf("disable with another admin: %v", err)
	}
	if err := store.Disable(ctx, orgID, secondID, 1); !errors.Is(err, domain.ErrLastActiveAdmin) {
		t.Fatalf("disable remaining admin = %v, want ErrLastActiveAdmin", err)
	}
	disabled, err := store.FindByID(ctx, orgID, firstID)
	if err != nil || disabled.Status != domain.StatusDisabled || disabled.SessionEpoch != 2 || disabled.Revision != 2 {
		t.Fatalf("disabled account = status %s, epoch %d, revision %d, error %v", disabled.Status, disabled.SessionEpoch, disabled.Revision, err)
	}
	concurrentOrg := uuid.New()
	concurrentIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range concurrentIDs {
		if err := store.Create(ctx, domain.User{
			ID: id, OrgID: concurrentOrg, LoginName: id.String(), DisplayName: "Admin",
			Role: domain.RoleAdmin, PasswordHash: hash,
		}); err != nil {
			t.Fatalf("create concurrent admin: %v", err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, len(concurrentIDs))
	for _, id := range concurrentIDs {
		go func() {
			<-start
			results <- store.Disable(ctx, concurrentOrg, id, 1)
		}()
	}
	close(start)
	var disabledCount, protectedCount int
	for range concurrentIDs {
		switch err := <-results; {
		case err == nil:
			disabledCount++
		case errors.Is(err, domain.ErrLastActiveAdmin):
			protectedCount++
		default:
			t.Fatalf("concurrent disable: %v", err)
		}
	}
	if disabledCount != 1 || protectedCount != 1 {
		t.Fatalf("concurrent disable = %d successful, %d protected", disabledCount, protectedCount)
	}
}
