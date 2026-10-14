package application

import (
	"context"
	"errors"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	"github.com/google/uuid"
	"math"
	"time"
	"unicode/utf8"
)

var ErrAvatarUnavailable = errors.New("avatar unavailable")

type ProfileInput struct {
	ExpectedRevision int64
	DisplayName      *string
	AvatarID         *uuid.UUID
	ClearAvatar      bool
}
type Profile struct {
	ID              uuid.UUID
	Email           string
	EmailVerifiedAt time.Time
	DisplayName     string
	AvatarID        *uuid.UUID
	PlatformRole    string
	Status          domain.Status
	Revision        int64
}
type Preferences struct {
	Theme, PromptSuffix string
	Revision            int64
}
type AvatarAccess interface {
	Validate(context.Context, uuid.UUID, uuid.UUID) error
}
type SelfProfileTx interface {
	Account() domain.User
	ValidateAvatar(context.Context, uuid.UUID) error
	SaveProfile(domain.User, int64, time.Time) error
	SavePreferences(domain.User, int64, time.Time) error
}
type SelfProfileStore interface {
	ProfileTransaction(context.Context, Principal, time.Time, func(SelfProfileTx) error) error
}
type SelfProfile struct {
	store SelfProfileStore
	now   func() time.Time
}

func NewSelfProfile(store SelfProfileStore, now func() time.Time) *SelfProfile {
	return &SelfProfile{store: store, now: now}
}
func profileView(u domain.User) Profile {
	return Profile{ID: u.ID, Email: u.Email, EmailVerifiedAt: u.EmailVerifiedAt, DisplayName: u.DisplayName, AvatarID: u.AvatarAssetID, PlatformRole: u.PlatformRole, Status: u.Status, Revision: u.Revision}
}
func (s *SelfProfile) Read(ctx context.Context, actor Principal) (Profile, error) {
	var out Profile
	err := s.store.ProfileTransaction(ctx, actor, s.now().UTC(), func(tx SelfProfileTx) error { out = profileView(tx.Account()); return nil })
	return out, err
}
func (s *SelfProfile) ReadPreferences(ctx context.Context, actor Principal) (Preferences, error) {
	var out Preferences
	err := s.store.ProfileTransaction(ctx, actor, s.now().UTC(), func(tx SelfProfileTx) error {
		u := tx.Account()
		out = Preferences{Theme: u.Theme, PromptSuffix: u.PromptSuffix, Revision: u.Revision}
		return nil
	})
	return out, err
}
func (s *SelfProfile) Update(ctx context.Context, actor Principal, in ProfileInput) (Profile, error) {
	if in.ExpectedRevision < 1 || in.ExpectedRevision >= math.MaxInt32 || (in.DisplayName == nil && in.AvatarID == nil && !in.ClearAvatar) || (in.AvatarID != nil && (in.ClearAvatar || *in.AvatarID == uuid.Nil)) || (in.DisplayName != nil && !validDisplayName(*in.DisplayName)) {
		return Profile{}, domain.ErrInvalidProfile
	}
	var out Profile
	now := s.now().UTC()
	err := s.store.ProfileTransaction(ctx, actor, now, func(tx SelfProfileTx) error {
		u := tx.Account()
		if u.Revision != in.ExpectedRevision {
			return domain.ErrRevisionConflict
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
func (s *SelfProfile) UpdatePreferences(ctx context.Context, actor Principal, expected int64, theme, suffix string) (int64, error) {
	if expected < 1 || expected >= math.MaxInt32 || (theme != "light" && theme != "dark" && theme != "system") || !utf8.ValidString(suffix) || utf8.RuneCountInString(suffix) > 2000 {
		return 0, domain.ErrInvalidProfile
	}
	var revision int64
	now := s.now().UTC()
	err := s.store.ProfileTransaction(ctx, actor, now, func(tx SelfProfileTx) error {
		u := tx.Account()
		if u.Revision != expected {
			return domain.ErrRevisionConflict
		}
		u.Theme = theme
		u.PromptSuffix = suffix
		u.Revision++
		if err := tx.SavePreferences(u, expected, now); err != nil {
			return err
		}
		revision = u.Revision
		return nil
	})
	return revision, err
}
