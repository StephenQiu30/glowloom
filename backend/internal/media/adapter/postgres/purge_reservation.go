package postgres

import (
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// requireNoPurgeReservation runs under the caller's project/library/media locks.
// Even a retained history proof cannot authorize new work on a purged original.
func requireNoPurgeReservation(tx *gorm.DB, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var present bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.purge_item WHERE item_id IN ? AND status IN ('queued','running','needs_reconciliation','succeeded'))`, ids).Scan(&present).Error; err != nil {
		return err
	}
	if present {
		return domain.ErrPurgeConflict
	}
	return nil
}

func validatePurgeFile(file application.LibraryMediaFile) error {
	a := file.Asset
	if a.Validate() != nil || a.IsDelete || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed || a.SHA256 == nil || !validTransferDigest(*a.SHA256, a.ByteSize) || len(file.Renditions) > 16 {
		return application.ErrUnavailable
	}
	prefix := strings.TrimSuffix(a.ObjectKey, path.Ext(a.ObjectKey)) + "/"
	seen := make(map[domain.RenditionKind]bool, len(file.Renditions))
	for _, r := range file.Renditions {
		if r.Validate() != nil || r.MediaAssetID != a.ID || r.IsDelete || seen[r.Kind] || r.ObjectKey != prefix+string(r.Kind)+path.Ext(r.ObjectKey) || r.ByteSize != nil && (*r.ByteSize < 1 || *r.ByteSize > 2<<30) {
			return application.ErrObjectMismatch
		}
		seen[r.Kind] = true
	}
	return nil
}
