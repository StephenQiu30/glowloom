package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

var (
	// ErrMainCanvasManagedByProject prevents bypassing the project name authority.
	ErrMainCanvasManagedByProject = errors.New("main canvas name is managed by project")
	// ErrMainCanvasRequired prevents removing the stable main canvas.
	ErrMainCanvasRequired = errors.New("main canvas is required")
)

// ProjectPresentationStatus describes whether the project presentation is currently writable.
type ProjectPresentationStatus string

// Project presentation states determine whether canvas presentation writes are permitted.
const (
	ProjectPresentationStatusActive   ProjectPresentationStatus = ProjectPresentationStatus(workspacedomain.ProjectStatusActive)
	ProjectPresentationStatusArchived ProjectPresentationStatus = ProjectPresentationStatus(workspacedomain.ProjectStatusArchived)
)

// ProjectPresentation contains the project facts exposed to canvas presentation readers.
type ProjectPresentation struct {
	MainCanvasID uuid.UUID
	Name         string
	Status       ProjectPresentationStatus
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
