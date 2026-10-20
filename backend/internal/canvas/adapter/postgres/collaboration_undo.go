package postgres

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func collaborationUndoHistory(tx *gorm.DB, head domain.Document, actor uuid.UUID, target string) (domain.Document, []domain.ContributionBatch, error) {
	var targetRevision int64
	r := tx.Raw(`SELECT revision FROM canvas.command_log WHERE document_id=? AND create_by=? AND op_id=? AND operation_version=1 AND NOT is_delete`, head.ID, actor, target).Scan(&targetRevision)
	if r.Error != nil {
		return domain.Document{}, nil, r.Error
	}
	if r.RowsAffected != 1 {
		return domain.Document{}, nil, &domain.OperationError{Code: "undo_unavailable"}
	}
	floorRevision := targetRevision - 1
	var rows []collaborationRow
	for {
		var err error
		rows, err = collaborationHistory(tx, head.ID, floorRevision, head.Revision)
		if err != nil {
			return domain.Document{}, nil, err
		}
		next := floorRevision
		for _, row := range rows {
			if row.Log.OriginalBase < 1 || row.Log.OriginalOperations == nil {
				return domain.Document{}, nil, &domain.OperationError{Code: "undo_unavailable"}
			}
			next = min(next, row.Log.OriginalBase)
		}
		if next == floorRevision {
			break
		}
		floorRevision = next
	}
	floor := head
	for i := len(rows) - 1; i >= 0; i-- {
		var err error
		floor, err = domain.RewindEffects(floor, rows[i].Log.Effects)
		if err != nil {
			return domain.Document{}, nil, err
		}
		floor.Revision--
	}
	history := make([]domain.ContributionBatch, len(rows))
	for i, row := range rows {
		history[i] = domain.ContributionBatch{Revision: row.Revision, ActorID: row.CreateBy, OpID: row.OpID, UndoOf: row.Log.Original.UndoOf, OriginalBase: row.Log.OriginalBase, Original: row.Log.OriginalOperations, Operations: row.Log.Operations, Effects: row.Log.Effects, Touches: row.Log.Touches}
	}
	return floor, history, nil
}
