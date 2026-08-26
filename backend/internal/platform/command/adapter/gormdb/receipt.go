package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

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
	record, err := receiptRecord(receipt)
	if err != nil {
		return err
	}
	return database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func Ensure(ctx context.Context, database *gorm.DB, receipt platformcommand.Receipt) (platformcommand.Receipt, error) {
	record, err := receiptRecord(receipt)
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	if err = database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "operation"}, {Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&record).Error; err != nil {
		return platformcommand.Receipt{}, err
	}
	persisted, err := Find(ctx, database, receipt.WorkspaceID, receipt.Operation, receipt.IdempotencyKey)
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	if persisted.InputHash != receipt.InputHash {
		return platformcommand.Receipt{}, platformcommand.ErrInputMismatch
	}
	if persisted.ResourceID != receipt.ResourceID || persisted.CreatedBy != receipt.CreatedBy || !equalJSON(persisted.Result, receipt.Result) {
		return platformcommand.Receipt{}, fmt.Errorf("command receipt result has drifted")
	}
	return persisted, nil
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func receiptRecord(receipt platformcommand.Receipt) (model.CommandReceipt, error) {
	id, err := uuid.Parse(receipt.ID)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	workspaceID, err := uuid.Parse(receipt.WorkspaceID)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	resourceID, err := uuid.Parse(receipt.ResourceID)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	createdBy, err := uuid.Parse(receipt.CreatedBy)
	if err != nil {
		return model.CommandReceipt{}, err
	}
	record := model.CommandReceipt{
		ID: id, WorkspaceID: workspaceID, Operation: receipt.Operation, IdempotencyKey: receipt.IdempotencyKey,
		InputHash: receipt.InputHash, ResourceID: resourceID, Result: datatypes.JSON(receipt.Result),
		CreatedBy: createdBy, CreatedAt: receipt.CreatedAt,
	}
	return record, nil
}
