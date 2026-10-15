package identity_test

import (
	"context"
	"testing"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type policyStore bool

func (s policyStore) ReadRegistrationEnabled(context.Context) (bool, error) { return bool(s), nil }
func TestPublicAuthPolicyConditions(t *testing.T) {
	for _, tc := range []struct {
		name, configured string
		enabled, mail    bool
		want             bool
		reason           string
	}{
		{"accepted fixture", "accepted-fixture", true, true, true, ""}, {"operator closed", "accepted-fixture", false, true, false, "registration_closed"}, {"no mail", "accepted-fixture", true, false, false, "mail_unavailable"}, {"unaccepted package", "", true, true, false, "agreement_not_accepted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := identity.NewAuthPolicyQuery(policyStore(tc.enabled), tc.configured, "accepted-fixture", tc.mail).Read(t.Context())
			if err != nil || p.RegistrationEnabled != tc.want || p.ClosedReason != tc.reason {
				t.Fatalf("policy=%+v err=%v", p, err)
			}
		})
	}
}
