package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// ApplyEnvironment records the validated process configuration and makes its
// default profile the shared write route. Environment configuration is the only
// source of route changes; revisions cannot replace a newer projection.
func (s *StorageRoutes) ApplyEnvironment(ctx context.Context, environment application.StorageEnvironment) (application.StorageRoute, error) {
	var route application.StorageRoute
	if s == nil || s.db == nil || environment.Revision == "" || environment.DefaultWriteProfile == "" || !storageDigest(environment.ConfigSHA256) || environment.UploadPolicy.Validate() != nil || environment.UploadSessionTTL < time.Minute {
		return route, application.ErrStorageNotReady
	}
	profiles := make([]application.StorageProfile, 0, len(environment.Profiles))
	nextRevision, err := config.ParseStorageRevision(environment.Revision)
	if err != nil {
		return route, application.ErrStorageNotReady
	}
	profileModes := make(map[string]application.StorageProfileMode, len(environment.Profiles))
	for _, configured := range environment.Profiles {
		if configured.Mode != application.StorageProfileModeReadWrite && configured.Mode != application.StorageProfileModeReadOnly && configured.Mode != application.StorageProfileModeDisabled {
			return route, application.ErrStorageNotReady
		}
		if _, exists := profileModes[configured.ID]; exists {
			return route, application.ErrStorageIdentityConflict
		}
		profileModes[configured.ID] = configured.Mode
		profiles = append(profiles, configured.StorageProfile)
	}
	if profileModes[environment.DefaultWriteProfile] != application.StorageProfileModeReadWrite || len(profiles) == 0 {
		return route, application.ErrStorageNotReady
	}
	policy, err := json.Marshal(environment.UploadPolicy)
	if err != nil {
		return route, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStorageRoute(tx); err != nil {
			return err
		}
		var knownRevisions []string
		if err := tx.Model(&storageConfigRevisionRecord{}).Pluck("revision", &knownRevisions).Error; err != nil {
			return err
		}
		var highestRevision uint64
		for _, known := range knownRevisions {
			revision, err := config.ParseStorageRevision(known)
			if err != nil {
				return application.ErrStorageRouteConflict
			}
			highestRevision = max(highestRevision, revision)
		}
		if nextRevision < highestRevision {
			return application.ErrStorageRouteConflict
		}
		activeRevision := false
		currentRoute, err := readStorageRoute(tx, false)
		if err == nil {
			currentRevision, parseErr := config.ParseStorageRevision(currentRoute.Revision)
			if parseErr != nil || nextRevision < currentRevision || nextRevision == currentRevision && currentRoute.ProfileID != environment.DefaultWriteProfile {
				return application.ErrStorageRouteConflict
			}
			activeRevision = nextRevision == currentRevision
		} else if !errors.Is(err, application.ErrStorageNotReady) {
			return err
		}
		if err := registerStorageProfiles(tx, profiles); err != nil {
			return err
		}
		var referencedProfiles []string
		if err := tx.Model(&objectLocationRow{}).Distinct("profile_id").Where("state <> ?", domain.ObjectLocationDeleted).Pluck("profile_id", &referencedProfiles).Error; err != nil {
			return err
		}
		if err := requireConfiguredProfiles(profileModes, referencedProfiles, false); err != nil {
			return err
		}
		var openWorkProfiles []string
		if err := tx.Model(&storageWorkRow{}).Distinct("profile_id").Where("state = ?", application.StorageWorkOpen).Pluck("profile_id", &openWorkProfiles).Error; err != nil {
			return err
		}
		if err := requireConfiguredProfiles(profileModes, openWorkProfiles, true); err != nil {
			return err
		}
		var pendingUploadProfiles []string
		if err := tx.Model(&uploadAdmissionRow{}).Distinct("profile_id").Where("accepted_asset_id IS NULL AND expires_at > ?", time.Now().UTC()).Pluck("profile_id", &pendingUploadProfiles).Error; err != nil {
			return err
		}
		if err := requireConfiguredProfiles(profileModes, pendingUploadProfiles, true); err != nil {
			return err
		}
		var profile storageProfileRecord
		if err := tx.Where("id = ?", environment.DefaultWriteProfile).Take(&profile).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return application.ErrStorageNotReady
			}
			return err
		}
		var snapshot storageConfigRevisionRecord
		result := tx.Where("revision = ?", environment.Revision).Take(&snapshot)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}
		if result.Error == nil {
			var savedPolicy application.UploadPolicy
			decoder := json.NewDecoder(bytes.NewReader(snapshot.UploadPolicy))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&savedPolicy) != nil || snapshot.ConfigSHA256 != environment.ConfigSHA256 || snapshot.ProfileID != environment.DefaultWriteProfile || savedPolicy != environment.UploadPolicy || snapshot.SessionTTLSeconds != int64(environment.UploadSessionTTL/time.Second) {
				return application.ErrStorageRouteConflict
			}
		} else {
			if activeRevision {
				return application.ErrStorageRouteConflict
			}
			snapshot = storageConfigRevisionRecord{
				Revision: environment.Revision, ConfigSHA256: environment.ConfigSHA256,
				ProfileID: environment.DefaultWriteProfile, UploadPolicy: jsonDocument(policy),
				SessionTTLSeconds: int64(environment.UploadSessionTTL / time.Second), CreatedAt: time.Now().UTC(),
			}
			if err := tx.Create(&snapshot).Error; err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		record := storageRouteRecord{Singleton: storageRouteSingleton, Revision: environment.Revision, ProfileID: environment.DefaultWriteProfile, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "singleton"}}, DoUpdates: clause.Assignments(map[string]any{"revision": environment.Revision, "profile_id": environment.DefaultWriteProfile, "updated_at": now})}).Create(&record).Error; err != nil {
			return err
		}
		route = application.StorageRoute{ProfileID: environment.DefaultWriteProfile, Bucket: profile.Bucket, Revision: environment.Revision}
		return nil
	})
	return route, err
}

func requireConfiguredProfiles(configured map[string]application.StorageProfileMode, profileIDs []string, writable bool) error {
	for _, profileID := range profileIDs {
		mode, exists := configured[profileID]
		if !exists || mode == application.StorageProfileModeDisabled || writable && mode != application.StorageProfileModeReadWrite {
			return application.ErrStorageNotReady
		}
	}
	return nil
}
