package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	redisidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/redis"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestLoginUsesRealPostgresRedisAndAtomicAudit(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	redisURL := os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to a disposable database with identity and Outbox migrations, and LV_TEST_IDENTITY_REDIS_URL to disposable Redis")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	sessions, err := redisidentity.NewSessionStore(redisConn.Client, time.Hour, 24*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := redisidentity.NewLoginLimiter(redisConn.Client)
	if err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewStore(conn.DB)
	orgID, userID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, domain.User{
		ID: userID, OrgID: orgID, LoginName: "alice", DisplayName: "Alice",
		Role: domain.RoleProducer, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("create test account: %v", err)
	}
	unique := uuid.New()
	ip := fmt.Sprintf("127.%d.%d.%d", unique[0], unique[1], unique[2])
	digest := sha256.Sum256([]byte(ip))
	t.Cleanup(func() {
		_ = redisConn.Client.Del(context.Background(), "login_fail_ip:"+hex.EncodeToString(digest[:])).Err()
	})
	command := identityapp.NewLoginCommand(store, sessions, limiter, time.Now)
	input := identityapp.LoginInput{
		OrgID: orgID, LoginName: "alice", Password: "initialPassword123",
		ClientIP: ip, RequestID: "real-login-success",
	}
	result, err := command.Execute(ctx, input)
	if err != nil || result.Token == "" || result.User.ID != userID || !result.User.MustChangePassword {
		t.Fatalf("login result identity = %s, token present %t, error %v", result.User.ID, result.Token != "", err)
	}
	t.Cleanup(func() { _ = sessions.Destroy(context.Background(), result.Token) })
	principal, err := identityapp.NewAuthenticator(store, sessions).Authenticate(ctx, result.Token)
	if err != nil || principal.ID != userID {
		t.Fatalf("authenticate issued session = %s, error %v", principal.ID, err)
	}
	logout := identityapp.NewLogoutCommand(identityapp.NewAuthenticator(store, sessions), sessions, store, time.Now)
	loggedOut, err := logout.Execute(ctx, identityapp.LogoutInput{
		Token: result.Token, RequestID: "real-logout-success", ClientIP: ip,
	})
	if err != nil || !loggedOut.Revoked {
		t.Fatalf("logout result = %+v, error %v", loggedOut, err)
	}
	if _, err := identityapp.NewAuthenticator(store, sessions).Authenticate(ctx, result.Token); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("revoked session authentication = %v, want unauthenticated", err)
	}
	current, err := store.FindByID(ctx, orgID, userID)
	if err != nil || current.Revision != 2 || current.LastLoginAt.IsZero() {
		t.Fatalf("saved successful login revision = %d, last login set %t, error %v", current.Revision, !current.LastLoginAt.IsZero(), err)
	}
	input.Password = "wrongPassword123"
	for attempt := 1; attempt <= 5; attempt++ {
		input.RequestID = fmt.Sprintf("real-login-failure-%d", attempt)
		if _, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrInvalidCredentials) {
			t.Fatalf("wrong password attempt %d = %v", attempt, err)
		}
	}
	current, err = store.FindByID(ctx, orgID, userID)
	if err != nil || current.FailedLoginCount != 5 || !current.LockedUntil.After(time.Now()) {
		t.Fatalf("locked account count = %d, locked until set %t, error %v", current.FailedLoginCount, current.LockedUntil.After(time.Now()), err)
	}
	input.Password = "initialPassword123"
	input.RequestID = "real-login-locked"
	locked, err := command.Execute(ctx, input)
	if !errors.Is(err, domain.ErrAccountLocked) || locked.RetryAfter <= 0 || locked.Token != "" {
		t.Fatalf("locked login retry = %v, token present %t, error %v", locked.RetryAfter, locked.Token != "", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid
	`, userID.String()).Error; err != nil {
		t.Fatalf("disable test account: %v", err)
	}
	input.RequestID = "real-login-disabled"
	if _, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrInvalidCredentials) {
		t.Fatalf("disabled account error = %v", err)
	}
	input.LoginName = "missing"
	input.RequestID = "real-login-missing"
	if _, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrInvalidCredentials) {
		t.Fatalf("missing account error = %v", err)
	}
	var auditRows []struct {
		Topic        string
		PartitionKey string
		Payload      string
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload::text AS payload FROM infra.outbox
	`).Scan(&auditRows).Error; err != nil {
		t.Fatalf("read login Outbox: %v", err)
	}
	if len(auditRows) != 11 {
		t.Fatalf("login/logout audit event rows = %d, want 11", len(auditRows))
	}
	for _, row := range auditRows {
		if row.Topic != "lanverse.audit.recorded.v1" || row.PartitionKey != orgID.String() ||
			strings.Contains(row.Payload, input.Password) || strings.Contains(row.Payload, hash) {
			t.Fatal("login audit exposed a credential or used wrong routing")
		}
		if _, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
			Topic: row.Topic, Key: []byte(row.PartitionKey), Value: []byte(row.Payload),
		}); err != nil {
			t.Fatalf("persisted login audit rejected by consumer policy: %v", err)
		}
	}
	var disabledReason, missingReason int64
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox
		WHERE payload->'data'->'after'->>'reason' = 'account_disabled'
	`).Scan(&disabledReason).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox
		WHERE payload->'data'->'after'->>'reason' = 'login_not_found'
	`).Scan(&missingReason).Error; err != nil {
		t.Fatal(err)
	}
	if disabledReason != 1 || missingReason != 1 {
		t.Fatalf("internal audit reasons disabled/missing = %d/%d", disabledReason, missingReason)
	}

	otherID := uuid.New()
	if err := store.Create(ctx, domain.User{
		ID: otherID, OrgID: orgID, LoginName: "bob", DisplayName: "Bob",
		Role: domain.RoleProducer, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("create rollback account: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_login_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN
		    RAISE EXCEPTION 'reject login audit test row';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatalf("install test failure function: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_login_test_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION reject_login_test_audit()
	`).Error; err != nil {
		t.Fatalf("install test failure trigger: %v", err)
	}
	input.LoginName = "bob"
	input.RequestID = "real-login-rollback"
	if failed, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrLoginUnavailable) || failed.Token != "" {
		t.Fatalf("failed audit login token present %t, error %v", failed.Token != "", err)
	}
	other, err := store.FindByID(ctx, orgID, otherID)
	if err != nil || other.Revision != 1 || !other.LastLoginAt.IsZero() {
		t.Fatalf("rollback account revision = %d, last login set %t, error %v", other.Revision, !other.LastLoginAt.IsZero(), err)
	}
	bobToken, _, err := sessions.Create(ctx, orgID, otherID, 1)
	if err != nil {
		t.Fatalf("create session for audit failure: %v", err)
	}
	failedLogout, err := logout.Execute(ctx, identityapp.LogoutInput{
		Token: bobToken, RequestID: "real-logout-audit-failure", ClientIP: ip,
	})
	if !errors.Is(err, identityapp.ErrLogoutUnavailable) || !failedLogout.Revoked {
		t.Fatalf("logout audit failure result = %+v, error %v", failedLogout, err)
	}
	if _, err := identityapp.NewAuthenticator(store, sessions).Authenticate(ctx, bobToken); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("session after audit failure = %v, want unauthenticated", err)
	}
}
