package application

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrInvalidMemberFilter rejects unsupported filters or unbound member cursors.
var ErrInvalidMemberFilter = errors.New("invalid member filter or cursor")

// MemberFilter limits a directory query to one team and explicit membership fields.
type MemberFilter struct {
	Search, Role, Status, Cursor string
	From, To                     *time.Time
	Limit                        int
	UserID                       uuid.UUID
}

// MemberPosition fixes the join-time keyset and the first page's upper timestamp.
type MemberPosition struct {
	AsOf, JoinedAt time.Time
	UserID         uuid.UUID
}

// MemberPage is a bounded role-appropriate member directory result.
type MemberPage struct {
	Items      []TeamMember
	NextCursor *string
	AsOf       time.Time
}

// TeamCursorSigner binds a cursor to actor, session, team revision and filters.
type TeamCursorSigner interface {
	Digest(string, uuid.UUID, string) []byte
}

// TeamDirectory serves filtered current membership and removed-member history.
type TeamDirectory struct {
	store  TeamStore
	signer TeamCursorSigner
	now    func() time.Time
}

// NewTeamDirectory injects the existing transaction store, cursor key and clock.
func NewTeamDirectory(store TeamStore, signer TeamCursorSigner, now func() time.Time) *TeamDirectory {
	return &TeamDirectory{store: store, signer: signer, now: now}
}

func normalizeMemberFilter(f MemberFilter) (MemberFilter, error) {
	f.Search = strings.TrimSpace(f.Search)
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 || !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 100 || len(f.Cursor) > 4096 || (f.Role != "" && f.Role != "member" && f.Role != "team_admin") || (f.Status != "" && f.Status != "active" && f.Status != "suspended" && f.Status != "removed") || f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return f, ErrInvalidMemberFilter
	}
	return f, nil
}
func (s *TeamDirectory) cursorDigest(actor identity.Principal, team Team, filter MemberFilter, payload string) []byte {
	filter.Cursor = ""
	binding, _ := json.Marshal(struct {
		Actor, Session, Team          uuid.UUID
		CredentialEpoch, TeamRevision int64
		Filter                        MemberFilter
		Payload                       string
	}{actor.ID, actor.SessionID, team.ID, actor.CredentialEpoch, team.Revision, filter, payload})
	return s.signer.Digest("team_member_cursor", team.ID, string(binding))
}

// List rechecks current membership before validating a ten-minute bound cursor.
func (s *TeamDirectory) List(ctx context.Context, actor identity.Principal, id uuid.UUID, filter MemberFilter) (MemberPage, error) {
	f, err := normalizeMemberFilter(filter)
	if err != nil || s.signer == nil {
		return MemberPage{}, ErrInvalidMemberFilter
	}
	now := s.now().UTC()
	page := MemberPage{Items: []TeamMember{}, AsOf: now}
	err = s.store.TeamTransaction(ctx, actor, now, func(tx TeamTx) error {
		team, err := tx.LoadTeam(id)
		if err != nil {
			return err
		}
		admin := team.ActorRole == "team_admin"
		if !admin && (f.Status != "" && f.Status != "active" || f.From != nil || f.To != nil || f.UserID != uuid.Nil) {
			return ErrTeamForbidden
		}
		position := MemberPosition{AsOf: now}
		if f.Cursor != "" {
			payload, signature, ok := strings.Cut(f.Cursor, ".")
			if !ok {
				return ErrInvalidMemberFilter
			}
			digest, err := base64.RawURLEncoding.DecodeString(signature)
			if err != nil || !hmac.Equal(digest, s.cursorDigest(actor, team, f, payload)) {
				return ErrInvalidMemberFilter
			}
			raw, err := base64.RawURLEncoding.DecodeString(payload)
			if err != nil || json.Unmarshal(raw, &position) != nil || position.UserID == uuid.Nil || position.JoinedAt.IsZero() || position.AsOf.IsZero() || position.AsOf.After(now) || !now.Before(position.AsOf.Add(10*time.Minute)) {
				return ErrInvalidMemberFilter
			}
		}
		values, err := tx.QueryMembers(id, admin, f, position)
		if err != nil {
			return err
		}
		page.AsOf = position.AsOf
		if len(values) > f.Limit {
			values = values[:f.Limit]
			last := values[len(values)-1]
			raw, err := json.Marshal(MemberPosition{AsOf: position.AsOf, JoinedAt: last.JoinedAt, UserID: last.UserID})
			if err != nil {
				return err
			}
			payload := base64.RawURLEncoding.EncodeToString(raw)
			cursor := payload + "." + base64.RawURLEncoding.EncodeToString(s.cursorDigest(actor, team, f, payload))
			page.NextCursor = &cursor
		}
		page.Items = values
		return nil
	})
	return page, err
}

// Detail exposes one current or removed membership only to its team administrator.
func (s *TeamDirectory) Detail(ctx context.Context, actor identity.Principal, team, user uuid.UUID) (TeamMember, error) {
	if user == uuid.Nil {
		return TeamMember{}, ErrInvalidMemberFilter
	}
	page, err := s.List(ctx, actor, team, MemberFilter{UserID: user, Limit: 1})
	if err != nil {
		return TeamMember{}, err
	}
	if len(page.Items) != 1 {
		return TeamMember{}, ErrScopeNotFound
	}
	return page.Items[0], nil
}
