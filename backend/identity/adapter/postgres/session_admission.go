package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// SessionAdmissionStore verifies current actor policy before recording admission.
// It does not authorize the command's scope, resource or business action.
type SessionAdmissionStore struct{ db *gorm.DB }

// NewSessionAdmissionStore requires an Identity root pool, not an open transaction.
func NewSessionAdmissionStore(db *gorm.DB) *SessionAdmissionStore {
	return &SessionAdmissionStore{db: db}
}

var _ application.SessionAdmissionStore = (*SessionAdmissionStore)(nil)

// RegisterSessionCommand atomically binds live account/session policy and exact
// authority generations. Other owners must still verify their independent policy.
func (s *SessionAdmissionStore) RegisterSessionCommand(ctx context.Context, actor application.Principal, command writefence.Command, target writefence.Writer, clock func() time.Time) error {
	if s == nil || clock == nil {
		return writefence.ErrUnavailable
	}
	if _, err := command.AuthorityDigest(); err != nil {
		return err
	}
	var account, session bool
	for _, authority := range command.Authorities {
		if authority.Issuer != writefence.Identity {
			continue
		}
		switch {
		case authority.Kind == writefence.Account && authority.ID == actor.ID && !account:
			account = true
		case authority.Kind == writefence.Session && authority.ID == actor.SessionID && !session:
			session = true
		default:
			return application.ErrForbidden
		}
	}
	if !account || !session {
		return application.ErrForbidden
	}
	return identityWriterTransaction(ctx, s.db, func(tx *gorm.DB) error {
		if _, err := lockSelfActor(tx, actor, clock); err != nil {
			return err
		}
		if err := requireExistingAdmissionProof(ctx, tx, command.ID, target); err != nil {
			return err
		}
		if err := pgfence.NewAuthorityStore(tx).Register(ctx, tx, command, target); err != nil {
			return err
		}
		return recordSessionAdmission(ctx, tx, actor, command, target)
	})
}
