package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

const storageBucketContract = "private-flat-never-versioned-standard-v1"

// RecordStorageProbe retains a finite acceptance receipt for the exact resource
// and configuration. Only the operator's complete SDK/bucket proof may call it.
func (s *StorageRoutes) RecordStorageProbe(ctx context.Context, profile, configSHA, evidenceSHA string, ttl time.Duration) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if !storageDigest(configSHA) || !storageDigest(evidenceSHA) || ttl < time.Minute || ttl > 24*time.Hour {
		return application.ErrStorageNotReady
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity storageProfileRecord
		if err := tx.Where("id = ?", profile).Take(&identity).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrStorageNotReady
			}
			return err
		}
		now := time.Now().UTC()
		record := storageProbeEvidenceRecord{ID: uuid.New(), ProfileID: profile, ResourceIdentity: identity.ResourceIdentity, ConfigSHA256: configSHA, EvidenceSHA256: evidenceSHA, ExpiresAt: now.Add(ttl), RecordedAt: now}
		return tx.Create(&record).Error
	})
}

func storageExists(query *gorm.DB) (bool, error) {
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
