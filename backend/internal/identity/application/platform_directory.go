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
)

// ErrInvalidDirectoryFilter rejects unsupported directory filters or cursors.
var ErrInvalidDirectoryFilter = errors.New("invalid platform directory filter")

// DirectoryFilter defines the supported platform account search and ordering.
type DirectoryFilter struct {
	Search, Status, PlatformRole, Order, Cursor string
	Verified, InTeam                            *bool
	From, To                                    *time.Time
	Limit                                       int
}

// UserSummary is a directory projection, never a persisted user or session.
type UserSummary struct {
	ID                                       uuid.UUID
	Email, DisplayName, Status, PlatformRole string
	Verified                                 bool
	EmailVerifiedAt                          *time.Time
	AvatarAssetID                            *uuid.UUID
	ActiveTeamCount                          int
	Revision                                 int64
	CreatedAt                                time.Time
	LastLoginAt                              *time.Time
}

// UserPage contains authorized account summaries and a continuation cursor.
type UserPage struct {
	Items      []UserSummary
	NextCursor *string
	AsOf       time.Time
}

// TeamMembershipSummary contains team relationship metadata without private content.
type TeamMembershipSummary struct {
	TeamID       uuid.UUID
	Role, Status string
}

// UserDetail combines account metadata with permitted governance summaries.
type UserDetail struct {
	User           UserSummary
	Teams          []TeamMembershipSummary
	AllowedActions []string
}

// DirectoryPosition stores a stable keyset position and snapshot bound.
type DirectoryPosition struct {
	AsOf      time.Time `json:"as_of"`
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// PlatformDirectoryStore supplies platform-authorized account projections.
type PlatformDirectoryStore interface {
	ReadDirectory(context.Context, Principal, DirectoryFilter, DirectoryPosition, func() time.Time) ([]UserSummary, error)
	ReadUserDetail(context.Context, Principal, uuid.UUID, func() time.Time) (UserDetail, error)
}

// DirectorySigner authenticates directory cursors for one authority and filter.
type DirectorySigner interface {
	Digest(string, uuid.UUID, string) []byte
}

// PlatformDirectory validates directory queries and enforces cursor binding.
type PlatformDirectory struct {
	store  PlatformDirectoryStore
	signer DirectorySigner
	now    func() time.Time
}

// NewPlatformDirectory constructs PlatformDirectory with explicit caller-supplied dependencies.
func NewPlatformDirectory(store PlatformDirectoryStore, signer DirectorySigner, now func() time.Time) *PlatformDirectory {
	return &PlatformDirectory{store: store, signer: signer, now: now}
}

func normalizeDirectoryFilter(f DirectoryFilter) (DirectoryFilter, error) {
	f.Search = strings.TrimSpace(f.Search)
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Order == "" {
		f.Order = "created_desc"
	}
	if f.Limit < 1 || f.Limit > 100 || utf8.RuneCountInString(f.Search) > 100 || len(f.Cursor) > 4096 {
		return f, ErrInvalidDirectoryFilter
	}
	if f.Order != "created_desc" && f.Order != "created_asc" {
		return f, ErrInvalidDirectoryFilter
	}
	if f.Status != "" && f.Status != "active" && f.Status != "disabled" && f.Status != "deleted" {
		return f, ErrInvalidDirectoryFilter
	}
	if f.PlatformRole != "" && f.PlatformRole != "creator" && f.PlatformRole != "platform_admin" {
		return f, ErrInvalidDirectoryFilter
	}
	if f.From != nil {
		v := f.From.UTC()
		f.From = &v
	}
	if f.To != nil {
		v := f.To.UTC()
		f.To = &v
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return f, ErrInvalidDirectoryFilter
	}
	return f, nil
}

func (d *PlatformDirectory) cursorDigest(actor Principal, f DirectoryFilter, payload string) []byte {
	f.Cursor = ""
	bound, _ := json.Marshal(struct {
		Session                        uuid.UUID
		CredentialEpoch, PlatformEpoch int64
		Filter                         DirectoryFilter
		Payload                        string
	}{actor.SessionID, actor.CredentialEpoch, actor.PlatformEpoch, f, payload})
	return d.signer.Digest("platform_directory_cursor", actor.ID, string(bound))
}

// List validates filters and a cursor bound to current authority.
func (d *PlatformDirectory) List(ctx context.Context, actor Principal, filter DirectoryFilter) (UserPage, error) {
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil {
		return UserPage{}, ErrSessionInvalid
	}
	f, err := normalizeDirectoryFilter(filter)
	if err != nil || d.signer == nil {
		return UserPage{}, ErrInvalidDirectoryFilter
	}
	now := d.now().UTC()
	position := DirectoryPosition{AsOf: now}
	if f.Cursor != "" {
		parts := strings.Split(f.Cursor, ".")
		if len(parts) != 2 {
			return UserPage{}, ErrInvalidDirectoryFilter
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil || !hmac.Equal(sig, d.cursorDigest(actor, f, parts[0])) {
			return UserPage{}, ErrInvalidDirectoryFilter
		}
		raw, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil || json.Unmarshal(raw, &position) != nil || position.ID == uuid.Nil || position.AsOf.After(now) || now.Sub(position.AsOf) >= 10*time.Minute || position.CreatedAt.IsZero() || position.CreatedAt.After(position.AsOf) {
			return UserPage{}, ErrInvalidDirectoryFilter
		}
	}
	items, err := d.store.ReadDirectory(ctx, actor, f, position, d.now)
	if err != nil {
		return UserPage{}, err
	}
	page := UserPage{Items: items, AsOf: position.AsOf}
	if page.Items == nil {
		page.Items = []UserSummary{}
	}
	if len(items) > f.Limit {
		page.Items = items[:f.Limit]
		last := page.Items[len(page.Items)-1]
		raw, err := json.Marshal(DirectoryPosition{AsOf: position.AsOf, CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return UserPage{}, err
		}
		payload := base64.RawURLEncoding.EncodeToString(raw)
		cursor := payload + "." + base64.RawURLEncoding.EncodeToString(d.cursorDigest(actor, f, payload))
		page.NextCursor = &cursor
	}
	return page, nil
}

// Detail returns an authorized account detail by stable ID.
func (d *PlatformDirectory) Detail(ctx context.Context, actor Principal, id uuid.UUID) (UserDetail, error) {
	if id == uuid.Nil {
		return UserDetail{}, ErrInvalidDirectoryFilter
	}
	return d.store.ReadUserDetail(ctx, actor, id, d.now)
}
