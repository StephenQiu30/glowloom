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
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestDisableUserCommitsStateAndEventsWithRealPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN to disposable local services")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, adminAID, adminBID, producerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct {
		id   uuid.UUID
		name string
		role domain.Role
	}{
		{id: adminAID, name: "admin-a", role: domain.RoleAdmin},
		{id: adminBID, name: "admin-b", role: domain.RoleAdmin},
		{id: producerID, name: "producer", role: domain.RoleProducer},
	} {
		if err := store.Create(ctx, domain.User{
			ID: account.id, OrgID: orgID, LoginName: account.name, DisplayName: account.name,
			Role: account.role, PasswordHash: hash,
		}); err != nil {
			t.Fatalf("create %s: %v", account.name, err)
		}
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = false
		WHERE org_id = ?::uuid AND role = 'admin'
	`, orgID.String()).Error; err != nil {
		t.Fatalf("prepare administrators: %v", err)
	}
	actor := identityapp.Principal{ID: adminAID, OrgID: orgID, Role: domain.RoleAdmin}
	command := identityapp.NewDisableUserCommand(store, time.Now)
	result, err := command.Execute(ctx, actor, identityapp.DisableUserInput{
		TargetID: producerID, ExpectedRevision: 1, RequestID: "disable-producer",
	})
	if err != nil || result.ID != producerID || result.Status != domain.StatusDisabled || result.Revision != 2 {
		t.Fatalf("disable producer result = %+v, error %v", result, err)
	}
	current, err := store.FindByID(ctx, orgID, producerID)
	if err != nil || current.SessionEpoch != 2 || current.Revision != 2 || current.Status != domain.StatusDisabled {
		t.Fatalf("disabled producer state = %+v, error %v", current, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      string
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload::text AS payload
		FROM infra.outbox WHERE partition_key = ? ORDER BY create_time, id
	`, orgID.String()).Scan(&events).Error; err != nil {
		t.Fatalf("read account events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("account event rows = %d, want 2", len(events))
	}
	var auditSeen, identitySeen bool
	for _, event := range events {
		switch event.Topic {
		case "lanverse.audit.recorded.v1":
			auditSeen = true
			record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: []byte(event.Payload),
			})
			if err != nil || record.Action != "user.disabled" || record.ObjectID != producerID.String() {
				t.Fatalf("account audit = %+v, error %v", record, err)
			}
		case "lanverse.identity.user_changed.v1":
			identitySeen = true
		default:
			t.Fatalf("unexpected account event topic %s", event.Topic)
		}
	}
	if !auditSeen || !identitySeen {
		t.Fatal("disabled account did not write both event topics")
	}

	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_disable_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
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
		CREATE TRIGGER reject_disable_test_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW
		EXECUTE FUNCTION reject_disable_test_audit()
	`).Error; err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	adminBInput := identityapp.DisableUserInput{
		TargetID: adminBID, ExpectedRevision: 1, RequestID: "disable-admin-b-rollback",
	}
	if _, err := command.Execute(ctx, actor, adminBInput); err == nil {
		t.Fatal("account disable succeeded while audit insertion failed")
	}
	adminB, err := store.FindByID(ctx, orgID, adminBID)
	if err != nil || adminB.Status != domain.StatusActive || adminB.SessionEpoch != 1 || adminB.Revision != 1 {
		t.Fatalf("admin B after rollback = %+v, error %v", adminB, err)
	}
	var eventCount int64
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.outbox WHERE partition_key = ?", orgID.String()).Scan(&eventCount).Error; err != nil || eventCount != 2 {
		t.Fatalf("events after rollback = %d, error %v", eventCount, err)
	}
	if err := conn.DB.WithContext(ctx).Exec("DROP TRIGGER reject_disable_test_audit_before_insert ON infra.outbox").Error; err != nil {
		t.Fatalf("remove audit failure trigger: %v", err)
	}
	adminBInput.RequestID = "disable-admin-b"
	if _, err := command.Execute(ctx, actor, adminBInput); err != nil {
		t.Fatalf("disable second administrator: %v", err)
	}
	if _, err := command.Execute(ctx, actor, identityapp.DisableUserInput{
		TargetID: adminAID, ExpectedRevision: 1, RequestID: "disable-last-admin",
	}); !errors.Is(err, domain.ErrLastActiveAdmin) {
		t.Fatalf("disable last administrator = %v, want last-active-admin error", err)
	}
	staleActor := identityapp.Principal{ID: adminBID, OrgID: orgID, Role: domain.RoleAdmin}
	if _, err := command.Execute(ctx, staleActor, identityapp.DisableUserInput{
		TargetID: adminAID, ExpectedRevision: 1, RequestID: "disabled-admin-attempt",
	}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("disabled administrator reused stale principal: %v", err)
	}
}
