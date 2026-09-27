package identity_test

import (
	"context"
	"errors"
	"os"
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

type observedPasswordSessions struct {
	*redisidentity.SessionStore
	prepared string
}

func (s *observedPasswordSessions) Create(ctx context.Context, orgID, userID uuid.UUID, epoch int64) (string, domain.Session, error) {
	token, session, err := s.SessionStore.Create(ctx, orgID, userID, epoch)
	s.prepared = token
	return token, session, err
}

func TestChangePasswordInvalidatesOtherSessionsWithRealPostgresRedis(t *testing.T) {
	dsn, redisURL := os.Getenv("LV_TEST_IDENTITY_DB_DSN"), os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN and LV_TEST_IDENTITY_REDIS_URL for disposable PostgreSQL and Redis")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable identity database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open local Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	baseSessions, err := redisidentity.NewSessionStore(redisConn.Client, time.Hour, 24*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	sessions := &observedPasswordSessions{SessionStore: baseSessions}
	store := pgidentity.NewStore(conn.DB)
	orgID, userID := uuid.New(), uuid.New()
	oldHash, err := domain.HashPassword("oldPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, domain.User{
		ID: userID, OrgID: orgID, LoginName: "alice", DisplayName: "Alice",
		Role: domain.RoleProducer, PasswordHash: oldHash,
	}); err != nil {
		t.Fatalf("create test account: %v", err)
	}
	currentToken, _, err := sessions.Create(ctx, orgID, userID, 1)
	if err != nil {
		t.Fatal(err)
	}
	otherToken, _, err := sessions.Create(ctx, orgID, userID, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sessions.Destroy(context.Background(), currentToken)
		_ = sessions.Destroy(context.Background(), otherToken)
	})
	auth := identityapp.NewAuthenticator(store, sessions)
	command := identityapp.NewChangePasswordCommand(auth, store, sessions, store, time.Now)
	result, err := command.Execute(ctx, identityapp.ChangePasswordInput{
		Token: currentToken, CurrentPassword: "oldPassword123", NewPassword: "newPassword123",
		RequestID: "change-password-real", ClientIP: "127.0.0.1",
	})
	if err != nil || result.Token == "" || result.Token == currentToken || result.MustChangePassword || result.Revision != 2 {
		t.Fatalf("changed password result = %+v, error %v", result, err)
	}
	t.Cleanup(func() { _ = sessions.Destroy(context.Background(), result.Token) })
	changed, err := store.FindByID(ctx, orgID, userID)
	if err != nil || changed.Revision != 2 || changed.SessionEpoch != 2 || changed.MustChangePassword ||
		!domain.VerifyPassword(changed.PasswordHash, "newPassword123") || changed.PasswordChangedAt.IsZero() {
		t.Fatalf("changed account = %+v, error %v", changed, err)
	}
	for _, token := range []string{currentToken, otherToken} {
		if _, err := sessions.Load(ctx, token); err != nil {
			t.Fatalf("old Redis session should still exist until expiry: %v", err)
		}
		if _, err := auth.Authenticate(ctx, token); !errors.Is(err, identityapp.ErrUnauthenticated) {
			t.Fatalf("old session authenticated after password change: %v", err)
		}
	}
	if principal, err := auth.Authenticate(ctx, result.Token); err != nil ||
		principal.ID != userID || principal.MustChangePassword {
		t.Fatalf("replacement session principal = %+v, error %v", principal, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload::text AS payload FROM infra.outbox
	`).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("password change Outbox events = %d, error %v", len(events), err)
	}
	for _, event := range events {
		if event.PartitionKey != orgID.String() {
			t.Fatalf("wrong Outbox partition key: %s", event.PartitionKey)
		}
		if event.Topic == "lanverse.audit.recorded.v1" {
			record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || record.Action != "user.password_changed" {
				t.Fatalf("password change audit = %+v, error %v", record, err)
			}
		}
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_password_change_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN
		    RAISE EXCEPTION 'reject test audit row';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatalf("install audit failure function: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_password_change_test_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW
		EXECUTE FUNCTION reject_password_change_test_audit()
	`).Error; err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	if _, err := command.Execute(ctx, identityapp.ChangePasswordInput{
		Token: result.Token, CurrentPassword: "newPassword123", NewPassword: "thirdPassword123",
		RequestID: "change-password-rollback", ClientIP: "127.0.0.1",
	}); err == nil {
		t.Fatal("password change succeeded despite audit insertion failure")
	}
	if _, err := sessions.Load(ctx, sessions.prepared); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("uncommitted replacement session remained in Redis: %v", err)
	}
	unchanged, err := store.FindByID(ctx, orgID, userID)
	if err != nil || unchanged.Revision != 2 || unchanged.SessionEpoch != 2 ||
		!domain.VerifyPassword(unchanged.PasswordHash, "newPassword123") {
		t.Fatalf("account changed despite audit rollback = %+v, error %v", unchanged, err)
	}
	if _, err := auth.Authenticate(ctx, result.Token); err != nil {
		t.Fatalf("current session was lost after failed change: %v", err)
	}
}
