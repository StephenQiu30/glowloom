package postgres

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func equalCopyMedia(want, got any) bool {
	// An old immutable snapshot predates physical-location metadata. A later
	// owner-verified backfill may add transport facts without rewriting history.
	switch expected := want.(type) {
	case domain.MediaAsset:
		if current, ok := got.(domain.MediaAsset); ok && expected.ObjectLocation == nil && current.ObjectLocation != nil && current.ObjectLocation.State == domain.ObjectLocationAccepted && current.ObjectLocation.Ref.Key == current.ObjectKey {
			if expected.SHA256 == nil && current.ObjectLocation.SHA256 != nil && current.SHA256 != nil && *current.ObjectLocation.SHA256 == *current.SHA256 {
				current.SHA256 = nil
			}
			current.ObjectLocation = nil
			got = current
		}
	case domain.Rendition:
		if current, ok := got.(domain.Rendition); ok && expected.ObjectLocation == nil && current.ObjectLocation != nil && current.ObjectLocation.State == domain.ObjectLocationAccepted && current.ObjectLocation.Ref.Key == current.ObjectKey {
			if expected.ByteSize == nil && current.ObjectLocation.ByteSize != nil && current.ByteSize != nil && *current.ObjectLocation.ByteSize == *current.ByteSize {
				current.ByteSize = nil
			}
			current.ObjectLocation = nil
			got = current
		}
	}
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
		if item.ObjectLocation != nil {
			l := *item.ObjectLocation
			l.State = ""
			l.ByteSize = nil
			l.SHA256 = nil
			l.MIMEType = nil
			item.ObjectLocation = &l
		}
		item.CreateTime = item.CreateTime.UTC()
		item.UpdateTime = item.UpdateTime.UTC()
		if item.DeleteTime != nil {
			stamp := item.DeleteTime.UTC()
			item.DeleteTime = &stamp
		}
		if item.PurgeAfter != nil {
			stamp := item.PurgeAfter.UTC()
			item.PurgeAfter = &stamp
		}
		return item
	case domain.LibraryFolder:
		item.CreatedAt = item.CreatedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		return item
	case domain.LibraryItem:
		item.CreatedAt = item.CreatedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		if item.TrashedAt != nil {
			stamp := item.TrashedAt.UTC()
			item.TrashedAt = &stamp
		}
		return item
	case domain.Rendition:
		if item.ObjectLocation != nil {
			l := *item.ObjectLocation
			l.State = ""
			l.ByteSize = nil
			l.SHA256 = nil
			l.MIMEType = nil
			item.ObjectLocation = &l
		}
		item.CreateTime = item.CreateTime.UTC()
		item.UpdateTime = item.UpdateTime.UTC()
		return item
	default:
		return value
	}
}

func verifyCopiedMedia(tx *gorm.DB, assets []application.ProjectCopySourceAsset, target uuid.UUID) error {
	var count int64
	if err := tx.Model(&assetRow{}).Where("project_id = ? AND is_delete = ?", target, false).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(assets)) {
		return application.ErrObjectMismatch
	}
	for _, item := range assets {
		var row assetRow
		read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND project_id = ? AND is_delete = ?", item.Asset.ID, target, false).Take(&row)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return application.ErrObjectMismatch
		}
		if read.Error != nil {
			return read.Error
		}
		if err := row.loadLocation(tx); err != nil {
			return err
		}
		if !equalCopyMedia(item.Asset, row.domain()) {
			return application.ErrObjectMismatch
		}
		var rows []renditionRow
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND is_delete = ?", item.Asset.ID, false).Find(&rows).Error; err != nil {
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
			if err := row.loadLocation(tx); err != nil {
				return err
			}
			if !equalCopyMedia(expected[row.ID], row.domain()) {
				return application.ErrObjectMismatch
			}
		}
	}
	return nil
}
