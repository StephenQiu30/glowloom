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

type revokeAfterCredentialReadStore struct {
	store  *pgcatalog.Store
	revoke func(context.Context) error
}

func (s *revokeAfterCredentialReadStore) FindCredentialForAdmin(ctx context.Context, actorID, orgID, providerID, credentialID uuid.UUID) (catalogapp.CredentialToDisable, error) {
	target, err := s.store.FindCredentialForAdmin(ctx, actorID, orgID, providerID, credentialID)
	if err != nil {
		return catalogapp.CredentialToDisable{}, err
	}
	if err := s.revoke(ctx); err != nil {
		return catalogapp.CredentialToDisable{}, err
	}
	return target, nil
}

func (s *revokeAfterCredentialReadStore) DisableCredentialWithEvents(ctx context.Context, actorID, orgID uuid.UUID, target catalogapp.CredentialToDisable, events []identityapp.OutboxEvent) (catalogapp.SavedCredential, error) {
	return s.store.DisableCredentialWithEvents(ctx, actorID, orgID, target, events)
}

func TestDisableCredentialCommitsSafeEventsAndRollsBackOnAuditFailure(t *testing.T) {
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
	actor := adminPrincipal()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Test Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "disable-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	provider := validProvider()
	provider.Key = "credential-disable-" + provider.ID.String()
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
	// A disabled provider must not prevent revocation of its still-active key.
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE catalog.provider SET status = 'disabled' WHERE id = ?::uuid
	`, provider.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	command := catalogapp.NewDisableCredentialCommand(store, time.Now)
	input := catalogapp.DisableCredentialInput{
		ProviderID: provider.ID, CredentialID: credentialID, RequestID: uuid.NewString(),
	}
	summary, err := command.Execute(ctx, actor, input)
	if err != nil || summary.ID != credentialID || summary.Status != domain.CredentialDisabled || summary.Last4 != "1234" {
		t.Fatalf("disable inactive provider credential: %+v error %v", summary, err)
	}
	if _, err := store.FindActiveCredential(ctx, provider.ID); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("disabled key remained available: %v", err)
	}
	var status string
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT status FROM catalog.provider_credential WHERE id = ?::uuid
	`, credentialID.String()).Scan(&status).Error; err != nil || status != "disabled" {
		t.Fatalf("credential status %q: %v", status, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("disable event count %d: %v", len(events), err)
	}
	var sawChange, sawAudit bool
	for _, event := range events {
		if event.PartitionKey != actor.OrgID.String() || bytes.Contains(event.Payload, []byte("ciphertext")) ||
			bytes.Contains(event.Payload, []byte("agent-2026")) {
			t.Fatal("disable event disclosed key material or changed partition")
		}
		switch event.Topic {
		case "lanverse.catalog.credential_changed.v1":
			var payload struct {
				Data struct {
					Change string `json:"change"`
				} `json:"data"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Data.Change != "disabled" {
				t.Fatalf("wrong change event: %v %+v", err, payload)
			}
			sawChange = true
		case "lanverse.audit.recorded.v1":
			audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
			})
			if err != nil || audit.Action != "credential.disabled" {
				t.Fatalf("wrong disable audit: %+v error %v", audit, err)
			}
			var after struct {
				Last4 string `json:"last4"`
			}
			if err := json.Unmarshal(audit.After, &after); err != nil || after.Last4 != "1234" {
				t.Fatalf("unsafe disable audit after %s: %v", audit.After, err)
			}
			sawAudit = true
		}
	}
	if !sawChange || !sawAudit {
		t.Fatalf("missing disable event: change %t audit %t", sawChange, sawAudit)
	}
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("disabled credential accepted a second command: %v", err)
	}
	secondProvider := validProvider()
	secondProvider.Key = "credential-disable-" + secondProvider.ID.String()
	if err := store.CreateProvider(ctx, secondProvider); err != nil {
		t.Fatal(err)
	}
	secondCredentialID := uuid.New()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.provider_credential (id, provider_id, label, ciphertext, key_id, last4)
		VALUES (?::uuid, ?::uuid, 'fallback', ?::bytea, 'agent-2026', '5678')
	`, secondCredentialID.String(), secondProvider.ID.String(), []byte{4, 5, 6}).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_disable_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN RAISE EXCEPTION 'reject disable audit'; END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_disable_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_disable_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_disable_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_disable_audit()`).Error
	})
	secondInput := catalogapp.DisableCredentialInput{
		ProviderID: secondProvider.ID, CredentialID: secondCredentialID, RequestID: uuid.NewString(),
	}
	if _, err := command.Execute(ctx, actor, secondInput); err == nil {
		t.Fatal("audit failure allowed credential disable")
	}
	active, err := store.FindActiveCredential(ctx, secondProvider.ID)
	if err != nil || active.ID != secondCredentialID {
		t.Fatalf("audit failure changed active credential: %s error %v", active.ID, err)
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("audit failure left %d events: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_disable_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_disable_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	lateCommand := catalogapp.NewDisableCredentialCommand(&revokeAfterCredentialReadStore{
		store: store,
		revoke: func(ctx context.Context) error {
			return conn.DB.WithContext(ctx).Exec(`
				UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
			`, actor.ID.String()).Error
		},
	}, time.Now)
	if _, err := lateCommand.Execute(ctx, actor, secondInput); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator disabled key: %v", err)
	}
	active, err = store.FindActiveCredential(ctx, secondProvider.ID)
	if err != nil || active.ID != secondCredentialID {
		t.Fatalf("late revocation changed active credential: %s error %v", active.ID, err)
	}
}
