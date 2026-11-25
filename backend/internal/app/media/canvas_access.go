// Package media wires media authority and cross-module dependencies.
package media

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// ProjectNavigation reads workspace-owned presentation. The process binds it so this package does not import the workspace assembler.
type ProjectNavigation interface {
	ReadProjectNavigation(context.Context, identity.Principal, uuid.UUID) (workspaceapp.ProjectNavigation, error)
	MarkProjectContentChanged(context.Context, identity.Principal, uuid.UUID) error
}

var projectNavigationOf func(*gorm.DB) ProjectNavigation

// BindProjectNavigation installs the workspace presentation reader used by canvas access.
func BindProjectNavigation(fn func(*gorm.DB) ProjectNavigation) {
	projectNavigationOf = fn
}

func navigation(tx *gorm.DB) ProjectNavigation {
	if projectNavigationOf == nil {
		panic("media project navigation is not bound")
	}
	return projectNavigationOf(tx)
}

func (a canvasProjectAccess) ReadProjectPresentation(ctx context.Context, actor identity.Principal, id uuid.UUID) (canvasapp.ProjectPresentation, error) {
	facts, err := navigation(a.tx).ReadProjectNavigation(ctx, actor, id)
	if err != nil {
		return canvasapp.ProjectPresentation{}, err
	}
	return canvasapp.ProjectPresentation{MainCanvasID: facts.MainCanvasID, Name: facts.Name, Status: canvasapp.ProjectPresentationStatus(facts.Status), OwnerName: facts.OwnerName}, nil
}

func (a canvasProjectAccess) MarkProjectContentChanged(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	return navigation(a.tx).MarkProjectContentChanged(ctx, actor, id)
}

// canvasProjectAccess is the legacy Canvas composition adapter. Its Principal
// contract stays separate when Media consumes its own minimal Actor reference.
type canvasProjectAccess struct{ mediaScopeAccess }

// canvasScopedAccess preserves the injected media authorization for internal consumers.
type canvasScopedAccess struct {
	mediaapp.ScopeAccess
	tx *gorm.DB
}

// NewWorkerCanvasProjectAccess binds internal canvas consumers to worker scope checks.
func NewWorkerCanvasProjectAccess(tx *gorm.DB) canvasapp.ProjectAccess {
	return canvasProjectAccess{mediaScopeAccess{tx: tx, worker: true}}
}

// NewCanvasProjectAccessWithScope preserves the caller's authorization factory and adds workspace presentation ports.
func NewCanvasProjectAccessWithScope(tx *gorm.DB, access mediaapp.ScopeAccess) canvasapp.ProjectAccess {
	return canvasScopedAccess{ScopeAccess: access, tx: tx}
}

func (a canvasScopedAccess) ReadProjectPresentation(ctx context.Context, actor identity.Principal, id uuid.UUID) (canvasapp.ProjectPresentation, error) {
	if err := a.AuthorizeProject(ctx, actor, id, false); err != nil {
		return canvasapp.ProjectPresentation{}, err
	}
	return (canvasProjectAccess{mediaScopeAccess{tx: a.tx}}).ReadProjectPresentation(ctx, actor, id)
}

func (a canvasScopedAccess) MarkProjectContentChanged(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	return navigation(a.tx).MarkProjectContentChanged(ctx, actor, id)
}

func (a canvasProjectAccess) AuthorizeProject(ctx context.Context, actor identity.Principal, id uuid.UUID, write bool) error {
	return a.mediaScopeAccess.AuthorizeProject(ctx, Actor(actor), id, write)
}
func (a canvasProjectAccess) AuthorizeCopyTarget(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	return a.mediaScopeAccess.AuthorizeCopyTarget(ctx, Actor(actor), id)
}
func (a canvasScopedAccess) AuthorizeProject(ctx context.Context, actor identity.Principal, id uuid.UUID, write bool) error {
	return a.ScopeAccess.AuthorizeProject(ctx, Actor(actor), id, write)
}
func (a canvasScopedAccess) AuthorizeCopyTarget(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	return a.ScopeAccess.AuthorizeCopyTarget(ctx, Actor(actor), id)
}
