package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
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
		prefixes := make([]application.StorageUsagePrefix, 0, len(s.buckets))
		seen := make(map[string]string)
		for _, bucket := range s.buckets {
			if seen[bucket.ProfileID] != "" || (domain.ObjectRef{ProfileID: bucket.ProfileID, Bucket: bucket.Bucket, Key: "validation"}).Validate() != nil {
				return application.ErrUnavailable
			}
			seen[bucket.ProfileID] = bucket.Bucket
			var count int64
			query := tx.Model(&storageProfileRecord{}).Where("id = ? AND bucket = ?", bucket.ProfileID, bucket.Bucket)
			if bucket.ResourceIdentity != "" {
				if !storageDigest(bucket.ResourceIdentity) {
					return application.ErrStorageIdentityConflict
				}
				query = query.Where("resource_identity = ?", bucket.ResourceIdentity)
			}
			if err := query.Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return application.ErrStorageNotReady
			}
			prefixes = append(prefixes, application.StorageUsagePrefix{ProfileID: bucket.ProfileID, Bucket: bucket.Bucket, Prefix: base})
		}
		for _, ref := range refs {
			if seen[ref.ProfileID] != ref.Bucket {
				return application.ErrStorageNotReady
			}
		}
		return consume(application.StorageUsageInventory{LibraryID: library.ID, Locations: refs, PhysicalPrefixes: prefixes})
	})
}

func storageUsageMediaLocations(tx *gorm.DB, actor identityapp.Principal, scope domain.LibraryScope) ([]domain.ObjectRef, error) {
	var assetFilter string
	var assetArgs []any
	if scope.Kind == domain.LibraryProject {
		assetFilter = "a.project_id = ? AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL"
		assetArgs = []any{*scope.ProjectID}
	} else {
		assetFilter = "a.project_id IS NULL AND a.personal_org_id = ? AND a.personal_actor_id = ?"
		assetArgs = []any{actor.OrgID, actor.ID}
	}
	rows := make(map[string]storageUsageLocationCandidate)
	assetRows := tx.Table("media.media_asset AS a").
		Select("a.object_location_id AS location_id, a.object_key").Where(assetFilter, assetArgs...)
	if err := appendStorageUsageCandidates(assetRows, rows); err != nil {
		return nil, fmt.Errorf("read media assets for physical usage: %w", err)
	}
	renditionRows := tx.Table("media.rendition AS r").
		Select("r.object_location_id AS location_id, r.object_key").
		Joins("JOIN media.media_asset AS a ON a.id = r.media_asset_id").Where(assetFilter, assetArgs...)
	if err := appendStorageUsageCandidates(renditionRows, rows); err != nil {
		return nil, fmt.Errorf("read media renditions for physical usage: %w", err)
	}
	if scope.Kind == domain.LibraryProject {
		project := *scope.ProjectID
		queries := []*gorm.DB{
			tx.Table("media.transfer_object AS o").Select("o.source_location_id AS location_id, o.source_object_key AS object_key").Joins("JOIN media.transfer_job AS j ON j.id = o.job_id").Where("j.source_project_id = ? AND j.org_id = ?", project, actor.OrgID),
			tx.Table("media.transfer_object AS o").Select("o.target_location_id AS location_id, o.target_object_key AS object_key").Joins("JOIN media.transfer_job AS j ON j.id = o.job_id").Where("j.target_project_id = ? AND j.org_id = ?", project, actor.OrgID),
			tx.Table("media.project_copy_object AS o").Select("o.source_location_id AS location_id, o.source_object_key AS object_key").Joins("JOIN media.project_copy_snapshot AS s ON s.id = o.snapshot_id").Where("s.source_project_id = ? AND s.org_id = ?", project, actor.OrgID),
			tx.Table("media.project_copy_object AS o").Select("o.target_location_id AS location_id, o.target_object_key AS object_key").Joins("JOIN media.project_copy_snapshot AS s ON s.id = o.snapshot_id").Where("s.target_project_id = ? AND s.org_id = ?", project, actor.OrgID),
		}
		for _, query := range queries {
			if err := appendStorageUsageCandidates(query, rows); err != nil {
				return nil, fmt.Errorf("read project work for physical usage: %w", err)
			}
		}
	} else {
		queries := []*gorm.DB{
			tx.Table("media.transfer_object AS o").Select("o.source_location_id AS location_id, o.source_object_key AS object_key").Joins("JOIN media.transfer_job AS j ON j.id = o.job_id").Where("j.source_kind = ? AND j.source_org_id = ? AND j.actor_id = ?", domain.LibraryPersonal, actor.OrgID, actor.ID),
			tx.Table("media.transfer_object AS o").Select("o.target_location_id AS location_id, o.target_object_key AS object_key").Joins("JOIN media.transfer_job AS j ON j.id = o.job_id").Where("j.target_kind = ? AND j.target_org_id = ? AND j.actor_id = ?", domain.LibraryPersonal, actor.OrgID, actor.ID),
		}
		for _, query := range queries {
			if err := appendStorageUsageCandidates(query, rows); err != nil {
				return nil, fmt.Errorf("read personal work for physical usage: %w", err)
			}
		}
	}
	if len(rows) > application.MaxStorageUsageObjects {
		return nil, application.ErrUnavailable
	}

	locationIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.LocationID == nil {
			return nil, application.ErrStorageNotReady
		}
		locationIDs = append(locationIDs, *row.LocationID)
	}
	var locations []objectLocationRow
	if len(locationIDs) > 0 {
		if err := tx.Where("id IN ?", locationIDs).Find(&locations).Error; err != nil {
			return nil, fmt.Errorf("resolve physical usage locations: %w", err)
		}
	}
	byID := make(map[uuid.UUID]objectLocationRow, len(locations))
	for _, location := range locations {
		byID[location.ID] = location
	}
	purged, err := readPurgedStorageLocations(tx, locationIDs)
	if err != nil {
		return nil, err
	}
	refsByObject := make(map[domain.ObjectRef]struct{}, len(rows))
	for _, row := range rows {
		location, ok := byID[*row.LocationID]
		if !ok || location.ObjectKey != row.ObjectKey {
			return nil, application.ErrStorageNotReady
		}
		if location.State == domain.ObjectLocationDeleted || purged[storageLocationIdentity(location.ID, row.ObjectKey)] {
			continue
		}
		ref := domain.ObjectRef{ProfileID: location.ProfileID, Bucket: location.Bucket, Key: row.ObjectKey}
		if ref.Validate() != nil {
			return nil, application.ErrStorageNotReady
		}
		refsByObject[ref] = struct{}{}
	}
	if len(refsByObject) > application.MaxStorageUsageObjects {
		return nil, application.ErrUnavailable
	}
	refs := make([]domain.ObjectRef, 0, len(refsByObject))
	for ref := range refsByObject {
		refs = append(refs, ref)
	}
	return refs, nil
}

type storageUsageLocationCandidate struct {
	LocationID *uuid.UUID
	ObjectKey  string
}

func appendStorageUsageCandidates(query *gorm.DB, rows map[string]storageUsageLocationCandidate) error {
	var candidates []storageUsageLocationCandidate
	if err := query.Order("object_key").Limit(application.MaxStorageUsageObjects + 1).Find(&candidates).Error; err != nil {
		return err
	}
	if len(candidates) > application.MaxStorageUsageObjects {
		return application.ErrUnavailable
	}
	for _, candidate := range candidates {
		id := "missing"
		if candidate.LocationID != nil {
			id = candidate.LocationID.String()
		}
		rows[id+"/"+candidate.ObjectKey] = candidate
		if len(rows) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}
	}
	return nil
}
