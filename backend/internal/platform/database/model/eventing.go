package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type OutboxEvent struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	EventType         string         `gorm:"type:varchar(100);not null;index:ix_evt_outbox_pending,priority:2"`
	EventVersion      int            `gorm:"not null;check:ck_evt_outbox_version,event_version >= 1"`
	WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID      `gorm:"type:uuid;not null;index:ix_evt_outbox_project_occurred,priority:1"`
	AggregateKind     string         `gorm:"type:varchar(80);not null"`
	AggregateID       string         `gorm:"type:varchar(120);not null"`
	AggregateRevision int64          `gorm:"not null;check:ck_evt_outbox_aggregate_revision,aggregate_revision >= 1"`
	SourceReceiptID   uuid.UUID      `gorm:"type:uuid;not null;index"`
	Payload           datatypes.JSON `gorm:"type:jsonb;not null;check:ck_evt_outbox_payload,jsonb_typeof(payload) = 'object'"`
	PayloadHash       string         `gorm:"type:char(64);not null;check:ck_evt_outbox_payload_hash,char_length(payload_hash) = 64"`
	Status            string         `gorm:"type:varchar(20);not null;index:ix_evt_outbox_pending,priority:1;check:ck_evt_outbox_status,status IN ('pending','published')"`
	Attempts          int            `gorm:"not null;default:0;check:ck_evt_outbox_attempts,attempts >= 0"`
	OccurredAt        time.Time      `gorm:"type:timestamptz;not null;index:ix_evt_outbox_project_occurred,priority:2"`
	PublishedAt       *time.Time     `gorm:"type:timestamptz"`
	CreatedAt         time.Time      `gorm:"type:timestamptz;not null;index:ix_evt_outbox_pending,priority:3"`
	Workspace         Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (OutboxEvent) TableName() string { return "evt_outbox_events" }
