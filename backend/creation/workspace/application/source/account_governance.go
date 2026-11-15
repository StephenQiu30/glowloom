package source

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// AccountGovernanceIntent is the exact original Identity account closure. It is
// received from the fixed authenticated source, never from a public Actor/header.
type AccountGovernanceIntent struct {
	RequestID, AccountID uuid.UUID
	Action, InputSHA256  string
	CloseControl         writefence.Control
	EligibleAfter        bool
}

// Validate accepts only the three retained-resource account effects.
func (p AccountGovernanceIntent) Validate() error {
	if p.RequestID == uuid.Nil || p.AccountID == uuid.Nil || !governanceDigest(p.InputSHA256) || p.CloseControl.Validate() != nil || p.CloseControl.Issuer != writefence.Identity || p.CloseControl.Kind != writefence.Account || p.CloseControl.ID != p.AccountID || !p.CloseControl.Blocked {
		return writefence.ErrInvalid
	}
	switch p.Action {
	case "disable", "delete":
		if p.EligibleAfter {
			return writefence.ErrInvalid
		}
	case "enable":
	default:
		return writefence.ErrInvalid
	}
	return nil
}

// GovernanceMember is an exact Creation member fact, not an Identity eligibility vote.
type GovernanceMember struct {
	ID                  uuid.UUID
	Role, Status        string
	Revision, AuthEpoch int64
}

// GovernanceTeam freezes only management facts while a responsibility is pending.
// Content commands keep their existing scope generation unless a real freeze wins.
type GovernanceTeam struct {
	ID, ScopeID         uuid.UUID
	Status              string
	GovernanceBlocked   bool
	Revision, AuthEpoch int64
	Target              GovernanceMember
	Candidates          []GovernanceMember
	SnapshotSHA256      string
}

// GovernanceQualification is an account-only LastTeamAdministrator proof. It
// conveys no session, team role or general business-write authorization.
type GovernanceQualification struct {
	ID, PreviousProofID uuid.UUID
	Intent              AccountGovernanceIntent
	TeamID, CandidateID uuid.UUID
	SnapshotSHA256      string
	Eligible            bool
	Control             writefence.Control
	Command             writefence.Command
}

// Validate checks the source purpose and exact account-only command binding.
func (p GovernanceQualification) Validate() error {
	if p.ID == uuid.Nil || p.PreviousProofID == p.ID || p.Intent.Validate() != nil || p.TeamID == uuid.Nil || p.CandidateID == uuid.Nil || p.CandidateID == p.Intent.AccountID || !governanceDigest(p.SnapshotSHA256) || p.Control.Validate() != nil || p.Control.Issuer != writefence.Identity || p.Control.Kind != writefence.Account || p.Control.ID != p.CandidateID || p.Control.Blocked == p.Eligible || p.Command.ID != p.ID || !governanceDigest(p.Command.SHA256) || len(p.Command.Authorities) != 1 || p.Command.Authorities[0] != p.Control.Authority {
		return writefence.ErrInvalid
	}
	action := "identity.team_administrator.ineligible"
	if p.Eligible {
		action = "identity.team_administrator.eligible"
	}
	if p.Command.Action != action {
		return writefence.ErrInvalid
	}
	_, err := p.Command.AuthorityDigest()
	return err
}

// GovernanceQualificationRequest explicitly binds a read/successor to the fixed
// candidate set and original operation; an older command is never rewritten.
type GovernanceQualificationRequest struct {
	RequestID, TeamID, CandidateID, PreviousProofID uuid.UUID
	SnapshotSHA256                                  string
}

// AccountGovernanceReceipt describes only actual original local outcomes.
// RejectedDuties never closes/reopens a shared scope or cancels another member's work.
type AccountGovernanceReceipt struct {
	RequestID, AccountID uuid.UUID
	Action, InputSHA256  string
	IdentityClose        writefence.Control
	RejectedDuties       bool
	BlockingTeams        []uuid.UUID
	Teams                []uuid.UUID
}

// AccountGovernancePreparation retains the fixed team/member snapshot, proof
// history and only genuinely permanent scope-freeze decisions.
type AccountGovernancePreparation struct {
	Intent         AccountGovernanceIntent
	Teams          []GovernanceTeam
	Proofs         []GovernanceQualification
	FreezeControls []writefence.Control
	Applied        bool
	Receipt        *AccountGovernanceReceipt
	AcceptedAt     time.Time
}

// GovernanceQualificationSource consumes the unique generated Identity contract
// before SQL starts. Resolve and explicit successor reads cannot mint a session.
type GovernanceQualificationSource interface {
	ReadAccountGovernanceIntent(context.Context, uuid.UUID) (AccountGovernanceIntent, error)
	ReadGovernanceQualification(context.Context, GovernanceQualificationRequest) (GovernanceQualification, error)
}

// AccountGovernanceStore owns the real pending latch and final local effect.
// Apply must lock the original team/member snapshot, source target barrier and
// exact positive Guard or blocked negative candidate fence in one transaction.
// It commits actual effects and permanent-freeze source decisions together.
type AccountGovernanceStore interface {
	AcceptAccountGovernance(context.Context, AccountGovernanceIntent) (AccountGovernancePreparation, error)
	LoadAccountGovernance(context.Context, uuid.UUID) (AccountGovernancePreparation, error)
	SaveGovernanceQualification(context.Context, AccountGovernancePreparation, GovernanceQualification) error
	ApplyAccountGovernance(context.Context, AccountGovernancePreparation, []GovernanceQualification) (AccountGovernancePreparation, error)
	CompleteAccountGovernance(context.Context, AccountGovernancePreparation) (AccountGovernanceReceipt, error)
}

// GovernanceScopeInstaller propagates only an actual permanent freeze.
type GovernanceScopeInstaller interface {
	Set(context.Context, writefence.Authority, uuid.UUID, bool) (writefence.Coordination, error)
}

// AccountGovernance completes the retained team effects of an Identity account
// responsibility. There is deliberately no generic operation executor or preparing DTO.
type AccountGovernance struct {
	store    AccountGovernanceStore
	identity GovernanceQualificationSource
	writers  GovernanceScopeInstaller
}

// AccountGovernanceRecoveryTask resumes only original Creation responsibilities.
const AccountGovernanceRecoveryTask = "creation.account_governance"

// NewAccountGovernance requires the actual resource store, fixed Identity peer
// and Creation issuer's four-writer coordinator, without a legacy fallback.
func NewAccountGovernance(store AccountGovernanceStore, identity GovernanceQualificationSource, writers GovernanceScopeInstaller) (*AccountGovernance, error) {
	if store == nil || identity == nil || writers == nil {
		return nil, writefence.ErrUnavailable
	}
	return &AccountGovernance{store: store, identity: identity, writers: writers}, nil
}

// Prepare authenticates a new intent at its source before accepting local facts.
// An existing original receipt is immutable even after later account changes.
func (s *AccountGovernance) Prepare(ctx context.Context, intent AccountGovernanceIntent) (AccountGovernanceReceipt, error) {
	if s == nil || s.store == nil || s.identity == nil || s.writers == nil {
		return AccountGovernanceReceipt{}, writefence.ErrUnavailable
	}
	if err := intent.Validate(); err != nil {
		return AccountGovernanceReceipt{}, err
	}
	p, err := s.store.LoadAccountGovernance(ctx, intent.RequestID)
	if errors.Is(err, writefence.ErrNotFound) {
		actual, readErr := s.identity.ReadAccountGovernanceIntent(ctx, intent.RequestID)
		if readErr != nil {
			return AccountGovernanceReceipt{}, readErr
		}
		if actual != intent {
			return AccountGovernanceReceipt{}, writefence.ErrConflict
		}
		p, err = s.store.AcceptAccountGovernance(ctx, intent)
	}
	if err != nil {
		return AccountGovernanceReceipt{}, err
	}
	if p.Intent != intent {
		return AccountGovernanceReceipt{}, writefence.ErrConflict
	}
	return s.resume(ctx, p)
}

// Resolve continues only a persisted original operation and never changes its inputs.
func (s *AccountGovernance) Resolve(ctx context.Context, id uuid.UUID) (AccountGovernanceReceipt, error) {
	if s == nil || s.store == nil || id == uuid.Nil {
		return AccountGovernanceReceipt{}, writefence.ErrInvalid
	}
	p, err := s.store.LoadAccountGovernance(ctx, id)
	if err != nil {
		return AccountGovernanceReceipt{}, err
	}
	if p.Intent.RequestID != id {
		return AccountGovernanceReceipt{}, writefence.ErrConflict
	}
	return s.resume(ctx, p)
}

// ResumeAccountGovernance is the owned durable-worker handler, not a public API.
func (s *AccountGovernance) ResumeAccountGovernance(ctx context.Context, id uuid.UUID) error {
	_, err := s.Resolve(ctx, id)
	return err
}

func (s *AccountGovernance) resume(ctx context.Context, p AccountGovernancePreparation) (AccountGovernanceReceipt, error) {
	if p.Intent.Validate() != nil {
		return AccountGovernanceReceipt{}, writefence.ErrInvalid
	}
	if p.Receipt != nil {
		return *p.Receipt, nil
	}
	if !p.Applied {
		proofs, err := s.qualifications(ctx, p)
		if err != nil {
			return AccountGovernanceReceipt{}, err
		}
		p, err = s.store.ApplyAccountGovernance(ctx, p, proofs)
		if err != nil {
			return AccountGovernanceReceipt{}, err
		}
		if p.Receipt != nil {
			return *p.Receipt, nil
		}
		if !p.Applied {
			return AccountGovernanceReceipt{}, writefence.ErrUnavailable
		}
	}
	for _, control := range p.FreezeControls {
		if control.Validate() != nil || control.Issuer != writefence.Creation || control.Kind != writefence.Scope || !control.Blocked {
			return AccountGovernanceReceipt{}, writefence.ErrInvalid
		}
		expected := control.Authority
		expected.Epoch--
		state, err := s.writers.Set(ctx, expected, control.CoordinationID, true)
		if err != nil {
			return AccountGovernanceReceipt{}, err
		}
		if !state.Complete || state.Control != control || len(state.Acknowledged) != len(writefence.Writers()) {
			return AccountGovernanceReceipt{}, writefence.ErrUnavailable
		}
		seen := map[writefence.Writer]bool{}
		for _, writer := range state.Acknowledged {
			if writer.Validate() != nil || seen[writer] {
				return AccountGovernanceReceipt{}, writefence.ErrUnavailable
			}
			seen[writer] = true
		}
	}
	return s.store.CompleteAccountGovernance(ctx, p)
}

func (s *AccountGovernance) qualifications(ctx context.Context, p AccountGovernancePreparation) ([]GovernanceQualification, error) {
	var out []GovernanceQualification
	for _, team := range p.Teams {
		if p.Intent.Action == "delete" && (team.Target.Role != "team_admin" || team.Target.Status != "active") {
			continue
		}
		if p.Intent.Action == "enable" && p.Intent.EligibleAfter && team.Target.Role == "team_admin" && team.Target.Status == "active" {
			continue
		}
		if p.Intent.Action != "delete" && team.GovernanceBlocked {
			continue
		}
		var unavailable error
		positive := false
		for _, candidate := range team.Candidates {
			request := GovernanceQualificationRequest{RequestID: p.Intent.RequestID, TeamID: team.ID, CandidateID: candidate.ID, SnapshotSHA256: team.SnapshotSHA256}
			var latestEpoch int64
			for _, old := range p.Proofs {
				if old.TeamID == team.ID && old.CandidateID == candidate.ID && old.Control.Epoch > latestEpoch {
					request.PreviousProofID = old.ID
					latestEpoch = old.Control.Epoch
				}
			}
			proof, err := s.identity.ReadGovernanceQualification(ctx, request)
			if err != nil {
				unavailable = err
				continue
			}
			if proof.Intent != p.Intent || proof.TeamID != team.ID || proof.CandidateID != candidate.ID || proof.SnapshotSHA256 != team.SnapshotSHA256 {
				return nil, writefence.ErrConflict
			}
			if err := s.store.SaveGovernanceQualification(ctx, p, proof); err != nil {
				return nil, err
			}
			out = append(out, proof)
			if proof.Eligible {
				positive = true
				break
			}
		}
		if !positive && unavailable != nil {
			return nil, unavailable
		}
	}
	return out, nil
}

func governanceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
