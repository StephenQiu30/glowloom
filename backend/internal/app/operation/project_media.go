package operation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/netip"
	"net/url"
	"strings"
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
	var signedURL string
	if f.LocationID != nil {
		if s.registry == nil {
			return application.ProviderReference{}, application.ErrWorkflowInputNotReady
		}
		grant, err := s.registry.PresignGet(ctx, ref, ttl)
		if err != nil {
			return application.ProviderReference{}, err
		}
		signedURL = grant.URL
	} else {
		if s.legacy == nil {
			return application.ProviderReference{}, application.ErrWorkflowInputNotReady
		}
		signedURL, err = s.legacy.PresignGet(ctx, f.Key, ttl)
		if err != nil {
			return application.ProviderReference{}, err
		}
	}
	inline := f.Kind == "image" && privateImageURL(signedURL)
	if inline && f.ByteSize > 30<<20 {
		return application.ProviderReference{}, application.ErrWorkflowInputNotReady
	}
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
	var imageData bytes.Buffer
	var checked io.Writer = sum
	if inline {
		checked = io.MultiWriter(sum, &imageData)
	}
	n, err := io.Copy(checked, io.LimitReader(reader, f.ByteSize+1))
	if err != nil {
		return application.ProviderReference{}, err
	}
	if n != f.ByteSize || hex.EncodeToString(sum.Sum(nil)) != f.SHA256 {
		return application.ProviderReference{}, application.ErrWorkflowInputNotReady
	}
	return application.ProviderReference{Role: f.Role, MediaKind: f.Kind, URL: signedURL, ImageBytes: imageData.Bytes()}, nil
}

// Private object URLs remain valid for local previews, but not for cloud fetches.
func privateImageURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback())
}
