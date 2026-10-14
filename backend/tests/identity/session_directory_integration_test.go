package identity_test

import (
	"errors"
	"testing"
	"time"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestSessionDirectoryOwnership(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC()
	auth, a := verifiedTestActor(t, tx, "device-a@example.invalid", now)
	_, b := verifiedTestActor(t, tx, "device-b@example.invalid", now)
	second, err := auth.Login(t.Context(), "device-a@example.invalid", "long password for tests", false)
	if err != nil {
		t.Fatal(err)
	}
	sessions := identity.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(tx), func() time.Time { return now })
	items, err := sessions.List(t.Context(), a)
	if err != nil || len(items) != 2 {
		t.Fatal("session directory lost devices")
	}
	current := 0
	for _, item := range items {
		if item.Current {
			current++
		}
	}
	if current != 1 {
		t.Fatal("current session not identified")
	}
	if err := sessions.Revoke(t.Context(), a, b.SessionID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("cross-user revocation accepted")
	}
	if err := sessions.Revoke(t.Context(), a, second.Session.ID); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Revoke(t.Context(), a, second.Session.ID); err != nil {
		t.Fatal("revocation not idempotent")
	}
	if _, err := auth.Current(t.Context(), second.Token); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("revoked device still authenticated")
	}
	if err := sessions.RevokeOthers(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	items, err = sessions.List(t.Context(), a)
	if err != nil || len(items) != 1 || !items[0].Current {
		t.Fatal("current device was revoked")
	}
	if items, err := sessions.List(t.Context(), b); err != nil || len(items) != 1 {
		t.Fatal("other account affected by revoke-others")
	}
}
