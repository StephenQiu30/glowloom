package application

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// ErrAvatarUnavailable hides absent and unauthorized avatar assets.
var ErrAvatarUnavailable = errors.New("avatar unavailable")

// ProfileInput expresses an optimistic-concurrency update to allowed self fields.
type ProfileInput struct {
	ExpectedRevision int64
	DisplayName      *string
	Bio              *string
	AvatarID         *uuid.UUID
	ClearAvatar      bool
}

// Profile projects the current account without credentials.
type Profile struct {
	ID              uuid.UUID
	Email           string
	EmailVerifiedAt time.Time
	DisplayName     string
	Bio             string
	AvatarID        *uuid.UUID
	PlatformRole    string
	Status          domain.Status
	Revision        int64
}

// Preferences stores account-wide creation and appearance choices.
type Preferences struct {
	Theme, PromptSuffix, CanvasBackground string
	Revision                              int64
}

// AvatarAccess validates media eligibility and ownership before profile binding.
type AvatarAccess interface {
	Validate(context.Context, uuid.UUID, uuid.UUID) error
}

// SelfProfileTx keeps identity, avatar validation and profile updates atomic.
type SelfProfileTx interface {
	Account() domain.User
	ValidateAvatar(context.Context, uuid.UUID) error
	SaveProfile(domain.User, int64, time.Time) error
	SavePreferences(domain.User, int64, time.Time) error
}

// SelfProfileStore coordinates current-identity checks and profile updates.
type SelfProfileStore interface {
	ProfileTransaction(context.Context, Principal, func() time.Time, func(SelfProfileTx) error) error
}

// SelfProfile validates and updates the current account profile and preferences.
type SelfProfile struct {
	store SelfProfileStore
	now   func() time.Time
}

// NewSelfProfile constructs SelfProfile with explicit caller-supplied dependencies.
func NewSelfProfile(store SelfProfileStore, now func() time.Time) *SelfProfile {
	return &SelfProfile{store: store, now: now}
}
func profileView(u domain.User) Profile {
	return Profile{ID: u.ID, Email: u.Email, EmailVerifiedAt: u.EmailVerifiedAt, DisplayName: u.DisplayName, Bio: u.Bio, AvatarID: u.AvatarAssetID, PlatformRole: u.PlatformRole, Status: u.Status, Revision: u.Revision}
}
func (s *SelfProfile) Read(ctx context.Context, actor Principal) (Profile, error) {
	var out Profile
	err := s.store.ProfileTransaction(ctx, actor, s.now, func(tx SelfProfileTx) error { out = profileView(tx.Account()); return nil })
	return out, err
}

// ReadPreferences loads preferences after checking the live session.
func (s *SelfProfile) ReadPreferences(ctx context.Context, actor Principal) (Preferences, error) {
	var out Preferences
	err := s.store.ProfileTransaction(ctx, actor, s.now, func(tx SelfProfileTx) error {
		u := tx.Account()
		out = Preferences{Theme: u.Theme, PromptSuffix: u.PromptSuffix, CanvasBackground: u.CanvasBackground, Revision: u.Revision}
		return nil
	})
	return out, err
}

// Update validates allowed fields and applies a revision-checked profile change.
func (s *SelfProfile) Update(ctx context.Context, actor Principal, in ProfileInput) (Profile, error) {
	if in.ExpectedRevision < 1 || in.ExpectedRevision >= math.MaxInt32 || (in.Bio == nil && in.DisplayName == nil && in.AvatarID == nil && !in.ClearAvatar) || (in.AvatarID != nil && (in.ClearAvatar || *in.AvatarID == uuid.Nil)) || (in.DisplayName != nil && !validDisplayName(*in.DisplayName)) || (in.Bio != nil && (!utf8.ValidString(*in.Bio) || utf8.RuneCountInString(*in.Bio) > 100 || strings.ContainsRune(*in.Bio, 0))) {
		return Profile{}, domain.ErrInvalidProfile
	}
	var out Profile
	err := s.store.ProfileTransaction(ctx, actor, s.now, func(tx SelfProfileTx) error {
		now := s.now().UTC()
		u := tx.Account()
		if u.Revision != in.ExpectedRevision {
			return domain.ErrRevisionConflict
		}
		if in.Bio != nil {
			u.Bio = *in.Bio
		}
		if in.DisplayName != nil {
			u.DisplayName = *in.DisplayName
		}
		if in.AvatarID != nil {
			if err := tx.ValidateAvatar(ctx, *in.AvatarID); err != nil {
				return err
			}
			u.AvatarAssetID = in.AvatarID
		}
		if in.ClearAvatar {
			u.AvatarAssetID = nil
		}
		u.Revision++
		if err := tx.SaveProfile(u, in.ExpectedRevision, now); err != nil {
			return err
		}
		out = profileView(u)
		return nil
	})
	return out, err
}

// PreferencesInput updates only supplied fields under the shared profile revision.
type PreferencesInput struct {
	ExpectedRevision int64
	Theme            *string
	PromptSuffix     *string
	CanvasBackground *string
}

// PatchPreferences atomically validates and updates account preferences.
func (s *SelfProfile) PatchPreferences(ctx context.Context, actor Principal, in PreferencesInput) (Preferences, error) {
	if in.ExpectedRevision < 1 || in.ExpectedRevision >= math.MaxInt32 || (in.Theme == nil && in.PromptSuffix == nil && in.CanvasBackground == nil) {
		return Preferences{}, domain.ErrInvalidProfile
	}
	if in.Theme != nil && *in.Theme != "light" && *in.Theme != "dark" && *in.Theme != "system" {
		return Preferences{}, domain.ErrInvalidProfile
	}
	if in.PromptSuffix != nil && (!utf8.ValidString(*in.PromptSuffix) || utf8.RuneCountInString(*in.PromptSuffix) > 2000 || strings.ContainsRune(*in.PromptSuffix, 0)) {
		return Preferences{}, domain.ErrInvalidProfile
	}
	if in.CanvasBackground != nil && *in.CanvasBackground != "dots" && *in.CanvasBackground != "lines" && *in.CanvasBackground != "blank" {
		return Preferences{}, domain.ErrInvalidProfile
	}
	var out Preferences
	err := s.store.ProfileTransaction(ctx, actor, s.now, func(tx SelfProfileTx) error {
		u := tx.Account()
		if u.Revision != in.ExpectedRevision {
			return domain.ErrRevisionConflict
		}
		if in.Theme != nil {
			u.Theme = *in.Theme
		}
		if in.PromptSuffix != nil {
			u.PromptSuffix = *in.PromptSuffix
		}
		if in.CanvasBackground != nil {
			u.CanvasBackground = *in.CanvasBackground
		}
		u.Revision++
		if err := tx.SavePreferences(u, in.ExpectedRevision, s.now().UTC()); err != nil {
			return err
		}
		out = Preferences{Theme: u.Theme, PromptSuffix: u.PromptSuffix, CanvasBackground: u.CanvasBackground, Revision: u.Revision}
		return nil
	})
	return out, err
}
