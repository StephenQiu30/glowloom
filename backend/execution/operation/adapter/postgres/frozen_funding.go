package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// checkAcceptedBatchFunding takes every selected handoff gate in stable UUID order
// before lockBatchQuotes locks the batch and its operations in either phase.
func (s *Store) checkAcceptedBatchFunding(ctx context.Context, tx *gorm.DB, batchID uuid.UUID, excluded map[uuid.UUID]bool) error {
	var owner string
	if err := tx.WithContext(ctx).Raw(`SELECT infra.runtime_owner()`).Scan(&owner).Error; err != nil {
		return err
	}
	if owner == "legacy" {
		return nil
	}
	if owner != "execution" {
		return writefence.ErrInvalid
	}
	var ids []uuid.UUID
	if err := tx.WithContext(ctx).Raw(`SELECT id FROM operation.operation WHERE batch_id=? AND status='quoted' AND NOT is_delete ORDER BY id`, batchID).Scan(&ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if !excluded[id] {
			if err := s.checkAcceptedOperationFunding(ctx, tx, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// acceptedPayerScope reads only the original accepted Creation scope after the
// real funding gate is held. This supplies Billing metadata, not a spending grant.
func (s *Store) acceptedPayerScope(ctx context.Context, tx *gorm.DB, operationID, projectID uuid.UUID) (uuid.UUID, error) {
	var owner string
	if err := tx.WithContext(ctx).Raw(`SELECT infra.runtime_owner()`).Scan(&owner).Error; err != nil {
		return uuid.Nil, err
	}
	if owner == "legacy" {
		return uuid.Nil, nil
	}
	if owner != "execution" {
		return uuid.Nil, writefence.ErrInvalid
	}
	if err := s.checkAcceptedOperationFunding(ctx, tx, operationID); err != nil {
		return uuid.Nil, err
	}
	var row struct{ ScopeID uuid.UUID }
	read := tx.WithContext(ctx).Raw(`SELECT (snapshot->>'scope_id')::uuid AS scope_id FROM operation.frozen_input_acceptance WHERE operation_id=? AND snapshot->>'project_id'=?`, operationID, projectID.String()).Scan(&row)
	if read.Error != nil {
		return uuid.Nil, read.Error
	}
	if read.RowsAffected != 1 || row.ScopeID == uuid.Nil {
		return uuid.Nil, application.ErrFrozenInputConflict
	}
	return row.ScopeID, nil
}

// checkAcceptedOperationFunding acquires the permanent handoff gate before any
// operation or reservation lock. Current quote/source/auth assertions still run
// in the confirmation transaction after this original-input check.
func (s *Store) checkAcceptedOperationFunding(ctx context.Context, tx *gorm.DB, operationID uuid.UUID) error {
	var owner string
	if err := tx.WithContext(ctx).Raw(`SELECT infra.runtime_owner()`).Scan(&owner).Error; err != nil {
		return err
	}
	if owner == "legacy" {
		return nil
	}
	if owner != "execution" {
		return writefence.ErrInvalid
	}
	var row struct {
		HandoffID, CommandID          uuid.UUID
		CommandSHA256, SnapshotSHA256 string
	}
	r := tx.WithContext(ctx).Raw(`SELECT handoff_id,command_id,command_sha256,snapshot_sha256 FROM operation.frozen_input_acceptance WHERE operation_id=?`, operationID).Scan(&row)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return application.ErrFrozenInputInvalid
	}
	key := application.FrozenInputKey{HandoffID: row.HandoffID, CommandID: row.CommandID, CommandSHA256: row.CommandSHA256, SnapshotSHA256: row.SnapshotSHA256}
	return NewFrozenInputStore(s.db, nil).CheckFrozenInputFunding(ctx, tx, key, row.CommandSHA256)
}
