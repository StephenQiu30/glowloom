package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// NewInvitationEmailStore binds workspace invitation checks to registration transactions.
func NewInvitationEmailStore(db *gorm.DB, teams TeamGovernanceFactory) *Store {
	return &Store{db: db, invitationTeams: teams}
}

// RegistrationInvitation verifies invitation and inviter under the caller's settings lock.
func (t *authTx) RegistrationInvitation(id uuid.UUID, hash []byte, email string, now time.Time) (application.Invitation, error) {
	g := &governanceTx{authTx: t}
	i, err := g.Invitation(id, hash)
	if err != nil {
		return i, err
	}
	if i.Status != "pending" || !now.Before(i.ExpiresAt) || (email != "" && i.EmailKey != email) {
		return i, application.ErrProofInvalid
	}
	u, err := t.AccountByID(i.InviterID)
	if err != nil {
		return i, application.ErrProofInvalid
	}
	if u.Status != domain.StatusActive || u.EmailVerifiedAt.IsZero() {
		return i, application.ErrProofInvalid
	}
	if i.Kind == "account" || i.PlatformIssued {
		if u.PlatformRole != "platform_admin" {
			return i, application.ErrProofInvalid
		}
		if i.Kind == "account" {
			return i, nil
		}
	}
	if t.invitationTeams == nil || i.TeamID == nil || i.TeamRole == nil {
		return i, application.ErrProofInvalid
	}
	gateway, ok := t.invitationTeams(t.db).(application.TeamInvitationGateway)
	if !ok {
		return i, application.ErrProofInvalid
	}
	var revision *int64
	if i.PlatformIssued {
		revision, err = gateway.CheckPlatformInvitation(*i.TeamID, i.EmailKey, *i.TeamRole)
	} else {
		revision, err = gateway.CheckInvitation(*i.TeamID, i.InviterID, i.EmailKey, *i.TeamRole)
	}
	if err != nil || (revision == nil) != (i.MembershipRevision == nil) || revision != nil && *revision != *i.MembershipRevision {
		return i, application.ErrProofInvalid
	}
	return i, nil
}

// PreviewInvitationProof returns only a valid invitation to the application projection.
func (s *GovernanceStore) PreviewInvitationProof(ctx context.Context, hash []byte, now time.Time) (application.Invitation, error) {
	var i application.Invitation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		t := &authTx{db: tx, invitationTeams: s.teams}
		if _, err := t.RegistrationEnabled(); err != nil {
			return err
		}
		var err error
		i, err = t.RegistrationInvitation(uuid.Nil, hash, "", now)
		return err
	})
	return i, err
}
