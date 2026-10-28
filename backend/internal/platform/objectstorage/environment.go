package objectstorage

import (
	"net/http"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

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
