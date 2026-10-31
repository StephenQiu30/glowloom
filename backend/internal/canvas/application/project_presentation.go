package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrMainCanvasManagedByProject prevents bypassing the project name authority.
	ErrMainCanvasManagedByProject = errors.New("main canvas name is managed by project")
	// ErrMainCanvasRequired prevents removing the stable main canvas.
	ErrMainCanvasRequired = errors.New("main canvas is required")
)

// ProjectPresentation contains workspace-owned identity and display metadata.
type ProjectPresentation struct {
	MainCanvasID uuid.UUID
	Name, Status string
	OwnerName    string
}

// ProjectPresentationReader projects current workspace facts inside the caller transaction.
type ProjectPresentationReader interface {
	ReadProjectPresentation(context.Context, identity.Principal, uuid.UUID) (ProjectPresentation, error)
}

// ProjectActivityWriter records a committed primary content change in the same transaction.
type ProjectActivityWriter interface {
	MarkProjectContentChanged(context.Context, identity.Principal, uuid.UUID) error
}
