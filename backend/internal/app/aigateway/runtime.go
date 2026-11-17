// Package aigateway composes the rebuilt AI business boundary with Identity and Creation.
package aigateway

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/credentialseal"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/provider"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	identitypg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Authority rechecks the live session and Creation scope in the consumer transaction.
func Authority(ctx context.Context, tx *gorm.DB, actor identity.Principal, scope, project uuid.UUID, admin bool) error {
	if admin {
		if err := identitypg.NewStore(tx).CheckPlatformSession(ctx, actor, time.Now); err != nil {
			return domain.ErrForbidden
		}
		return nil
	}
	if _, err := scopewire.ProvideRequestScopeAccess(tx).Resolve(ctx, actor, scope, true); err != nil {
		return domain.ErrForbidden
	}
	return projectScope(tx, scope, project)
}
func projectScope(tx *gorm.DB, scope, project uuid.UUID) error {
	if project == uuid.Nil {
		return nil
	}
	var row struct{ ID uuid.UUID }
	r := tx.Raw("SELECT id FROM workspace.project WHERE id=? AND owner_scope_id=? AND NOT is_delete AND status='active' FOR SHARE", project, scope).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return domain.ErrForbidden
	}
	return nil
}

// Runtime shares only cryptography and protocol transport across new consumers.
type Runtime struct {
	Keys provider.Keys
	// MediaArchive and VoiceMedia are optional byte-processing dependencies for isolated tests.
	MediaArchive    GatewayArchiver
	VoiceMedia      VoiceMediaPreparer
	OutputReads     GatewayObjectReads
	ResultDownloads GatewayResultDownloads
	Objects         *objectstorage.Registry
	DB              *gorm.DB
	Provider        application.Provider
	Sealer          gatewayhttp.Sealer
}

// New composes the gateway transport and credential cryptography.
func New(database *gorm.DB, cfg config.Config) (*Runtime, error) {
	r := &Runtime{DB: database}
	if cfg.CredentialPrivateKeyPEM != "" || cfg.CredentialPrivateKeyFile != "" {
		private, err := cfg.CredentialPrivateBytes()
		if err != nil {
			return nil, err
		}
		defer clear(private)
		keys, err := credentialseal.NewUnsealer(cfg.CredentialKeyID, private)
		if err != nil {
			return nil, err
		}
		r.Keys = keys
		r.Provider = provider.New(keys, &http.Client{}).WithMediaReferences(r.mediaReferences)
	}
	if cfg.CredentialPublicKeyPEM != "" || cfg.CredentialPublicKeyFile != "" {
		public, err := cfg.CredentialPublicBytes()
		if err != nil {
			return nil, err
		}
		r.Sealer, err = credentialseal.NewSealer(cfg.CredentialKeyID, public)
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Store binds request authority to the exact current actor.
func (r *Runtime) Store(actor identity.Principal) *pg.Store { return pg.New(r.DB, actor, Authority) }
