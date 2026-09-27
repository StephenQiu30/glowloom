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
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestSetCredentialCommitsSealedDataAndEventsOnLocalPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to an isolated database with catalog, identity, and Outbox migrations")
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
	hash, err := identitydomain.HashPassword("temporary-admin-password1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := pgidentity.NewStore(conn.DB).Create(ctx, identitydomain.User{
		ID: actor.ID, OrgID: actor.OrgID, LoginName: "admin", DisplayName: "Admin",
		Role: identitydomain.RoleAdmin, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("create test administrator: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = false WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	provider := validProvider()
	provider.Key = "credential-command-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	command := newCredentialCommand(t, store)
	input := catalogapp.SetCredentialInput{
		ProviderID: provider.ID, Label: "primary",
		Secret:    json.RawMessage(`{"api_key":"local-test-value","group_id":"test-group"}`),
		RequestID: uuid.NewString(),
	}
	first, err := command.Execute(ctx, actor, input)
	if err != nil {
		t.Fatalf("first credential: %v", err)
	}
	if first.Last4 != "alue" || first.ID == uuid.Nil {
		t.Fatalf("unsafe first summary: %+v", first)
	}
	input.RequestID = uuid.NewString()
	second, err := command.Execute(ctx, actor, input)
	if err != nil {
		t.Fatalf("replace credential: %v", err)
	}
	active, err := store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.ID != second.ID || active.KeyID != "agent-2026" ||
		bytes.Contains(active.Ciphertext, []byte("local-test-value")) {
		t.Fatalf("unsafe active credential: id %s error %v", active.ID, err)
	}
	var oldStatus string
	if err := conn.DB.WithContext(ctx).Raw(`SELECT status FROM catalog.provider_credential WHERE id = ?::uuid`, first.ID.String()).Scan(&oldStatus).Error; err != nil || oldStatus != "disabled" {
		t.Fatalf("old credential status %q: %v", oldStatus, err)
	}
	var rows []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox
		WHERE partition_key = ? ORDER BY create_time, id
	`, actor.OrgID.String()).Scan(&rows).Error; err != nil || len(rows) != 4 {
		t.Fatalf("outbox rows %d: %v", len(rows), err)
	}
	var auditCount int
	for _, row := range rows {
		if row.PartitionKey != actor.OrgID.String() || bytes.Contains(row.Payload, []byte("local-test-value")) ||
			bytes.Contains(row.Payload, []byte("test-group")) || bytes.Contains(row.Payload, []byte("ciphertext")) {
			t.Fatal("outbox leaked plaintext or changed organization partition")
		}
		if row.Topic == "lanverse.audit.recorded.v1" {
			audit, err := auditapp.NewRecordedActionParser().Parse(inbox.Record{
				Topic: row.Topic, Key: []byte(row.PartitionKey), Value: row.Payload,
			})
			if err != nil || audit.Action != "credential.set" || string(audit.After) != `{"last4": "alue"}` && string(audit.After) != `{"last4":"alue"}` {
				t.Fatalf("invalid saved audit: action %q after %s error %v", audit.Action, audit.After, err)
			}
			auditCount++
		}
	}
	if auditCount != 2 {
		t.Fatalf("audit event count %d, want 2", auditCount)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_credential_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN
		    RAISE EXCEPTION 'reject test audit row';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_credential_audit_before_insert
		BEFORE INSERT ON infra.outbox FOR EACH ROW
		EXECUTE FUNCTION reject_credential_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_credential_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_credential_audit()`).Error
	})
	input.RequestID = uuid.NewString()
	if _, err := command.Execute(ctx, actor, input); err == nil {
		t.Fatal("credential replacement survived audit Outbox failure")
	}
	active, err = store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.ID != second.ID {
		t.Fatalf("failed audit replaced active credential: id %s error %v", active.ID, err)
	}
	var credentialCount, eventCount int64
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.provider_credential WHERE provider_id = ?::uuid`, provider.ID.String()).Scan(&credentialCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&eventCount).Error; err != nil || credentialCount != 2 || eventCount != 4 {
		t.Fatalf("rollback left credential=%d events=%d error=%v", credentialCount, eventCount, err)
	}
	revocation := conn.DB.WithContext(ctx).Begin()
	if revocation.Error != nil {
		t.Fatal(revocation.Error)
	}
	t.Cleanup(func() { _ = revocation.Rollback().Error })
	if err := revocation.Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() {
		_, err := command.Execute(ctx, actor, input)
		blocked <- err
	}()
	select {
	case err := <-blocked:
		t.Fatalf("credential read bypassed in-flight admin revocation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := revocation.Commit().Error; err != nil {
		t.Fatalf("commit administrator revocation: %v", err)
	}
	select {
	case err := <-blocked:
		if !errors.Is(err, identityapp.ErrForbidden) {
			t.Fatalf("in-flight revocation did not block credential save: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("credential command did not finish after revocation")
	}
	if _, err := command.Execute(ctx, actor, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("stale administrator saved a credential: %v", err)
	}
	active, err = store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.ID != second.ID || active.Status != domain.CredentialActive {
		t.Fatalf("stale administrator changed credential: id %s error %v", active.ID, err)
	}
}
