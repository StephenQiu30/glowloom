package http

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

// executionGrant derives the current capability from an admitted, scoped run.
func (h *BusinessHandler) executionGrant(ctx context.Context, runID uuid.UUID, caller domain.Scope) (domain.Authority, domain.Grant, error) {
	if runID == uuid.Nil || h.store == nil {
		return domain.Authority{}, domain.Grant{}, domain.ErrGrant
	}
	authority, err := h.store.CurrentAuthority(ctx, runID)
	if err != nil {
		return authority, domain.Grant{}, err
	}
	if authority.Scope.ActorID != caller.ActorID || authority.Scope.OrgID != caller.OrgID ||
		authority.Scope.ScopeID != caller.ScopeID || authority.Scope.ProjectID != caller.ProjectID {
		return authority, domain.Grant{}, domain.ErrGrant
	}
	grant, err := h.store.RegisterGrant(ctx, domain.GrantRequest{
		RootRunID: runID, SubmissionID: authority.SubmissionID,
		OwnerEpoch: max(1, authority.OwnerEpoch), AuthorizationRevision: authority.AuthorizationRevision,
		LeaseUntil: time.Now().Add(30 * time.Second),
	})
	return authority, grant, err
}
