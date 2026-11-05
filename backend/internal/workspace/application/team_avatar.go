package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// TeamAvatarObject is an internal immutable normalized object, never a transport DTO.
type TeamAvatarObject struct {
	ID                uuid.UUID
	ProfileID, Bucket string
	ObjectKey         string
}

// TeamAvatarReader reads normalized media selected by an authorized team binding.
type TeamAvatarReader interface {
	ReadBound(context.Context, uuid.UUID) (TeamAvatarObject, error)
}

// RoutedTeamAvatarSigner selects the immutable physical location attached to
// the media asset, so switching the active write route leaves old avatars readable.
type RoutedTeamAvatarSigner interface {
	PresignLocationGet(context.Context, string, string, string, time.Duration) (TeamAvatarGrant, error)
}

// TeamAvatarGrant carries the provider's actual signed URL expiry.
type TeamAvatarGrant struct {
	URL       string
	ExpiresAt time.Time
}

// TeamAvatarPreview contains no storage key and has a configured expiry.
type TeamAvatarPreview struct {
	AssetID   uuid.UUID `json:"asset_id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TeamAvatars serves team and member avatars through current directory authority.
type TeamAvatars struct {
	store  TeamStore
	signer RoutedTeamAvatarSigner
	now    func() time.Time
	ttl    time.Duration
}

// NewRoutedTeamAvatars uses immutable asset locations and never infers a bucket
// from the current default write profile.
func NewRoutedTeamAvatars(store TeamStore, signer RoutedTeamAvatarSigner, now func() time.Time, ttl time.Duration) *TeamAvatars {
	return &TeamAvatars{store: store, signer: signer, now: now, ttl: ttl}
}

// Read checks scope before and after signing, without requiring the uploader to remain a member.
func (s *TeamAvatars) Read(ctx context.Context, a identity.Principal, team uuid.UUID, member *uuid.UUID, platform bool) (TeamAvatarPreview, error) {
	var object TeamAvatarObject
	read := func() (TeamAvatarObject, error) {
		var out TeamAvatarObject
		err := s.store.TeamTransaction(ctx, a, s.now, func(tx TeamTx) error {
			var err error
			out, err = tx.BoundTeamAvatar(team, member, platform)
			return err
		})
		return out, err
	}
	if s == nil || s.store == nil || s.signer == nil || s.now == nil || s.ttl < time.Second || s.ttl > 15*time.Minute {
		return TeamAvatarPreview{}, identity.ErrAvatarUnavailable
	}
	var err error
	object, err = read()
	if err != nil {
		return TeamAvatarPreview{}, err
	}
	now := s.now().UTC()
	signCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if object.ProfileID == "" || object.Bucket == "" || object.ObjectKey == "" {
		return TeamAvatarPreview{}, identity.ErrAvatarUnavailable
	}
	grant, err := s.signer.PresignLocationGet(signCtx, object.ProfileID, object.Bucket, object.ObjectKey, s.ttl)
	if err != nil {
		return TeamAvatarPreview{}, err
	}
	if grant.URL == "" || !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(s.ttl+5*time.Second)) || grant.ExpiresAt.After(now.Add(15*time.Minute)) {
		return TeamAvatarPreview{}, identity.ErrAvatarUnavailable
	}
	current, err := read()
	if err != nil {
		return TeamAvatarPreview{}, err
	}
	if current != object {
		return TeamAvatarPreview{}, identity.ErrAvatarUnavailable
	}
	return TeamAvatarPreview{AssetID: object.ID, URL: grant.URL, ExpiresAt: grant.ExpiresAt}, nil
}
