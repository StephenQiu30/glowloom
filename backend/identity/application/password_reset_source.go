package application

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// PasswordResetResponsibility is an accepted mailbox proof, not a fake session.
type PasswordResetResponsibility struct {
	RequestID, AccountID, ProofOperationID uuid.UUID
	InputSHA256, ProofSHA256               string
	Candidate                              domain.User
	CloseControl, ReopenControl            writefence.Control
	Applied, Completed                     bool
	AcceptedAt                             time.Time
}

// PasswordResetSourceTx persists proof consumption and original source closure.
type PasswordResetSourceTx interface {
	PreparePasswordReset(PasswordResetResponsibility) (PasswordResetResponsibility, error)
}

// PasswordResetSourceStore applies only the explicit mailbox-password reset policy.
type PasswordResetSourceStore interface {
	LoadPasswordReset(context.Context, uuid.UUID, []byte, []byte) (PasswordResetResponsibility, bool, error)
	LoadAcceptedPasswordReset(context.Context, uuid.UUID) (PasswordResetResponsibility, error)
	CommitPasswordReset(context.Context, PasswordResetResponsibility, func(EmailTx, domain.User, time.Time) error) (PasswordResetResponsibility, error)
	CompletePasswordReset(context.Context, PasswordResetResponsibility) error
}

// PasswordResets completes accepted anonymous proof responsibilities without minting sessions.
type PasswordResets struct {
	store     PasswordResetSourceStore
	writers   SessionFenceCoordinator
	protector ProofProtector
	auth      *Auth
}

// PasswordResetRecoveryTask identifies only previously accepted reset requests.
const PasswordResetRecoveryTask = "identity.password_reset"

// WithPasswordResets requires every source dependency and never falls back locally.
func (a *Auth) WithPasswordResets(store PasswordResetSourceStore, writers SessionFenceCoordinator, protector ProofProtector) error {
	if store == nil || writers == nil || protector == nil {
		return ErrPasswordChangeUnavailable
	}
	a.passwordResets = &PasswordResets{store: store, writers: writers, protector: protector, auth: a}
	return nil
}

// PasswordResetsReady selects the source-owned complete-reset public handler.
func (a *Auth) PasswordResetsReady() bool { return a != nil && a.passwordResets != nil }

// CompletePasswordResetCommand recovers an accepted original proof before its
// consumed/expired email token is rechecked or a new password policy is applied.
func (a *Auth) CompletePasswordResetCommand(ctx context.Context, id uuid.UUID, token, password, confirmation string) error {
	q := a.passwordResets
	if q == nil || id == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	encoded, err := json.Marshal(struct{ Token, Password, Confirmation string }{token, password, confirmation})
	if err != nil {
		return err
	}
	digest := q.protector.Digest("password_reset_input", id, string(encoded))
	clear(encoded)
	defer clear(digest)
	proof := q.protector.Digest("password_reset_credential", id, token)
	defer clear(proof)
	if len(digest) != 32 || len(proof) != 32 {
		return ErrProofInvalid
	}
	saved, found, err := q.store.LoadPasswordReset(ctx, id, digest, proof)
	if err != nil {
		return err
	}
	if found {
		return q.finish(ctx, saved)
	}
	err = a.completePasswordResetPrepared(ctx, token, password, confirmation, func(tx EmailTx, user domain.User, challenge domain.EmailChallenge, hash string, now time.Time) error {
		source, ok := tx.(PasswordResetSourceTx)
		if !ok {
			return ErrPasswordChangeUnavailable
		}
		user.PasswordHash = hash
		p := PasswordResetResponsibility{RequestID: id, AccountID: user.ID, ProofOperationID: challenge.OperationID, InputSHA256: hex.EncodeToString(digest), ProofSHA256: hex.EncodeToString(proof), Candidate: user, AcceptedAt: now.UTC().Truncate(time.Microsecond)}
		challenge.ConsumedAt = &p.AcceptedAt
		if err := tx.SaveChallenge(challenge); err != nil {
			return err
		}
		_, err := source.PreparePasswordReset(p)
		return err
	})
	if err != nil && !errors.Is(err, ErrPasswordChangeExists) {
		return err
	}
	saved, found, err = q.store.LoadPasswordReset(ctx, id, digest, proof)
	if err != nil {
		return err
	}
	if !found {
		return ErrPasswordChangeUnavailable
	}
	return q.finish(ctx, saved)
}
func (q *PasswordResets) finish(ctx context.Context, p PasswordResetResponsibility) error {
	if p.RequestID == uuid.Nil || p.AccountID == uuid.Nil || p.ProofOperationID == uuid.Nil || p.CloseControl.Validate() != nil || p.CloseControl.Kind != writefence.Account || p.CloseControl.ID != p.AccountID || !p.CloseControl.Blocked {
		return ErrPasswordChangeUnavailable
	}
	if p.Completed {
		return nil
	}
	expected := p.CloseControl.Authority
	expected.Epoch--
	state, err := q.writers.Set(ctx, expected, p.CloseControl.CoordinationID, true)
	if err != nil {
		return err
	}
	if !exactWriterCompletion(state, p.CloseControl) {
		return ErrPasswordChangeUnavailable
	}
	p, err = q.store.CommitPasswordReset(ctx, p, func(tx EmailTx, user domain.User, now time.Time) error {
		return q.auth.applyResetPassword(tx, user, p.Candidate.PasswordHash, now)
	})
	if err != nil {
		return err
	}
	if p.Candidate.Status == domain.StatusActive {
		if p.ReopenControl.Validate() != nil || p.ReopenControl.Blocked {
			return ErrPasswordChangeUnavailable
		}
		expected = p.ReopenControl.Authority
		expected.Epoch--
		state, err = q.writers.Set(ctx, expected, p.ReopenControl.CoordinationID, false)
		if err != nil {
			return err
		}
		if !exactWriterCompletion(state, p.ReopenControl) {
			return ErrPasswordChangeUnavailable
		}
	} else if p.Candidate.Status != domain.StatusDisabled || p.ReopenControl.CoordinationID != uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	return q.store.CompletePasswordReset(ctx, p)
}

// ResumePasswordReset never authenticates a new actor or changes the saved proof.
func (a *Auth) ResumePasswordReset(ctx context.Context, id uuid.UUID) error {
	if a == nil || a.passwordResets == nil || id == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	p, err := a.passwordResets.store.LoadAcceptedPasswordReset(ctx, id)
	if err != nil {
		return err
	}
	if p.RequestID != id {
		return ErrPasswordChangeUnavailable
	}
	return a.passwordResets.finish(ctx, p)
}
