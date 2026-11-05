package postgres

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
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
		var item libraryItemRow
		read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("catalog_state").Where(map[string]any{"library_id": library.ID, "id": id}).Take(&item)
		if read.Error != nil && !errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return read.Error
		}
		if read.Error == nil && item.CatalogState != domain.LibraryItemStateActive {
			return application.ErrNotFound
		}
		if err := requireNoNewMediaPurge(tx, []uuid.UUID{id}); err != nil {
			return err
		}
		var row assetRow
		read = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": id, "project_id": project, "personal_actor_id": nil, "personal_org_id": nil, "kind": string(kind), "is_delete": false, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "contains_real_person": false, "consent_record_id": nil}).Take(&row)
		if read.Error != nil {
			return read.Error
		}
		if err := row.loadLocation(tx); err != nil {
			return err
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
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND NOT is_delete", id).Order("kind").Order("id").Limit(17).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 16 {
			return application.ErrUnavailable
		}
		seen := make(map[domain.RenditionKind]bool, len(rows))
		prefix := strings.TrimSuffix(result.Asset.ObjectKey, path.Ext(result.Asset.ObjectKey)) + "/"
		for _, row := range rows {
			if err := row.loadLocation(tx); err != nil {
				return err
			}
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
