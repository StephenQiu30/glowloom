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

// ReadReferenceFactSource is stricter than preview: only active project catalog
// originals may form new bindings. Existing historical reference reads retain
// their separate owning eligibility and do not use this new-binding reader.
func (s *LibraryStore) ReadReferenceFactSource(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID, kind domain.Kind) (application.LibraryMediaFile, error) {
	if project == uuid.Nil || id == uuid.Nil || kind != domain.KindImage && kind != domain.KindAudio {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	var result application.LibraryMediaFile
	err := s.read(ctx, actor, domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}, func(tx *gorm.DB, library libraryRow) error {
		var state string
		read := tx.Raw(`SELECT catalog_state FROM media.library_item WHERE library_id=? AND id=? FOR SHARE`, library.ID, id).Scan(&state)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 && state != "active" {
			return application.ErrNotFound
		}
		if err := requireNoNewMediaPurge(tx, []uuid.UUID{id}); err != nil {
			return err
		}
		var row assetRow
		read = tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND project_id=? AND personal_actor_id IS NULL AND personal_org_id IS NULL AND kind=? AND NOT is_delete AND status='ready' AND moderation_status='passed' AND NOT contains_real_person AND consent_record_id IS NULL FOR SHARE`, id, project, string(kind)).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		result.Asset = row.domain()
		if !result.Asset.CanReference() {
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
		seen := make(map[domain.RenditionKind]bool, len(rows))
		prefix := strings.TrimSuffix(result.Asset.ObjectKey, path.Ext(result.Asset.ObjectKey)) + "/"
		for _, row := range rows {
			r := row.domain()
			ext := path.Ext(r.ObjectKey)
			if r.Validate() != nil || seen[r.Kind] || r.ObjectKey != prefix+string(r.Kind)+ext {
				return application.ErrUnavailable
			}
			seen[r.Kind] = true
			result.Renditions = append(result.Renditions, r)
		}
		return nil
	})
	return result, err
}
