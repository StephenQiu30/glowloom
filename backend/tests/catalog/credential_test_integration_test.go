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

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
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
	store := app.NewCatalogStore(conn.DB)
	service := catalogapp.NewCredentialTestService(store)
	actor := adminPrincipal()
	provider := validProvider()
	provider.Key = "credential-test-" + provider.ID.String()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password, platform_role)
		VALUES (?::uuid, ?::uuid, ?, 'Test Admin', 'admin', 'test-hash', false, 'platform_admin')
	`, actor.ID.String(), actor.OrgID.String(), "test-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}

	actor = identityfixture.Session(t, conn.DB, actor)
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
	requestTest := func(credential uuid.UUID) catalogapp.CredentialTestRequest {
		t.Helper()
		key := uuid.NewString()
		accepted, err := catalogapp.NewRequestCredentialTestCommand(store).Execute(ctx, actor, provider.ID, credential, key)
		if err != nil || !accepted.Accepted {
			t.Fatal("credential test admission", accepted, err)
		}
		return catalogapp.CredentialTestRequest{TestID: accepted.TestID, ProviderID: provider.ID, CredentialID: credential, ActorID: actor.ID, OrgID: actor.OrgID, RequestID: key}
	}
	request := requestTest(credentialID)
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
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 3 {
		t.Fatalf("test outbox rows %d: %v", len(events), err)
	}
	var sawAudit, sawChange, sawRequest bool
	for _, event := range events {
		if bytes.Contains(event.Payload, []byte("ciphertext")) || bytes.Contains(event.Payload, []byte("agent-2026")) {
			t.Fatal("credential test outbox contains sealed credential material")
		}
		switch event.Topic {
		case "lanverse.catalog.credential_test_requested.v1":
			var payload struct {
				Data struct {
					TestID       uuid.UUID `json:"test_id"`
					ProviderID   uuid.UUID `json:"provider_id"`
					CredentialID uuid.UUID `json:"credential_id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Data.TestID != request.TestID || payload.Data.ProviderID != provider.ID || payload.Data.CredentialID != credentialID {
				t.Fatal("invalid accepted test request", err)
			}
			sawRequest = true
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
	if !sawAudit || !sawChange || !sawRequest {
		t.Fatalf("missing safe events: audit %t change %t", sawAudit, sawChange)
	}
	stale := first
	stale.CredentialTestRequest = requestTest(credentialID)
	stale.Result = domain.TestAuthFailed
	stale.TestedAt = first.TestedAt.Add(-time.Second)
	if err := service.Record(ctx, stale); !errors.Is(err, pgcatalog.ErrTestSuperseded) {
		t.Fatalf("older test replaced current result: %v", err)
	}
	failed := first
	failed.CredentialTestRequest = requestTest(credentialID)
	failed.Result = domain.TestAuthFailed
	failed.TestedAt = first.TestedAt.Add(time.Second)
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
	revokedRequest := requestTest(newCredential.ID)
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET platform_role = 'creator' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	request = revokedRequest
	if _, _, err := service.Load(ctx, request); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator loaded credential: %v", err)
	}
	revoked := failed
	revoked.CredentialTestRequest = revokedRequest
	revoked.Last4 = "5678"
	if err := service.Record(ctx, revoked); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator recorded result: %v", err)
	}
}
