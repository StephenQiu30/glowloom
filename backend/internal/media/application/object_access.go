package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ErrObjectMissing is confirmed physical absence, distinct from denied reads.
var ErrObjectMissing = errors.New("media object missing")

// AccessGrant carries the signer's actual expiry, including temporary credentials.
// Only an authorized response projection may expose its private URL.
type AccessGrant struct {
	URL       string    `json:"-" yaml:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Format keeps private access URLs out of generic diagnostics.
func (g AccessGrant) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "[private access grant expires %s]", g.ExpiresAt.UTC().Format(time.RFC3339))
}

// ObjectSigner signs an exact persisted physical identity.
type ObjectSigner interface {
	PresignGet(context.Context, domain.ObjectRef, time.Duration) (AccessGrant, error)
}

// ObjectReader opens one exact physical identity, never a default write profile.
type ObjectReader interface {
	Get(context.Context, domain.ObjectRef) (io.ReadCloser, error)
}

func readableRef(location *domain.ObjectLocation, key string) (domain.ObjectRef, error) {
	if location == nil || location.ID == uuid.Nil || location.Ref.Validate() != nil || location.Ref.Key != key || location.State != "accepted" && location.State != "present" {
		return domain.ObjectRef{}, ErrUnavailable
	}
	return location.Ref, nil
}

func signMediaObject(ctx context.Context, location *domain.ObjectLocation, key string, signer ObjectSigner, legacy PreviewSigner, ttl time.Duration) (AccessGrant, error) {
	if ttl < time.Second || ttl > 15*time.Minute {
		return AccessGrant{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return AccessGrant{}, err
	}
	now := time.Now().UTC()
	if signer == nil && location == nil && legacy != nil {
		url, err := legacy.PresignGet(ctx, key, ttl)
		if err != nil || url == "" {
			return AccessGrant{}, errors.Join(ErrUnavailable, err)
		}
		return AccessGrant{URL: url, ExpiresAt: now.Add(ttl)}, nil
	}
	ref, err := readableRef(location, key)
	if err != nil || signer == nil {
		return AccessGrant{}, ErrUnavailable
	}
	grant, err := signer.PresignGet(ctx, ref, ttl)
	if err != nil {
		return AccessGrant{}, errors.Join(ErrUnavailable, err)
	}
	if grant.URL == "" || !grant.ExpiresAt.After(time.Now()) || grant.ExpiresAt.After(now.Add(ttl+5*time.Second)) || grant.ExpiresAt.After(time.Now().Add(15*time.Minute)) {
		return AccessGrant{}, ErrUnavailable
	}
	return grant, nil
}

func readMediaObject(ctx context.Context, location *domain.ObjectLocation, key string, reader ObjectReader, legacy DocumentObjectGetter) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader == nil && location == nil && legacy != nil {
		return legacy.Get(ctx, key)
	}
	ref, err := readableRef(location, key)
	if err != nil || reader == nil {
		return nil, ErrUnavailable
	}
	return reader.Get(ctx, ref)
}

func sameObjectLocation(a, b *domain.ObjectLocation) bool {
	return a != nil && b != nil && a.ID == b.ID && a.Ref == b.Ref
}

func sameLibraryLocations(a, b LibraryMediaFile) bool {
	if a.Asset.ID != b.Asset.ID || a.Asset.Revision != b.Asset.Revision || !sameObjectLocation(a.Asset.ObjectLocation, b.Asset.ObjectLocation) || len(a.Renditions) != len(b.Renditions) {
		return false
	}
	byID := make(map[uuid.UUID]domain.Rendition, len(a.Renditions))
	for _, r := range a.Renditions {
		if _, exists := byID[r.ID]; exists {
			return false
		}
		byID[r.ID] = r
	}
	for _, r := range b.Renditions {
		current, ok := byID[r.ID]
		if !ok || current.Kind != r.Kind || !sameObjectLocation(current.ObjectLocation, r.ObjectLocation) {
			return false
		}
	}
	return true
}
