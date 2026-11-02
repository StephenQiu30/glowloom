package objectstorage

import (
	"net/http"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// OpenFromConfig instantiates the active object storage client according to
// the environment configuration (GLOWLOOM_OBJECT_STORAGE_PROVIDER).
// When GLOWLOOM_OBJECT_STORAGE_PROVIDER is "cos", it returns a Tencent Cloud COS client.
// When it is "minio" (or empty with standard keys), it returns a MinIO client.
// If storage configuration is empty, it returns (nil, nil) allowing optional operation.
func OpenFromConfig(cfg config.Config) (*Client, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.ObjectStorageProvider))
	endpoint := strings.TrimSpace(cfg.ObjectStorageEndpoint)
	bucket := strings.TrimSpace(cfg.ObjectStorageBucket)
	region := strings.TrimSpace(cfg.ObjectStorageRegion)
	accessKey := cfg.ObjectStorageAccessKey
	secretKey := cfg.ObjectStorageSecretKey

	if cfg.ObjectStorage != nil && cfg.ObjectStorage.DefaultWriteProfile != "" {
		profileID := cfg.ObjectStorage.DefaultWriteProfile
		if p, ok := cfg.ObjectStorage.Profiles[profileID]; ok {
			creds, err := cfg.ObjectStorage.ResolveCredentials(p.Credentials)
			if err == nil {
				return NewStorage(Options{
					Provider:  p.Provider,
					Endpoint:  p.Endpoint,
					Bucket:    p.Bucket,
					Region:    p.Region,
					AccessKey: creds.AccessKey,
					SecretKey: creds.SecretKey,
				})
			}
		}
	}

	if endpoint == "" && bucket == "" && accessKey == "" && secretKey == "" {
		return nil, nil
	}
	if provider == "" {
		provider = "minio"
	}
	return NewStorage(Options{
		Provider:  provider,
		Endpoint:  endpoint,
		Bucket:    bucket,
		Region:    region,
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
}

// OpenLegacy preserves only a known original bucket. A newly configured native
// target never implicitly receives the existing database's unbound bare keys.
// Construction validates credentials but performs no network requests.
func OpenLegacy(cfg config.Config) (*Client, error) {
	legacy, err := cfg.LegacyStorage()
	if err != nil {
		return nil, err
	}
	if legacy != nil {
		p := legacy.Profiles[legacy.DefaultWriteProfile]
		credentials, err := legacy.ResolveCredentials(p.Credentials)
		if err != nil {
			return nil, err
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		native, err := newBackend(p, credentials, transport)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, err
		}
		return &Client{native: native, bucket: p.Bucket, readOnly: true, transport: transport}, nil
	}
	if cfg.ObjectStorageProvider != "" || cfg.ObjectStorage != nil {
		return nil, nil
	}
	if cfg.ObjectStorageEndpoint == "" && cfg.ObjectStorageBucket == "" && cfg.ObjectStorageAccessKey == "" && cfg.ObjectStorageSecretKey == "" {
		return nil, nil
	}
	return Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
}
