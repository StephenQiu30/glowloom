package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// BeginStorageProbe persists exact owned keys before SDK I/O.
func (s *StorageRoutes) BeginStorageProbe(ctx context.Context, in application.StorageProbeIntent) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if in.ID == uuid.Nil || !storageDigest(in.ConfigSHA256) || (domain.ObjectRef{ProfileID: in.ProfileID, Bucket: in.Bucket, Key: in.SentinelKey}).Validate() != nil || in.Mode != "read_only" && in.Mode != "read_write" || in.Mode == "read_write" && in.RemovalKey != in.SentinelKey+".remove" || in.Mode == "read_only" && in.RemovalKey != "" {
		return application.ErrStorageNotReady
	}
	var removal *string
	if in.RemovalKey != "" {
		removal = &in.RemovalKey
	}
	return s.db.WithContext(ctx).Exec(`INSERT INTO media.storage_probe_intent(id,profile_id,bucket,config_sha256,sentinel_key,removal_key,mode) VALUES(?,?,?,?,?,?,?)`, in.ID, in.ProfileID, in.Bucket, in.ConfigSHA256, in.SentinelKey, removal, in.Mode).Error
}

// CompleteStorageProbe cannot publish success after cleanup acquired ownership.
func (s *StorageRoutes) CompleteStorageProbe(ctx context.Context, id uuid.UUID, evidence string) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if !storageDigest(evidence) {
		return application.ErrStorageNotReady
	}
	update := s.db.WithContext(ctx).Exec(`UPDATE media.storage_probe_intent SET state='complete',evidence_sha256=?,updated_at=clock_timestamp() WHERE id=? AND (state='running' OR state='complete' AND evidence_sha256=?)`, evidence, id, evidence)
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
	update := s.db.WithContext(ctx).Exec(`INSERT INTO media.storage_bucket_evidence(id,profile_id,resource_identity,config_sha256,evidence_sha256,contract,expires_at) SELECT ?,id,resource_identity,?,?,'private-flat-never-versioned-standard-v1',clock_timestamp()+?*interval '1 second' FROM media.storage_profile WHERE id=?`, uuid.New(), configSHA, evidenceSHA, int64(ttl/time.Second), profile)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageNotReady
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
	read := s.db.WithContext(ctx).Raw(`UPDATE media.storage_probe_intent SET state='cleaning',updated_at=clock_timestamp() WHERE id=? AND state IN ('running','cleaning') AND created_at<clock_timestamp()-interval '10 minutes' RETURNING id,profile_id,bucket,config_sha256,sentinel_key,COALESCE(removal_key,'') AS removal_key,mode,state`, id).Scan(&intent)
	if read.Error != nil {
		return intent, read.Error
	}
	if read.RowsAffected != 1 {
		read = s.db.WithContext(ctx).Raw(`SELECT id,profile_id,bucket,config_sha256,sentinel_key,COALESCE(removal_key,'') AS removal_key,mode,state FROM media.storage_probe_intent WHERE id=? AND state='cleaned'`, id).Scan(&intent)
		if read.Error != nil {
			return intent, read.Error
		}
		if read.RowsAffected != 1 {
			return intent, application.ErrStorageNotReady
		}
	}
	return intent, nil
}

// CompleteStorageProbeCleanup follows authoritative absence of every owned key.
func (s *StorageRoutes) CompleteStorageProbeCleanup(ctx context.Context, id uuid.UUID) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	update := s.db.WithContext(ctx).Exec(`UPDATE media.storage_probe_intent SET state='cleaned',updated_at=clock_timestamp() WHERE id=? AND state IN ('cleaning','cleaned')`, id)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageNotReady
	}
	return nil
}
