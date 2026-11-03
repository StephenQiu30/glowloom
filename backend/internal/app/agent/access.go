// Package agent wires Agent authority to existing workspace services.
package agent

import (
	"context"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type projectAuthorizer interface {
	Authorize(context.Context, identity.Principal, uuid.UUID, bool) (workspace.ProjectContentAccess, error)
}
type controlAccess struct{ request, worker projectAuthorizer }

func (a controlAccess) AuthorizeRequest(ctx context.Context, actor identity.Principal, pid uuid.UUID, write bool) error {
	_, err := a.request.Authorize(ctx, actor, pid, write)
	return err
}
func (a controlAccess) AuthorizeWorker(ctx context.Context, actor identity.Principal, pid uuid.UUID) error {
	_, err := a.worker.Authorize(ctx, actor, pid, true)
	return err
}

// ProvideControlAccess binds permission checks to the consuming transaction.
func ProvideControlAccess(tx *gorm.DB) app.ControlAccess {
	return controlAccess{request: scopewire.ProvideProjectContentAccess(tx), worker: scopewire.ProvideWorkerProjectContentAccess(tx)}
}
