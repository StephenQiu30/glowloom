package identityregistration_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestSourceAccountPreferencesRetainExactSourceAndLocalTransaction(t *testing.T) {
	for _, kind := range []writefence.Kind{writefence.Account, writefence.Session} {
		t.Run(string(kind), func(t *testing.T) {
			f, login := passwordFixture(t)
			current, err := f.auth.Current(t.Context(), login.Credential.LegacyToken)
			if err != nil {
				t.Fatal(err)
			}
			actor := identity.Principal{ID: current.User.ID, SessionID: current.Session.ID, CredentialEpoch: current.User.SessionEpoch}
			service := identity.NewSelfProfile(pgidentity.NewSourceSelfProfileStore(f.idb), func() time.Time { return f.clock })
			before, err := service.ReadPreferences(t.Context(), actor)
			if err != nil {
				t.Fatal(err)
			}
			theme := "dark"
			input := identity.PreferencesInput{ExpectedRevision: before.Revision, Theme: &theme}
			after, err := service.PatchPreferences(t.Context(), actor, input)
			if err != nil || after.Theme != theme || after.Revision != before.Revision+1 {
				t.Fatalf("account preference mutation: %v", err)
			}
			if _, err := service.PatchPreferences(t.Context(), actor, input); err == nil {
				t.Fatal("stale revision overwrote preferences")
			}
			f.controlLoss.kind = kind
			f.controlLoss.lose.Store(true)
			id := actor.ID
			if kind == writefence.Session {
				id = actor.SessionID
			}
			if _, err := f.writers.Set(t.Context(), writefence.Authority{Issuer: writefence.Identity, Kind: kind, ID: id, Epoch: 1}, uuid.New(), true); err == nil {
				t.Fatal("source closure loss not exercised")
			}
			if _, err := service.ReadPreferences(t.Context(), actor); err == nil {
				t.Fatal("closed source exposed an unready account profile")
			}
			input.ExpectedRevision = after.Revision
			if _, err := service.PatchPreferences(t.Context(), actor, input); err == nil {
				t.Fatal("preference write crossed a durable source closure")
			}
			if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='preferences.updated'`, actor.ID) != 1 {
				t.Fatal("closed/stale profile write produced an event")
			}
			outer := f.idb.Begin()
			if outer.Error != nil {
				t.Fatal(outer.Error)
			}
			defer func() { _ = outer.Rollback().Error }()
			inside := identity.NewSelfProfile(pgidentity.NewSourceSelfProfileStore(outer), func() time.Time { return f.clock })
			if _, err := inside.ReadPreferences(t.Context(), actor); !errors.Is(err, writefence.ErrTransaction) {
				t.Fatalf("native profile did not reject caller-owned SQL transaction: %v", err)
			}
			wrong := identity.NewSelfProfile(pgidentity.NewSourceSelfProfileStore(f.cdb), func() time.Time { return f.clock })
			if _, err := wrong.ReadPreferences(t.Context(), actor); !errors.Is(err, writefence.ErrUnavailable) {
				t.Fatalf("native profile did not reject Creation role: %v", err)
			}
		})
	}
}
