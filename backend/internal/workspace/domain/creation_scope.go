package domain

import "github.com/google/uuid"

// Scope separates content ownership from physical storage and authorship.
type Scope struct {
	ID, PhysicalScopeID uuid.UUID
	Kind                string
	OwnerUserID, TeamID *uuid.UUID
}
