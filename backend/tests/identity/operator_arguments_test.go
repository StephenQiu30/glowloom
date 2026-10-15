package identity_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityoperator"
)

func TestIdentityOperatorRequiresExplicitTarget(t *testing.T) {
	for _, args := range [][]string{{}, {"bind-start"}, {"bind-complete", "--operation", "invalid"}, {"mail-run"}, {"key-init", "extra"}, {"bind-start", "--login-name", "creator", "--email", "bad"}} {
		if _, err := identityoperator.ParseCommand(args); err == nil {
			t.Fatalf("unsafe operator arguments accepted: %v", args)
		}
	}
	start, err := identityoperator.ParseCommand([]string{"bind-start", "--login-name", "creator", "--email", "USER@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if start.PlatformAdmin {
		t.Fatal("platform authority was inferred")
	}
	elevated, err := identityoperator.ParseCommand([]string{"bind-start", "--login-name", "creator", "--email", "user@example.invalid", "--platform-admin"})
	if err != nil || !elevated.PlatformAdmin {
		t.Fatal("explicit administrator binding lost")
	}
}
