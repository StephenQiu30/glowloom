// Package storageoperator provides explicit maintenance commands. Loading a
// configuration never activates it or changes an existing object's identity.
package storageoperator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// Configuration binds the normalized public configuration, including policy and
// credential source references, without resolving or persisting secret values.
func Configuration(cfg config.ObjectStorageConfig, expected string) (mediaapp.StorageActivation, error) {
	cfg, err := cfg.Validated()
	if err != nil {
		return mediaapp.StorageActivation{}, err
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		return mediaapp.StorageActivation{}, err
	}
	sum := sha256.Sum256(body)
	in := mediaapp.StorageActivation{ExpectedRevision: expected, Revision: cfg.Revision, DefaultWriteProfile: cfg.DefaultWriteProfile, ConfigSHA256: hex.EncodeToString(sum[:]), UploadSessionTTL: cfg.Upload.SessionTTL,
		UploadPolicy: mediaapp.UploadPolicy{MaxImageBytes: cfg.Limits.MaxImageBytes, MaxVideoBytes: cfg.Limits.MaxVideoBytes, MaxAudioBytes: cfg.Limits.MaxAudioBytes, MaxModelBytes: cfg.Limits.MaxModelBytes, MaxDocumentBytes: cfg.Limits.MaxDocumentBytes}}
	ids := make([]string, 0, len(cfg.Profiles))
	for id := range cfg.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := cfg.Profiles[id]
		in.Profiles = append(in.Profiles, mediaapp.StorageDeploymentProfile{StorageProfile: mediaapp.StorageProfile{ID: id, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: p.AddressingStyle}, Mode: p.Mode})
	}
	return in, nil
}
