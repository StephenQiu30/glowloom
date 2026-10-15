package application

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GovernanceFilter accepts only an explicit status and a bounded signed continuation.
type GovernanceFilter struct {
	Status, Cursor string
	Limit          int
}

// GovernancePosition provides a stable descending keyset within a ten-minute snapshot.
type GovernancePosition struct {
	AsOf, CreatedAt time.Time
	ID              uuid.UUID
}

// InvitationListPage exposes safe metadata and an opaque continuation.
type InvitationListPage struct {
	Items      []Invitation `json:"items"`
	NextCursor *string      `json:"next_cursor"`
	AsOf       time.Time    `json:"as_of"`
}

// ProposalListPage exposes safe metadata and an opaque continuation.
type ProposalListPage struct {
	Items      []AuthorityProposal `json:"items"`
	NextCursor *string             `json:"next_cursor"`
	AsOf       time.Time           `json:"as_of"`
}

func normalizeGovernanceFilter(f GovernanceFilter, invitation bool) (GovernanceFilter, error) {
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 || len(f.Cursor) > 4096 {
		return f, ErrInvalidGovernance
	}
	allowed := map[string]bool{"": true, "pending": true, "accepted": true, "declined": true, "expired": true}
	if invitation {
		allowed["revoked"] = true
	} else {
		allowed["cancelled"] = true
	}
	if !allowed[f.Status] {
		return f, ErrInvalidGovernance
	}
	return f, nil
}
func (s *Governance) directorySignature(a Principal, team *uuid.UUID, scope string, revision int64, f GovernanceFilter, payload string) []byte {
	f.Cursor = ""
	raw, _ := json.Marshal([]any{a.ID, a.SessionID, a.CredentialEpoch, a.PlatformEpoch, team, scope, revision, f, payload})
	return s.protector.Digest("governance_cursor", a.ID, string(raw))
}
func (s *Governance) directoryPosition(a Principal, team *uuid.UUID, scope string, revision int64, f GovernanceFilter, now time.Time) (GovernancePosition, error) {
	p := GovernancePosition{AsOf: now}
	if f.Cursor == "" {
		return p, nil
	}
	payload, sig, ok := strings.Cut(f.Cursor, ".")
	if !ok {
		return p, ErrInvalidGovernance
	}
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(signature, s.directorySignature(a, team, scope, revision, f, payload)) {
		return p, ErrInvalidGovernance
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(raw, &p) != nil || p.ID == uuid.Nil || p.AsOf.IsZero() || p.CreatedAt.IsZero() || p.CreatedAt.After(p.AsOf) || p.AsOf.After(now) || !now.Before(p.AsOf.Add(10*time.Minute)) {
		return p, ErrInvalidGovernance
	}
	return p, nil
}
func (s *Governance) directoryCursor(a Principal, team *uuid.UUID, scope string, revision int64, f GovernanceFilter, p GovernancePosition) *string {
	raw, _ := json.Marshal(p)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	cursor := payload + "." + base64.RawURLEncoding.EncodeToString(s.directorySignature(a, team, scope, revision, f, payload))
	return &cursor
}
func directoryAuthority(tx GovernanceTx, a Principal, team *uuid.UUID, scope string) (int64, error) {
	if scope == "mine" {
		return tx.Actor().PlatformEpoch, nil
	}
	if team == nil || (scope == "platform_team" || scope == "platform_team_invitations") {
		return tx.Actor().PlatformEpoch, tx.RequirePlatform()
	}
	if tx.TeamGovernance() == nil {
		return 0, ErrGovernanceConflict
	}
	return tx.TeamGovernance().InvitationAuthority(*team, a.ID)
}

// ListInvitations authorizes before validating actor/session/team/filter-bound pagination.
func (s *Governance) ListInvitations(ctx context.Context, a Principal, team *uuid.UUID, f GovernanceFilter) (InvitationListPage, error) {
	return s.listInvitations(ctx, a, team, "invitations", f)
}

// ListPlatformTeamInvitations exposes governance metadata without granting team access.
func (s *Governance) ListPlatformTeamInvitations(ctx context.Context, a Principal, team uuid.UUID, f GovernanceFilter) (InvitationListPage, error) {
	if team == uuid.Nil {
		return InvitationListPage{}, ErrInvalidGovernance
	}
	return s.listInvitations(ctx, a, &team, "platform_team_invitations", f)
}

func (s *Governance) listInvitations(ctx context.Context, a Principal, team *uuid.UUID, scope string, f GovernanceFilter) (InvitationListPage, error) {
	f, err := normalizeGovernanceFilter(f, true)
	page := InvitationListPage{Items: []Invitation{}}
	if err != nil || s.protector == nil {
		return page, ErrInvalidGovernance
	}
	err = s.store.GovernanceTransaction(ctx, a, false, s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		revision, err := directoryAuthority(tx, a, team, scope)
		if err != nil {
			return err
		}
		p, err := s.directoryPosition(a, team, scope, revision, f, now)
		if err != nil {
			return err
		}
		items, err := tx.QueryInvitations(team, f, p)
		if err != nil {
			return err
		}
		page.AsOf = p.AsOf
		if len(items) > f.Limit {
			items = items[:f.Limit]
			last := items[len(items)-1]
			page.NextCursor = s.directoryCursor(a, team, scope, revision, f, GovernancePosition{AsOf: p.AsOf, CreatedAt: last.CreatedAt, ID: last.ID})
		}
		page.Items = items
		return nil
	})
	return page, err
}

// ListProposals distinguishes addressed proposals, platform grants and one team's transfers.
func (s *Governance) ListProposals(ctx context.Context, a Principal, team *uuid.UUID, scope string, f GovernanceFilter) (ProposalListPage, error) {
	f, err := normalizeGovernanceFilter(f, false)
	page := ProposalListPage{Items: []AuthorityProposal{}}
	if err != nil || s.protector == nil || (scope != "mine" && scope != "platform" && scope != "team" && scope != "platform_team") || (scope == "team" || scope == "platform_team") != (team != nil) {
		return page, ErrInvalidGovernance
	}
	err = s.store.GovernanceTransaction(ctx, a, false, s.now, func(tx GovernanceTx) error {
		now := s.now().UTC()
		revision, err := directoryAuthority(tx, a, team, scope)
		if err != nil {
			return err
		}
		p, err := s.directoryPosition(a, team, scope, revision, f, now)
		if err != nil {
			return err
		}
		items, err := tx.QueryProposals(team, scope, f, p)
		if err != nil {
			return err
		}
		page.AsOf = p.AsOf
		if len(items) > f.Limit {
			items = items[:f.Limit]
			last := items[len(items)-1]
			page.NextCursor = s.directoryCursor(a, team, scope, revision, f, GovernancePosition{AsOf: p.AsOf, CreatedAt: last.CreatedAt, ID: last.ID})
		}
		page.Items = items
		return nil
	})
	return page, err
}
