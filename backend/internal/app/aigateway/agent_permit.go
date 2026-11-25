package aigateway

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type agentPermit struct {
	Root, Grant, Submission uuid.UUID
	Epoch, Revision         int64
}

// AgentPermit freezes the exact approved Agent segment for a queued generation.
func AgentPermit(ctx context.Context, tx *gorm.DB, call domain.Call) (json.RawMessage, error) {
	var proof agentPermit
	q := tx.WithContext(ctx).Raw(`SELECT a.root_run_id AS root,g.grant_id AS grant,a.submission_id AS submission,a.owner_epoch AS epoch,a.authorization_revision AS revision FROM agent_business.run_authorization a
 JOIN agent_business.execution_grant g ON g.root_run_id=a.root_run_id AND g.submission_id=a.submission_id AND g.owner_epoch=a.owner_epoch AND g.authorization_revision=a.authorization_revision AND g.expires_at>clock_timestamp()
 WHERE a.root_run_id=? AND a.scope->>'actor_id'=? AND a.scope->>'scope_id'=? AND a.scope->>'project_id'=? AND NOT a.barrier AND NOT a.terminal ORDER BY g.expires_at DESC LIMIT 1 FOR SHARE OF a,g`, call.Run, call.Actor.String(), call.Scope.String(), call.Project.String()).Scan(&proof)
	if q.Error != nil {
		return nil, q.Error
	}
	if q.RowsAffected != 1 {
		return nil, domain.ErrForbidden
	}
	return json.Marshal(proof)
}

// AgentWorkerAuthority rechecks the original approval and live content ownership at the send boundary.
func AgentWorkerAuthority(raw json.RawMessage) pg.Authority {
	var proof agentPermit
	valid := json.Unmarshal(raw, &proof) == nil && proof.Root != uuid.Nil && proof.Grant != uuid.Nil
	return func(ctx context.Context, tx *gorm.DB, a identity.Principal, scope, project uuid.UUID, admin bool) error {
		if !valid || admin || a.SessionID != uuid.Nil || scope != a.ScopeID {
			return domain.ErrForbidden
		}
		if _, err := scopewire.ProvideScopeAccess(tx).Resolve(ctx, a.ID, scope, true); err != nil {
			return err
		}
		// LoadQuote checks scope before it knows the project; the frozen grant still binds the actual project.
		var n int
		q := tx.Raw(`SELECT 1 FROM agent_business.run_authorization a JOIN agent_business.execution_grant g ON g.grant_id=? AND g.root_run_id=a.root_run_id AND g.submission_id=a.submission_id AND g.owner_epoch=a.owner_epoch AND g.authorization_revision=a.authorization_revision AND g.expires_at>clock_timestamp()
 WHERE a.root_run_id=? AND a.submission_id=? AND a.owner_epoch=? AND a.authorization_revision=? AND a.scope->>'actor_id'=? AND a.scope->>'scope_id'=? AND (?='00000000-0000-0000-0000-000000000000' OR a.scope->>'project_id'=?) AND NOT a.barrier AND NOT a.terminal FOR SHARE OF a,g`, proof.Grant, proof.Root, proof.Submission, proof.Epoch, proof.Revision, a.ID.String(), scope.String(), project.String(), project.String()).Scan(&n)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || n != 1 {
			return domain.ErrForbidden
		}
		return projectScope(tx, scope, project)
	}
}
