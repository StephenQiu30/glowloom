package application

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// EmailChangeCommand binds the original mailbox operation and browser proof.
// Credentials remain inside Identity and are persisted only as keyed digests.
type EmailChangeCommand struct {
	RequestID, OperationID uuid.UUID
	Revision               int64
	Credential             SessionCredential
}

// EmailChangeResponsibility retains the exact accepted account and new address.
type EmailChangeResponsibility struct {
	RequestID, OperationID, AccountID, ActorSessionID uuid.UUID
	InputSHA256, ProofSHA256, NewEmail                string
	Candidate                                         domain.User
	CloseControl, ReopenControl                       writefence.Control
	Applied, Completed                                bool
	AcceptedAt                                        time.Time
}

// EmailChangeSourceTx records source closure inside the original two-proof Tx.
type EmailChangeSourceTx interface {
	PrepareEmailChange(EmailChangeResponsibility) error
}

// EmailChangeSourceStore owns mailbox reservation, mutation and final source checks.
// The commit callback performs only Identity SQL and notification enqueueing.
type EmailChangeSourceStore interface {
	LoadEmailChange(context.Context, uuid.UUID, []byte, []byte) (EmailChangeResponsibility, bool, error)
	LoadAcceptedEmailChange(context.Context, uuid.UUID) (EmailChangeResponsibility, error)
	CommitEmailChange(context.Context, EmailChangeResponsibility, func(EmailChangeTx, time.Time) error) (EmailChangeResponsibility, error)
	CompleteEmailChange(context.Context, EmailChangeResponsibility) error
}

// EmailChangeSource installs all writers before publishing mailbox success.
type EmailChangeSource struct {
	store     EmailChangeSourceStore
	writers   SessionFenceCoordinator
	protector ProofProtector
}

// EmailChangeRecoveryTask resumes only previously accepted mailbox operations.
const EmailChangeRecoveryTask = "identity.email_change"

// WithSource requires the full source dependency set without a local fallback.
func (s *EmailChange) WithSource(store EmailChangeSourceStore, writers SessionFenceCoordinator, protector ProofProtector) error {
	if store == nil || writers == nil || protector == nil {
		return ErrPasswordChangeUnavailable
	}
	s.source = &EmailChangeSource{store: store, writers: writers, protector: protector}
	return nil
}

// SourceReady selects the original-proof completion route in a trusted root.
func (s *EmailChange) SourceReady() bool { return s != nil && s.source != nil }

// CompleteCommand resumes an accepted request before rechecking its old session
// or proof expiry. A new request retains every original two-mailbox check.
func (s *EmailChange) CompleteCommand(ctx context.Context, input EmailChangeCommand) error {
	q := s.source
	if q == nil || input.RequestID == uuid.Nil || input.OperationID == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	digest := q.protector.Digest("email_change_input", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(digest)
	encoded, err = json.Marshal(input.Credential)
	if err != nil {
		return err
	}
	proof := q.protector.Digest("email_change_credential", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(proof)
	if len(digest) != 32 || len(proof) != 32 {
		return ErrProofInvalid
	}
	p, found, err := q.store.LoadEmailChange(ctx, input.RequestID, digest, proof)
	if err != nil {
		return err
	}
	if found {
		return s.finishSource(ctx, p)
	}
	credential := input.Credential
	if (credential.LegacyToken == "") == (credential.AccessToken == "") || credential.CSRFToken == "" {
		return ErrSessionCSRFInvalid
	}
	var current AuthResult
	if credential.AccessToken != "" {
		current, err = s.auth.CurrentAccess(ctx, credential.AccessToken, credential.CSRFToken, UserActivity)
	} else {
		current, err = s.auth.CurrentWithActivity(ctx, credential.LegacyToken, UserActivity)
		if err == nil && !hmac.Equal([]byte(current.CSRFToken), []byte(credential.CSRFToken)) {
			err = ErrSessionCSRFInvalid
		}
	}
	if err != nil {
		return err
	}
	actor := Principal{ID: current.User.ID, SessionID: current.Session.ID, CredentialEpoch: current.User.SessionEpoch}
	err = s.completePrepared(ctx, actor, input.OperationID, input.Revision, func(tx EmailChangeTx, user domain.User, _, next domain.EmailChallenge, now time.Time) error {
		source, ok := tx.(EmailChangeSourceTx)
		if !ok {
			return ErrPasswordChangeUnavailable
		}
		user.PasswordHash = ""
		p = EmailChangeResponsibility{RequestID: input.RequestID, OperationID: input.OperationID, AccountID: user.ID, ActorSessionID: current.Session.ID, InputSHA256: hex.EncodeToString(digest), ProofSHA256: hex.EncodeToString(proof), Candidate: user, NewEmail: next.EmailKey, AcceptedAt: now.UTC().Truncate(time.Microsecond)}
		return source.PrepareEmailChange(p)
	})
	if err != nil && !errors.Is(err, ErrPasswordChangeExists) {
		return err
	}
	p, found, err = q.store.LoadEmailChange(ctx, input.RequestID, digest, proof)
	if err != nil {
		return err
	}
	if !found {
		return ErrPasswordChangeUnavailable
	}
	return s.finishSource(ctx, p)
}

func (s *EmailChange) finishSource(ctx context.Context, p EmailChangeResponsibility) error {
	if p.RequestID == uuid.Nil || p.OperationID == uuid.Nil || p.AccountID == uuid.Nil || p.CloseControl.Validate() != nil || p.CloseControl.Kind != writefence.Account || p.CloseControl.ID != p.AccountID || !p.CloseControl.Blocked {
		return ErrPasswordChangeUnavailable
	}
	if p.Completed {
		return nil
	}
	expected := p.CloseControl.Authority
	expected.Epoch--
	state, err := s.source.writers.Set(ctx, expected, p.CloseControl.CoordinationID, true)
	if err != nil {
		return err
	}
	if !exactWriterCompletion(state, p.CloseControl) {
		return ErrPasswordChangeUnavailable
	}
	p, err = s.source.store.CommitEmailChange(ctx, p, func(tx EmailChangeTx, now time.Time) error {
		return s.applyEmailChange(tx, p.Candidate.Email, p.NewEmail, p.Candidate.Revision, now)
	})
	if err != nil {
		return err
	}
	if p.ReopenControl.Validate() != nil || p.ReopenControl.Blocked {
		return ErrPasswordChangeUnavailable
	}
	expected = p.ReopenControl.Authority
	expected.Epoch--
	state, err = s.source.writers.Set(ctx, expected, p.ReopenControl.CoordinationID, false)
	if err != nil {
		return err
	}
	if !exactWriterCompletion(state, p.ReopenControl) {
		return ErrPasswordChangeUnavailable
	}
	return s.source.store.CompleteEmailChange(ctx, p)
}

// ResumeEmailChange uses the persisted original operation, never a new actor.
func (s *EmailChange) ResumeEmailChange(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.source == nil || id == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	p, err := s.source.store.LoadAcceptedEmailChange(ctx, id)
	if err != nil {
		return err
	}
	if p.RequestID != id {
		return ErrPasswordChangeUnavailable
	}
	return s.finishSource(ctx, p)
}
