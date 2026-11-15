package application

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// GovernanceSourceCommand retains the public explicit action, target and version.
// Original browser credentials never leave Identity or enter its durable journal.
type GovernanceSourceCommand struct {
	RequestID, TargetID uuid.UUID
	Revision            int64
	Action, Reason      string
	Credential          SessionCredential
}

// GovernanceResponsibility is one accepted platform qualification decision.
type GovernanceResponsibility struct {
	RequestID, ActorSessionID                uuid.UUID
	ProofOperationID                         uuid.UUID
	Actor, Candidate                         domain.User
	Action, Reason, InputSHA256, ProofSHA256 string
	CloseControl, ReopenControl              writefence.Control
	Applied, Completed                       bool
	Rejected                                 bool
	CreationReceipt                          *GovernanceTeamReceipt
	AcceptedAt                               time.Time
}

// GovernanceSourceTx persists an already authorized original platform decision.
type GovernanceSourceTx interface {
	PrepareGovernance(GovernanceResponsibility) error
}

// GovernanceSourceStore owns only Identity platform qualification effects.
// Commit runs only the original local mutation, audit and notification callback.
type GovernanceSourceStore interface {
	LoadGovernance(context.Context, uuid.UUID, []byte, []byte) (GovernanceResponsibility, bool, error)
	LoadAcceptedGovernance(context.Context, uuid.UUID) (GovernanceResponsibility, error)
	CommitGovernance(context.Context, GovernanceResponsibility, func(GovernanceTx, domain.User, time.Time) error) (GovernanceResponsibility, error)
	CompleteGovernance(context.Context, GovernanceResponsibility, UserCommandReceipt) error
}

// GovernanceSource closes all writers before the original local qualification change.
type GovernanceSource struct {
	auth      *Auth
	store     GovernanceSourceStore
	writers   SessionFenceCoordinator
	protector ProofProtector
	teams     GovernanceTeamParticipant
}

// GovernanceRecoveryTask identifies previously accepted platform decisions.
const GovernanceRecoveryTask = "identity.platform_qualification"

// WithSource requires every original-proof and source coordination dependency.
func (s *Governance) WithSource(auth *Auth, store GovernanceSourceStore, writers SessionFenceCoordinator, protector ProofProtector) error {
	if auth == nil || store == nil || writers == nil || protector == nil {
		return ErrPasswordChangeUnavailable
	}
	s.source = &GovernanceSource{auth: auth, store: store, writers: writers, protector: protector}
	return nil
}

// ExecuteSourceCommand handles explicit source-coordinated platform actions.
// Team-dependent mutations require the fixed Creation receipt participant;
// unknown action strings cannot enter this protocol.
func (s *Governance) ExecuteSourceCommand(ctx context.Context, input GovernanceSourceCommand) error {
	q := s.source
	if q == nil || input.RequestID == uuid.Nil || input.TargetID == uuid.Nil || input.Action == "self_delete" || (input.Action != "revoke_sessions" && input.Action != "revoke_admin" && !teamGovernanceAction(input.Action)) || (teamGovernanceAction(input.Action) && q.teams == nil) {
		return ErrInvalidGovernance
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	digest := q.protector.Digest("platform_qualification_input", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(digest)
	encoded, err = json.Marshal(input.Credential)
	if err != nil {
		return err
	}
	proof := q.protector.Digest("platform_qualification_credential", input.RequestID, string(encoded))
	clear(encoded)
	defer clear(proof)
	if len(digest) != 32 || len(proof) != 32 {
		return ErrProofInvalid
	}
	p, found, err := q.store.LoadGovernance(ctx, input.RequestID, digest, proof)
	if err != nil {
		return err
	}
	if found {
		return s.finishGovernance(ctx, p)
	}
	credential := input.Credential
	if (credential.LegacyToken == "") == (credential.AccessToken == "") || credential.CSRFToken == "" {
		return ErrSessionCSRFInvalid
	}
	var current AuthResult
	if credential.AccessToken != "" {
		current, err = q.auth.CurrentAccess(ctx, credential.AccessToken, credential.CSRFToken, UserActivity)
	} else {
		current, err = q.auth.CurrentWithActivity(ctx, credential.LegacyToken, UserActivity)
		if err == nil && !hmac.Equal([]byte(current.CSRFToken), []byte(credential.CSRFToken)) {
			err = ErrSessionCSRFInvalid
		}
	}
	if err != nil {
		return err
	}
	if input.Action == "revoke_sessions" && input.TargetID == current.User.ID {
		return ErrInvalidGovernance
	}
	actor := Principal{ID: current.User.ID, SessionID: current.Session.ID, CredentialEpoch: current.User.SessionEpoch, PlatformEpoch: current.User.PlatformEpoch}
	err = s.mutateUserPrepared(ctx, actor, input.TargetID, input.Revision, input.Reason, UserGovernanceChange{Action: input.Action}, func(tx GovernanceTx, target domain.User, now time.Time) error {
		source, ok := tx.(GovernanceSourceTx)
		if !ok {
			return ErrPasswordChangeUnavailable
		}
		author := tx.Actor()
		author.PasswordHash, target.PasswordHash = "", ""
		p = GovernanceResponsibility{RequestID: input.RequestID, ActorSessionID: actor.SessionID, Actor: author, Candidate: target, Action: input.Action, Reason: strings.TrimSpace(input.Reason), InputSHA256: hex.EncodeToString(digest), ProofSHA256: hex.EncodeToString(proof), AcceptedAt: now.UTC().Truncate(time.Microsecond)}
		return source.PrepareGovernance(p)
	})
	if err != nil && !errors.Is(err, ErrPasswordChangeExists) {
		return err
	}
	p, found, err = q.store.LoadGovernance(ctx, input.RequestID, digest, proof)
	if err != nil {
		return err
	}
	if !found {
		return ErrPasswordChangeUnavailable
	}
	return s.finishGovernance(ctx, p)
}
func (s *Governance) finishGovernance(ctx context.Context, p GovernanceResponsibility) error {
	if p.RequestID == uuid.Nil || p.Candidate.ID == uuid.Nil || p.CloseControl.Validate() != nil || p.CloseControl.Kind != writefence.Account || p.CloseControl.ID != p.Candidate.ID || !p.CloseControl.Blocked {
		return ErrPasswordChangeUnavailable
	}
	if p.Completed {
		if p.Rejected {
			return ErrDeletionDuties
		}
		return nil
	}
	if p.Rejected {
		return s.finishGovernanceRejection(ctx, p)
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
	p, err = s.ensureGovernanceTeamReceipt(ctx, p)
	if err != nil {
		return err
	}
	if p.Rejected {
		return s.finishGovernanceRejection(ctx, p)
	}
	p, err = s.source.store.CommitGovernance(ctx, p, func(tx GovernanceTx, user domain.User, now time.Time) error {
		if teamGovernanceAction(p.Action) {
			return s.applyOwnedGovernanceChange(tx, user, UserGovernanceChange{Action: p.Action}, p.Reason, now)
		}
		return s.applyGovernanceChange(tx, user, UserGovernanceChange{Action: p.Action}, p.Reason, now)
	})
	if err != nil {
		return err
	}
	if governancePostActive(p) {
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
	} else if p.ReopenControl.CoordinationID != uuid.Nil || (p.Candidate.Status != domain.StatusDisabled && p.Candidate.Status != domain.StatusActive) {
		return ErrPasswordChangeUnavailable
	}
	receipt := UserCommandReceipt{RequestID: p.RequestID, TargetID: p.Candidate.ID, Action: p.Action, RequestHash: userCommandHash(p.Candidate.ID, p.Candidate.Revision, p.Reason, p.Action)}
	return s.source.store.CompleteGovernance(ctx, p, receipt)
}

// ResumeGovernance resumes only the saved original platform decision. A completed
// duties rejection is terminal work; its public command still returns the conflict.
func (s *Governance) ResumeGovernance(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.source == nil || id == uuid.Nil {
		return ErrPasswordChangeUnavailable
	}
	p, err := s.source.store.LoadAcceptedGovernance(ctx, id)
	if err != nil {
		return err
	}
	if p.RequestID != id {
		return ErrPasswordChangeUnavailable
	}
	if err := s.finishGovernance(ctx, p); err != nil {
		if !errors.Is(err, ErrDeletionDuties) {
			return err
		}
		terminal, err := s.source.store.LoadAcceptedGovernance(ctx, id)
		if err != nil {
			return err
		}
		if terminal.RequestID != id || !terminal.Completed || !terminal.Rejected {
			return ErrPasswordChangeUnavailable
		}
	}
	return nil
}
