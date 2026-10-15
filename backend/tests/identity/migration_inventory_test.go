package identity_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestMigrationInventoryBlocksAmbiguity(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	org := uuid.New()
	verified := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	input := identity.MigrationInventory{
		Users:    []identity.LegacyUser{{ID: a, OrgID: org, LegacyRole: "admin"}, {ID: b, OrgID: org, LegacyRole: "producer"}, {ID: c, OrgID: org, LegacyRole: "producer"}, {ID: d, OrgID: org, LegacyRole: "producer"}},
		Bindings: []identity.VerifiedBinding{{UserID: a, Email: " OWNER@example.com ", VerifiedAt: verified}, {UserID: b, Email: "duplicate@example.com", VerifiedAt: verified}, {UserID: c, Email: "DUPLICATE@example.com", VerifiedAt: verified}},
	}
	got, err := identity.AssessMigration(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserCount != 4 || len(got.BlockedUserIDs) != 3 {
		t.Fatalf("blocked identities = %d, want 3", len(got.BlockedUserIDs))
	}
	if len(got.VerifiedBindings) != 1 || got.VerifiedBindings[0].UserID != a || got.VerifiedBindings[0].Email != "owner@example.com" || got.VerifiedBindings[0].PlatformAdmin {
		t.Fatal("preflight merged identities or inferred platform authority")
	}
	if input.Bindings[0].Email != " OWNER@example.com " {
		t.Fatal("preflight mutated its input")
	}
	for _, bad := range []identity.MigrationInventory{
		{Users: []identity.LegacyUser{{ID: a}, {ID: a}}},
		{Users: input.Users, Bindings: []identity.VerifiedBinding{{UserID: uuid.New(), Email: "unknown@example.com", VerifiedAt: verified}}},
		{Users: input.Users, Bindings: []identity.VerifiedBinding{{UserID: a, Email: "owner@example.com", VerifiedAt: verified}, {UserID: a, Email: "second@example.com", VerifiedAt: verified}}},
	} {
		if _, err := identity.AssessMigration(bad); !errors.Is(err, identity.ErrInvalidMigrationInventory) {
			t.Fatalf("invalid inventory accepted: %v", err)
		}
	}
}

func TestMigrationPreservesOwnership(t *testing.T) {
	user, team, org, personalScope, teamScope := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	personal, shared := uuid.New(), uuid.New()
	resources := []identity.LegacyResource{
		{ID: personal, OrgID: org, CreatedBy: user, Kind: "project", OwnerUserID: &user},
		{ID: shared, OrgID: org, CreatedBy: user, Kind: "project", TeamID: &team},
	}
	mappings := []identity.ResourceOwnership{
		{ResourceID: personal, ScopeID: personalScope, Kind: "personal", OwnerID: user},
		{ResourceID: shared, ScopeID: teamScope, Kind: "team", OwnerID: team},
	}
	if err := identity.ValidateOwnership(resources, mappings); err != nil {
		t.Fatal(err)
	}
	before := append([]identity.ResourceOwnership(nil), mappings...)
	tests := []struct {
		name string
		maps []identity.ResourceOwnership
	}{
		{"missing", mappings[:1]},
		{"duplicate", []identity.ResourceOwnership{mappings[0], mappings[0]}},
		{"private becomes shared", []identity.ResourceOwnership{{ResourceID: personal, ScopeID: teamScope, Kind: "team", OwnerID: team}, mappings[1]}},
		{"scope has two owners", []identity.ResourceOwnership{mappings[0], {ResourceID: shared, ScopeID: personalScope, Kind: "team", OwnerID: team}}},
		{"unknown resource", append(append([]identity.ResourceOwnership(nil), mappings...), identity.ResourceOwnership{ResourceID: uuid.New(), ScopeID: personalScope, Kind: "personal", OwnerID: user})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := identity.ValidateOwnership(resources, tt.maps); !errors.Is(err, identity.ErrAmbiguousResourceOwnership) {
				t.Fatalf("unsafe ownership accepted: %v", err)
			}
		})
	}
	if !reflect.DeepEqual(before, mappings) {
		t.Fatal("preflight mutated mapping")
	}
	unknown := []identity.LegacyResource{{ID: personal, OrgID: org, CreatedBy: user, Kind: "project"}}
	if err := identity.ValidateOwnership(unknown, mappings[:1]); !errors.Is(err, identity.ErrAmbiguousResourceOwnership) {
		t.Fatal("creator was silently promoted to owner")
	}
}
