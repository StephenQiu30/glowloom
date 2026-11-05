package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type transferObjectRow struct {
	JobID                                                        uuid.UUID
	ItemIndex, OutputAttempt                                     int
	SourceLocationID, TargetLocationID                           *uuid.UUID
	SourceObjectKey, TargetObjectKey, RenditionKind, ContentType string
	Status                                                       application.ProjectCopyObjectStatus
	ByteSize                                                     *int64
	SHA256                                                       *string
	SourceVerified, WriteStarted                                 bool
}

func (transferObjectRow) TableName() string { return "media.transfer_object" }

func validTransferDigest(sha string, size int64) bool {
	decoded, err := hex.DecodeString(sha)
	return err == nil && len(decoded) == 32 && sha == strings.ToLower(sha) && size >= 1 && size <= 2<<30
}

func transferObjectMatches(row transferObjectRow, frozen application.ProjectCopyObject) bool {
	return row.RenditionKind == frozen.RenditionKind && row.SourceObjectKey == frozen.SourceObjectKey && row.TargetObjectKey == frozen.TargetObjectKey && row.ContentType == frozen.ContentType &&
		row.SourceLocationID != nil && frozen.SourceLocation != nil && *row.SourceLocationID == frozen.SourceLocation.ID &&
		row.TargetLocationID != nil && frozen.TargetLocation != nil && *row.TargetLocationID == frozen.TargetLocation.ID &&
		(frozen.ByteSize == nil || row.ByteSize != nil && *row.ByteSize == *frozen.ByteSize) &&
		(frozen.SHA256 == nil || row.SHA256 != nil && *row.SHA256 == *frozen.SHA256)
}

func readTransferObject(tx *gorm.DB, row transferJobRow, index int, kind string) (transferObjectRow, error) {
	var result transferObjectRow
	_, initial, err := readTransferItem(tx, row.ID, index)
	if err != nil {
		return result, err
	}
	output, item, err := readTransferAttempt(tx, row, index, initial)
	if err != nil {
		return result, err
	}
	read := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND item_index = ? AND output_attempt = ? AND rendition_kind = ?", row.ID, index, output.OutputAttempt, kind).Take(&result)
	if read.Error != nil {
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return result, application.ErrNotFound
		}
		return result, read.Error
	}
	for _, frozen := range item.Objects {
		if transferObjectMatches(result, frozen) {
			return result, nil
		}
	}
	return result, application.ErrObjectMismatch
}

// TransferObjects returns only immutable keys owned by the exact fenced item.
func (s *TransferStore) TransferObjects(ctx context.Context, lease application.TransferLease, index int) ([]application.ProjectCopyObject, error) {
	result := []application.ProjectCopyObject{}
	err := s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		_, initial, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		output, item, err := readTransferAttempt(tx, row, index, initial)
		if err != nil {
			return err
		}
		var rows []transferObjectRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id = ? AND item_index = ? AND output_attempt = ?", row.ID, index, output.OutputAttempt).Order("rendition_kind").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(item.Objects) {
			return application.ErrUnavailable
		}
		for _, r := range rows {
			var found bool
			for _, frozen := range item.Objects {
				if transferObjectMatches(r, frozen) {
					frozen.ByteSize, frozen.SHA256, frozen.Status = r.ByteSize, r.SHA256, r.Status
					frozen.SourceVerified, frozen.WriteStarted = r.SourceVerified, r.WriteStarted
					source, err := readRowLocation(tx, r.SourceLocationID, r.SourceObjectKey)
					if err != nil {
						return err
					}
					target, err := readRowLocation(tx, r.TargetLocationID, r.TargetObjectKey)
					if err != nil {
						return err
					}
					if frozen.SourceLocation != nil && (source == nil || frozen.SourceLocation.ID != source.ID || frozen.SourceLocation.Ref != source.Ref) || frozen.TargetLocation != nil && (target == nil || frozen.TargetLocation.ID != target.ID || frozen.TargetLocation.Ref != target.Ref) {
						return application.ErrObjectMismatch
					}
					frozen.SourceLocation, frozen.TargetLocation = source, target
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
		if object.Status != application.ProjectCopyObjectPending || object.WriteStarted || object.SHA256 != nil && *object.SHA256 != sha || object.ByteSize != nil && *object.ByteSize != size {
			return application.ErrObjectMismatch
		}
		return updateTransferObject(tx, row.ID, index, object.OutputAttempt, kind, map[string]any{"sha256": sha, "byte_size": size, "source_verified": true})
	})
}

// BeginTransferWrite durably records uncertainty before conditional object I/O.
func (s *TransferStore) BeginTransferWrite(ctx context.Context, lease application.TransferLease, index int, kind string) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.Status != application.ProjectCopyObjectPending || object.WriteStarted || !object.SourceVerified || object.SHA256 == nil || object.ByteSize == nil || !validTransferDigest(*object.SHA256, *object.ByteSize) {
			return application.ErrObjectMismatch
		}
		if object.TargetLocationID != nil {
			if err := beginCopyLocation(ctx, tx, transferWorkOwner(row.ID, row.TargetLibraryID, index, object.OutputAttempt), object.TargetLocationID, object.TargetObjectKey); err != nil {
				return err
			}
		}
		return updateTransferObject(tx, row.ID, index, object.OutputAttempt, kind, map[string]any{"write_started": true})
	})
}

// ConfirmTransferObject records the exact digest after actual private readback.
func (s *TransferStore) ConfirmTransferObject(ctx context.Context, lease application.TransferLease, index int, kind, sha string, size int64) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.Status != application.ProjectCopyObjectPending && object.Status != application.ProjectCopyObjectVerified || !object.SourceVerified || !object.WriteStarted || object.SHA256 == nil || *object.SHA256 != sha || object.ByteSize == nil || *object.ByteSize != size || !validTransferDigest(sha, size) {
			return application.ErrObjectMismatch
		}
		if object.TargetLocationID != nil {
			if err := confirmCopyLocation(ctx, tx, transferWorkOwner(row.ID, row.TargetLibraryID, index, object.OutputAttempt), object.TargetLocationID, object.TargetObjectKey, object.ContentType, sha, size); err != nil {
				return err
			}
		}
		return updateTransferObject(tx, row.ID, index, object.OutputAttempt, kind, map[string]any{"status": application.ProjectCopyObjectVerified})
	})
}

// AuthorizeTransferRemoval forbids deleting any already published item's object.
func (s *TransferStore) AuthorizeTransferRemoval(ctx context.Context, lease application.TransferLease, index int, kind string) error {
	return s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		state, _, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == domain.TransferItemStatusSucceeded {
			return domain.ErrTransferConflict
		}
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.TargetLocationID != nil {
			return beginCopyRemoval(ctx, tx, transferWorkOwner(row.ID, row.TargetLibraryID, index, object.OutputAttempt), object.TargetLocationID, object.TargetObjectKey)
		}
		return nil
	})
}

// ConfirmTransferRemoval follows exact-key absence proof; it never edits source.
func (s *TransferStore) ConfirmTransferRemoval(ctx context.Context, lease application.TransferLease, index int, kind string) error {
	return s.withLease(ctx, lease, false, true, func(tx *gorm.DB, row transferJobRow, _, _ libraryRow) error {
		state, _, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == domain.TransferItemStatusSucceeded {
			return domain.ErrTransferConflict
		}
		object, err := readTransferObject(tx, row, index, kind)
		if err != nil {
			return err
		}
		if object.TargetLocationID != nil {
			if err := confirmCopyRemoval(ctx, tx, transferWorkOwner(row.ID, row.TargetLibraryID, index, object.OutputAttempt), object.TargetLocationID, object.TargetObjectKey); err != nil {
				return err
			}
		}
		return updateTransferObject(tx, row.ID, index, object.OutputAttempt, kind, map[string]any{"status": application.ProjectCopyObjectRemoved})
	})
}

func updateTransferObject(tx *gorm.DB, jobID uuid.UUID, itemIndex, outputAttempt int, renditionKind string, values map[string]any) error {
	result := tx.Model(&transferObjectRow{}).
		Where("job_id = ? AND item_index = ? AND output_attempt = ? AND rendition_kind = ?", jobID, itemIndex, outputAttempt, renditionKind).
		Updates(values)
	if result.Error != nil {
		return fmt.Errorf("persist transfer object state: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return application.ErrLibraryConflict
	}
	return nil
}
