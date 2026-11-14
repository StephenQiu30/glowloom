package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ProviderLogoActorAccess checks live platform authority before accepting an image.
type ProviderLogoActorAccess interface {
	CheckPlatformSession(context.Context, identityapp.Principal, func() time.Time) error
}

// NewRoutedProviderLogoService reuses bounded decoding and the durable object ledger.
func NewRoutedProviderLogoService(repo RoutedAvatarRepository, objects AvatarRoutedStorage, now func() time.Time) *AvatarService {
	s := NewRoutedAvatarService(repo, objects, now)
	s.purpose = "provider_logo"
	return s
}

// ProviderLogoReader authorizes the exact image and its current supplier binding.
type ProviderLogoReader interface {
	ProviderLogoTransaction(context.Context, identityapp.Principal, uuid.UUID, func() time.Time, func(domain.MediaAsset) error) error
}

// ProviderLogoQuery issues short preview leases using the existing sign-and-recheck flow.
type ProviderLogoQuery struct {
	reader ProviderLogoReader
	signer ObjectSigner
	now    func() time.Time
	ttl    time.Duration
}

// NewProviderLogoQuery injects current binding authority and bounded URL signing.
func NewProviderLogoQuery(reader ProviderLogoReader, signer ObjectSigner, now func() time.Time, ttl time.Duration) *ProviderLogoQuery {
	return &ProviderLogoQuery{reader: reader, signer: signer, now: now, ttl: ttl}
}

type boundProviderLogoReader struct {
	reader ProviderLogoReader
	id     uuid.UUID
}

func (r boundProviderLogoReader) AvatarTransaction(ctx context.Context, actor identityapp.Principal, clock func() time.Time, run func(domain.MediaAsset) error) error {
	return r.reader.ProviderLogoTransaction(ctx, actor, r.id, clock, run)
}

func (q *ProviderLogoQuery) Read(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (AvatarPreview, error) {
	if q == nil || q.reader == nil || id == uuid.Nil {
		return AvatarPreview{}, ErrNotFound
	}
	return NewRoutedAvatarQuery(boundProviderLogoReader{q.reader, id}, q.signer, q.now, q.ttl).Read(ctx, actor)
}
