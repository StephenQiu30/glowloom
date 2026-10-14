package identity_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestMigrationBindingRequiresProofAndKeepsID(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	id, org := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'migration fixture')`, org).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash) VALUES(?,?,?,'fixture','producer','fixture-hash')`, id, org, id.String()).Error; err != nil {
		t.Fatal(err)
	}
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	var existingAdministrator bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM identity."user" WHERE platform_role='platform_admin' AND status='active' AND NOT is_delete AND email_verified_at IS NOT NULL)`).Scan(&existingAdministrator).Error; err != nil {
		t.Fatal(err)
	}
	grantAdmin := !existingAdministrator
	if existingAdministrator {
		if _, err := auth.StartMigrationBinding(t.Context(), id.String(), "migration@example.invalid", true); !errors.Is(err, identity.ErrAlreadyBootstrapped) {
			t.Fatal("operator replaced existing platform administration")
		}
	}
	op, err := auth.StartMigrationBinding(t.Context(), id.String(), "migration@example.invalid", grantAdmin)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		EventID          uuid.UUID
		KeyID            string
		EncryptedPayload []byte
	}
	if err := tx.Raw(`SELECT event_id,key_id,encrypted_payload FROM identity.mail_delivery WHERE recipient='migration@example.invalid'`).Scan(&m).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := protector.Open(m.EventID, m.KeyID, m.EncryptedPayload)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct{ Code string }
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CompleteMigrationBinding(t.Context(), op.OperationID, "bad", grantAdmin); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("unverified migration accepted")
	}
	if _, err := auth.CompleteMigrationBinding(t.Context(), op.OperationID, payload.Code, !grantAdmin); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("migration authority was not bound")
	}
	saved, err := auth.CompleteMigrationBinding(t.Context(), op.OperationID, payload.Code, grantAdmin)
	if err != nil || saved != id {
		t.Fatalf("stable migration identity lost: %v", err)
	}
	var row struct {
		EmailKey, PlatformRole string
		SessionEpoch           int64
	}
	if err := tx.Raw(`SELECT email_key,platform_role,session_epoch FROM identity."user" WHERE id=?`, id).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	expectedRole := "creator"
	if grantAdmin {
		expectedRole = "platform_admin"
	}
	if row.EmailKey != "migration@example.invalid" || row.PlatformRole != expectedRole || row.SessionEpoch != 2 {
		t.Fatal("verified binding missing or old sessions preserved")
	}
	if _, err := auth.CompleteMigrationBinding(t.Context(), op.OperationID, payload.Code, grantAdmin); !errors.Is(err, identity.ErrProofInvalid) {
		t.Fatal("migration proof replayed")
	}
}

func TestRevokedProofMailIsNotSent(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := identity.NewEmailAuth(pgidentity.NewStore(tx), protector, identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	op, err := auth.StartRegistration(t.Context(), "stale-mail@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := auth.ResendRegistration(t.Context(), op.OperationID, op.FlowSecret); err != nil {
		t.Fatal(err)
	}
	jobs, err := pgidentity.NewMailStore(tx).Claim(t.Context(), 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("claimed %d messages; revoked proof must not be sent", len(jobs))
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM identity.mail_delivery WHERE status='expired' AND encrypted_payload IS NULL AND key_id IS NULL`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("stale mail secrets not cleared")
	}
}
