package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// TeamAvatarObject is an internal immutable normalized object, never a transport DTO.
type TeamAvatarObject struct {
	ID        uuid.UUID
	ObjectKey string
}

// TeamAvatarReader reads normalized media selected by an authorized team binding.
type TeamAvatarReader interface {
	ReadBound(context.Context, uuid.UUID) (TeamAvatarObject, error)
}

// TeamAvatarSigner signs only server-selected keys with a bounded lifetime.
type TeamAvatarSigner interface {
	PresignGet(context.Context, string, time.Duration) (string, error)
}

// TeamAvatarPreview contains no storage key and expires after one minute.
type TeamAvatarPreview struct {
	AssetID   uuid.UUID `json:"asset_id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TeamAvatars serves team and member avatars through current directory authority.
type TeamAvatars struct {
	store  TeamStore
	signer TeamAvatarSigner
	now    func() time.Time
}

// NewTeamAvatars injects the same team transaction store and private object signer.
func NewTeamAvatars(store TeamStore, signer TeamAvatarSigner, now func() time.Time) *TeamAvatars {
	return &TeamAvatars{store: store, signer: signer, now: now}
}

// Read checks scope before and after signing, without requiring the uploader to remain a member.
func (s *TeamAvatars) Read(ctx context.Context, a identity.Principal, team uuid.UUID, member *uuid.UUID, platform bool) (TeamAvatarPreview, error) {
	var object TeamAvatarObject
	read := func() (TeamAvatarObject, error) {
		var out TeamAvatarObject
		err := s.store.TeamTransaction(ctx, a, s.now().UTC(), func(tx TeamTx) error {
			var err error
			out, err = tx.BoundTeamAvatar(team, member, platform)
			return err
		})
		return out, err
	}
	if s.signer == nil {
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
	url, err := s.signer.PresignGet(signCtx, object.ObjectKey, time.Minute)
	if err != nil {
		return TeamAvatarPreview{}, err
	}
	current, err := read()
	if err != nil {
		return TeamAvatarPreview{}, err
	}
	if current != object {
		return TeamAvatarPreview{}, identity.ErrAvatarUnavailable
	}
	return TeamAvatarPreview{AssetID: object.ID, URL: url, ExpiresAt: now.Add(time.Minute)}, nil
}
