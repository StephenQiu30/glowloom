package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

var (
	// ErrInvalidGovernance rejects malformed governance requests.
	ErrInvalidGovernance = errors.New("invalid governance input")
	// ErrLastPlatformAdmin protects the final available platform administrator.
	ErrLastPlatformAdmin = errors.New("last available platform administrator")
	// ErrGovernanceConflict rejects expired or incompatible governance state.
	ErrGovernanceConflict = errors.New("governance state conflict")
)

// PlatformSettings is the complete public settings allowlist.
type PlatformSettings struct {
	RegistrationEnabled bool  `json:"registration_enabled"`
	TeamCreationEnabled bool  `json:"team_creation_enabled"`
	Revision            int64 `json:"revision,string" swaggertype:"string"`
}

// AuthorityProposal grants nothing until the named recipient accepts it.
type AuthorityProposal struct {
	ID                      uuid.UUID  `json:"id"`
	Kind                    string     `json:"kind"`
	TeamID                  *uuid.UUID `json:"team_id"`
	SourceUserID            uuid.UUID  `json:"source_user_id"`
	TargetUserID            uuid.UUID  `json:"target_user_id"`
	Status                  string     `json:"status"`
	Reason                  string     `json:"reason"`
	Revision                int64      `json:"revision,string" swaggertype:"string"`
	SourceAuthorityRevision int64      `json:"-"`
	TargetRevision          int64      `json:"-"`
	TargetEmailVersion      int64      `json:"-"`
	TargetCredentialEpoch   int64      `json:"-"`
	TeamRevision            *int64     `json:"-"`
	ExpiresAt               time.Time  `json:"expires_at"`
	CreatedAt               time.Time  `json:"created_at"`
}

// UserGovernanceChange is a closed internal command, never a generic HTTP action.
type UserGovernanceChange struct {
	LoginGuardKey          string
	Action                 string
	ResetName, ResetAvatar bool
}

// TeamGovernance is implemented by workspace on the same locked transaction.
type TeamGovernance interface {
	AccountAccessChanged(uuid.UUID, time.Time) error
	InvitationAuthority(uuid.UUID, uuid.UUID) (int64, error)
	PrepareTransfer(uuid.UUID, uuid.UUID, uuid.UUID, bool, int64) (sourceRevision, teamRevision int64, err error)
	AcceptTransfer(AuthorityProposal, time.Time) error
}

// GovernanceTx keeps authority, audit and encrypted mail atomic.
type GovernanceTx interface {
	EmailTx
	Actor() domain.User
	RequirePlatform() error
	User(uuid.UUID) (domain.User, error)
	AvailablePlatformAdmins() (int64, error)
	ChangeUser(domain.User, UserGovernanceChange, time.Time) error
	Settings() PlatformSettings
	SaveSettings(PlatformSettings, uuid.UUID, time.Time) error
	TeamGovernance() TeamGovernance
	InsertProposal(AuthorityProposal) error
	ProposalReceipt(uuid.UUID, string) (AuthorityProposal, bool, error)
	SaveProposalReceipt(uuid.UUID, string, AuthorityProposal, time.Time) error
	Proposal(uuid.UUID) (AuthorityProposal, error)
	SaveProposal(AuthorityProposal, time.Time) error
	QueryProposals(*uuid.UUID, string, GovernanceFilter, GovernancePosition) ([]AuthorityProposal, error)
	QueryInvitations(*uuid.UUID, GovernanceFilter, GovernancePosition) ([]Invitation, error)
	AppendGovernanceEvent(*uuid.UUID, *uuid.UUID, string, string, time.Time) error
	EnqueueMail(QueuedMail) error
}

// GovernanceStore authenticates inside the shared governance transaction.
type GovernanceStore interface {
	GovernanceTransaction(context.Context, Principal, bool, func() time.Time, func(GovernanceTx) error) error
	PreviewInvitationProof(context.Context, []byte, time.Time) (Invitation, error)
}

// Governance coordinates account policy and explicit authority acceptance.
type Governance struct {
	store     GovernanceStore
	protector ProofProtector
	now       func() time.Time
}

// NewGovernance injects the atomic store, encrypted proof protector and clock.
func NewGovernance(store GovernanceStore, protector ProofProtector, now func() time.Time) *Governance {
	return &Governance{store: store, protector: protector, now: now}
}
func validGovernanceReason(reason string) bool {
	if !utf8.ValidString(reason) || utf8.RuneCountInString(reason) < 5 || utf8.RuneCountInString(reason) > 500 {
		return false
	}
	for _, r := range reason {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Governance) notify(tx GovernanceTx, recipient, template string, now time.Time) error {
	if s.protector == nil {
		return ErrRegistrationUnavailable
	}
	id := uuid.New()
	payload, err := json.Marshal(map[string]string{})
	if err != nil {
		return err
	}
	sealed, err := s.protector.Seal(id, payload)
	if err != nil {
		return err
	}
	return tx.EnqueueMail(QueuedMail{EventID: id, Template: template, Recipient: recipient, KeyID: s.protector.KeyID(), EncryptedPayload: sealed, CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour)})
}
func (s *Governance) mutateUser(ctx context.Context, actor Principal, target uuid.UUID, revision int64, reason string, change UserGovernanceChange) error {
	reason = strings.TrimSpace(reason)
	if target == uuid.Nil || revision < 1 || !validGovernanceReason(reason) {
		return ErrInvalidGovernance
	}
	return s.store.GovernanceTransaction(ctx, actor, true, s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		u, err := tx.User(target)
		if err != nil {
			return err
		}
		if u.Revision != revision {
			return domain.ErrRevisionConflict
		}
		if u.Status == domain.StatusDisabled && change.Action != "enable" && change.Action != "revoke_admin" && change.Action != "profile_reset" {
			return ErrGovernanceConflict
		}
		if change.Action == "enable" && u.Status != domain.StatusDisabled {
			return ErrGovernanceConflict
		}
		if change.Action == "revoke_admin" && u.PlatformRole != "platform_admin" {
			return ErrGovernanceConflict
		}
		if (change.Action == "disable" || change.Action == "revoke_admin") && u.PlatformRole == "platform_admin" && u.Status == domain.StatusActive && !u.EmailVerifiedAt.IsZero() {
			n, err := tx.AvailablePlatformAdmins()
			if err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastPlatformAdmin
			}
		}
		if change.Action == "unlock" {
			if s.protector == nil {
				return ErrRegistrationUnavailable
			}
			change.LoginGuardKey = hex.EncodeToString(s.protector.Digest("login_guard", uuid.Nil, u.Email))
		}
		if err := tx.ChangeUser(u, change, now); err != nil {
			return err
		}
		if change.Action == "disable" || change.Action == "enable" {
			if tx.TeamGovernance() == nil {
				return ErrGovernanceConflict
			}
			if err := tx.TeamGovernance().AccountAccessChanged(target, now); err != nil {
				return err
			}
		}
		if err := tx.AppendGovernanceEvent(&target, nil, "account."+change.Action, reason, now); err != nil {
			return err
		}
		if u.Email != "" {
			return s.notify(tx, u.Email, "account_governance", now)
		}
		return nil
	})
}

// Disable revokes account access and freezes teams that lose their administrator.
func (s *Governance) Disable(ctx context.Context, a Principal, id uuid.UUID, v int64, r string) error {
	return s.mutateUser(ctx, a, id, v, r, UserGovernanceChange{Action: "disable"})
}

// Enable reopens an account without restoring revoked sessions or frozen teams.
func (s *Governance) Enable(ctx context.Context, a Principal, id uuid.UUID, v int64, r string) error {
	return s.mutateUser(ctx, a, id, v, r, UserGovernanceChange{Action: "enable"})
}

// Unlock clears login failure state without changing credentials.
func (s *Governance) Unlock(ctx context.Context, a Principal, id uuid.UUID, v int64, r string) error {
	return s.mutateUser(ctx, a, id, v, r, UserGovernanceChange{Action: "unlock"})
}

// RevokeAdmin removes platform authority while retaining the last available administrator.
func (s *Governance) RevokeAdmin(ctx context.Context, a Principal, id uuid.UUID, v int64, r string) error {
	return s.mutateUser(ctx, a, id, v, r, UserGovernanceChange{Action: "revoke_admin"})
}

// RevokeSessions revokes all sessions without changing the account credential.
func (s *Governance) RevokeSessions(ctx context.Context, a Principal, id uuid.UUID, v int64, r string) error {
	return s.mutateUser(ctx, a, id, v, r, UserGovernanceChange{Action: "revoke_sessions"})
}

// ResetProfile resets the selected public profile fields without changing account credentials.
func (s *Governance) ResetProfile(ctx context.Context, a Principal, id uuid.UUID, v int64, r string, name, avatar bool) error {
	if !name && !avatar {
		return ErrInvalidGovernance
	}
	return s.mutateUser(ctx, a, id, v, r, UserGovernanceChange{Action: "profile_reset", ResetName: name, ResetAvatar: avatar})
}

// Settings reads current platform policy under current platform authorization.
func (s *Governance) Settings(ctx context.Context, a Principal) (PlatformSettings, error) {
	var out PlatformSettings
	err := s.store.GovernanceTransaction(ctx, a, false, s.now, func(tx GovernanceTx) error {
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		out = tx.Settings()
		return nil
	})
	return out, err
}

// UpdateSettings changes explicit platform policy fields with version checking and audit.
func (s *Governance) UpdateSettings(ctx context.Context, a Principal, revision int64, registration, teamCreation *bool, reason string) (PlatformSettings, error) {
	var out PlatformSettings
	reason = strings.TrimSpace(reason)
	if revision < 1 || !validGovernanceReason(reason) || (registration == nil && teamCreation == nil) {
		return out, ErrInvalidGovernance
	}
	err := s.store.GovernanceTransaction(ctx, a, true, s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		out = tx.Settings()
		if out.Revision != revision {
			return domain.ErrRevisionConflict
		}
		if registration != nil {
			out.RegistrationEnabled = *registration
		}
		if teamCreation != nil {
			out.TeamCreationEnabled = *teamCreation
		}
		out.Revision++
		if err := tx.SaveSettings(out, a.ID, now); err != nil {
			return err
		}
		if err := tx.AppendGovernanceEvent(nil, nil, "platform.settings_updated", reason, now); err != nil {
			return err
		}
		return s.notify(tx, tx.Actor().Email, "account_governance", now)
	})
	return out, err
}

// ProposeAdmin records a platform grant requiring explicit recipient acceptance.
func (s *Governance) ProposeAdmin(ctx context.Context, a Principal, target uuid.UUID, revision int64, reason string) (AuthorityProposal, error) {
	return s.propose(ctx, a, uuid.New(), target, nil, revision, reason, "platform_grant")
}

// ProposeTransfer records a team administrator handover without immediately changing roles.
func (s *Governance) ProposeTransfer(ctx context.Context, a Principal, team, target uuid.UUID, revision int64, reason string) (AuthorityProposal, error) {
	return s.propose(ctx, a, uuid.New(), target, &team, revision, reason, "team_transfer")
}

// ProposeTakeover records a platform recovery proposal for a frozen team.
func (s *Governance) ProposeTakeover(ctx context.Context, a Principal, team, target uuid.UUID, revision int64, reason string) (AuthorityProposal, error) {
	return s.propose(ctx, a, uuid.New(), target, &team, revision, reason, "platform_takeover")
}
func (s *Governance) propose(ctx context.Context, a Principal, key, target uuid.UUID, team *uuid.UUID, revision int64, reason, kind string) (AuthorityProposal, error) {
	var out AuthorityProposal
	reason = strings.TrimSpace(reason)
	if key == uuid.Nil || target == uuid.Nil || target == a.ID || revision < 1 || !validGovernanceReason(reason) {
		return out, ErrInvalidGovernance
	}
	err := s.store.GovernanceTransaction(ctx, a, true, s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		if kind != "team_transfer" {
			if err := tx.RequirePlatform(); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal([]any{target, team, revision, reason, kind})
		sum := sha256.Sum256(raw)
		requestHash := hex.EncodeToString(sum[:])
		// Replay is still authorized against the current route scope.
		if kind == "team_transfer" {
			gateway, ok := tx.TeamGovernance().(TeamInvitationGateway)
			if !ok {
				return ErrGovernanceConflict
			}
			if err := gateway.AuthorizeInvitations(*team, a.ID); err != nil {
				return err
			}
		}
		prior, found, err := tx.ProposalReceipt(key, requestHash)
		if err != nil {
			return err
		}
		if found {
			out = prior
			return nil
		}
		u, err := tx.User(target)
		if err != nil {
			return err
		}
		if u.Status != domain.StatusActive || u.EmailVerifiedAt.IsZero() {
			return ErrGovernanceConflict
		}
		out = AuthorityProposal{ID: uuid.New(), Kind: kind, TeamID: team, SourceUserID: a.ID, TargetUserID: target, Reason: reason, Status: "pending", Revision: 1, SourceAuthorityRevision: tx.Actor().PlatformEpoch, TargetRevision: u.PlatformEpoch, TargetEmailVersion: u.EmailVersion, TargetCredentialEpoch: u.SessionEpoch, ExpiresAt: now.Add(48 * time.Hour), CreatedAt: now}
		if kind == "platform_grant" {
			if u.Revision != revision {
				return domain.ErrRevisionConflict
			}
			if u.PlatformRole == "platform_admin" {
				return ErrGovernanceConflict
			}
		} else {
			if tx.TeamGovernance() == nil {
				return ErrGovernanceConflict
			}
			src, tr, err := tx.TeamGovernance().PrepareTransfer(*team, a.ID, target, kind == "platform_takeover", revision)
			if err != nil {
				return err
			}
			out.TeamRevision = &tr
			if kind == "team_transfer" {
				out.SourceAuthorityRevision = src
			}
		}
		if err := tx.InsertProposal(out); err != nil {
			return err
		}
		if err := tx.AppendGovernanceEvent(&target, team, "authority.proposed", reason, now); err != nil {
			return err
		}
		if err := s.notify(tx, u.Email, "authority_proposal", now); err != nil {
			return err
		}
		return tx.SaveProposalReceipt(key, requestHash, out, now)
	})
	return out, err
}

// AcceptProposal requires recent proof and rechecks source authority before granting access.
func (s *Governance) AcceptProposal(ctx context.Context, a Principal, id uuid.UUID, revision int64) error {
	return s.finishProposal(ctx, a, id, revision, "accepted", nil, "")
}

// DeclineProposal lets only the named target reject a pending proposal.
func (s *Governance) DeclineProposal(ctx context.Context, a Principal, id uuid.UUID, revision int64) error {
	return s.finishProposal(ctx, a, id, revision, "declined", nil, "")
}

// CancelProposal lets the still-authorized source withdraw a platform grant.
func (s *Governance) CancelProposal(ctx context.Context, a Principal, id uuid.UUID, revision int64, reason string) error {
	return s.finishProposal(ctx, a, id, revision, "cancelled", nil, reason)
}
func (s *Governance) finishProposal(ctx context.Context, a Principal, id uuid.UUID, revision int64, status string, expectedTeam *uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if status == "cancelled" && !validGovernanceReason(reason) {
		return ErrInvalidGovernance
	}
	if id == uuid.Nil || revision < 1 {
		return ErrInvalidGovernance
	}
	return s.store.GovernanceTransaction(ctx, a, status != "declined", s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		p, err := tx.Proposal(id)
		if err != nil {
			return err
		}
		if status == "cancelled" && ((expectedTeam == nil && p.Kind != "platform_grant") || (expectedTeam != nil && (p.Kind != "team_transfer" || p.TeamID == nil || *p.TeamID != *expectedTeam))) {
			return ErrForbidden
		}
		if status == "cancelled" {
			if p.SourceUserID != a.ID {
				return ErrForbidden
			}
		} else if p.TargetUserID != a.ID {
			return ErrForbidden
		}
		if p.Revision != revision {
			return domain.ErrRevisionConflict
		}
		if p.Status != "pending" || !now.Before(p.ExpiresAt) {
			return ErrGovernanceConflict
		}
		if status == "accepted" || status == "cancelled" {
			source, err := tx.User(p.SourceUserID)
			if err != nil {
				return err
			}
			if source.Status != domain.StatusActive || source.EmailVerifiedAt.IsZero() {
				return ErrForbidden
			}
			if p.Kind != "team_transfer" && (source.PlatformRole != "platform_admin" || source.PlatformEpoch != p.SourceAuthorityRevision) {
				return ErrForbidden
			}
			if status == "accepted" {
				target, err := tx.User(p.TargetUserID)
				if err != nil {
					return err
				}
				if target.PlatformEpoch != p.TargetRevision || target.EmailVersion != p.TargetEmailVersion || target.SessionEpoch != p.TargetCredentialEpoch {
					return domain.ErrRevisionConflict
				}
				if p.Kind == "platform_grant" {
					if err := tx.ChangeUser(target, UserGovernanceChange{Action: "grant_admin"}, now); err != nil {
						return err
					}
				} else {
					if tx.TeamGovernance() == nil {
						return ErrGovernanceConflict
					}
					if err := tx.TeamGovernance().AcceptTransfer(p, now); err != nil {
						return err
					}
				}
			} else if p.Kind == "team_transfer" {
				if tx.TeamGovernance() == nil {
					return ErrGovernanceConflict
				}
				gateway, ok := tx.TeamGovernance().(TeamInvitationGateway)
				if !ok {
					return ErrGovernanceConflict
				}
				if err := gateway.AuthorizeInvitations(*p.TeamID, p.SourceUserID); err != nil {
					return err
				}
			}
		}
		p.Status = status
		p.Revision++
		if err := tx.SaveProposal(p, now); err != nil {
			return err
		}
		auditReason := p.Reason
		if status == "cancelled" {
			auditReason = reason
		}
		if err := tx.AppendGovernanceEvent(&p.TargetUserID, p.TeamID, "authority."+status, auditReason, now); err != nil {
			return err
		}
		target, err := tx.User(p.TargetUserID)
		if err != nil {
			return err
		}
		if err := s.notify(tx, target.Email, "account_governance", now); err != nil {
			return err
		}
		source, err := tx.User(p.SourceUserID)
		if err != nil {
			return err
		}
		return s.notify(tx, source.Email, "account_governance", now)
	})
}

// CancelTransfer withdraws only a proposal belonging to the route's team.
func (s *Governance) CancelTransfer(ctx context.Context, a Principal, team, id uuid.UUID, revision int64, reason string) error {
	return s.finishProposal(ctx, a, id, revision, "cancelled", &team, reason)
}

// ProposeAdminWithKey replays the original result without duplicate grant or notification.
func (s *Governance) ProposeAdminWithKey(ctx context.Context, a Principal, key, target uuid.UUID, revision int64, reason string) (AuthorityProposal, error) {
	return s.propose(ctx, a, key, target, nil, revision, reason, "platform_grant")
}

// ProposeTransferWithKey creates one explicit handover for a request key.
func (s *Governance) ProposeTransferWithKey(ctx context.Context, a Principal, key, team, target uuid.UUID, revision int64, reason string) (AuthorityProposal, error) {
	return s.propose(ctx, a, key, target, &team, revision, reason, "team_transfer")
}

// ProposeTakeoverWithKey creates one explicit recovery proposal for a request key.
func (s *Governance) ProposeTakeoverWithKey(ctx context.Context, a Principal, key, team, target uuid.UUID, revision int64, reason string) (AuthorityProposal, error) {
	return s.propose(ctx, a, key, target, &team, revision, reason, "platform_takeover")
}
