// Package application coordinates authentication and organization-scoped account commands.
package application

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// Principal identifies the actor recorded by organization-scoped commands.
// The public API resolves it from a valid server-side session.
type Principal struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               domain.Role
	MustChangePassword bool
}
