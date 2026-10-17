package storageoperator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// ProbeReceipt contains public evidence only. SentinelKey is intentionally
// retained so a later read-only configuration can verify this same resource.
type ProbeReceipt struct {
	ProbeID                                                                    uuid.UUID
	ProfileID, ConfigSHA256, ResourceSHA256, Mode, SentinelKey, EvidenceSHA256 string
	VerifiedAt                                                                 time.Time
}

// Probe uses the official SDK to test conditional create, actual readback,
// signed GET and confirmed removal in a unique owned prefix. A read-only probe
// requires a previously retained sentinel; it never attempts a write.
func Probe(ctx context.Context, cfg config.ObjectStorageConfig, profileID, sentinel string, tracker mediaapp.StorageProbeTracker) (result ProbeReceipt, err error) {
	if tracker == nil {
		return result, mediaapp.ErrStorageNotReady
	}
	in, err := Configuration(cfg, "")
	if err != nil {
		return result, err
	}
	p, found := cfg.Profiles[profileID]
	if !found || p.Mode == "disabled" {
		return result, objectstorage.ErrInvalidObject
	}
	profiles := make([]mediaapp.StorageProfile, 0, len(in.Profiles))
	for _, profile := range in.Profiles {
		profiles = append(profiles, profile.StorageProfile)
	}
	if err := tracker.RegisterProfiles(ctx, profiles); err != nil {
		return result, err
	}
	r, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		return result, err
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	data := []byte("glowloom-object-storage-probe-v1\n")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	resource := sha256.Sum256([]byte(p.ResourceIdentity()))
	result = ProbeReceipt{ProbeID: uuid.New(), ProfileID: profileID, ConfigSHA256: in.ConfigSHA256, ResourceSHA256: hex.EncodeToString(resource[:]), Mode: p.Mode, VerifiedAt: time.Now().UTC()}
	if p.Mode == "read_write" {
		if sentinel != "" {
			return result, objectstorage.ErrInvalidObject
		}
		sentinel = "private/storage-probes/" + uuid.NewString() + "/sentinel.txt"
	}
	if !probeSentinel(sentinel) {
		return result, objectstorage.ErrInvalidObject
	}
	result.SentinelKey = sentinel
	intent := mediaapp.StorageProbeIntent{ID: result.ProbeID, ProfileID: profileID, Bucket: p.Bucket, ConfigSHA256: in.ConfigSHA256, SentinelKey: sentinel, Mode: p.Mode}
	if p.Mode == "read_write" {
		intent.RemovalKey = sentinel + ".remove"
	}
	if err := tracker.BeginStorageProbe(ctx, intent); err != nil {
		return result, err
	}
	if p.Mode == "read_write" {
		ref := objectstorage.Ref{ProfileID: profileID, Bucket: p.Bucket, Key: sentinel}
		if err := r.PutIfAbsent(ctx, ref, bytes.NewReader(data), int64(len(data)), "text/plain", digest); err != nil {
			return result, err
		}
		other := []byte("must-never-overwrite")
		otherSum := sha256.Sum256(other)
		if err := r.PutIfAbsent(ctx, ref, bytes.NewReader(other), int64(len(other)), "text/plain", hex.EncodeToString(otherSum[:])); !errors.Is(err, objectstorage.ErrObjectExists) {
			return result, errors.New("storage probe conditional create not proved")
		}
		removeRef := ref
		removeRef.Key = sentinel + ".remove"
		if err := r.PutIfAbsent(ctx, removeRef, bytes.NewReader(data), int64(len(data)), "text/plain", digest); err != nil {
			return result, err
		}
		if err := r.Remove(ctx, removeRef); err != nil {
			return result, err
		}
		if _, err := r.Stat(ctx, removeRef); !errors.Is(err, objectstorage.ErrObjectMissing) {
			return result, errors.New("storage probe removal not proved")
		}
	}
	if len(sentinel) == 0 || !probeSentinel(sentinel) {
		return result, objectstorage.ErrInvalidObject
	}
	result.SentinelKey = sentinel
	ref := objectstorage.Ref{ProfileID: profileID, Bucket: p.Bucket, Key: sentinel}
	info, err := r.Stat(ctx, ref)
	if err != nil {
		return result, err
	}
	if info.Size != int64(len(data)) || info.SHA256 != digest || info.ContentType != "text/plain" {
		return result, errors.New("storage probe metadata mismatch")
	}
	body, err := r.Get(ctx, ref)
	if err != nil {
		return result, err
	}
	actual, readErr := io.ReadAll(io.LimitReader(body, int64(len(data)+1)))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(actual, data) {
		return result, errors.New("storage probe readback mismatch")
	}
	grant, err := r.PresignGet(ctx, ref, min(time.Minute, cfg.Access.MaxSignedURLTTL))
	if err != nil {
		return result, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, grant.URL, nil)
	if err != nil {
		return result, errors.New("storage probe signed request invalid")
	}
	// A redirect must not move an access-bearing URL to another origin.
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return result, errors.New("storage probe signed request failed")
	}
	actual, readErr = io.ReadAll(io.LimitReader(response.Body, int64(len(data)+1)))
	closeErr = response.Body.Close()
	if response.StatusCode != http.StatusOK || readErr != nil || closeErr != nil || !bytes.Equal(actual, data) || !time.Now().Before(grant.ExpiresAt) {
		return result, errors.New("storage probe signed read not proved")
	}
	publicJSON, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	evidence := sha256.Sum256(publicJSON)
	result.EvidenceSHA256 = hex.EncodeToString(evidence[:])
	if err := tracker.CompleteStorageProbe(ctx, result.ProbeID, result.EvidenceSHA256); err != nil {
		return result, err
	}
	return result, nil
}

func probeSentinel(key string) bool {
	const prefix = "private/storage-probes/"
	if len(key) != len(prefix)+36+len("/sentinel.txt") || key[:len(prefix)] != prefix || key[len(prefix)+36:] != "/sentinel.txt" {
		return false
	}
	_, err := uuid.Parse(key[len(prefix) : len(prefix)+36])
	return err == nil
}

// CleanupProbe recovers failed/unknown diagnostics using their persisted exact
// keys. Read-only runs never own the pre-existing sentinel and cannot remove it.
func CleanupProbe(ctx context.Context, cfg config.ObjectStorageConfig, id uuid.UUID, tracker mediaapp.StorageProbeCleanup) error {
	if tracker == nil {
		return mediaapp.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	configuration, err := Configuration(cfg, "")
	if err != nil {
		return err
	}
	profiles := make([]mediaapp.StorageProfile, 0, len(configuration.Profiles))
	for _, profile := range configuration.Profiles {
		profiles = append(profiles, profile.StorageProfile)
	}
	if err := tracker.RegisterProfiles(ctx, profiles); err != nil {
		return err
	}
	intent, err := tracker.ClaimStorageProbeCleanup(ctx, id)
	if err != nil {
		return err
	}
	if intent.ID != id || !probeSentinel(intent.SentinelKey) || intent.Mode != "read_only" && intent.Mode != "read_write" {
		return mediaapp.ErrStorageIdentityConflict
	}
	if intent.State == "cleaned" {
		return nil
	}
	if intent.Mode == "read_only" {
		return tracker.CompleteStorageProbeCleanup(ctx, id)
	}
	if intent.RemovalKey != intent.SentinelKey+".remove" {
		return mediaapp.ErrStorageIdentityConflict
	}
	r, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, key := range []string{intent.SentinelKey, intent.RemovalKey} {
		ref := objectstorage.Ref{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}
		if err := r.Remove(ctx, ref); err != nil && !errors.Is(err, objectstorage.ErrObjectMissing) {
			return err
		}
		if _, err := r.Stat(ctx, ref); !errors.Is(err, objectstorage.ErrObjectMissing) {
			return errors.New("probe cleanup absence not proved")
		}
	}
	return tracker.CompleteStorageProbeCleanup(ctx, id)
}
