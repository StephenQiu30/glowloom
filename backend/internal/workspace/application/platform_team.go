package application

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// PlatformTeam contains governance metadata only, never a scope or content identifier.
type PlatformTeam struct {
	ID                 uuid.UUID  `json:"id"`
	Name               string     `json:"name"`
	AvatarID           *uuid.UUID `json:"avatar_asset_id"`
	CreatedBy          uuid.UUID  `json:"created_by"`
	Status             string     `json:"status"`
	ResumeStatus       *string    `json:"resume_status"`
	GovernanceBlocked  bool       `json:"governance_blocked"`
	InvitationEnabled  bool       `json:"invitation_enabled"`
	Revision           int64      `json:"revision,string" swaggertype:"string"`
	MemberCount        int64      `json:"member_count"`
	AdministratorCount int64      `json:"administrator_count"`
	CreatedAt          time.Time  `json:"created_at"`
}

// PlatformTeamFilter is the strict allowlist of platform team directory filters.
type PlatformTeamFilter struct {
	Search, Status, Cursor string
	GovernanceBlocked      *bool
	ID, AdministratorID    *uuid.UUID
	CreatedFrom, CreatedTo *time.Time
	Limit                  int
}

// PlatformTeamPosition binds a stable descending keyset to a snapshot time.
type PlatformTeamPosition struct {
	AsOf, CreatedAt time.Time
	ID              uuid.UUID
}

// PlatformTeamPage is a safe bounded team metadata page.
type PlatformTeamPage struct {
	Items      []PlatformTeam `json:"items"`
	NextCursor *string        `json:"next_cursor"`
	AsOf       time.Time      `json:"as_of"`
}

// PlatformTeams governs team lifecycle without becoming a team member.
type PlatformTeams struct {
	store  TeamStore
	signer TeamCursorSigner
	now    func() time.Time
}

// NewPlatformTeams injects current identity checks, signing and the clock.
func NewPlatformTeams(store TeamStore, signer TeamCursorSigner, now func() time.Time) *PlatformTeams {
	return &PlatformTeams{store: store, signer: signer, now: now}
}
func (s *PlatformTeams) Read(ctx context.Context, a identity.Principal, id uuid.UUID) (PlatformTeam, error) {
	var out PlatformTeam
	err := s.store.TeamTransaction(ctx, a, s.now().UTC(), func(tx TeamTx) error {
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		var err error
		out, err = tx.LoadPlatformTeam(id)
		return err
	})
	return out, err
}
func (s *PlatformTeams) signature(a identity.Principal, f PlatformTeamFilter, payload string) []byte {
	f.Cursor = ""
	raw, _ := json.Marshal([]any{a.ID, a.SessionID, a.CredentialEpoch, a.PlatformEpoch, f, payload})
	return s.signer.Digest("platform_team_cursor", a.ID, string(raw))
}

// List validates signed filters and rechecks platform authority in the read transaction.
func (s *PlatformTeams) List(ctx context.Context, a identity.Principal, f PlatformTeamFilter) (PlatformTeamPage, error) {
	page := PlatformTeamPage{Items: []PlatformTeam{}}
	f.Search = strings.TrimSpace(f.Search)
	if (f.ID != nil && *f.ID == uuid.Nil) || (f.AdministratorID != nil && *f.AdministratorID == uuid.Nil) {
		return page, ErrInvalidTeam
	}
	for _, bound := range []**time.Time{&f.CreatedFrom, &f.CreatedTo} {
		if *bound != nil {
			if (*bound).IsZero() {
				return page, ErrInvalidTeam
			}
			value := (*bound).UTC()
			*bound = &value
		}
	}
	if f.CreatedFrom != nil && f.CreatedTo != nil && !f.CreatedFrom.Before(*f.CreatedTo) {
		return page, ErrInvalidTeam
	}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if s.signer == nil || f.Limit < 1 || f.Limit > 100 || len(f.Cursor) > 4096 || !validTeamText(f.Search, 0, 100) || (f.Status != "" && f.Status != "active" && f.Status != "archived" && f.Status != "suspended") {
		return page, ErrInvalidTeam
	}
	now := s.now().UTC()
	p := PlatformTeamPosition{AsOf: now}
	err := s.store.TeamTransaction(ctx, a, now, func(tx TeamTx) error {
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		if f.Cursor != "" {
			payload, sig, ok := strings.Cut(f.Cursor, ".")
			signature, err := base64.RawURLEncoding.DecodeString(sig)
			if !ok || err != nil || !hmac.Equal(signature, s.signature(a, f, payload)) {
				return ErrInvalidTeam
			}
			raw, err := base64.RawURLEncoding.DecodeString(payload)
			if err != nil || json.Unmarshal(raw, &p) != nil || p.ID == uuid.Nil || p.CreatedAt.IsZero() || p.CreatedAt.After(p.AsOf) || p.AsOf.After(now) || !now.Before(p.AsOf.Add(10*time.Minute)) {
				return ErrInvalidTeam
			}
		}
		items, err := tx.QueryPlatformTeams(f, p)
		if err != nil {
			return err
		}
		page.AsOf = p.AsOf
		if len(items) > f.Limit {
			items = items[:f.Limit]
			last := items[len(items)-1]
			raw, _ := json.Marshal(PlatformTeamPosition{AsOf: p.AsOf, CreatedAt: last.CreatedAt, ID: last.ID})
			payload := base64.RawURLEncoding.EncodeToString(raw)
			cursor := payload + "." + base64.RawURLEncoding.EncodeToString(s.signature(a, f, payload))
			page.NextCursor = &cursor
		}
		page.Items = items
		return nil
	})
	return page, err
}

// Suspend remembers the prior archive state and immediately invalidates team scope access.
func (s *PlatformTeams) Suspend(ctx context.Context, a identity.Principal, id uuid.UUID, v int64, reason string) error {
	return s.change(ctx, a, id, v, reason, true)
}

// Restore preserves the prior archive state and never clears a governance freeze.
func (s *PlatformTeams) Restore(ctx context.Context, a identity.Principal, id uuid.UUID, v int64, reason string) error {
	return s.change(ctx, a, id, v, reason, false)
}
func (s *PlatformTeams) change(ctx context.Context, a identity.Principal, id uuid.UUID, v int64, reason string, suspend bool) error {
	reason = strings.TrimSpace(reason)
	if id == uuid.Nil || v < 1 || !validTeamText(reason, 5, 500) {
		return ErrInvalidTeam
	}
	now := s.now().UTC()
	return s.store.TeamTransaction(ctx, a, now, func(tx TeamTx) error {
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		if err := tx.RequireRecentAuthentication(); err != nil {
			return err
		}
		team, err := tx.LoadPlatformTeam(id)
		if err != nil {
			return err
		}
		if team.Revision != v {
			return ErrTeamRevisionConflict
		}
		action := "team.platform_restored"
		if suspend {
			if team.Status == "suspended" {
				return ErrTeamStateConflict
			}
			prior := team.Status
			team.ResumeStatus = &prior
			team.Status = "suspended"
			action = "team.platform_suspended"
		} else {
			if team.Status != "suspended" || team.ResumeStatus == nil {
				return ErrTeamStateConflict
			}
			team.Status = *team.ResumeStatus
			team.ResumeStatus = nil
		}
		team.Revision++
		if err := tx.SavePlatformTeam(team, v, now); err != nil {
			return err
		}
		return tx.AppendTeamEvent(id, nil, action, reason, now)
	})
}

// TeamProfileInput distinguishes omitted and explicitly removed avatar bindings.
type TeamProfileInput struct {
	ExpectedRevision int64
	Name             *string
	AvatarID         *uuid.UUID
	ClearAvatar      bool
	Reason           string
}

// Update changes only governance-visible team profile metadata.
func (s *PlatformTeams) Update(ctx context.Context, a identity.Principal, id uuid.UUID, in TeamProfileInput) (PlatformTeam, error) {
	var out PlatformTeam
	if err := validateTeamProfile(in, true); err != nil {
		return out, err
	}
	now := s.now().UTC()
	err := s.store.TeamTransaction(ctx, a, now, func(tx TeamTx) error {
		if err := tx.RequirePlatform(); err != nil {
			return err
		}
		if err := tx.RequireRecentAuthentication(); err != nil {
			return err
		}
		team, err := tx.LoadPlatformTeam(id)
		if err != nil {
			return err
		}
		if team.Revision != in.ExpectedRevision {
			return ErrTeamRevisionConflict
		}
		if in.Name != nil {
			team.Name = strings.TrimSpace(*in.Name)
		}
		if in.AvatarID != nil {
			if err := tx.ValidateTeamAvatar(*in.AvatarID); err != nil {
				return err
			}
			team.AvatarID = in.AvatarID
		}
		if in.ClearAvatar {
			team.AvatarID = nil
		}
		team.Revision++
		if err := tx.SavePlatformTeam(team, in.ExpectedRevision, now); err != nil {
			return err
		}
		out = team
		return tx.AppendTeamEvent(id, nil, "team.platform_updated", strings.TrimSpace(in.Reason), now)
	})
	return out, err
}
func validateTeamProfile(in TeamProfileInput, platform bool) error {
	if in.ExpectedRevision < 1 || (in.Name == nil && in.AvatarID == nil && !in.ClearAvatar) || (in.AvatarID != nil && (*in.AvatarID == uuid.Nil || in.ClearAvatar)) || (in.Name != nil && !validTeamText(strings.TrimSpace(*in.Name), 2, 50)) || (platform && !validTeamText(strings.TrimSpace(in.Reason), 5, 500)) {
		return ErrInvalidTeam
	}
	return nil
}

// UpdateProfile binds a normalized avatar using current team administrator authority.
func (s *Teams) UpdateProfile(ctx context.Context, a identity.Principal, id uuid.UUID, in TeamProfileInput) (Team, error) {
	var out Team
	if err := validateTeamProfile(in, false); err != nil {
		return out, err
	}
	now := s.now().UTC()
	err := s.store.TeamTransaction(ctx, a, now, func(tx TeamTx) error {
		team, err := tx.LoadTeam(id)
		if err != nil {
			return err
		}
		if team.ActorRole != "team_admin" {
			return ErrTeamForbidden
		}
		if team.Status != "active" || team.GovernanceBlocked {
			return ErrTeamStateConflict
		}
		if team.Revision != in.ExpectedRevision {
			return ErrTeamRevisionConflict
		}
		if in.Name != nil {
			team.Name = strings.TrimSpace(*in.Name)
		}
		if in.AvatarID != nil {
			if err := tx.ValidateTeamAvatar(*in.AvatarID); err != nil {
				return err
			}
			team.AvatarID = in.AvatarID
		}
		if in.ClearAvatar {
			team.AvatarID = nil
		}
		team.Revision++
		if err := tx.SaveTeam(team, in.ExpectedRevision, now); err != nil {
			return err
		}
		out = team
		return tx.AppendTeamEvent(id, nil, "team.updated", "", now)
	})
	return out, err
}
