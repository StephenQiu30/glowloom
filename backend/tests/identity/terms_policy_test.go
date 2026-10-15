package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestUnpublishedTermsCannotEnableRegistration(t *testing.T) {
	key, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"", " ", "2026-10-15-draft.1", "2026-10-15-DRAFT.1", " 2026-10-15.1"} {
		t.Run(version, func(t *testing.T) {
			auth := identity.NewEmailAuth(nil, key, identity.EmailPolicy{TermsVersion: version, SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, time.Now)
			if _, err := auth.StartRegistration(t.Context(), "invalid email", ""); !errors.Is(err, identity.ErrRegistrationUnavailable) {
				t.Fatalf("unpublished policy reached registration: %v", err)
			}
			if _, err := auth.CompleteRegistration(t.Context(), identity.CompleteRegistrationInput{}); !errors.Is(err, identity.ErrRegistrationUnavailable) {
				t.Fatal("unpublished policy reached account creation")
			}
		})
	}
}
