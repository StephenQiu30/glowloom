package media_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type readStore struct {
	asset domain.MediaAsset
	err   error
}

func (s readStore) FindAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.MediaAsset, error) {
	return s.asset, s.err
}
func (s readStore) ListReadyAssets(context.Context, identityapp.Principal, uuid.UUID, string, uuid.UUID, int) ([]domain.MediaAsset, error) {
	return []domain.MediaAsset{s.asset}, s.err
}

type previewSigner struct {
	calls int
	key   string
	ttl   time.Duration
}

func (s *previewSigner) PresignGet(_ context.Context, ref domain.ObjectRef, ttl time.Duration) (application.AccessGrant, error) {
	s.calls++
	s.key = ref.Key
	s.ttl = ttl
	return application.AccessGrant{URL: "https://fixture.invalid/signed", ExpiresAt: time.Now().Add(ttl)}, nil
}
func TestMediaPreviewAuthorizesBeforeSigningAndDoesNotExposeStorageFacts(t *testing.T) {
	p, id := uuid.New(), uuid.New()
	signer := &previewSigner{}
	store := readStore{asset: domain.MediaAsset{ID: id, ProjectID: p, Kind: domain.KindImage, Status: domain.StatusReady, ModerationStatus: domain.ModerationPassed, ObjectKey: "projects/" + p.String() + "/image/safe.png", FileName: "safe.png", MimeType: "image/png", ByteSize: 1, Revision: 1}}
	store.asset.ObjectLocation = readLocation("minio-main", store.asset.ObjectKey)
	query := application.NewRoutedAssetQuery(store, signer, 5*time.Minute)
	result, err := query.Preview(t.Context(), identityapp.Principal{}, p, id)
	if err != nil || result.Asset.ID != id || signer.calls != 1 || signer.ttl > 15*time.Minute || result.ExpiresAt.IsZero() {
		t.Fatalf("preview %+v %v", result, err)
	}
	store.err = application.ErrNotFound
	query = application.NewRoutedAssetQuery(store, signer, 5*time.Minute)
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, p, id); !errors.Is(err, application.ErrNotFound) || signer.calls != 1 {
		t.Fatalf("unauthorized signing: %v calls %d", err, signer.calls)
	}
	store.err = nil
	store.asset.ModerationStatus = domain.ModerationRejected
	query = application.NewRoutedAssetQuery(store, signer, 5*time.Minute)
	if _, err := query.Reference(t.Context(), identityapp.Principal{}, p, id); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("unsafe reference accepted: %v", err)
	}
	store.asset.ModerationStatus = domain.ModerationPassed
	query = application.NewAssetQuery(store)
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, p, id); !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("nil signer: %v", err)
	}
}
