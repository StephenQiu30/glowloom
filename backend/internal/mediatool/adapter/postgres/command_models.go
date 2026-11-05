package postgres

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type principalLockRecord struct {
	ID uuid.UUID
}

func lockMediatoolPrincipal(tx *gorm.DB, actorID uuid.UUID) error {
	var row principalLockRecord
	return tx.Table(`identity."user"`).Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", actorID).Take(&row).Error
}

type projectLockRecord struct {
	ID uuid.UUID
}

func lockMediatoolProject(tx *gorm.DB, projectID uuid.UUID) error {
	var row projectLockRecord
	return tx.Table("workspace.project").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", projectID).Take(&row).Error
}

type exportCommandRecord struct {
	ActorID      uuid.UUID `gorm:"primaryKey"`
	RequestID    uuid.UUID `gorm:"primaryKey"`
	JobID        uuid.UUID
	RequestHash  string
	Response     jsonDocument
	EventID      *uuid.UUID
	EventAction  *string
	EventAttempt *int
	CreatedAt    time.Time
}

func (exportCommandRecord) TableName() string { return "mediatool.export_command" }

type depthCommandRecord struct {
	ActorID      uuid.UUID `gorm:"primaryKey"`
	RequestID    uuid.UUID `gorm:"primaryKey"`
	JobID        uuid.UUID
	RequestHash  string
	Response     jsonDocument
	EventID      *uuid.UUID
	EventAction  *string
	EventAttempt *int
	CreatedAt    time.Time
}

func (depthCommandRecord) TableName() string { return "mediatool.depth_command" }

type transcriptionCommandRecord struct {
	ActorID      uuid.UUID `gorm:"primaryKey"`
	RequestID    uuid.UUID `gorm:"primaryKey"`
	JobID        uuid.UUID
	RequestHash  string
	Response     jsonDocument
	EventID      *uuid.UUID
	EventAction  *string
	EventAttempt *int
	CreatedAt    time.Time
}

func (transcriptionCommandRecord) TableName() string { return "mediatool.transcription_command" }

type mediatoolOutboxRecord struct {
	ID           uuid.UUID `gorm:"primaryKey"`
	Topic        string
	PartitionKey string
	Payload      jsonDocument
}

func (mediatoolOutboxRecord) TableName() string { return "infra.outbox" }
