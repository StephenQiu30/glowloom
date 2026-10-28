package operation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type projectMediaSigner struct {
	registry *objectstorage.Registry
	legacy   *objectstorage.Client
}

// NewProjectMediaSigner keeps exact saved profiles distinct from original keys.
func NewProjectMediaSigner(registry *objectstorage.Registry, legacy *objectstorage.Client) application.ProjectMediaSigner {
	return projectMediaSigner{registry: registry, legacy: legacy}
}
func (s projectMediaSigner) SignProjectMedia(ctx context.Context, f application.ProjectMediaObject, ttl time.Duration) (application.ProviderReference, error) {
	if f.AssetID == [16]byte{} || f.ProjectID == [16]byte{} || f.Key == "" || f.ByteSize < 1 || f.ByteSize > 500<<20 || len(f.SHA256) != 64 || ttl < time.Second || ttl > 15*time.Minute {
		return application.ProviderReference{}, application.ErrWorkflowInputNotReady
	}
	var reader io.ReadCloser
	var err error
	ref := objectstorage.Ref{ProfileID: f.ProfileID, Bucket: f.Bucket, Key: f.Key}
	if f.LocationID != nil {
		if s.registry == nil {
			return application.ProviderReference{}, application.ErrWorkflowInputNotReady
		}
		reader, err = s.registry.Get(ctx, ref)
	} else {
		if s.legacy == nil {
			return application.ProviderReference{}, application.ErrWorkflowInputNotReady
		}
		reader, err = s.legacy.Get(ctx, f.Key)
	}
	if err != nil {
		return application.ProviderReference{}, err
	}
	defer func() { _ = reader.Close() }()
	sum := sha256.New()
	n, err := io.Copy(sum, io.LimitReader(reader, f.ByteSize+1))
	if err != nil {
		return application.ProviderReference{}, err
	}
	if n != f.ByteSize || hex.EncodeToString(sum.Sum(nil)) != f.SHA256 {
		return application.ProviderReference{}, application.ErrWorkflowInputNotReady
	}
	var url string
	if f.LocationID != nil {
		grant, err := s.registry.PresignGet(ctx, ref, ttl)
		if err != nil {
			return application.ProviderReference{}, err
		}
		url = grant.URL
	} else {
		url, err = s.legacy.PresignGet(ctx, f.Key, ttl)
		if err != nil {
			return application.ProviderReference{}, err
		}
	}
	return application.ProviderReference{Role: f.Role, MediaKind: f.Kind, URL: url}, nil
}
