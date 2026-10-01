package postgres

import (
	"encoding/json"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func equalCopyMedia(want, got any) bool {
	a, err := json.Marshal(canonicalCopyMediaTime(want))
	if err != nil {
		return false
	}
	b, err := json.Marshal(canonicalCopyMediaTime(got))
	if err != nil {
		return false
	}
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// PostgreSQL sessions can render the same timestamptz instant with different
// offsets. Keep the timestamps in the comparison while canonicalizing their zone.
func canonicalCopyMediaTime(value any) any {
	switch item := value.(type) {
	case domain.MediaAsset:
		item.CreateTime = item.CreateTime.UTC()
		item.UpdateTime = item.UpdateTime.UTC()
		return item
	case domain.Rendition:
		item.CreateTime = item.CreateTime.UTC()
		item.UpdateTime = item.UpdateTime.UTC()
		return item
	default:
		return value
	}
}

func verifyCopiedMedia(tx *gorm.DB, assets []application.ProjectCopySourceAsset, target uuid.UUID) error {
	var count int
	if err := tx.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=? AND NOT is_delete`, target).Scan(&count).Error; err != nil {
		return err
	}
	if count != len(assets) {
		return application.ErrObjectMismatch
	}
	for _, item := range assets {
		var row assetRow
		read := tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, item.Asset.ID, target).Scan(&row)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || !equalCopyMedia(item.Asset, row.domain()) {
			return application.ErrObjectMismatch
		}
		var rows []renditionRow
		if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete FOR SHARE`, item.Asset.ID).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(item.Renditions) {
			return application.ErrObjectMismatch
		}
		expected := make(map[uuid.UUID]domain.Rendition, len(rows))
		for _, r := range item.Renditions {
			expected[r.ID] = r
		}
		for _, row := range rows {
			if !equalCopyMedia(expected[row.ID], row.domain()) {
				return application.ErrObjectMismatch
			}
		}
	}
	return nil
}
