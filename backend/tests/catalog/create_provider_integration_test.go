package catalog_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestCreateProviderCommitsAuditAndRejectsStaleAdminOnLocalPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to an isolated migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	actor := adminPrincipal()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Test Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "provider-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	command := catalogapp.NewCreateProviderCommand(store, credentialschema.NewRegistry(), time.Now)
	key := "openrouter_" + uuid.NewString()
	input := catalogapp.CreateProviderInput{
		Key: key, Name: "OpenRouter", AdapterKey: "openrouter", Region: domain.RegionOverseas,
		ConcurrencyLimit: 10, RateLimitPerMin: 60, RequestID: uuid.NewString(),
	}
	created, err := command.Execute(ctx, actor, input)
	if err != nil || created.ID == uuid.Nil || created.Key != key || created.Revision != 1 || created.Status != domain.ProviderActive {
		t.Fatalf("create provider %+v: %v", created, err)
	}
	loaded, err := store.FindProvider(ctx, created.ID)
	if err != nil || loaded.ID != created.ID || loaded.AdapterKey != "openrouter" {
		t.Fatalf("load created provider %+v: %v", loaded, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 1 {
		t.Fatalf("created provider outbox %d: %v", len(events), err)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: events[0].Topic, Key: []byte(events[0].PartitionKey), Value: events[0].Payload,
	})
	if err != nil || audit.Action != "provider.created" || audit.ObjectID != created.ID.String() {
		t.Fatalf("created provider audit %+v: %v", audit, err)
	}
	input.RequestID = uuid.NewString()
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, pgcatalog.ErrProviderKeyExists) {
		t.Fatalf("duplicate provider key accepted: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_provider_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN RAISE EXCEPTION 'reject provider audit'; END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_provider_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_provider_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_provider_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_provider_audit()`).Error
	})
	input.Key = "openrouter_" + uuid.NewString()
	if _, err := command.Execute(ctx, actor, input); err == nil {
		t.Fatal("audit failure committed provider")
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.provider WHERE key = ?`, input.Key).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("audit failure left %d provider rows: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit failure left %d events: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_provider_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_provider_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("stale administrator registered provider: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.provider WHERE key = ?`, input.Key).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("stale administrator left %d provider rows: %v", count, err)
	}
}
