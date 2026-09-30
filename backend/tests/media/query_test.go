package media_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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

func (s *previewSigner) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	s.calls++
	s.key = key
	s.ttl = ttl
	return "https://fixture.invalid/signed", nil
}
func TestMediaPreviewAuthorizesBeforeSigningAndDoesNotExposeStorageFacts(t *testing.T) {
	p, id := uuid.New(), uuid.New()
	signer := &previewSigner{}
	store := readStore{asset: domain.MediaAsset{ID: id, ProjectID: p, Kind: domain.KindImage, Status: domain.StatusReady, ModerationStatus: domain.ModerationPassed, ObjectKey: "projects/" + p.String() + "/image/safe.png", FileName: "safe.png", MimeType: "image/png", ByteSize: 1, Revision: 1}}
	query := application.NewAssetQuery(store, signer)
	result, err := query.Preview(t.Context(), identityapp.Principal{}, p, id)
	if err != nil || result.Asset.ID != id || signer.calls != 1 || signer.ttl > 15*time.Minute || result.ExpiresAt.IsZero() {
		t.Fatalf("preview %+v %v", result, err)
	}
	store.err = application.ErrNotFound
	query = application.NewAssetQuery(store, signer)
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, p, id); !errors.Is(err, application.ErrNotFound) || signer.calls != 1 {
		t.Fatalf("unauthorized signing: %v calls %d", err, signer.calls)
	}
	store.err = nil
	store.asset.ModerationStatus = domain.ModerationRejected
	query = application.NewAssetQuery(store, signer)
	if _, err := query.Reference(t.Context(), identityapp.Principal{}, p, id); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("unsafe reference accepted: %v", err)
	}
	store.asset.ModerationStatus = domain.ModerationPassed
	query = application.NewAssetQuery(store, nil)
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, p, id); !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("nil signer: %v", err)
	}
}
