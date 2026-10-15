package postgres

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// StorageUsageLocationFactory retains the caller's transaction for other owners.
type StorageUsageLocationFactory func(*gorm.DB) application.StorageUsageLocationSource

// RoutedStorageUsageStore combines persisted locations with explicitly admitted
// historical buckets. Missing legacy proofs or installed intent owners fail closed.
type RoutedStorageUsageStore struct {
	library *LibraryStore
	buckets []application.StorageUsageBucket
	intents StorageUsageLocationFactory
}

// NewRoutedStorageUsageStore injects current authority and all readable profiles.
func NewRoutedStorageUsageStore(library *LibraryStore, buckets []application.StorageUsageBucket, intents StorageUsageLocationFactory) *RoutedStorageUsageStore {
	return &RoutedStorageUsageStore{library: library, buckets: append([]application.StorageUsageBucket(nil), buckets...), intents: intents}
}

// InspectStorageUsage holds the owning authority locks through the entire inventory.
func (s *RoutedStorageUsageStore) InspectStorageUsage(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, consume func(application.StorageUsageInventory) error) error {
	if s == nil || s.library == nil || s.intents == nil || consume == nil || len(s.buckets) < 1 || len(s.buckets) > 16 {
		return application.ErrUnavailable
	}
	return s.library.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		refs, err := storageUsageMediaLocations(tx, actor, scope)
		if err != nil {
			return err
		}
		intents := s.intents(tx)
		if intents == nil {
			return application.ErrUnavailable
		}
		extra, err := intents.StorageUsageLocations(ctx, actor, scope)
		if err != nil {
			return err
		}
		if len(extra) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}
		unique := make(map[domain.ObjectRef]bool, len(refs)+len(extra))
		for _, ref := range append(refs, extra...) {
			if ref.Validate() != nil {
				return application.ErrUnavailable
			}
			unique[ref] = true
			if len(unique) > application.MaxStorageUsageObjects {
				return application.ErrUnavailable
			}
		}
		refs = make([]domain.ObjectRef, 0, len(unique))
		for ref := range unique {
			refs = append(refs, ref)
		}
		sort.Slice(refs, func(i, j int) bool {
			a, b := refs[i], refs[j]
			if a.ProfileID != b.ProfileID {
				return a.ProfileID < b.ProfileID
			}
			if a.Bucket != b.Bucket {
				return a.Bucket < b.Bucket
			}
			return a.Key < b.Key
		})
		base := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/"
		if scope.Kind == domain.LibraryProject {
			base = "projects/" + scope.ProjectID.String() + "/"
		}
		prefixes := make([]application.StorageUsagePrefix, 0, 5*len(s.buckets))
		seen := make(map[application.StorageUsageBucket]bool)
		for _, bucket := range s.buckets {
			if seen[bucket] || (domain.ObjectRef{ProfileID: bucket.ProfileID, Bucket: bucket.Bucket, Key: "validation"}).Validate() != nil {
				return application.ErrUnavailable
			}
			seen[bucket] = true
			var count int64
			if err := tx.Raw(`SELECT count(*) FROM media.storage_profile WHERE id=? AND bucket=?`, bucket.ProfileID, bucket.Bucket).Scan(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return application.ErrStorageNotReady
			}
			for _, kind := range []string{"image", "video", "audio", "model", "document"} {
				prefixes = append(prefixes, application.StorageUsagePrefix{ProfileID: bucket.ProfileID, Bucket: bucket.Bucket, Prefix: base + kind + "/"})
			}
		}
		for _, ref := range refs {
			if !seen[application.StorageUsageBucket{ProfileID: ref.ProfileID, Bucket: ref.Bucket}] {
				return application.ErrStorageNotReady
			}
		}
		return consume(application.StorageUsageInventory{LibraryID: library.ID, Locations: refs, PhysicalPrefixes: prefixes})
	})
}

func storageUsageMediaLocations(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope) ([]domain.ObjectRef, error) {
	condition := "a.project_id=? AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL"
	var args []any
	if scope.Kind == domain.LibraryProject {
		args = []any{*scope.ProjectID, *scope.ProjectID, *scope.ProjectID, actor.OrgID, *scope.ProjectID, actor.OrgID, *scope.ProjectID, actor.OrgID, *scope.ProjectID, actor.OrgID}
	} else {
		condition = "a.project_id IS NULL AND a.personal_org_id=? AND a.personal_actor_id=?"
		args = []any{actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID, actor.OrgID, actor.ID}
	}
	query := `SELECT DISTINCT held.location_id,held.object_key,l.profile_id,l.bucket FROM (
 SELECT a.object_location_id AS location_id,a.object_key FROM media.media_asset a WHERE ` + condition + `
 UNION ALL SELECT r.object_location_id,r.object_key FROM media.rendition r JOIN media.media_asset a ON a.id=r.media_asset_id WHERE ` + condition
	if scope.Kind == domain.LibraryProject {
		query += ` UNION ALL SELECT o.source_location_id,o.source_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.source_project_id=? AND j.org_id=?
  UNION ALL SELECT o.target_location_id,o.target_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.target_project_id=? AND j.org_id=?
  UNION ALL SELECT o.source_location_id,o.source_object_key FROM media.project_copy_object o JOIN media.project_copy_snapshot s ON s.id=o.snapshot_id WHERE s.source_project_id=? AND s.org_id=?
  UNION ALL SELECT o.target_location_id,o.target_object_key FROM media.project_copy_object o JOIN media.project_copy_snapshot s ON s.id=o.snapshot_id WHERE s.target_project_id=? AND s.org_id=?`
	} else {
		query += ` UNION ALL SELECT o.source_location_id,o.source_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.source_kind='personal' AND j.source_org_id=? AND j.actor_id=?
  UNION ALL SELECT o.target_location_id,o.target_object_key FROM media.transfer_object o JOIN media.transfer_job j ON j.id=o.job_id WHERE j.target_kind='personal' AND j.target_org_id=? AND j.actor_id=?`
	}
	query += `) held LEFT JOIN media.object_location l ON l.id=held.location_id AND l.object_key=held.object_key ORDER BY held.location_id,held.object_key LIMIT 50001`
	var rows []struct{ ProfileID, Bucket, ObjectKey string }
	if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read media physical usage: %w", err)
	}
	if len(rows) > application.MaxStorageUsageObjects {
		return nil, application.ErrUnavailable
	}
	refs := make([]domain.ObjectRef, 0, len(rows))
	for _, row := range rows {
		ref := domain.ObjectRef{ProfileID: row.ProfileID, Bucket: row.Bucket, Key: row.ObjectKey}
		if ref.Validate() != nil {
			return nil, application.ErrStorageNotReady
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
