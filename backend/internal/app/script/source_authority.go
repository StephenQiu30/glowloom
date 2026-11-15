package script

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	script "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	agentpostgres "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
)

type sourceControl struct{ tx *gorm.DB }

type sourcePublication struct{ store *agentpostgres.Store }

func (p sourcePublication) AuthorizeSourcePublication(ctx context.Context, actor script.Actor, project, key uuid.UUID, hash string) error {
	if p.store == nil {
		return script.ErrUnavailable
	}
	return OwningError(p.store.AuthorizeSourcePublication(ctx, IdentityActor(actor), project, key, hash))
}

func (s sourceControl) AuthorizeWorker(ctx context.Context, a identity.Principal, pid uuid.UUID) error {
	_, err := scopewire.ProvideWorkerProjectContentAccess(s.tx).Authorize(ctx, a, pid, true)
	return err
}
func (s sourceControl) AuthorizeRequest(ctx context.Context, a identity.Principal, pid uuid.UUID, write bool) error {
	_, err := scopewire.ProvideProjectContentAccess(s.tx).Authorize(ctx, a, pid, write)
	return err
}

// ProvideSourcePublicationAuthorization joins Agent-origin source plans to
// durable current Go authority. Human-origin plans retain normal script rules.
func ProvideSourcePublicationAuthorization(tx *gorm.DB) script.PublicationAuthorization {
	return sourcePublication{store: agentpostgres.NewStore(tx, func(inner *gorm.DB) agent.ControlAccess { return sourceControl{inner} }, time.Now)}
}
