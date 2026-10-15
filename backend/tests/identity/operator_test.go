package identity_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestMigrationOperatorUsesConfiguredKeysAndExistingDatabase(t *testing.T) {
	tx := existingIdentityTransaction(t)
	cfg := config.Config{IdentityKeyFile: filepath.Join(t.TempDir(), "identity.key"), IdentityMailSourceHourlyLimit: 100, IdentityMailGlobalHourlyLimit: 1000}
	if _, err := identityruntime.NewAuth(tx, cfg, time.Now); err == nil {
		t.Fatal("operator generated unconfigured keys")
	}
	if err := credentialproof.CreateFile(cfg.IdentityKeyFile); err != nil {
		t.Fatal(err)
	}
	auth, err := identityruntime.NewAuth(tx, cfg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	id, org := uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'operator fixture')`, org).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash) VALUES(?,?,?,'fixture','producer','fixture-hash')`, id, org, id.String()).Error; err != nil {
		t.Fatal(err)
	}
	op, err := auth.StartMigrationBinding(t.Context(), id.String(), "operator@example.invalid", false)
	if err != nil || op.OperationID == uuid.Nil {
		t.Fatalf("configured migration rejected: %v", err)
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM identity.mail_delivery m JOIN identity.auth_challenge c ON c.id=m.challenge_id WHERE c.operation_id=? AND m.template='migration_bind'`, op.OperationID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("migration not bound to mail operation")
	}
}

func TestOperatorMailCannotSendAnotherFlow(t *testing.T) {
	tx := existingIdentityTransaction(t)
	keyPath := filepath.Join(t.TempDir(), "identity.key")
	if err := credentialproof.CreateFile(keyPath); err != nil {
		t.Fatal(err)
	}
	auth, err := identityruntime.NewAuth(tx, config.Config{IdentityKeyFile: keyPath, IdentityTermsVersion: "test", IdentityMailSourceHourlyLimit: 100, IdentityMailGlobalHourlyLimit: 1000}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = verifiedTestActor(t, tx, "first@example.invalid", time.Now())
	_, _ = verifiedTestActor(t, tx, "second@example.invalid", time.Now())
	first, err := auth.StartPasswordReset(t.Context(), "first@example.invalid", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.StartPasswordReset(t.Context(), "second@example.invalid", ""); err != nil {
		t.Fatal(err)
	}
	jobs, err := pgidentity.NewOperationMailStore(tx, first.OperationID).Claim(t.Context(), 10, time.Now())
	if err != nil || len(jobs) != 1 || jobs[0].Recipient != "first@example.invalid" {
		t.Fatal("operator crossed mail operation boundary")
	}
}
