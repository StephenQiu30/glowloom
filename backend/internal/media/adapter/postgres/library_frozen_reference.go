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
		var item libraryItemRow
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").Where(map[string]any{"library_id": library.ID, "id": id}).Find(&item).Error; err != nil {
			return err
		}
		var row assetRow
		read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": id, "project_id": project, "personal_actor_id": nil, "personal_org_id": nil, "kind": string(kind)}).Take(&row)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return read.Error
		}
		if err := row.loadLocation(tx); err != nil {
			return err
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
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND NOT is_delete", id).Order("kind").Order("id").Limit(17).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 16 {
			return application.ErrUnavailable
		}
		prefix := strings.TrimSuffix(result.Asset.ObjectKey, path.Ext(result.Asset.ObjectKey)) + "/"
		seen := make(map[domain.RenditionKind]bool, len(rows))
		for _, row := range rows {
			if err := row.loadLocation(tx); err != nil {
				return err
			}
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
