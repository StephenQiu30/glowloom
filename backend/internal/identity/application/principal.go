// Package application coordinates durable workspace identity and account commands.
package application

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

// Principal identifies the actor recorded by organization-scoped commands.
// The current API supplies it from the single durable workspace.
type Principal struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	DisplayName        string
	Role               domain.Role
	MustChangePassword bool
}
