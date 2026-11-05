package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type storageUsagePackageObject struct {
	ObjectKey        string
	ObjectLocationID *uuid.UUID
	ProfileID        string
	Bucket           string
	State            *string
}

type purgedStorageLocation struct {
	ObjectLocationID uuid.UUID
	ObjectKey        string
}

// StorageUsageLocations reads the media-owned location ledger under current
// library authority. Namespace-owned work covers script/transcription outputs;
// media-owned package archives additionally require their exact library binding.
func (s *LibraryStore) StorageUsageLocations(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) ([]domain.ObjectRef, error) {
	var result []domain.ObjectRef
	err := s.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		base := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/"
		if scope.Kind == domain.LibraryProject {
			base = "projects/" + scope.ProjectID.String() + "/"
		}

		var locations []objectLocationRow
		if err := tx.Where("object_key LIKE ? AND state <> ?", base+"%", domain.ObjectLocationDeleted).
			Order("profile_id").Order("bucket").Order("object_key").Limit(application.MaxStorageUsageObjects + 1).
			Find(&locations).Error; err != nil {
			return fmt.Errorf("read scoped storage locations: %w", err)
		}
		if len(locations) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}

		var packages []storageUsagePackageObject
		if err := tx.Table("media.package_object AS o").
			Select("o.object_key, o.object_location_id, l.profile_id, l.bucket, l.state").
			Joins("JOIN media.package_job AS j ON j.id = o.job_id").
			Joins("LEFT JOIN media.object_location AS l ON l.id = o.object_location_id AND l.object_key = o.object_key").
			Where("j.org_id = ? AND j.library_id = ? AND o.status <> ?", actor.OrgID, library.ID, application.PackageObjectStatusRemoved).
			Where("l.state IS NULL OR l.state <> ?", domain.ObjectLocationDeleted).
			Order("o.object_key").Limit(application.MaxStorageUsageObjects + 1).Find(&packages).Error; err != nil {
			return fmt.Errorf("read package storage locations: %w", err)
		}
		if len(packages) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}

		locationIDs := make([]uuid.UUID, 0, len(locations)+len(packages))
		for _, location := range locations {
			locationIDs = append(locationIDs, location.ID)
		}
		for _, pkg := range packages {
			if pkg.ObjectLocationID == nil {
				return application.ErrStorageNotReady
			}
			locationIDs = append(locationIDs, *pkg.ObjectLocationID)
		}
		purged, err := readPurgedStorageLocations(tx, locationIDs)
		if err != nil {
			return err
		}

		refs := make(map[domain.ObjectRef]struct{}, len(locations)+len(packages))
		for _, location := range locations {
			if purged[storageLocationIdentity(location.ID, location.ObjectKey)] {
				continue
			}
			ref := domain.ObjectRef{ProfileID: location.ProfileID, Bucket: location.Bucket, Key: location.ObjectKey}
			if ref.Validate() != nil {
				return application.ErrStorageNotReady
			}
			refs[ref] = struct{}{}
		}
		for _, pkg := range packages {
			if purged[storageLocationIdentity(*pkg.ObjectLocationID, pkg.ObjectKey)] {
				continue
			}
			ref := domain.ObjectRef{ProfileID: pkg.ProfileID, Bucket: pkg.Bucket, Key: pkg.ObjectKey}
			if ref.Validate() != nil {
				return application.ErrStorageNotReady
			}
			refs[ref] = struct{}{}
		}
		if len(refs) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}
		result = make([]domain.ObjectRef, 0, len(refs))
		for ref := range refs {
			result = append(result, ref)
		}
		return nil
	})
	return result, err
}

func readPurgedStorageLocations(tx *gorm.DB, ids []uuid.UUID) (map[string]bool, error) {
	purged := make(map[string]bool)
	if len(ids) == 0 {
		return purged, nil
	}
	var rows []purgedStorageLocation
	if err := tx.Table("media.purge_object").Select("object_location_id, object_key").
		Where("object_location_id IN ? AND removed = ?", ids, true).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read purged storage locations: %w", err)
	}
	for _, row := range rows {
		purged[storageLocationIdentity(row.ObjectLocationID, row.ObjectKey)] = true
	}
	return purged, nil
}

func storageLocationIdentity(id uuid.UUID, key string) string {
	return id.String() + "/" + key
}

var _ application.StorageUsageLocationSource = (*LibraryStore)(nil)
