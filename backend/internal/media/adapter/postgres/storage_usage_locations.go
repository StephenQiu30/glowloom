package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

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
		var rows []struct{ ProfileID, Bucket, ObjectKey string }
		query := `SELECT profile_id,bucket,object_key FROM (
 SELECT l.profile_id,l.bucket,l.object_key FROM media.object_location l WHERE l.object_key LIKE ? AND l.state<>'deleted' AND NOT EXISTS(SELECT 1 FROM media.purge_object p WHERE p.object_location_id=l.id AND p.object_key=l.object_key AND p.removed)
 UNION
 SELECT l.profile_id,l.bucket,o.object_key FROM media.package_object o JOIN media.package_job j ON j.id=o.job_id LEFT JOIN media.object_location l ON l.id=o.object_location_id AND l.object_key=o.object_key WHERE j.org_id=? AND j.library_id=? AND o.status<>'removed' AND l.state IS DISTINCT FROM 'deleted' AND NOT EXISTS(SELECT 1 FROM media.purge_object p WHERE p.object_location_id=o.object_location_id AND p.object_key=o.object_key AND p.removed)
 ) held ORDER BY profile_id,bucket,object_key LIMIT 50001`
		if err := tx.Raw(query, base+"%", actor.OrgID, library.ID).Scan(&rows).Error; err != nil {
			return fmt.Errorf("read scoped storage location inventory: %w", err)
		}
		if len(rows) > application.MaxStorageUsageObjects {
			return application.ErrUnavailable
		}
		result = make([]domain.ObjectRef, 0, len(rows))
		for _, row := range rows {
			ref := domain.ObjectRef{ProfileID: row.ProfileID, Bucket: row.Bucket, Key: row.ObjectKey}
			if ref.Validate() != nil {
				return application.ErrStorageNotReady
			}
			result = append(result, ref)
		}
		return nil
	})
	return result, err
}

var _ application.StorageUsageLocationSource = (*LibraryStore)(nil)
