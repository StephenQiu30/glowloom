package application

import (
	"context"
	"io"
	"time"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type resultAccess struct {
	signer       media.ObjectSigner
	reader       media.ObjectReader
	legacySigner media.PreviewSigner
	legacyReader ResultReader
}

func (r *resultAccess) sign(ctx context.Context, key string, l *domain.ObjectLocation, ttl time.Duration) (media.AccessGrant, error) {
	if l == nil {
		if r.legacySigner == nil {
			return media.AccessGrant{}, ErrUnavailable
		}
		url, e := r.legacySigner.PresignGet(ctx, key, ttl)
		return media.AccessGrant{URL: url, ExpiresAt: time.Now().UTC().Add(ttl)}, e
	}
	if r.signer == nil || l.Ref.Key != key || l.State != "accepted" || l.Ref.Validate() != nil {
		return media.AccessGrant{}, ErrConflict
	}
	return r.signer.PresignGet(ctx, l.Ref, ttl)
}
func (r *resultAccess) read(ctx context.Context, key string, l *domain.ObjectLocation) (io.ReadCloser, error) {
	if l == nil {
		if r.legacyReader == nil {
			return nil, ErrUnavailable
		}
		return r.legacyReader.Get(ctx, key)
	}
	if r.reader == nil || l.Ref.Key != key || l.State != "accepted" || l.Ref.Validate() != nil {
		return nil, ErrConflict
	}
	return r.reader.Get(ctx, l.Ref)
}
