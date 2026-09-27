package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	redisidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/redis"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestAdminLifecycleRevokesSessionsAndRollsBackAuditFailureWithRealStores(t *testing.T) {
	dsn, redisURL := os.Getenv("LV_TEST_ACCOUNT_LIFECYCLE_DB_DSN"), os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("set LV_TEST_ACCOUNT_LIFECYCLE_DB_DSN and LV_TEST_IDENTITY_REDIS_URL to disposable local services")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open local Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	sessions, err := redisidentity.NewSessionStore(redisConn.Client, time.Hour, 24*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewStore(conn.DB)
	orgID, adminID, memberID := uuid.New(), uuid.New(), uuid.New()
	oldHash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []domain.User{
		{ID: adminID, OrgID: orgID, LoginName: "admin", DisplayName: "Admin", Role: domain.RoleAdmin, PasswordHash: oldHash},
		{ID: memberID, OrgID: orgID, LoginName: "member", DisplayName: "Member", Role: domain.RoleProducer, PasswordHash: oldHash},
	} {
		if err := store.Create(ctx, user); err != nil {
			t.Fatalf("create disposable account: %v", err)
		}
	}
	if err := conn.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET must_change_password = false WHERE org_id = ?::uuid`, orgID.String()).Error; err != nil {
		t.Fatalf("prepare accounts: %v", err)
	}
	oldToken, _, err := sessions.Create(ctx, orgID, memberID, 1)
	if err != nil {
		t.Fatalf("create original session: %v", err)
	}
	t.Cleanup(func() { _ = sessions.Destroy(context.Background(), oldToken) })
	auth := identityapp.NewAuthenticator(store, sessions)
	actor := identityapp.Principal{ID: adminID, OrgID: orgID, Role: domain.RoleAdmin}
	disable := identityapp.NewDisableUserCommand(store, time.Now)
	if _, err := disable.Execute(ctx, actor, identityapp.DisableUserInput{
		TargetID: memberID, ExpectedRevision: 1, RequestID: "disable-member",
	}); err != nil {
		t.Fatalf("disable member: %v", err)
	}
	if _, err := auth.Authenticate(ctx, oldToken); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("original session after disable: %v", err)
	}
	enable := identityapp.NewEnableUserCommand(store, time.Now)
	installAccountAuditRejection(ctx, t, conn.DB)
	if _, err := enable.Execute(ctx, actor, identityapp.EnableUserInput{
		TargetID: memberID, ExpectedRevision: 2, RequestID: "enable-fail",
	}); err == nil {
		t.Fatal("enable succeeded despite audit insertion failure")
	}
	assertAccountState(ctx, t, store, orgID, memberID, domain.StatusDisabled, 2, 2, oldHash, false)
	removeAccountAuditRejection(ctx, t, conn.DB)
	if _, err := enable.Execute(ctx, actor, identityapp.EnableUserInput{
		TargetID: memberID, ExpectedRevision: 2, RequestID: "enable-member",
	}); err != nil {
		t.Fatalf("enable member: %v", err)
	}
	assertAccountState(ctx, t, store, orgID, memberID, domain.StatusActive, 3, 3, oldHash, false)
	if _, err := auth.Authenticate(ctx, oldToken); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("disabled-era session revived after enable: %v", err)
	}
	currentToken, _, err := sessions.Create(ctx, orgID, memberID, 3)
	if err != nil {
		t.Fatalf("create current session: %v", err)
	}
	t.Cleanup(func() { _ = sessions.Destroy(context.Background(), currentToken) })
	if _, err := auth.Authenticate(ctx, currentToken); err != nil {
		t.Fatalf("authenticate enabled member: %v", err)
	}
	reset := identityapp.NewResetPasswordCommand(store, time.Now)
	installAccountAuditRejection(ctx, t, conn.DB)
	if _, err := reset.Execute(ctx, actor, identityapp.ResetPasswordInput{
		TargetID: memberID, ExpectedRevision: 3,
		NewPassword: "replacementPassword456", RequestID: "reset-fail",
	}); err == nil {
		t.Fatal("reset succeeded despite audit insertion failure")
	}
	assertAccountState(ctx, t, store, orgID, memberID, domain.StatusActive, 3, 3, oldHash, false)
	removeAccountAuditRejection(ctx, t, conn.DB)
	if _, err := reset.Execute(ctx, actor, identityapp.ResetPasswordInput{
		TargetID: memberID, ExpectedRevision: 3,
		NewPassword: "replacementPassword456", RequestID: "reset-member",
	}); err != nil {
		t.Fatalf("reset member password: %v", err)
	}
	member, err := store.FindByID(ctx, orgID, memberID)
	if err != nil || member.SessionEpoch != 4 || member.Revision != 4 || !member.MustChangePassword ||
		!domain.VerifyPassword(member.PasswordHash, "replacementPassword456") ||
		domain.VerifyPassword(member.PasswordHash, "initialPassword123") {
		t.Fatalf("reset member state: epoch %d, revision %d, flag %t, error %v",
			member.SessionEpoch, member.Revision, member.MustChangePassword, err)
	}
	if _, err := auth.Authenticate(ctx, currentToken); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("pre-reset session after reset: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET status = 'disabled' WHERE org_id = ?::uuid AND id = ?::uuid
	`, orgID.String(), adminID.String()).Error; err != nil {
		t.Fatalf("revoke administrator in disposable database: %v", err)
	}
	if _, err := reset.Execute(ctx, actor, identityapp.ResetPasswordInput{
		TargetID: memberID, ExpectedRevision: 4,
		NewPassword: "anotherPassword789", RequestID: "stale-admin-reset",
	}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("stale administrator password reset: %v", err)
	}
	member, err = store.FindByID(ctx, orgID, memberID)
	if err != nil || member.Revision != 4 || member.SessionEpoch != 4 ||
		!domain.VerifyPassword(member.PasswordHash, "replacementPassword456") {
		t.Fatalf("member changed after stale admin reset: revision %d, epoch %d, error %v",
			member.Revision, member.SessionEpoch, err)
	}
	var count int64
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, orgID.String()).Scan(&count).Error; err != nil || count != 6 {
		t.Fatalf("committed event count %d, error %v", count, err)
	}
}

func installAccountAuditRejection(ctx context.Context, t *testing.T, conn *gorm.DB) {
	t.Helper()
	if err := conn.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_account_lifecycle_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN
		    RAISE EXCEPTION 'reject test audit row';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatalf("install account audit rejection function: %v", err)
	}
	if err := conn.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_account_lifecycle_test_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW
		EXECUTE FUNCTION reject_account_lifecycle_test_audit()
	`).Error; err != nil {
		t.Fatalf("install account audit rejection trigger: %v", err)
	}
}

func removeAccountAuditRejection(ctx context.Context, t *testing.T, conn *gorm.DB) {
	t.Helper()
	if err := conn.WithContext(ctx).Exec(`DROP TRIGGER reject_account_lifecycle_test_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatalf("remove account audit rejection trigger: %v", err)
	}
	if err := conn.WithContext(ctx).Exec(`DROP FUNCTION reject_account_lifecycle_test_audit()`).Error; err != nil {
		t.Fatalf("remove account audit rejection function: %v", err)
	}
}

func assertAccountState(ctx context.Context, t *testing.T, store *pgidentity.Store, orgID, memberID uuid.UUID,
	status domain.Status, epoch, revision int64, hash string, mustChange bool) {
	t.Helper()
	member, err := store.FindByID(ctx, orgID, memberID)
	if err != nil || member.Status != status || member.SessionEpoch != epoch || member.Revision != revision ||
		member.PasswordHash != hash || member.MustChangePassword != mustChange {
		t.Fatalf("account state status %s, epoch %d, revision %d, flag %t, error %v",
			member.Status, member.SessionEpoch, member.Revision, member.MustChangePassword, err)
	}
}
