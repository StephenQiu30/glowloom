package postgres

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type transferCommandRecord struct {
	ActorID, OrgID, IdemKey, JobID uuid.UUID
	Action, RequestSHA256          string
	ResponseBody                   []byte
	EventID                        *uuid.UUID
	EventAction                    *string
	EventAttempt                   *int
}

func (transferCommandRecord) TableName() string { return "media.transfer_command" }

type transferItemViewRow struct {
	ItemIndex     int `gorm:"column:item_index"`
	SourceItemID  uuid.UUID
	TargetItemID  uuid.UUID
	TargetAssetID *uuid.UUID
	Status        domain.TransferItemStatus
	FailureCode   *string
}

func (transferItemViewRow) TableName() string { return "media.transfer_item" }
