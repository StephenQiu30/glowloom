package postgres

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

var storageRequiredCapabilities = []string{"refs-v1", "work-v1", "legacy-v1", "operation-v1", "media-v1", "script-v1", "mediatool-v1", "cleanup-v1"}

// ReportStorageRuntime renews only an identical process identity/configuration.
// Restarted binaries get a new instance ID; stale live instances cannot disappear
// by overwriting a different role's lease.
func (s *StorageRoutes) ReportStorageRuntime(ctx context.Context, r application.StorageRuntime, ttl time.Duration) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if r.InstanceID == uuid.Nil || r.Revision == "" || !storageDigest(r.ConfigSHA256) || ttl < 30*time.Second || ttl > 2*time.Minute || len(r.Capabilities) > 32 {
		return application.ErrStorageNotReady
	}
	switch r.Role {
	case "api", "worker", "relay", "setup":
	default:
		return application.ErrStorageNotReady
	}
	caps := append([]string{}, r.Capabilities...)
	sort.Strings(caps)
	for i, capability := range caps {
		if len(capability) < 1 || len(capability) > 64 || i > 0 && caps[i-1] == capability {
			return application.ErrStorageNotReady
		}
	}
	body, err := json.Marshal(caps)
	if err != nil {
		return err
	}
	update := s.db.WithContext(ctx).Exec(`INSERT INTO media.storage_runtime(instance_id,role,revision,config_sha256,capabilities,expires_at) VALUES(?,?,?,?,?::jsonb,now()+?*interval '1 second') ON CONFLICT(instance_id) DO UPDATE SET expires_at=excluded.expires_at,updated_at=now() WHERE storage_runtime.role=excluded.role AND storage_runtime.revision=excluded.revision AND storage_runtime.config_sha256=excluded.config_sha256 AND storage_runtime.capabilities=excluded.capabilities`, r.InstanceID, r.Role, r.Revision, r.ConfigSHA256, string(body), int64(ttl/time.Second))
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageNotReady
	}
	return nil
}

// RecordStorageDeployment records reviewed rollout/drain evidence. Heartbeats
// alone cannot prove that an old binary which never reported has stopped.
func (s *StorageRoutes) RecordStorageDeployment(ctx context.Context, configSHA, evidenceSHA string) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if !storageDigest(configSHA) || !storageDigest(evidenceSHA) {
		return application.ErrStorageNotReady
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO media.storage_deployment_evidence(config_sha256,evidence_sha256) VALUES(?,?) ON CONFLICT(config_sha256) DO NOTHING`, configSHA, evidenceSHA).Error; err != nil {
			return err
		}
		var stored string
		read := tx.Raw(`SELECT evidence_sha256 FROM media.storage_deployment_evidence WHERE config_sha256=?`, configSHA).Scan(&stored)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || stored != evidenceSHA {
			return application.ErrStorageRouteConflict
		}
		return nil
	})
}

// RecordStorageProbe retains a finite acceptance receipt for the exact resource
// and configuration. Only the operator's complete SDK/bucket proof may call it.
func (s *StorageRoutes) RecordStorageProbe(ctx context.Context, profile, configSHA, evidenceSHA string, ttl time.Duration) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	if !storageDigest(configSHA) || !storageDigest(evidenceSHA) || ttl < time.Minute || ttl > 24*time.Hour {
		return application.ErrStorageNotReady
	}
	update := s.db.WithContext(ctx).Exec(`INSERT INTO media.storage_probe_evidence(id,profile_id,resource_identity,config_sha256,evidence_sha256,expires_at) SELECT ?,id,resource_identity,?,?,now()+?*interval '1 second' FROM media.storage_profile WHERE id=?`, uuid.New(), configSHA, evidenceSHA, int64(ttl/time.Second), profile)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return application.ErrStorageNotReady
	}
	return nil
}

func storageProfileNeeds(tx *gorm.DB, id string) (readNeeded, writeNeeded bool, err error) {
	var facts struct{ ReadNeeded, WriteNeeded bool }
	result := tx.Raw(`SELECT
 EXISTS(SELECT 1 FROM media.storage_legacy_binding WHERE profile_id=?) OR EXISTS(SELECT 1 FROM media.object_location WHERE profile_id=? AND state<>'deleted') AS read_needed,
 EXISTS(SELECT 1 FROM media.storage_work_intent WHERE profile_id=? AND state='open') OR EXISTS(SELECT 1 FROM media.object_location WHERE profile_id=? AND state IN ('reserved','present','unknown','delete_pending')) OR EXISTS(SELECT 1 FROM media.upload_admission WHERE profile_id=? AND accepted_asset_id IS NULL AND expires_at>clock_timestamp()) AS write_needed`, id, id, id, id, id).Scan(&facts)
	return facts.ReadNeeded, facts.WriteNeeded, result.Error
}

func storageActivationReady(tx *gorm.DB, in application.StorageActivation) (string, error) {
	if _, err := readLegacyBinding(tx); err != nil {
		return "", err
	}
	for _, column := range legacyObjectColumns {
		var missing bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM ` + column.table + ` WHERE ` + column.location + ` IS NULL)`).Scan(&missing).Error; err != nil {
			return "", err
		}
		if missing {
			return "", application.ErrStorageNotReady
		}
	}
	var evidence string
	read := tx.Raw(`SELECT evidence_sha256 FROM media.storage_deployment_evidence WHERE config_sha256=? FOR SHARE`, in.ConfigSHA256).Scan(&evidence)
	if read.Error != nil {
		return "", read.Error
	}
	if read.RowsAffected != 1 {
		return "", application.ErrStorageNotReady
	}
	required, _ := json.Marshal(storageRequiredCapabilities)
	var stale bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_runtime WHERE expires_at>clock_timestamp() AND (revision<>? OR config_sha256<>? OR NOT capabilities @> ?::jsonb))`, in.Revision, in.ConfigSHA256, string(required)).Scan(&stale).Error; err != nil {
		return "", err
	}
	if stale {
		return "", application.ErrStorageNotReady
	}
	for _, role := range []string{"api", "worker", "relay", "setup"} {
		var ready bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_runtime WHERE role=? AND revision=? AND config_sha256=? AND expires_at>clock_timestamp() AND capabilities @> ?::jsonb)`, role, in.Revision, in.ConfigSHA256, string(required)).Scan(&ready).Error; err != nil {
			return "", err
		}
		if !ready {
			return "", application.ErrStorageNotReady
		}
	}
	configured := make(map[string]string, len(in.Profiles))
	for _, p := range in.Profiles {
		if _, exists := configured[p.ID]; exists {
			return "", application.ErrStorageIdentityConflict
		}
		configured[p.ID] = p.Mode
	}
	if configured[in.DefaultWriteProfile] != "read_write" {
		return "", application.ErrStorageNotReady
	}
	var profiles []struct{ ID, ResourceIdentity string }
	if err := tx.Raw(`SELECT id,resource_identity FROM media.storage_profile ORDER BY id FOR SHARE`).Scan(&profiles).Error; err != nil {
		return "", err
	}
	for _, p := range profiles {
		readNeeded, writeNeeded, err := storageProfileNeeds(tx, p.ID)
		if err != nil {
			return "", err
		}
		mode := configured[p.ID]
		if writeNeeded && mode != "read_write" || readNeeded && mode != "read_write" && mode != "read_only" {
			return "", application.ErrStorageNotReady
		}
		if mode != "" && mode != "disabled" {
			var proved bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_probe_evidence WHERE profile_id=? AND resource_identity=? AND config_sha256=? AND expires_at>clock_timestamp())`, p.ID, p.ResourceIdentity, in.ConfigSHA256).Scan(&proved).Error; err != nil {
				return "", err
			}
			if !proved {
				return "", application.ErrStorageNotReady
			}
		}
	}
	return evidence, nil
}

// ActivateStorage checks migration, probes, rollout and every live role while
// holding the same PostgreSQL linearization lock as admissions and route CAS.
// Exact lost-reply replay succeeds without requiring a fresh lease or probe.
func (s *StorageRoutes) ActivateStorage(ctx context.Context, in application.StorageActivation) (application.StorageRoute, error) {
	var route application.StorageRoute
	if s == nil || s.db == nil {
		return route, application.ErrUnavailable
	}
	if in.UploadPolicy.Validate() != nil || in.UploadSessionTTL < time.Minute || in.UploadSessionTTL > 7*24*time.Hour || !storageDigest(in.ConfigSHA256) || in.Revision == "" || in.DefaultWriteProfile == "" || len(in.Profiles) < 1 || len(in.Profiles) > 16 {
		return route, application.ErrStorageNotReady
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0))`).Error; err != nil {
			return err
		}
		var prior string
		read := tx.Raw(`SELECT config_sha256 FROM media.storage_activation_guard WHERE revision=?`, in.Revision).Scan(&prior)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			if prior != in.ConfigSHA256 {
				return application.ErrStorageRouteConflict
			}
			var err error
			route, err = NewStorageRoutes(tx).Activate(ctx, in.ExpectedRevision, in.Revision, in.DefaultWriteProfile)
			return err
		}
		profiles := make([]application.StorageProfile, 0, len(in.Profiles))
		for _, p := range in.Profiles {
			if p.Mode != "read_write" && p.Mode != "read_only" && p.Mode != "disabled" {
				return application.ErrStorageNotReady
			}
			profiles = append(profiles, p.StorageProfile)
		}
		if err := NewStorageRoutes(tx).RegisterProfiles(ctx, profiles); err != nil {
			return err
		}
		evidence, err := storageActivationReady(tx, in)
		if err != nil {
			return err
		}
		policy, err := json.Marshal(in.UploadPolicy)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds) VALUES(?,?,?,?::jsonb,?)`, in.Revision, in.ConfigSHA256, in.DefaultWriteProfile, string(policy), int64(in.UploadSessionTTL/time.Second)).Error; err != nil {
			return err
		}
		route, err = NewStorageRoutes(tx).Activate(ctx, in.ExpectedRevision, in.Revision, in.DefaultWriteProfile)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO media.storage_activation_guard(revision,config_sha256,deployment_evidence_sha256) VALUES(?,?,?)`, in.Revision, in.ConfigSHA256, evidence).Error
	})
	return route, err
}
