package catalog_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestCredentialTestRecordsCurrentResultAndEventsOnLocalPostgres(t *testing.T) {
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
	store := pgcatalog.NewStore(conn.DB)
	service := catalogapp.NewCredentialTestService(store)
	actor := adminPrincipal()
	provider := validProvider()
	provider.Key = "credential-test-" + provider.ID.String()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Test Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "test-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	credentialID := uuid.New()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.provider_credential (id, provider_id, label, ciphertext, key_id, last4)
		VALUES (?::uuid, ?::uuid, 'primary', ?::bytea, 'agent-2026', '1234')
	`, credentialID.String(), provider.ID.String(), []byte{0, 1, 2, 3}).Error; err != nil {
		t.Fatal(err)
	}
	request := catalogapp.CredentialTestRequest{
		TestID: uuid.New(), ProviderID: provider.ID, CredentialID: credentialID,
		ActorID: actor.ID, OrgID: actor.OrgID, RequestID: uuid.NewString(),
	}
	loadedProvider, loadedCredential, err := service.Load(ctx, request)
	if err != nil || loadedProvider.ID != provider.ID || loadedCredential.ID != credentialID ||
		!bytes.Equal(loadedCredential.Ciphertext, []byte{0, 1, 2, 3}) {
		t.Fatalf("load current sealed credential: provider %s credential %s error %v", loadedProvider.ID, loadedCredential.ID, err)
	}
	first := catalogapp.CredentialTestObservation{
		CredentialTestRequest: request, Result: domain.TestOK,
		TestedAt: time.Now().UTC().Truncate(time.Microsecond), Last4: "1234",
	}
	if err := service.Record(ctx, first); err != nil {
		t.Fatalf("record credential test: %v", err)
	}
	if err := service.Record(ctx, first); err != nil {
		t.Fatalf("replay credential test: %v", err)
	}
	active, err := store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.LastTestResult != domain.TestOK || !active.LastTestedAt.Equal(first.TestedAt) {
		t.Fatalf("wrong saved result: %+v error %v", active, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox
		WHERE partition_key = ? ORDER BY topic
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("test outbox rows %d: %v", len(events), err)
	}
	var sawAudit, sawChange bool
	for _, event := range events {
		if bytes.Contains(event.Payload, []byte("ciphertext")) || bytes.Contains(event.Payload, []byte("agent-2026")) {
			t.Fatal("credential test outbox contains sealed credential material")
		}
		switch event.Topic {
		case "lanverse.audit.recorded.v1":
			audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			var compact bytes.Buffer
			if err == nil {
				err = json.Compact(&compact, audit.After)
			}
			if err != nil || audit.Action != "credential.tested" || compact.String() != `{"last4":"1234"}` {
				t.Fatalf("invalid test audit action %q after %s: %v", audit.Action, audit.After, err)
			}
			var unsafe map[string]any
			if err := json.Unmarshal(event.Payload, &unsafe); err != nil {
				t.Fatal(err)
			}
			unsafe["data"].(map[string]any)["after"] = map[string]any{"last4": "1234", "ciphertext": "forbidden"}
			unsafePayload, err := json.Marshal(unsafe)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: unsafePayload,
			}); !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatalf("test audit accepted credential material: %v", err)
			}
			sawAudit = true
		case "lanverse.catalog.credential_changed.v1":
			var payload struct {
				Data struct {
					Result string `json:"result"`
				} `json:"data"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Data.Result != "ok" {
				t.Fatalf("change event omitted test category: %s", event.Payload)
			}
			sawChange = true
		}
	}
	if !sawAudit || !sawChange {
		t.Fatalf("missing safe events: audit %t change %t", sawAudit, sawChange)
	}
	stale := first
	stale.TestID = uuid.New()
	stale.Result = domain.TestAuthFailed
	stale.TestedAt = first.TestedAt.Add(-time.Second)
	if err := service.Record(ctx, stale); !errors.Is(err, pgcatalog.ErrTestSuperseded) {
		t.Fatalf("older test replaced current result: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN RAISE EXCEPTION 'reject test audit'; END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_test_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_test_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_test_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_test_audit()`).Error
	})
	failed := first
	failed.TestID = uuid.New()
	failed.Result = domain.TestAuthFailed
	failed.TestedAt = first.TestedAt.Add(time.Second)
	if err := service.Record(ctx, failed); err == nil {
		t.Fatal("audit insertion failure did not roll back test result")
	}
	active, err = store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.LastTestResult != domain.TestOK {
		t.Fatalf("failed audit changed result: %+v error %v", active, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_test_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_test_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	newCredential := domain.Credential{
		ID: uuid.New(), ProviderID: provider.ID, Label: "replacement",
		Ciphertext: []byte{4, 5, 6}, KeyID: "agent-2026", Last4: "5678", Status: domain.CredentialActive,
	}
	if err := store.ReplaceCredential(ctx, newCredential); err != nil {
		t.Fatal(err)
	}
	if err := service.Record(ctx, failed); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("replaced credential accepted old result: %v", err)
	}
	if _, _, err := service.Load(ctx, request); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("loaded replaced credential: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	request.CredentialID = newCredential.ID
	if _, _, err := service.Load(ctx, request); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator loaded credential: %v", err)
	}
	revoked := failed
	revoked.TestID = uuid.New()
	revoked.CredentialID = newCredential.ID
	revoked.Last4 = "5678"
	if err := service.Record(ctx, revoked); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator recorded result: %v", err)
	}
}
