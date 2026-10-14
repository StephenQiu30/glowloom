package identityoperator

import "testing"

func TestIdentityOperatorRequiresExplicitTarget(t *testing.T) {
	for _, args := range [][]string{{}, {"bind-start"}, {"bind-complete", "--operation", "invalid"}, {"mail-run"}, {"key-init", "extra"}, {"bind-start", "--login-name", "creator", "--email", "bad"}} {
		if _, err := parseIdentityCommand(args); err == nil {
			t.Fatalf("unsafe operator arguments accepted: %v", args)
		}
	}
	start, err := parseIdentityCommand([]string{"bind-start", "--login-name", "creator", "--email", "USER@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if start.platformAdmin {
		t.Fatal("platform authority was inferred")
	}
	elevated, err := parseIdentityCommand([]string{"bind-start", "--login-name", "creator", "--email", "user@example.invalid", "--platform-admin"})
	if err != nil || !elevated.platformAdmin {
		t.Fatal("explicit administrator binding lost")
	}
}
