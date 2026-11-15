package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// GovernanceTeamReceipt is Creation's exact retained-resource outcome. Identity
// never calculates team-admin counts or substitutes its own membership facts.
type GovernanceTeamReceipt struct {
	RequestID, AccountID uuid.UUID
	Action, InputSHA256  string
	IdentityClose        writefence.Control
	RejectedDuties       bool
	BlockingTeams, Teams []uuid.UUID
}

// Matches binds the entire original account decision and fixed resource outcome.
func (r GovernanceTeamReceipt) Matches(intent AccountQualificationIntent) bool {
	if r.RequestID != intent.RequestID || r.AccountID != intent.AccountID || r.Action != intent.Action || r.InputSHA256 != intent.InputSHA256 || r.IdentityClose != intent.CloseControl {
		return false
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range r.Teams {
		if id == uuid.Nil || seen[id] {
			return false
		}
		seen[id] = true
	}
	blocked := map[uuid.UUID]bool{}
	for _, id := range r.BlockingTeams {
		if !seen[id] || blocked[id] {
			return false
		}
		blocked[id] = true
	}
	return (!r.RejectedDuties && len(r.BlockingTeams) == 0) || (r.RejectedDuties && intent.Action == "delete" && len(r.BlockingTeams) > 0)
}

// GovernanceTeamParticipant is the fixed Creation responsibility peer. It uses no
// shared credentials and receives neither a browser token nor an Identity actor.
type GovernanceTeamParticipant interface {
	PrepareAccountGovernance(context.Context, AccountQualificationIntent) (GovernanceTeamReceipt, error)
	ResolveAccountGovernance(context.Context, AccountQualificationIntent) (GovernanceTeamReceipt, error)
}

// GovernanceTeamSourceStore adopts only source-owned receipts and durable account
// restoration decisions. It never performs a Creation query or a network call.
type GovernanceTeamSourceStore interface {
	AdoptGovernanceTeamReceipt(context.Context, GovernanceResponsibility, GovernanceTeamReceipt) (GovernanceResponsibility, error)
	CompleteGovernanceRejection(context.Context, GovernanceResponsibility) error
}

// WithTeamGovernance installs the actual Creation resource responsibility. The
// original Identity-only actions remain usable without a silent team fallback.
func (s *Governance) WithTeamGovernance(participant GovernanceTeamParticipant) error {
	if s == nil || s.source == nil || participant == nil {
		return ErrPasswordChangeUnavailable
	}
	if _, ok := s.source.store.(GovernanceTeamSourceStore); !ok {
		return ErrPasswordChangeUnavailable
	}
	s.source.teams = participant
	return nil
}
// SelfDeletionMutationTx applies only a previously accepted, self-bound deletion.
// It does not authorize a platform action or query retained Creation resources.
type SelfDeletionMutationTx interface {
	SoftDeleteSelf(domain.User, time.Time) error
}

func (s *Governance) applyOwnedGovernanceChange(tx GovernanceTx, user domain.User, change UserGovernanceChange, reason string, now time.Time) error {
	if change.Action == "self_delete" {
		self, ok := tx.(SelfDeletionMutationTx)
		if !ok {
			return ErrPasswordChangeUnavailable
		}
		return self.SoftDeleteSelf(user, now)
	}
	if err := tx.ChangeUser(user, change, now); err != nil {
		return err
	}
	if err := tx.AppendGovernanceEvent(&user.ID, nil, "account."+change.Action, reason, now); err != nil {
		return err
	}
	if user.Email != "" {
		return s.notify(tx, user.Email, "account_governance", now)
	}
	return nil
}

func teamGovernanceAction(action string) bool {
	return action == "disable" || action == "enable" || action == "delete" || action == "self_delete"
}
func governancePostActive(p GovernanceResponsibility) bool {
	if p.Rejected {
		return p.Candidate.Status == domain.StatusActive
	}
	switch p.Action {
	case "disable", "delete", "self_delete":
		return false
	case "enable":
		return true
	default:
		return p.Candidate.Status == domain.StatusActive
	}
}
func governanceTeamIntent(p GovernanceResponsibility) AccountQualificationIntent {
	action := p.Action
	if action == "self_delete" {
		action = "delete"
	}
	return AccountQualificationIntent{RequestID: p.RequestID, AccountID: p.Candidate.ID, Action: action, InputSHA256: p.InputSHA256, CloseControl: p.CloseControl, EligibleAfter: p.Action == "enable" && !p.Candidate.EmailVerifiedAt.IsZero() && p.Candidate.DeletedAt.IsZero()}
}
func (s *Governance) ensureGovernanceTeamReceipt(ctx context.Context, p GovernanceResponsibility) (GovernanceResponsibility, error) {
	if !teamGovernanceAction(p.Action) || p.CreationReceipt != nil {
		return p, nil
	}
	if s.source.teams == nil {
		return p, ErrPasswordChangeUnavailable
	}
	intent := governanceTeamIntent(p)
	receipt, err := s.source.teams.PrepareAccountGovernance(ctx, intent)
	if err != nil {
		return p, err
	}
	if !receipt.Matches(intent) {
		return p, ErrPasswordChangeUnavailable
	}
	store, ok := s.source.store.(GovernanceTeamSourceStore)
	if !ok {
		return p, ErrPasswordChangeUnavailable
	}
	return store.AdoptGovernanceTeamReceipt(ctx, p, receipt)
}
func (s *Governance) finishGovernanceRejection(ctx context.Context, p GovernanceResponsibility) error {
	if p.CreationReceipt == nil || !p.CreationReceipt.RejectedDuties || p.Applied || p.ReopenControl.Validate() != nil || p.ReopenControl.Blocked != (p.Candidate.Status == domain.StatusDisabled) {
		return ErrPasswordChangeUnavailable
	}
	expected := p.ReopenControl.Authority
	expected.Epoch--
	state, err := s.source.writers.Set(ctx, expected, p.ReopenControl.CoordinationID, p.ReopenControl.Blocked)
	if err != nil {
		return err
	}
	if !exactWriterCompletion(state, p.ReopenControl) {
		return ErrPasswordChangeUnavailable
	}
	store, ok := s.source.store.(GovernanceTeamSourceStore)
	if !ok {
		return ErrPasswordChangeUnavailable
	}
	if err := store.CompleteGovernanceRejection(ctx, p); err != nil {
		return err
	}
	return ErrDeletionDuties
}
