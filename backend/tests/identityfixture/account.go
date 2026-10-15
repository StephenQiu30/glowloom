// Package identityfixture supplies verified email sessions to optional integration tests.
package identityfixture

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// Configure creates a temporary test key shared by the router and session issuer.
func Configure(t *testing.T, cfg config.Config) config.Config {
	t.Helper()
	cfg.IdentityKeyFile = filepath.Join(t.TempDir(), "identity-test.key")
	if err := credentialproof.CreateFile(cfg.IdentityKeyFile); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Create seeds a synthetic verified personal account through the current store.
func Create(t *testing.T, db *gorm.DB, cfg config.Config, label, password string) identity.AuthResult {
	t.Helper()
	now := time.Now().UTC()
	hash, err := domain.HashPassword(password, "")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	email := id.String() + "@fixture.invalid"
	if err := pgidentity.NewStore(db).EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
		return tx.CreatePersonalAccount(domain.User{ID: id, OrgID: uuid.New(), LoginName: id.String(), Email: email, EmailVersion: 1, EmailVerifiedAt: now, DisplayName: label, PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "fixture", TermsAcceptedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	return Login(t, db, cfg, id, password)
}

// Login verifies an existing test account using the current email/session contract.
func Login(t *testing.T, db *gorm.DB, cfg config.Config, id uuid.UUID, password string) identity.AuthResult {
	t.Helper()
	auth, err := identityruntime.NewAuth(db, cfg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := db.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, id).Scan(&email).Error; err != nil {
		t.Fatal(err)
	}
	result, err := auth.Login(t.Context(), email, password, false)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// Scope reads the account's explicit personal scope for a test request.
func Scope(t *testing.T, db *gorm.DB, user uuid.UUID) uuid.UUID {
	t.Helper()
	var row struct{ ID uuid.UUID }
	if err := db.Raw(`SELECT id FROM workspace.creation_scope WHERE owner_user_id=? AND kind='personal'`, user).Scan(&row).Error; err != nil || row.ID == uuid.Nil {
		t.Fatal("personal scope unavailable", err)
	}
	return row.ID
}
