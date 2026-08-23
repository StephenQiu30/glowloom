package identity_test

import (
	"testing"

	. "github.com/stephenqiu30/lanverse/backend/src/identity"
)

func TestRoleCodeOnlySupportsAdminUserAndBan(t *testing.T) {
	for _, role := range []RoleCode{RoleAdmin, RoleUser, RoleBan} {
		if !role.IsValid() {
			t.Fatalf("role %q should be valid", role)
		}
	}
	for _, role := range []RoleCode{"owner", "producer", "operator", "reviewer", ""} {
		if role.IsValid() {
			t.Fatalf("role %q should be invalid", role)
		}
	}
}

func TestOnlyAdminCanEnterAdminBoundary(t *testing.T) {
	if !RoleAdmin.IsAdmin() {
		t.Fatal("admin should pass admin boundary")
	}
	for _, role := range []RoleCode{RoleUser, RoleBan} {
		if role.IsAdmin() {
			t.Fatalf("role %q should not pass admin boundary", role)
		}
	}
}
