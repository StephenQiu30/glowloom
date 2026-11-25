package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// Invitation is a metadata projection; token and binding versions never leave persistence.
type Invitation struct {
	ID                 uuid.UUID  `json:"id"`
	Kind               string     `json:"kind"`
	TeamID             *uuid.UUID `json:"team_id"`
	EmailKey           string     `json:"email"`
	TeamRole           *string    `json:"team_role"`
	InviterID          uuid.UUID  `json:"inviter_id"`
	Status             string     `json:"status"`
	Revision           int64      `json:"revision,string" swaggertype:"string"`
	ExpiresAt          time.Time  `json:"expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
	TokenHash          []byte     `json:"-"`
	MembershipRevision *int64     `json:"-"`
	AcceptedUserID     *uuid.UUID `json:"-"`
	PlatformIssued     bool       `json:"-"`
}

// InvitationTx is the small invitation-specific transaction extension.
type InvitationTx interface {
	GovernanceTx
	Invitation(uuid.UUID, []byte) (Invitation, error)
	InsertInvitation(Invitation) error
	SaveInvitation(Invitation, time.Time) error
	InvitationReceipt(uuid.UUID, string) (Invitation, bool, error)
	SaveInvitationReceipt(uuid.UUID, string, Invitation, time.Time) error
	ReserveInvitationMail(string, time.Time) error
}

// TeamInvitationGateway keeps team membership and capacity rules in workspace.
type TeamInvitationGateway interface {
	AuthorizeInvitations(uuid.UUID, uuid.UUID) error
	CheckInvitation(uuid.UUID, uuid.UUID, string, string) (*int64, error)
	CheckPlatformInvitation(uuid.UUID, string, string) (*int64, error)
	AcceptInvitation(Invitation, uuid.UUID, time.Time) error
}

// CreateAccountInvitation invites an ordinary account without setting its password or role.
func (s *Governance) CreateAccountInvitation(ctx context.Context, a Principal, key uuid.UUID, email string) (Invitation, error) {
	return s.createInvitation(ctx, a, key, nil, email, "", false, "")
}

// CreateTeamInvitation creates an email-bound team invitation with current membership version.
func (s *Governance) CreateTeamInvitation(ctx context.Context, a Principal, key, team uuid.UUID, email, role string) (Invitation, error) {
	return s.createInvitation(ctx, a, key, &team, email, role, false, "")
}
func (s *Governance) createInvitation(ctx context.Context, a Principal, key uuid.UUID, team *uuid.UUID, email, role string, platform bool, reason string) (Invitation, error) {
	var out Invitation
	normalized, err := domain.NormalizeEmail(email)
	if err != nil || key == uuid.Nil || platform && !validGovernanceReason(strings.TrimSpace(reason)) {
		return out, ErrInvalidGovernance
	}
	if team != nil && (role != "member" && role != "team_admin") {
		return out, ErrInvalidGovernance
	}
	raw, _ := json.Marshal([]any{team, normalized, role, platform, reason})
	hash := sha256.Sum256(raw)
	requestHash := hex.EncodeToString(hash[:])
	err = s.store.GovernanceTransaction(ctx, a, team == nil || platform || role == "team_admin", s.now, func(base GovernanceTx) error {
		now := s.now().UTC()
		tx, ok := base.(InvitationTx)
		if !ok {
			return ErrGovernanceConflict
		}
		var membership *int64
		if team == nil || platform {
			if err := tx.RequirePlatform(); err != nil {
				return err
			}
		} else {
			gateway, ok := tx.TeamGovernance().(TeamInvitationGateway)
			if !ok {
				return ErrGovernanceConflict
			}
			if err := gateway.AuthorizeInvitations(*team, a.ID); err != nil {
				return err
			}
		}
		prior, found, err := tx.InvitationReceipt(key, requestHash)
		if err != nil {
			return err
		}
		if found {
			out = prior
			return nil
		}
		if team != nil {
			gateway := tx.TeamGovernance().(TeamInvitationGateway)
			var err error
			if platform {
				membership, err = gateway.CheckPlatformInvitation(*team, normalized, role)
			} else {
				membership, err = gateway.CheckInvitation(*team, a.ID, normalized, role)
			}
			if err != nil {
				return err
			}
		}
		out = Invitation{ID: uuid.New(), Kind: "account", TeamID: team, EmailKey: normalized, InviterID: a.ID, Status: "pending", Revision: 1, ExpiresAt: now.Add(48 * time.Hour), CreatedAt: now, MembershipRevision: membership, PlatformIssued: platform}
		if team != nil {
			out.Kind = "team"
			out.TeamRole = &role
		}
		token, err := newInvitationToken()
		if err != nil {
			return err
		}
		sum := sha256.Sum256([]byte(token))
		out.TokenHash = sum[:]
		if err := tx.InsertInvitation(out); err != nil {
			return err
		}
		if err := s.queueInvitation(tx, out, token, now); err != nil {
			return err
		}
		if err := tx.AppendGovernanceEvent(nil, team, "invitation.created", strings.TrimSpace(reason), now); err != nil {
			return err
		}
		return tx.SaveInvitationReceipt(key, requestHash, out, now)
	})
	return out, err
}
func newInvitationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *Governance) queueInvitation(tx InvitationTx, i Invitation, token string, now time.Time) error {
	if s.protector == nil {
		return ErrRegistrationUnavailable
	}
	if err := tx.ReserveInvitationMail(i.EmailKey, now); err != nil {
		return err
	}
	event := uuid.New()
	payload, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return err
	}
	encrypted, err := s.protector.Seal(event, payload)
	clear(payload)
	if err != nil {
		return err
	}
	return tx.EnqueueMail(QueuedMail{EventID: event, InvitationID: &i.ID, InvitationRevision: &i.Revision, Template: i.Kind + "_invitation", Recipient: i.EmailKey, KeyID: s.protector.KeyID(), EncryptedPayload: encrypted, CreatedAt: now, ExpiresAt: i.ExpiresAt})
}

// AcceptInvitation joins only the verified addressed account after rechecking team authority.
func (s *Governance) AcceptInvitation(ctx context.Context, a Principal, token string) error {
	return s.consumeInvitation(ctx, a, token, true)
}

// DeclineInvitation records the addressed account rejection without changing membership.
func (s *Governance) DeclineInvitation(ctx context.Context, a Principal, token string) error {
	return s.consumeInvitation(ctx, a, token, false)
}
func (s *Governance) consumeInvitation(ctx context.Context, a Principal, token string, accept bool) error {
	if len(token) != 43 {
		return ErrProofInvalid
	}
	hash := sha256.Sum256([]byte(token))
	return s.store.GovernanceTransaction(ctx, a, false, s.now, func(base GovernanceTx) error {
		now := s.now().UTC()
		tx, ok := base.(InvitationTx)
		if !ok {
			return ErrGovernanceConflict
		}
		i, err := tx.Invitation(uuid.Nil, hash[:])
		if err != nil {
			return err
		}
		if i.Status != "pending" || !now.Before(i.ExpiresAt) || tx.Actor().Email != i.EmailKey {
			return ErrProofInvalid
		}
		if accept {
			source, err := tx.User(i.InviterID)
			if err != nil {
				return err
			}
			if source.Status != domain.StatusActive || source.EmailVerifiedAt.IsZero() {
				return ErrForbidden
			}
			if i.Kind == "account" || i.PlatformIssued {
				if source.PlatformRole != "platform_admin" {
					return ErrForbidden
				}
			}
			if i.Kind == "team" {
				gateway, ok := tx.TeamGovernance().(TeamInvitationGateway)
				if !ok {
					return ErrGovernanceConflict
				}
				if err := gateway.AcceptInvitation(i, a.ID, now); err != nil {
					return err
				}
			}
			i.Status = "accepted"
			i.AcceptedUserID = &a.ID
		} else {
			i.Status = "declined"
		}
		i.Revision++
		if err := tx.SaveInvitation(i, now); err != nil {
			return err
		}
		if err := tx.AppendGovernanceEvent(&a.ID, i.TeamID, "invitation."+i.Status, "", now); err != nil {
			return err
		}
		source, err := tx.User(i.InviterID)
		if err != nil {
			return err
		}
		return s.notify(tx, source.Email, "account_governance", now)
	})
}

// ResendInvitation rotates a pending invitation proof under current scoped authority.
func (s *Governance) ResendInvitation(ctx context.Context, a Principal, id uuid.UUID, revision int64, team *uuid.UUID) error {
	return s.changeInvitation(ctx, a, id, revision, "", false, team, false)
}

// RevokeInvitation revokes only an invitation matching the caller supplied route scope.
func (s *Governance) RevokeInvitation(ctx context.Context, a Principal, id uuid.UUID, revision int64, reason string, team *uuid.UUID) error {
	return s.changeInvitation(ctx, a, id, revision, reason, true, team, false)
}
func (s *Governance) changeInvitation(ctx context.Context, a Principal, id uuid.UUID, revision int64, reason string, revoke bool, expectedTeam *uuid.UUID, platform bool) error {
	reason = strings.TrimSpace(reason)
	if revision < 1 || (revoke && !validGovernanceReason(reason)) {
		return ErrInvalidGovernance
	}
	return s.store.GovernanceTransaction(ctx, a, true, s.now, func(base GovernanceTx) error {
		now := s.now().UTC()
		tx, ok := base.(InvitationTx)
		if !ok {
			return ErrGovernanceConflict
		}
		i, err := tx.Invitation(id, nil)
		if err != nil {
			return err
		}
		if (i.TeamID == nil) != (expectedTeam == nil) || (i.TeamID != nil && *i.TeamID != *expectedTeam) {
			return ErrForbidden
		}
		if i.Kind == "account" || platform {
			if err := tx.RequirePlatform(); err != nil {
				return err
			}
		} else {
			gateway, ok := tx.TeamGovernance().(TeamInvitationGateway)
			if !ok {
				return ErrGovernanceConflict
			}
			if err := gateway.AuthorizeInvitations(*i.TeamID, a.ID); err != nil {
				return err
			}
			if !revoke {
				if _, err := gateway.CheckInvitation(*i.TeamID, a.ID, i.EmailKey, *i.TeamRole); err != nil {
					return err
				}
			}
		}
		if i.Revision != revision {
			return domain.ErrRevisionConflict
		}
		if i.Status != "pending" || !now.Before(i.ExpiresAt) {
			return ErrGovernanceConflict
		}
		i.Revision++
		if revoke {
			i.Status = "revoked"
		} else {
			token, err := newInvitationToken()
			if err != nil {
				return err
			}
			sum := sha256.Sum256([]byte(token))
			i.TokenHash = sum[:]
			i.ExpiresAt = now.Add(48 * time.Hour)
			if err := s.queueInvitation(tx, i, token, now); err != nil {
				return err
			}
		}
		if err := tx.SaveInvitation(i, now); err != nil {
			return err
		}
		action := "invitation.resent"
		if revoke {
			action = "invitation.revoked"
		}
		return tx.AppendGovernanceEvent(nil, i.TeamID, action, reason, now)
	})
}

// InvitationPreview deliberately omits mailbox, actor IDs and governance reasons.
type InvitationPreview struct {
	Kind      string     `json:"kind"`
	TeamID    *uuid.UUID `json:"team_id"`
	TeamRole  *string    `json:"team_role"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// PublicInvitationPreview checks a bearer proof without disclosing the addressed mailbox.
func (s *Governance) PublicInvitationPreview(ctx context.Context, token string) (InvitationPreview, error) {
	if len(token) != 43 {
		return InvitationPreview{}, ErrProofInvalid
	}
	sum := sha256.Sum256([]byte(token))
	i, err := s.store.PreviewInvitationProof(ctx, sum[:], s.now().UTC())
	if err != nil {
		return InvitationPreview{}, err
	}
	return InvitationPreview{Kind: i.Kind, TeamID: i.TeamID, TeamRole: i.TeamRole, ExpiresAt: i.ExpiresAt}, nil
}

// CreatePlatformTeamInvitation explicitly scopes platform governance without granting content access.
func (s *Governance) CreatePlatformTeamInvitation(ctx context.Context, a Principal, key, team uuid.UUID, email, role, reason string) (Invitation, error) {
	return s.createInvitation(ctx, a, key, &team, email, role, true, reason)
}

// RevokePlatformTeamInvitation checks both platform authority and the route team.
func (s *Governance) RevokePlatformTeamInvitation(ctx context.Context, a Principal, team, id uuid.UUID, v int64, reason string) error {
	return s.changeInvitation(ctx, a, id, v, reason, true, &team, true)
}
