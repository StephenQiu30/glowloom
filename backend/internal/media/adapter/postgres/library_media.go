package postgres

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

var _ application.LibraryMediaReader = (*LibraryStore)(nil)

// FindLibraryMedia uses actor -> project -> library -> original/rendition locks.
// Catalog trash remains previewable; removed items and unreviewed files do not.
func (s *LibraryStore) FindLibraryMedia(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) (application.LibraryMediaFile, error) {
	if id == uuid.Nil {
		return application.LibraryMediaFile{}, application.ErrNotFound
	}
	var result application.LibraryMediaFile
	err := s.read(ctx, actor, scope, func(tx *gorm.DB, library libraryRow) error {
		var entry libraryEntryRow
		read := libraryEntries(tx, actor, scope, library.ID).Where("id = ? AND asset_id IS NOT NULL AND catalog_state IN ?", id, []domain.LibraryItemState{domain.LibraryItemStateActive, domain.LibraryItemStateTrashed}).Take(&entry)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return fmt.Errorf("read library media item: %w", read.Error)
		}
		if _, err := entry.detail(library.ID); err != nil {
			return err
		}
		if err := requireNoNewMediaPurge(tx, []uuid.UUID{id}); err != nil {
			return err
		}
		var a assetRow
		read = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": id, "is_delete": false, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "contains_real_person": false, "consent_record_id": nil}).Take(&a)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				return application.ErrNotFound
			}
			return read.Error
		}
		if err := a.loadLocation(tx); err != nil {
			return err
		}
		asset := a.domain()
		if asset.Validate() != nil {
			return application.ErrUnavailable
		}
		result.Asset = asset
		var rows []renditionRow
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND NOT is_delete", id).Order("kind").Order("id").Limit(17).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 16 {
			return application.ErrUnavailable
		}
		seen := make(map[domain.RenditionKind]bool, len(rows))
		prefix := strings.TrimSuffix(asset.ObjectKey, path.Ext(asset.ObjectKey)) + "/"
		result.Renditions = make([]domain.Rendition, 0, len(rows))
		for _, row := range rows {
			if err := row.loadLocation(tx); err != nil {
				return err
			}
			rend := row.domain()
			if rend.Validate() != nil || rend.MediaAssetID != asset.ID || seen[rend.Kind] || !strings.HasPrefix(rend.ObjectKey, prefix) || path.Base(rend.ObjectKey) != string(rend.Kind)+path.Ext(rend.ObjectKey) {
				return application.ErrUnavailable
			}
			seen[rend.Kind] = true
			result.Renditions = append(result.Renditions, rend)
		}
		return nil
	})
	return result, err
}
