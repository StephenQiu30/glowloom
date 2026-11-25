package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// BeginStorageProbe persists exact owned keys before SDK I/O.
func (s *StorageRoutes) BeginStorageProbe(ctx context.Context, in application.StorageProbeIntent) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if in.ID == uuid.Nil || !storageDigest(in.ConfigSHA256) || (domain.ObjectRef{ProfileID: in.ProfileID, Bucket: in.Bucket, Key: in.SentinelKey}).Validate() != nil || in.Mode != application.StorageProfileModeReadOnly && in.Mode != application.StorageProfileModeReadWrite || in.Mode == application.StorageProfileModeReadWrite && in.RemovalKey != in.SentinelKey+".remove" || in.Mode == application.StorageProfileModeReadOnly && in.RemovalKey != "" {
		return application.ErrStorageNotReady
	}
	var removal *string
	if in.RemovalKey != "" {
		removal = &in.RemovalKey
	}
	record := storageProbeIntentRecord{ID: in.ID, ProfileID: in.ProfileID, Bucket: in.Bucket, ConfigSHA256: in.ConfigSHA256, SentinelKey: in.SentinelKey, RemovalKey: removal, Mode: string(in.Mode), State: application.StorageProbeRunning}
	return s.db.WithContext(ctx).Create(&record).Error
}

// CompleteStorageProbe cannot publish success after cleanup acquired ownership.
func (s *StorageRoutes) CompleteStorageProbe(ctx context.Context, id uuid.UUID, evidence string) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if !storageDigest(evidence) {
		return application.ErrStorageNotReady
	}
	now := time.Now().UTC()
	update := s.db.WithContext(ctx).Model(&storageProbeIntentRecord{}).
		Where("id = ? AND (state = ? OR (state = ? AND evidence_sha256 = ?))", id, application.StorageProbeRunning, application.StorageProbeComplete, evidence).
		Updates(map[string]any{"state": application.StorageProbeComplete, "evidence_sha256": evidence, "updated_at": now})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageRouteConflict
	}
	return nil
}

// RecordStorageBucketEvidence records a separately reviewed actual-resource
// proof. Functional object probes cannot establish private/flat/version history.
func (s *StorageRoutes) RecordStorageBucketEvidence(ctx context.Context, profile, configSHA, evidenceSHA string, ttl time.Duration) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if !storageDigest(configSHA) || !storageDigest(evidenceSHA) || ttl < time.Minute || ttl > 24*time.Hour {
		return application.ErrStorageNotReady
	}
	var identity storageProfileRecord
	if err := s.db.WithContext(ctx).Where("id = ?", profile).Take(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrStorageNotReady
		}
		return err
	}
	now := time.Now().UTC()
	record := storageBucketEvidenceRecord{ID: uuid.New(), ProfileID: identity.ID, ResourceIdentity: identity.ResourceIdentity, ConfigSHA256: configSHA, EvidenceSHA256: evidenceSHA, Contract: storageBucketContract, ExpiresAt: now.Add(ttl), CreatedAt: now}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return err
	}
	return nil
}

// ClaimStorageProbeCleanup closes success publication before object I/O. The
// ten-minute drain exceeds the probe's single two-minute execution context.
func (s *StorageRoutes) ClaimStorageProbeCleanup(ctx context.Context, id uuid.UUID) (application.StorageProbeIntent, error) {
	var intent application.StorageProbeIntent
	if s == nil || s.db == nil || id == uuid.Nil {
		return intent, application.ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record storageProbeIntentRecord
		read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&record)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrStorageNotReady
			}
			return read.Error
		}
		if record.State != application.StorageProbeCleaned {
			now := time.Now().UTC()
			if record.State != application.StorageProbeRunning && record.State != application.StorageProbeCleaning || record.CreatedAt.After(now.Add(-10*time.Minute)) {
				return application.ErrStorageNotReady
			}
			record.State, record.UpdatedAt = application.StorageProbeCleaning, now
			if err := tx.Model(&record).Select("state", "updated_at").Updates(&record).Error; err != nil {
				return err
			}
		}
		intent = application.StorageProbeIntent{ID: record.ID, ProfileID: record.ProfileID, Bucket: record.Bucket, ConfigSHA256: record.ConfigSHA256, SentinelKey: record.SentinelKey, Mode: application.StorageProbeMode(record.Mode), State: record.State}
		if record.RemovalKey != nil {
			intent.RemovalKey = *record.RemovalKey
		}
		return nil
	})
	if err != nil {
		return intent, err
	}
	return intent, nil
}

// CompleteStorageProbeCleanup follows authoritative absence of every owned key.
func (s *StorageRoutes) CompleteStorageProbeCleanup(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	update := s.db.WithContext(ctx).Model(&storageProbeIntentRecord{}).
		Where("id = ? AND state IN ?", id, []application.StorageProbeState{application.StorageProbeCleaning, application.StorageProbeCleaned}).
		Updates(map[string]any{"state": application.StorageProbeCleaned, "updated_at": time.Now().UTC()})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageNotReady
	}
	return nil
}
