package agent

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type workerNavigationScope struct{ tx *gorm.DB }

func (s workerNavigationScope) Resolve(ctx context.Context, a identity.Principal, id uuid.UUID, write bool) (workspacedomain.Scope, error) {
	if id != a.ScopeID {
		return workspacedomain.Scope{}, workspace.ErrScopeNotFound
	}
	scope, err := scopewire.ProvideScopeAccess(s.tx).Resolve(ctx, a.ID, id, write)
	if err != nil {
		return scope, err
	}
	if scope.PhysicalScopeID != a.OrgID {
		return workspacedomain.Scope{}, workspace.ErrScopeNotFound
	}
	return scope, nil
}
func workerNavigation(tx *gorm.DB) *pgworkspace.Store {
	return pgworkspace.NewScopedStore(tx, func(inner *gorm.DB) workspace.RequestScopeResolver { return workerNavigationScope{inner} }, nil, nil)
}

type canvasAccess struct {
	canvas.ProjectAccess
	tx *gorm.DB
}

func newCanvasAccess(tx *gorm.DB) canvas.ProjectAccess {
	return canvasAccess{ProjectAccess: mediawire.NewWorkerCanvasProjectAccess(tx), tx: tx}
}
func (a canvasAccess) ReadProjectPresentation(ctx context.Context, actor identity.Principal, id uuid.UUID) (canvas.ProjectPresentation, error) {
	facts, err := workerNavigation(a.tx).ReadProjectNavigation(ctx, actor, id)
	if err != nil {
		return canvas.ProjectPresentation{}, err
	}
	return canvas.ProjectPresentation{MainCanvasID: facts.MainCanvasID, Name: facts.Name, Status: canvas.ProjectPresentationStatus(facts.Status), OwnerName: facts.OwnerName}, nil
}
func (a canvasAccess) MarkProjectContentChanged(ctx context.Context, actor identity.Principal, id uuid.UUID) error {
	return workerNavigation(a.tx).MarkProjectContentChanged(ctx, actor, id)
}
