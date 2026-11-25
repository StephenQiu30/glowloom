package identityruntime

import (
	"errors"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"

	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// mapCreationGovernanceError preserves the mixed root's existing HTTP outcomes
// until team effects are consumed through the owner protocol. It is intentionally
// absent from the independent Identity dependency graph.
func mapCreationGovernanceError(err error) error {
	switch {
	case errors.Is(err, workspace.ErrTeamStateConflict), errors.Is(err, workspace.ErrTeamCapacity):
		return errors.Join(identity.ErrGovernanceConflict, err)
	case errors.Is(err, workspace.ErrTeamRevisionConflict):
		return errors.Join(domain.ErrRevisionConflict, err)
	case errors.Is(err, workspace.ErrTeamForbidden):
		return errors.Join(identity.ErrForbidden, err)
	case errors.Is(err, workspacesource.ErrScopeNotFound):
		return errors.Join(domain.ErrUserNotFound, err)
	default:
		return err
	}
}
