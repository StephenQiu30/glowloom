package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// PlatformPasswordReset shares the existing reset proof, budget and encrypted delivery workflow.
type PlatformPasswordReset struct {
	store GovernanceStore
	auth  *Auth
	now   func() time.Time
}

// NewPlatformPasswordReset injects the same configured email authentication service.
func NewPlatformPasswordReset(store GovernanceStore, auth *Auth, now func() time.Time) *PlatformPasswordReset {
	return &PlatformPasswordReset{store: store, auth: auth, now: now}
}

// Send issues recovery without changing the target password or creating a privileged session.
func (s *PlatformPasswordReset) Send(ctx context.Context, a Principal, id uuid.UUID, v int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if id == uuid.Nil || v < 1 || !validGovernanceReason(reason) {
		return ErrInvalidGovernance
	}
	if s.auth == nil || !s.auth.mailReady() {
		return ErrRegistrationUnavailable
	}
	return s.store.GovernanceTransaction(ctx, a, true, s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		u, err := tx.User(id)
		if err != nil {
			return err
		}
		if u.Revision != v {
			return domain.ErrRevisionConflict
		}
		if u.EmailVerifiedAt.IsZero() || (u.Status != domain.StatusActive && u.Status != domain.StatusDisabled) {
			return ErrGovernanceConflict
		}
		if err := s.auth.reserveEmail(tx, u.Email, "platform/"+a.ID.String(), now); err != nil {
			return err
		}
		if err := s.auth.issuePasswordReset(tx, u, uuid.New(), now); err != nil {
			return err
		}
		return tx.AppendGovernanceEvent(&id, nil, "account.password_reset_requested", reason, now)
	})
}
