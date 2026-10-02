package postgres

import (
	"context"
	"encoding/hex"
	"strings"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type transferObjectRow struct {
	SourceObjectKey, TargetObjectKey, RenditionKind, ContentType, Status string
	ByteSize                                                             *int64
	SHA256                                                               *string
	SourceVerified, WriteStarted                                         bool
}

func validTransferDigest(sha string, size int64) bool {
	decoded, err := hex.DecodeString(sha)
	return err == nil && len(decoded) == 32 && sha == strings.ToLower(sha) && size >= 1 && size <= 2<<30
}

func readTransferObject(tx *gorm.DB, row transferJobRow, index int, kind string) (transferObjectRow, error) {
	var result transferObjectRow
	read := tx.Raw(`SELECT * FROM media.transfer_object WHERE job_id=? AND item_index=? AND rendition_kind=? FOR UPDATE`, row.ID, index, kind).Scan(&result)
	if read.Error != nil {
		return result, read.Error
	}
	if read.RowsAffected != 1 {
		return result, application.ErrNotFound
	}
	return result, nil
}

// TransferObjects returns only immutable keys owned by the exact fenced item.
func (s *TransferStore) TransferObjects(ctx context.Context, lease application.TransferLease, index int) ([]application.ProjectCopyObject, error) {
	result := []application.ProjectCopyObject{}
	err := s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		_, item, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		var rows []transferObjectRow
		if err := tx.Raw(`SELECT * FROM media.transfer_object WHERE job_id=? AND item_index=? ORDER BY rendition_kind FOR UPDATE`, row.ID, index).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(item.Objects) {
			return application.ErrUnavailable
		}
		for _, r := range rows {
			var found bool
			for _, frozen := range item.Objects {
				if frozen.SourceObjectKey == r.SourceObjectKey && frozen.TargetObjectKey == r.TargetObjectKey && frozen.RenditionKind == r.RenditionKind && frozen.ContentType == r.ContentType {
					frozen.ByteSize, frozen.SHA256, frozen.Status = r.ByteSize, r.SHA256, r.Status
					frozen.SourceVerified, frozen.WriteStarted = r.SourceVerified, r.WriteStarted
					result = append(result, frozen)
					found = true
				}
			}
			if !found {
				return application.ErrUnavailable
			}
		}
		return nil
	})
	return result, err
}

// RecordTransferDigest precedes the first physical target write.
func (s *TransferStore) RecordTransferDigest(ctx context.Context, lease application.TransferLease, index int, kind, sha string, size int64) error {
	if !validTransferDigest(sha, size) {
		return application.ErrObjectMismatch
	}
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.Status != "pending" || object.WriteStarted || object.SHA256 != nil && *object.SHA256 != sha || object.ByteSize != nil && *object.ByteSize != size {
			return application.ErrObjectMismatch
		}
		return libraryChanged(tx, `UPDATE media.transfer_object SET sha256=?,byte_size=?,source_verified=true WHERE job_id=? AND item_index=? AND rendition_kind=?`, sha, size, row.ID, index, kind)
	})
}

// BeginTransferWrite durably records uncertainty before conditional object I/O.
func (s *TransferStore) BeginTransferWrite(ctx context.Context, lease application.TransferLease, index int, kind string) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.Status != "pending" || object.WriteStarted || !object.SourceVerified || object.SHA256 == nil || object.ByteSize == nil || !validTransferDigest(*object.SHA256, *object.ByteSize) {
			return application.ErrObjectMismatch
		}
		return libraryChanged(tx, `UPDATE media.transfer_object SET write_started=true WHERE job_id=? AND item_index=? AND rendition_kind=?`, row.ID, index, kind)
	})
}

// ConfirmTransferObject records the exact digest after actual private readback.
func (s *TransferStore) ConfirmTransferObject(ctx context.Context, lease application.TransferLease, index int, kind, sha string, size int64) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.Status != "pending" && object.Status != "verified" || !object.SourceVerified || !object.WriteStarted || object.SHA256 == nil || *object.SHA256 != sha || object.ByteSize == nil || *object.ByteSize != size || !validTransferDigest(sha, size) {
			return application.ErrObjectMismatch
		}
		return libraryChanged(tx, `UPDATE media.transfer_object SET status='verified' WHERE job_id=? AND item_index=? AND rendition_kind=?`, row.ID, index, kind)
	})
}

// AuthorizeTransferRemoval forbids deleting any already published item's object.
func (s *TransferStore) AuthorizeTransferRemoval(ctx context.Context, lease application.TransferLease, index int, kind string) error {
	return s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		state, _, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "succeeded" {
			return domain.ErrTransferConflict
		}
		_, err = readTransferObject(tx, row, index, kind)
		return err
	})
}

// ConfirmTransferRemoval follows exact-key absence proof; it never edits source.
func (s *TransferStore) ConfirmTransferRemoval(ctx context.Context, lease application.TransferLease, index int, kind string) error {
	return s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		state, _, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "succeeded" {
			return domain.ErrTransferConflict
		}
		return libraryChanged(tx, `UPDATE media.transfer_object SET status='removed' WHERE job_id=? AND item_index=? AND rendition_kind=?`, row.ID, index, kind)
	})
}
