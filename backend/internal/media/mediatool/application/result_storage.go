package application

import (
	"context"
	"io"
	"time"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type resultAccess struct {
	signer     media.ObjectSigner
	reader     media.ObjectReader
	previewTTL time.Duration
}

func (r *resultAccess) sign(ctx context.Context, key string, l *domain.ObjectLocation) (media.AccessGrant, error) {
	if r == nil || l == nil {
		return media.AccessGrant{}, ErrUnavailable
	}
	ttl := r.previewTTL
	if r.signer == nil || l.Ref.Key != key || l.State != domain.ObjectLocationAccepted || l.Ref.Validate() != nil {
		return media.AccessGrant{}, ErrConflict
	}
	if ttl < time.Second || ttl > 15*time.Minute {
		return media.AccessGrant{}, ErrUnavailable
	}
	grant, err := r.signer.PresignGet(ctx, l.Ref, ttl)
	if err != nil {
		return media.AccessGrant{}, err
	}
	now := time.Now()
	if grant.URL == "" || !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(ttl+5*time.Second)) || grant.ExpiresAt.After(now.Add(15*time.Minute)) {
		return media.AccessGrant{}, ErrUnavailable
	}
	return grant, nil
}
func (r *resultAccess) read(ctx context.Context, key string, l *domain.ObjectLocation) (io.ReadCloser, error) {
	if l == nil {
		return nil, ErrUnavailable
	}
	if r.reader == nil || l.Ref.Key != key || l.State != domain.ObjectLocationAccepted || l.Ref.Validate() != nil {
		return nil, ErrConflict
	}
	return r.reader.Get(ctx, l.Ref)
}
