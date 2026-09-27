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

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestBootstrapAdminIsAtomicAndSingleUseWithRealPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_BOOTSTRAP_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_BOOTSTRAP_DB_DSN to a fresh disposable PostgreSQL database with Outbox, identity, and organization migrations")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable bootstrap database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	command := identityapp.NewBootstrapAdminCommand(store, time.Now)
	input := identityapp.BootstrapAdminInput{LoginName: "first-admin", InitialPassword: "initialPassword123"}
	first, err := command.Execute(ctx, input)
	if err != nil || first.OrgID == uuid.Nil || first.ID == uuid.Nil ||
		first.Role != domain.RoleAdmin || first.Status != domain.StatusActive || !first.MustChangePassword {
		t.Fatalf("first bootstrap result = %+v, error %v", first, err)
	}
	account, err := store.FindByID(ctx, first.OrgID, first.ID)
	if err != nil || !domain.VerifyPassword(account.PasswordHash, input.InitialPassword) ||
		account.Revision != 1 || account.SessionEpoch != 1 {
		t.Fatalf("first bootstrap account state invalid: revision %d, epoch %d, error %v", account.Revision, account.SessionEpoch, err)
	}
	var organizations []struct {
		ID     uuid.UUID
		Name   string
		Status string
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT id, name, status FROM workspace.organization`).Scan(&organizations).Error; err != nil ||
		len(organizations) != 1 || organizations[0].ID != first.OrgID || organizations[0].Name != "Lanverse" ||
		organizations[0].Status != "active" {
		t.Fatalf("bootstrap organization count %d, error %v", len(organizations), err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT topic, partition_key, payload::text AS payload FROM infra.outbox`).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("bootstrap Outbox event count %d, error %v", len(events), err)
	}
	for _, event := range events {
		if event.PartitionKey != first.OrgID.String() {
			t.Fatalf("bootstrap event partition key %q", event.PartitionKey)
		}
		if event.Topic == "lanverse.audit.recorded.v1" {
			record, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || record.Action != "user.created" || record.ActorKind != "system" || record.ActorID != nil {
				t.Fatalf("bootstrap audit = %+v, error %v", record, err)
			}
		}
	}
	if _, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrAlreadyBootstrapped) {
		t.Fatalf("second bootstrap = %v, want already bootstrapped", err)
	}
	truncateBootstrapData(ctx, t, conn.DB)
	existingOrgID := uuid.New()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO workspace.organization (id, name) VALUES (?::uuid, 'Seeded Team')
	`, existingOrgID.String()).Error; err != nil {
		t.Fatalf("create existing organization: %v", err)
	}
	reused, err := command.Execute(ctx, identityapp.BootstrapAdminInput{
		LoginName: "seeded-admin", OrganizationName: "Ignored for existing organization",
		InitialPassword: "initialPassword123",
	})
	if err != nil || reused.OrgID != existingOrgID {
		t.Fatalf("existing organization bootstrap = %+v, error %v", reused, err)
	}
	truncateBootstrapData(ctx, t, conn.DB)
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_bootstrap_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
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
		CREATE TRIGGER reject_bootstrap_test_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW
		EXECUTE FUNCTION reject_bootstrap_test_audit()
	`).Error; err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	if _, err := command.Execute(ctx, input); err == nil {
		t.Fatal("bootstrap succeeded despite audit failure")
	}
	for _, table := range []string{`workspace.organization`, `identity."user"`, `infra.outbox`} {
		var count int64
		if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM " + table).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("rollback table %s count %d, error %v", table, count, err)
		}
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_bootstrap_test_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatalf("remove audit failure trigger: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_bootstrap_test_audit()`).Error; err != nil {
		t.Fatalf("remove audit failure function: %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := command.Execute(ctx, input)
			results <- err
		}()
	}
	var succeeded, rejected int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, identityapp.ErrAlreadyBootstrapped):
			rejected++
		default:
			t.Fatalf("concurrent bootstrap error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent bootstrap succeeded %d, rejected %d", succeeded, rejected)
	}
	truncateBootstrapData(ctx, t, conn.DB)
	for range 2 {
		if err := conn.DB.WithContext(ctx).Exec(`
			INSERT INTO workspace.organization (id, name) VALUES (?::uuid, 'Unexpected team')
		`, uuid.NewString()).Error; err != nil {
			t.Fatalf("create conflicting organization: %v", err)
		}
	}
	if _, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrBootstrapOrganizationConflict) {
		t.Fatalf("multi-organization bootstrap = %v, want conflict", err)
	}
	truncateBootstrapData(ctx, t, conn.DB)
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO workspace.organization (id, name, status)
		VALUES (?::uuid, 'Disabled team', 'disabled')
	`, uuid.NewString()).Error; err != nil {
		t.Fatalf("create disabled organization: %v", err)
	}
	if _, err := command.Execute(ctx, input); !errors.Is(err, identityapp.ErrBootstrapOrganizationConflict) {
		t.Fatalf("disabled-organization bootstrap = %v, want conflict", err)
	}
}

func truncateBootstrapData(ctx context.Context, t *testing.T, conn *gorm.DB) {
	t.Helper()
	if err := conn.WithContext(ctx).Exec(`TRUNCATE identity."user", workspace.organization, infra.outbox`).Error; err != nil {
		t.Fatalf("reset disposable bootstrap database: %v", err)
	}
}
