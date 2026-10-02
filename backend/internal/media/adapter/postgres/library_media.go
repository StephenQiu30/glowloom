package postgres

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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
		query, args := libraryEntries(actor, scope, library.ID)
		args = append(args, id)
		var entry libraryEntryRow
		read := tx.Raw(query+`SELECT * FROM entries WHERE id=? AND asset_id IS NOT NULL AND catalog_state IN ('active','trashed')`, args...).Scan(&entry)
		if read.Error != nil {
			return fmt.Errorf("read library media item: %w", read.Error)
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		if _, err := entry.detail(library.ID); err != nil {
			return err
		}
		if err := requireNoNewMediaPurge(tx, []uuid.UUID{id}); err != nil {
			return err
		}
		var a assetRow
		read = tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND NOT is_delete AND status='ready' AND moderation_status='passed' AND NOT contains_real_person AND consent_record_id IS NULL FOR SHARE`, id).Scan(&a)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return application.ErrNotFound
		}
		asset := a.domain()
		if asset.Validate() != nil {
			return application.ErrUnavailable
		}
		result.Asset = asset
		var rows []renditionRow
		if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete ORDER BY kind,id LIMIT 17 FOR SHARE`, id).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 16 {
			return application.ErrUnavailable
		}
		seen := make(map[domain.RenditionKind]bool, len(rows))
		prefix := strings.TrimSuffix(asset.ObjectKey, path.Ext(asset.ObjectKey)) + "/"
		result.Renditions = make([]domain.Rendition, 0, len(rows))
		for _, row := range rows {
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
