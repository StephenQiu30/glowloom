// Package storageoperator provides explicit storage maintenance commands.
// Configuration normalization never applies a write route or changes object identity.
package storageoperator

import (
	"sort"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// Configuration binds the normalized public configuration and credential source
// references without resolving or persisting secret values.
func Configuration(cfg config.ObjectStorageConfig) (mediaapp.StorageEnvironment, error) {
	cfg, err := cfg.Validated()
	if err != nil {
		return mediaapp.StorageEnvironment{}, err
	}
	digest, err := cfg.ConfigSHA256()
	if err != nil {
		return mediaapp.StorageEnvironment{}, err
	}
	in := mediaapp.StorageEnvironment{Revision: cfg.Revision, DefaultWriteProfile: cfg.DefaultWriteProfile, ConfigSHA256: digest, UploadSessionTTL: cfg.Upload.SessionTTL,
		UploadPolicy: mediaapp.UploadPolicy{MaxImageBytes: cfg.Limits.MaxImageBytes, MaxVideoBytes: cfg.Limits.MaxVideoBytes, MaxAudioBytes: cfg.Limits.MaxAudioBytes, MaxModelBytes: cfg.Limits.MaxModelBytes, MaxDocumentBytes: cfg.Limits.MaxDocumentBytes}}
	ids := make([]string, 0, len(cfg.Profiles))
	for id := range cfg.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := cfg.Profiles[id]
		in.Profiles = append(in.Profiles, mediaapp.StorageDeploymentProfile{StorageProfile: mediaapp.StorageProfile{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: string(p.AddressingStyle)}, Mode: mediaapp.StorageProfileMode(p.Mode)})
	}
	return in, nil
}
