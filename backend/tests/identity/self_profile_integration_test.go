package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func verifiedTestActor(t *testing.T, tx *gorm.DB, email string, now time.Time) (*identity.Auth, identity.Principal) {
	t.Helper()
	protector, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewStore(tx)
	auth := identity.NewEmailAuth(store, protector, identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, func() time.Time { return now })
	hash, err := domain.HashPassword("long password for tests", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EmailTransaction(t.Context(), func(t identity.EmailTx) error {
		return t.CreatePersonalAccount(domain.User{ID: uuid.New(), OrgID: uuid.New(), LoginName: uuid.NewString(), Email: email, EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "创作者", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	login, err := auth.Login(t.Context(), email, "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	return auth, identity.Principal{ID: login.User.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformRole: login.User.PlatformRole, PlatformEpoch: login.User.PlatformEpoch}
}

func TestSelfProfileCAS(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	_, actor := verifiedTestActor(t, tx, "profile@example.invalid", now)
	profiles := identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, nil), func() time.Time { return now })
	initial, err := profiles.Read(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	name := "创作者甲"
	saved, err := profiles.Update(t.Context(), actor, identity.ProfileInput{ExpectedRevision: initial.Revision, DisplayName: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Update(t.Context(), actor, identity.ProfileInput{ExpectedRevision: initial.Revision, DisplayName: &name}); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("stale profile silently overwritten")
	}
	loaded, err := profiles.Read(t.Context(), actor)
	if err != nil || loaded.DisplayName != name || loaded.Revision != saved.Revision {
		t.Fatal("profile did not persist")
	}
	revision, err := profiles.UpdatePreferences(t.Context(), actor, saved.Revision, "dark", "画面清晰")
	if err != nil || revision != saved.Revision+1 {
		t.Fatal("preferences not sharing profile revision")
	}
	if _, err := profiles.UpdatePreferences(t.Context(), actor, revision, "dark", strings.Repeat("界", 2001)); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatal("oversized suffix accepted")
	}
	avatar := uuid.New()
	if _, err := profiles.Update(t.Context(), actor, identity.ProfileInput{ExpectedRevision: revision, AvatarID: &avatar}); !errors.Is(err, identity.ErrAvatarUnavailable) {
		t.Fatal("unvalidated avatar accepted")
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.Read(t.Context(), actor); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked session read private profile")
	}
}
