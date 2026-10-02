package postgres

import (
	"context"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// ReadFrozenReferenceFactSource ignores catalog hiding for an already frozen
// binding, but retains current authorization and exact original/rendition locks.
// A soft-deleted original has a changed revision and needs a separate retained
// history proof; this reader never infers that change to be the old evidence.
func (s *LibraryStore) ReadFrozenReferenceFactSource(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, kind domain.Kind) (application.LibraryMediaFile, error) {
	if project == uuid.Nil || id == uuid.Nil || kind != domain.KindImage && kind != domain.KindAudio {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	var result application.LibraryMediaFile
	err := s.read(ctx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(tx *gorm.DB, library libraryRow) error {
		var item struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT id FROM media.library_item WHERE library_id=? AND id=? FOR SHARE`, library.ID, id).Scan(&item).Error; err != nil {
			return err
		}
		var row assetRow
		read := tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND project_id=? AND personal_actor_id IS NULL AND personal_org_id IS NULL AND kind=? FOR SHARE`, id, project, string(kind)).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		result.Asset = row.domain()
		if !result.Asset.CanReference() || result.Asset.ContainsRealPerson || result.Asset.ConsentRecordID != nil {
			return application.ErrUnavailable
		}
		result.Renditions = []domain.Rendition{}
		if kind == domain.KindAudio {
			return nil
		}
		var rows []renditionRow
		if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete ORDER BY kind,id LIMIT 17 FOR SHARE`, id).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 16 {
			return application.ErrUnavailable
		}
		prefix := strings.TrimSuffix(result.Asset.ObjectKey, path.Ext(result.Asset.ObjectKey)) + "/"
		seen := make(map[domain.RenditionKind]bool, len(rows))
		for _, row := range rows {
			r := row.domain()
			if r.Validate() != nil || seen[r.Kind] || r.ObjectKey != prefix+string(r.Kind)+path.Ext(r.ObjectKey) {
				return application.ErrUnavailable
			}
			seen[r.Kind] = true
			result.Renditions = append(result.Renditions, r)
		}
		return nil
	})
	return result, err
}
