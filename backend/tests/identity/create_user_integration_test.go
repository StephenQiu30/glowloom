package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestCreateUserCommitsAccountAndTwoOutboxEventsAtomically(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to a disposable PostgreSQL database with identity and Outbox migrations")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable identity database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, actorID := uuid.New(), uuid.New()
	adminHash, err := domain.HashPassword("adminPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, domain.User{
		ID: actorID, OrgID: orgID, LoginName: "admin", DisplayName: "Admin",
		Role: domain.RoleAdmin, PasswordHash: adminHash,
	}); err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = false WHERE id = ?::uuid
	`, actorID.String()).Error; err != nil {
		t.Fatalf("clear administrator first-login flag: %v", err)
	}
	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin}
	command := identityapp.NewCreateUserCommand(store, time.Now)
	result, err := command.Execute(ctx, actor, identityapp.CreateUserInput{
		LoginName: "alice", DisplayName: "Alice", Role: domain.RoleProducer,
		InitialPassword: "initialPassword123", RequestID: "account-create-integration",
	})
	if err != nil {
		t.Fatalf("create account and events: %v", err)
	}
	created, err := store.FindByID(ctx, orgID, result.ID)
	if err != nil || created.LoginName != "alice" || created.Revision != 1 ||
		!created.MustChangePassword || !domain.VerifyPassword(created.PasswordHash, "initialPassword123") {
		t.Fatalf("persisted account = %+v, error %v", created, err)
	}
	var rows []struct {
		Topic        string
		PartitionKey string
		Payload      string
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload::text AS payload
		FROM infra.outbox
	`).Scan(&rows).Error; err != nil {
		t.Fatalf("read account outbox rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("account outbox rows = %d, want two", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.PartitionKey != orgID.String() ||
			(row.Topic != "lanverse.identity.user_changed.v1" && row.Topic != "lanverse.audit.recorded.v1") ||
			seen[row.Topic] {
			t.Fatalf("unexpected outbox routing: %+v", row)
		}
		seen[row.Topic] = true
	}

	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_identity_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN
		    RAISE EXCEPTION 'reject test audit row';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatalf("install test failure function: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_identity_test_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW
		EXECUTE FUNCTION reject_identity_test_audit()
	`).Error; err != nil {
		t.Fatalf("install test failure trigger: %v", err)
	}
	if _, err := command.Execute(ctx, actor, identityapp.CreateUserInput{
		LoginName: "rollback", DisplayName: "Rollback", Role: domain.RoleProducer,
		InitialPassword: "initialPassword123", RequestID: "account-rollback-integration",
	}); err == nil {
		t.Fatal("account creation unexpectedly succeeded when audit Outbox rejected")
	}
	if _, err := store.FindByLogin(ctx, orgID, "rollback"); !errors.Is(err, pgidentity.ErrNotFound) {
		t.Fatalf("account remained after event failure: %v", err)
	}
	var outboxCount int64
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.outbox").Scan(&outboxCount).Error; err != nil || outboxCount != 2 {
		t.Fatalf("outbox rows after rollback = %d, error %v", outboxCount, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actorID.String()).Error; err != nil {
		t.Fatalf("downgrade administrator for stale-principal check: %v", err)
	}
	if _, err := command.Execute(ctx, actor, identityapp.CreateUserInput{
		LoginName: "forbidden", DisplayName: "Forbidden", Role: domain.RoleProducer,
		InitialPassword: "initialPassword123", RequestID: "account-stale-admin-integration",
	}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("stale administrator create = %v, want forbidden", err)
	}
}
