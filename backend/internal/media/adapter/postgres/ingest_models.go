package postgres

import "github.com/google/uuid"

type operationOutputRecord struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	OperationID      uuid.UUID
	SeqNo            int
	Kind             string
	MediaAssetID     *uuid.UUID
	ModerationStatus string
	ModerationReason *string
	IsDelete         bool
}

func (operationOutputRecord) TableName() string { return "operation.operation_output" }
