package gormdb

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func Find(ctx context.Context, database *gorm.DB, workspaceID, operation, idempotencyKey string) (platformcommand.Receipt, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = database.WithContext(ctx).
		Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", workspace, operation, idempotencyKey).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return platformcommand.Receipt{
		ID: record.ID.String(), WorkspaceID: record.WorkspaceID.String(), Operation: record.Operation,
		IdempotencyKey: record.IdempotencyKey, InputHash: record.InputHash, ResourceID: record.ResourceID.String(),
		Result: append([]byte(nil), record.Result...), CreatedBy: record.CreatedBy.String(), CreatedAt: record.CreatedAt,
	}, nil
}

func Create(ctx context.Context, database *gorm.DB, receipt platformcommand.Receipt) error {
	id, err := uuid.Parse(receipt.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(receipt.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(receipt.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(receipt.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{
		ID: id, WorkspaceID: workspaceID, Operation: receipt.Operation, IdempotencyKey: receipt.IdempotencyKey,
		InputHash: receipt.InputHash, ResourceID: resourceID, Result: datatypes.JSON(receipt.Result),
		CreatedBy: createdBy, CreatedAt: receipt.CreatedAt,
	}
	return database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}
