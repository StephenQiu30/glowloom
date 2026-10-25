package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestProfileBioAndBackgroundAcrossSessions(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	auth, first := verifiedTestActor(t, tx, "cross-device@example.invalid", now)
	login, err := auth.Login(t.Context(), "cross-device@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.SessionID = login.Session.ID
	profiles := identity.NewSelfProfile(pgidentity.NewSelfProfileStore(tx, nil), func() time.Time { return now })
	p, err := profiles.ReadPreferences(t.Context(), first)
	if err != nil || p.CanvasBackground != "dots" {
		t.Fatalf("default: %+v %v", p, err)
	}
	bio := strings.Repeat("界", 100)
	saved, err := profiles.Update(t.Context(), first, identity.ProfileInput{ExpectedRevision: p.Revision, Bio: &bio})
	if err != nil {
		t.Fatal(err)
	}
	read, err := profiles.Read(t.Context(), second)
	if err != nil || read.Bio != bio {
		t.Fatal("second session did not read biography", err)
	}
	tooLong := bio + "界"
	if _, err := profiles.Update(t.Context(), second, identity.ProfileInput{ExpectedRevision: saved.Revision, Bio: &tooLong}); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatal("oversized bio accepted", err)
	}
	grid := "lines"
	updated, err := profiles.PatchPreferences(t.Context(), second, identity.PreferencesInput{ExpectedRevision: saved.Revision, CanvasBackground: &grid})
	if err != nil {
		t.Fatal(err)
	}
	other, err := profiles.ReadPreferences(t.Context(), first)
	if err != nil || other.CanvasBackground != grid || other.Theme != p.Theme || other.PromptSuffix != p.PromptSuffix {
		t.Fatal("cross-session preferences not preserved", err)
	}
	if _, err := profiles.PatchPreferences(t.Context(), first, identity.PreferencesInput{ExpectedRevision: saved.Revision, CanvasBackground: &grid}); !errors.Is(err, domain.ErrRevisionConflict) {
		t.Fatal("stale preferences accepted", err)
	}
	invalid := "checker"
	if _, err := profiles.PatchPreferences(t.Context(), first, identity.PreferencesInput{ExpectedRevision: updated.Revision, CanvasBackground: &invalid}); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatal("invalid background accepted", err)
	}
	empty := ""
	cleared, err := profiles.Update(t.Context(), first, identity.ProfileInput{ExpectedRevision: updated.Revision, Bio: &empty})
	if err != nil || cleared.Bio != "" {
		t.Fatal("bio clear failed", err)
	}
	_, stranger := verifiedTestActor(t, tx, "other-device@example.invalid", now)
	separate, err := profiles.ReadPreferences(t.Context(), stranger)
	if err != nil || separate.CanvasBackground != "dots" {
		t.Fatal("preferences leaked across accounts", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, second.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.PatchPreferences(t.Context(), second, identity.PreferencesInput{ExpectedRevision: cleared.Revision, CanvasBackground: &grid}); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked session wrote preferences", err)
	}
}
