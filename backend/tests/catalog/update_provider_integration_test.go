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

type interveningUpdateProviderStore struct {
	catalogapp.UpdateProviderStore
	beforeWrite func(context.Context) error
}

func (s interveningUpdateProviderStore) UpdateProviderWithEvents(ctx context.Context, actorID, orgID uuid.UUID, before, after domain.Provider, events []identityapp.OutboxEvent) (domain.Provider, error) {
	if err := s.beforeWrite(ctx); err != nil {
		return domain.Provider{}, err
	}
	return s.UpdateProviderStore.UpdateProviderWithEvents(ctx, actorID, orgID, before, after, events)
}

func TestUpdateProviderCommitsEventsAndRejectsStaleAdminOnLocalPostgres(t *testing.T) {
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
	`, actor.ID.String(), actor.OrgID.String(), "provider-update-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "provider-update-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	credential := validCredential(provider.ID)
	if err := store.ReplaceCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}
	command := catalogapp.NewUpdateProviderCommand(store, time.Now)
	status := domain.ProviderDisabled
	input := catalogapp.UpdateProviderInput{
		ProviderID: provider.ID, ExpectedRevision: 1, Status: &status, RequestID: uuid.NewString(),
	}
	updated, err := command.Execute(ctx, actor, input)
	if err != nil || updated.Status != status || updated.Revision != 2 {
		t.Fatalf("update provider %+v: %v", updated, err)
	}
	if _, err := store.FindActiveCredential(ctx, provider.ID); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("disabled provider exposed credential: %v", err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox WHERE partition_key = ? ORDER BY create_time, id
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("provider update events %d: %v", len(events), err)
	}
	var auditCount, changedCount int
	for _, event := range events {
		switch event.Topic {
		case "lanverse.audit.recorded.v1":
			auditCount++
			audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || audit.Action != "provider.updated" || audit.ObjectID != provider.ID.String() {
				t.Fatalf("provider update audit %+v: %v", audit, err)
			}
		case "lanverse.catalog.provider_changed.v1":
			changedCount++
		default:
			t.Fatalf("unexpected provider event %q", event.Topic)
		}
	}
	if auditCount != 1 || changedCount != 1 {
		t.Fatalf("audit=%d changed=%d", auditCount, changedCount)
	}
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, domain.ErrProviderRevisionConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_provider_update_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN RAISE EXCEPTION 'reject provider update audit'; END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_provider_update_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_provider_update_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_provider_update_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_provider_update_audit()`).Error
	})
	active := domain.ProviderActive
	input = catalogapp.UpdateProviderInput{
		ProviderID: provider.ID, ExpectedRevision: 2, Status: &active, RequestID: uuid.NewString(),
	}
	if _, err := command.Execute(ctx, actor, input); err == nil {
		t.Fatal("audit failure committed provider update")
	}
	loaded, err := store.FindProvider(ctx, provider.ID)
	if err != nil || loaded.Status != domain.ProviderDisabled || loaded.Revision != 2 {
		t.Fatalf("audit failure changed provider %+v: %v", loaded, err)
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("audit failure left %d events: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_provider_update_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_provider_update_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	changedAfterRead := catalogapp.NewUpdateProviderCommand(interveningUpdateProviderStore{
		UpdateProviderStore: store,
		beforeWrite: func(ctx context.Context) error {
			return conn.DB.WithContext(ctx).Exec(`
				UPDATE catalog.provider SET rate_limit_per_min = 90, revision = revision + 1
				WHERE id = ?::uuid
			`, provider.ID.String()).Error
		},
	}, time.Now)
	if _, err := changedAfterRead.Execute(ctx, actor, input); !errors.Is(err, domain.ErrProviderRevisionConflict) {
		t.Fatalf("intervening revision accepted: %v", err)
	}
	loaded, err = store.FindProvider(ctx, provider.ID)
	if err != nil || loaded.Status != domain.ProviderDisabled || loaded.RateLimitPerMin != 90 || loaded.Revision != 3 {
		t.Fatalf("intervening revision overwritten %+v: %v", loaded, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("intervening revision wrote %d events: %v", count, err)
	}
	input.ExpectedRevision = 3
	revoked := catalogapp.NewUpdateProviderCommand(interveningUpdateProviderStore{
		UpdateProviderStore: store,
		beforeWrite: func(ctx context.Context) error {
			return conn.DB.WithContext(ctx).Exec(`
				UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
			`, actor.ID.String()).Error
		},
	}, time.Now)
	if _, err := revoked.Execute(ctx, actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator updated provider: %v", err)
	}
	loaded, err = store.FindProvider(ctx, provider.ID)
	if err != nil || loaded.Status != domain.ProviderDisabled || loaded.Revision != 3 {
		t.Fatalf("revocation changed provider %+v: %v", loaded, err)
	}
}
