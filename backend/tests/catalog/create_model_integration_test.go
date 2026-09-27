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
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestCreateModelCommitsAuditAndRechecksAdminOnLocalPostgres(t *testing.T) {
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
		VALUES (?::uuid, ?::uuid, ?, 'Model Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "model-admin-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "model-admin-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	capability := domain.Capability{
		ID: uuid.New(), Key: "video.generate." + uuid.NewString(),
		OutputType: domain.OutputVideo, Modes: []string{"image2video"},
	}
	if err := store.CreateCapabilityForAdmin(ctx, actor.ID, actor.OrgID, capability); err != nil {
		t.Fatal(err)
	}
	command := catalogapp.NewCreateModelCommand(store, time.Now)
	input := catalogapp.CreateModelInput{
		Key: "ark.seedance-" + uuid.NewString(), ProviderID: provider.ID,
		Capability: capability.Key, DisplayName: "Seedance", RequestID: uuid.NewString(),
	}
	created, err := command.Execute(ctx, actor, input)
	if err != nil || created.ID == uuid.Nil || created.Status != domain.ModelDisabled || created.Revision != 1 {
		t.Fatalf("create model %+v: %v", created, err)
	}
	loaded, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, created.ID)
	if err != nil || loaded.Key != input.Key || loaded.ProviderID != provider.ID || loaded.Capability != capability.Key {
		t.Fatalf("persisted model %+v: %v", loaded, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 1 {
		t.Fatalf("model audit events %d: %v", len(events), err)
	}
	audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
		Topic: events[0].Topic, Key: []byte(events[0].PartitionKey), Value: events[0].Payload,
	})
	if err != nil || audit.Action != "model.created" || audit.ObjectID != created.ID.String() {
		t.Fatalf("model audit %+v: %v", audit, err)
	}
	input.RequestID = uuid.NewString()
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, pgcatalog.ErrModelKeyExists) {
		t.Fatalf("duplicate model key accepted: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE catalog.provider SET is_delete = true WHERE id = ?::uuid
	`, provider.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	input.Key = "ark.seedance-" + uuid.NewString()
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, pgcatalog.ErrModelSourceUnavailable) {
		t.Fatalf("deleted provider registered a model: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE catalog.provider SET is_delete = false WHERE id = ?::uuid
	`, provider.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_model_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN RAISE EXCEPTION 'reject model audit'; END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_model_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_model_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_model_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_model_audit()`).Error
	})
	input.Key = "ark.seedance-" + uuid.NewString()
	if _, err := command.Execute(ctx, actor, input); err == nil {
		t.Fatal("audit failure committed model")
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM catalog.model_profile WHERE model_key = ?
	`, input.Key).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("audit failure left %d model rows: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_model_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_model_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator registered model: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM catalog.model_profile WHERE model_key = ?
	`, input.Key).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("revoked administrator left %d model rows: %v", count, err)
	}
}
