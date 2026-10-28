package objectstorage

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// Registry owns immutable profile identities. The database, not this factory,
// decides which profile accepts a new intent.
type Registry struct {
	cfg       config.ObjectStorageConfig
	transport *http.Transport
}

type backend interface {
	put(context.Context, string, io.Reader, int64, string, string) error
	stat(context.Context, string) (ObjectInfo, error)
	get(context.Context, string) (io.ReadCloser, error)
	remove(context.Context, string) error
	sign(context.Context, string, time.Duration) (string, error)
	list(context.Context, string, int) ([]ObjectMetadata, error)
}

// OpenProfiles checks every declared identity and credential reference without
// contacting a provider. Credentials are resolved again per operation, so a
// replaced secret file or refreshed session token does not need a process restart.
func OpenProfiles(cfg config.ObjectStorageConfig) (*Registry, error) {
	cfg, err := cfg.Validated()
	if err != nil {
		return nil, err
	}
	r := &Registry{cfg: cfg, transport: http.DefaultTransport.(*http.Transport).Clone()}
	for _, p := range cfg.Profiles {
		if p.Mode != "disabled" {
			if _, err := cfg.ResolveCredentials(p.Credentials); err != nil {
				r.Close()
				return nil, err
			}
		}
	}
	return r, nil
}

// Close releases idle connections after all consumers have stopped.
func (r *Registry) Close() {
	if r != nil && r.transport != nil {
		r.transport.CloseIdleConnections()
	}
}

func (r *Registry) resolve(ref Ref, write bool, signing bool) (backend, config.StorageCredentials, error) {
	if r == nil || !validKey(ref.Key) || len(ref.Key) > 696 || strings.ContainsAny(ref.Key, "\x00\r\n") {
		return nil, config.StorageCredentials{}, ErrInvalidObject
	}
	p, ok := r.cfg.Profiles[ref.ProfileID]
	if !ok || p.Bucket != ref.Bucket {
		return nil, config.StorageCredentials{}, ErrInvalidObject
	}
	if p.Mode == "disabled" || write && p.Mode != "read_write" {
		return nil, config.StorageCredentials{}, ErrAccessDenied
	}
	cred, err := r.cfg.ResolveCredentials(p.Credentials)
	if err != nil {
		return nil, cred, err
	}
	if signing {
		p.Endpoint = p.SigningEndpoint
	}
	b, err := newBackend(p, cred, r.transport)
	if err != nil {
		return nil, cred, storageError("configure", ref.ProfileID, err)
	}
	return b, cred, nil
}

// CheckWrite resolves the exact immutable route and current credentials without
// network I/O. A paid producer can reject missing output admission before send.
func (r *Registry) CheckWrite(ctx context.Context, ref Ref) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, _, err := r.resolve(ref, true, false)
	return err
}

// PutIfAbsent performs one atomic create. The relay pipeline's bounded objects
// fit single PUT; automatic SDK multipart is deliberately disabled until its
// separate immutable completion contract is accepted.
func (r *Registry) PutIfAbsent(ctx context.Context, ref Ref, reader io.Reader, size int64, contentType, sha string) error {
	if reader == nil || size < 0 || size > 5<<30 || contentType == "" || strings.ContainsAny(contentType, "\r\n") || len(sha) != 64 {
		return ErrInvalidObject
	}
	if _, err := hex.DecodeString(sha); err != nil || sha != strings.ToLower(sha) {
		return ErrInvalidObject
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b, _, err := r.resolve(ref, true, false)
	if err != nil {
		return err
	}
	return storageError("create", ref.ProfileID, b.put(ctx, ref.Key, reader, size, contentType, sha))
}

// Stat reads facts from exactly the selected profile and bucket.
func (r *Registry) Stat(ctx context.Context, ref Ref) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	b, _, err := r.resolve(ref, false, false)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := b.stat(ctx, ref.Key)
	return info, storageError("stat", ref.ProfileID, err)
}

// Get opens one exact object and normalizes deferred read errors.
func (r *Registry) Get(ctx context.Context, ref Ref) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, _, err := r.resolve(ref, false, false)
	if err != nil {
		return nil, err
	}
	body, err := b.get(ctx, ref.Key)
	if err != nil {
		return nil, storageError("get", ref.ProfileID, err)
	}
	return &checkedReader{ReadCloser: body, profile: ref.ProfileID}, nil
}

type checkedReader struct {
	io.ReadCloser
	profile string
}

func (r *checkedReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err == io.EOF {
		return n, err
	}
	return n, storageError("read", r.profile, err)
}
func (r *checkedReader) Close() error { return storageError("close", r.profile, r.ReadCloser.Close()) }

// Remove deletes one owned reference; callers arbitrate acceptance and cleanup first.
func (r *Registry) Remove(ctx context.Context, ref Ref) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b, _, err := r.resolve(ref, true, false)
	if err != nil {
		return err
	}
	return storageError("remove", ref.ProfileID, b.remove(ctx, ref.Key))
}

// PresignGet uses the official signer and bounds its grant by credential expiry.
func (r *Registry) PresignGet(ctx context.Context, ref Ref, ttl time.Duration) (AccessGrant, error) {
	if r == nil || ttl < time.Second || ttl > r.cfg.Access.MaxSignedURLTTL {
		return AccessGrant{}, ErrInvalidObject
	}
	if err := ctx.Err(); err != nil {
		return AccessGrant{}, err
	}
	b, cred, err := r.resolve(ref, false, true)
	if err != nil {
		return AccessGrant{}, err
	}
	now := time.Now().UTC()
	if !cred.ExpiresAt.IsZero() {
		ttl = min(ttl, cred.ExpiresAt.Sub(now)-30*time.Second)
	}
	ttl = ttl.Truncate(time.Second)
	if ttl < time.Second {
		return AccessGrant{}, ErrAccessDenied
	}
	u, err := b.sign(ctx, ref.Key, ttl)
	if err != nil {
		return AccessGrant{}, storageError("sign", ref.ProfileID, err)
	}
	return AccessGrant{URL: u, ExpiresAt: now.Add(ttl)}, nil
}

// ListMetadata returns a complete bounded inventory or no data on any failure.
func (r *Registry) ListMetadata(ctx context.Context, profileID, bucket, prefix string, maxItems int) ([]ObjectMetadata, error) {
	if maxItems < 1 || maxItems > 50000 || !strings.HasSuffix(prefix, "/") || strings.Count(prefix, "/") < 2 {
		return nil, ErrInvalidObject
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ref := Ref{ProfileID: profileID, Bucket: bucket, Key: strings.TrimSuffix(prefix, "/")}
	b, _, err := r.resolve(ref, false, false)
	if err != nil {
		return nil, err
	}
	items, err := b.list(ctx, prefix, maxItems)
	if err != nil {
		return nil, storageError("list", profileID, err)
	}
	if len(items) > maxItems {
		return nil, ErrInventoryLimit
	}
	seen := make(map[string]bool, len(items))
	for i, item := range items {
		if !validKey(item.Key) || !strings.HasPrefix(item.Key, prefix) || item.Size < 0 || seen[item.Key] {
			return nil, ErrInvalidObject
		}
		seen[item.Key] = true
		items[i].Ref = Ref{ProfileID: profileID, Bucket: bucket, Key: item.Key}
	}
	return items, nil
}
