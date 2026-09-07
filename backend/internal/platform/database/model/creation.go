package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type CreationRun struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID      uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_creation_command_key,priority:1;index:ix_creation_project_created,priority:1"`
	ActorID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_creation_command_key,priority:2"`
	TokenVersion   int            `gorm:"not null;check:ck_creation_token,token_version >= 1"`
	IdempotencyKey string         `gorm:"type:varchar(200);not null;uniqueIndex:uq_creation_command_key,priority:3"`
	InputHash      string         `gorm:"type:char(64);not null"`
	PayloadHash    string         `gorm:"type:char(64);not null"`
	Command        datatypes.JSON `gorm:"type:jsonb;not null"`
	Endpoint       string         `gorm:"type:text;not null"`
	Status         string         `gorm:"type:varchar(24);not null;check:ck_creation_status,status IN ('queued','delivery_unknown','delivery_blocked','accepted')"`
	LastError      string         `gorm:"type:varchar(80);not null"`
	Revision       int64          `gorm:"not null;check:ck_creation_revision,revision >= 1"`
	Acceptance     datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt      time.Time      `gorm:"type:timestamptz;not null;index:ix_creation_project_created,priority:2,sort:desc"`
	UpdatedAt      time.Time      `gorm:"type:timestamptz;not null"`
	Workspace      Workspace      `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:RESTRICT"`
	Project        Project        `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	Actor          UserAccount    `gorm:"foreignKey:ActorID;constraint:OnDelete:RESTRICT"`
}

func (CreationRun) TableName() string { return "crn_runs" }

type CreationCommandOutbox struct {
	RunID         uuid.UUID   `gorm:"type:uuid;primaryKey"`
	Status        string      `gorm:"type:varchar(20);not null;check:ck_creation_outbox_status,status IN ('pending','leased','delivered','blocked');index:ix_creation_outbox_due,priority:1"`
	Fence         int64       `gorm:"not null;check:ck_creation_outbox_fence,fence >= 0"`
	Attempts      int         `gorm:"not null;check:ck_creation_outbox_attempts,attempts >= 0"`
	NextAttemptAt time.Time   `gorm:"type:timestamptz;not null;index:ix_creation_outbox_due,priority:2"`
	LeaseUntil    *time.Time  `gorm:"type:timestamptz"`
	Run           CreationRun `gorm:"foreignKey:RunID;constraint:OnDelete:RESTRICT"`
}

func (CreationCommandOutbox) TableName() string { return "crn_command_outbox" }
