package application

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

var (
	ErrInvalidMigrationInventory  = errors.New("invalid migration inventory")
	ErrAmbiguousResourceOwnership = errors.New("ambiguous resource ownership")
)

// LegacyUser carries stable identity, never authority inferred from the old role.
type LegacyUser struct {
	ID, OrgID  uuid.UUID
	LegacyRole string
}

// LegacyResource records established ownership separately from authorship.
type LegacyResource struct {
	ID, OrgID, CreatedBy uuid.UUID
	Kind                 string
	OwnerUserID, TeamID  *uuid.UUID
}

// VerifiedBinding must come from a controlled verification process, not a name match.
type VerifiedBinding struct {
	UserID        uuid.UUID
	Email         string
	VerifiedAt    time.Time
	TeamID        *uuid.UUID
	PlatformAdmin bool
}

type ResourceOwnership struct {
	ResourceID, ScopeID uuid.UUID
	Kind                string
	OwnerID             uuid.UUID
}

type MigrationInventory struct {
	Users     []LegacyUser
	Resources []LegacyResource
	Bindings  []VerifiedBinding
}

type MigrationAssessment struct {
	BlockedUserIDs, UnmappedResourceIDs []uuid.UUID
	VerifiedBindings                    []VerifiedBinding
	UserCount, ResourceCount            int
}

// AssessMigration is read-only and blocks every identity involved in a collision.
func AssessMigration(input MigrationInventory) (MigrationAssessment, error) {
	result := MigrationAssessment{UserCount: len(input.Users), ResourceCount: len(input.Resources)}
	users := make(map[uuid.UUID]bool, len(input.Users))
	for _, user := range input.Users {
		if user.ID == uuid.Nil || users[user.ID] {
			return MigrationAssessment{}, ErrInvalidMigrationInventory
		}
		users[user.ID] = true
	}
	bindings := make(map[uuid.UUID]VerifiedBinding, len(input.Bindings))
	emails := make(map[string]int, len(input.Bindings))
	for _, binding := range input.Bindings {
		if _, duplicate := bindings[binding.UserID]; duplicate || !users[binding.UserID] {
			return MigrationAssessment{}, ErrInvalidMigrationInventory
		}
		// Keep invalid proofs in the map so duplicate binding rows still fail.
		email, err := domain.NormalizeEmail(binding.Email)
		binding.Email = email
		bindings[binding.UserID] = binding
		if err == nil && !binding.VerifiedAt.IsZero() {
			emails[email]++
		}
	}
	for _, user := range input.Users {
		binding, exists := bindings[user.ID]
		if !exists || binding.Email == "" || binding.VerifiedAt.IsZero() || emails[binding.Email] != 1 {
			result.BlockedUserIDs = append(result.BlockedUserIDs, user.ID)
			continue
		}
		result.VerifiedBindings = append(result.VerifiedBindings, binding)
	}
	resources := make(map[uuid.UUID]bool, len(input.Resources))
	for _, resource := range input.Resources {
		if resource.ID == uuid.Nil || resources[resource.ID] {
			return MigrationAssessment{}, ErrInvalidMigrationInventory
		}
		resources[resource.ID] = true
		if _, _, ok := establishedOwner(resource); !ok {
			result.UnmappedResourceIDs = append(result.UnmappedResourceIDs, resource.ID)
		}
	}
	sort.Slice(result.BlockedUserIDs, func(i, j int) bool { return result.BlockedUserIDs[i].String() < result.BlockedUserIDs[j].String() })
	sort.Slice(result.UnmappedResourceIDs, func(i, j int) bool {
		return result.UnmappedResourceIDs[i].String() < result.UnmappedResourceIDs[j].String()
	})
	sort.Slice(result.VerifiedBindings, func(i, j int) bool {
		return result.VerifiedBindings[i].UserID.String() < result.VerifiedBindings[j].UserID.String()
	})
	return result, nil
}

// ValidateOwnership requires a complete one-to-one mapping and stable owners.
func ValidateOwnership(resources []LegacyResource, mappings []ResourceOwnership) error {
	if len(resources) != len(mappings) {
		return ErrAmbiguousResourceOwnership
	}
	byID := make(map[uuid.UUID]LegacyResource, len(resources))
	for _, resource := range resources {
		if _, duplicate := byID[resource.ID]; duplicate || resource.ID == uuid.Nil {
			return ErrAmbiguousResourceOwnership
		}
		byID[resource.ID] = resource
	}
	type owner struct {
		kind string
		id   uuid.UUID
	}
	scopes := make(map[uuid.UUID]owner)
	for _, mapping := range mappings {
		resource, exists := byID[mapping.ResourceID]
		kind, id, valid := establishedOwner(resource)
		if !exists || !valid || mapping.ScopeID == uuid.Nil || mapping.Kind != kind || mapping.OwnerID != id {
			return ErrAmbiguousResourceOwnership
		}
		next := owner{kind, id}
		if previous, exists := scopes[mapping.ScopeID]; exists && previous != next {
			return ErrAmbiguousResourceOwnership
		}
		scopes[mapping.ScopeID] = next
		delete(byID, mapping.ResourceID)
	}
	return nil
}

func establishedOwner(resource LegacyResource) (string, uuid.UUID, bool) {
	if resource.OwnerUserID != nil && resource.TeamID == nil && *resource.OwnerUserID != uuid.Nil {
		return "personal", *resource.OwnerUserID, true
	}
	if resource.TeamID != nil && resource.OwnerUserID == nil && *resource.TeamID != uuid.Nil {
		return "team", *resource.TeamID, true
	}
	return "", uuid.Nil, false
}
