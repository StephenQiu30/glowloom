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
	ClaimToken        string         `gorm:"type:varchar(120);not null;default:''"`
	ClaimExpiresAt    *time.Time     `gorm:"type:timestamptz;index:ix_evt_outbox_pending,priority:4"`
	LastError         string         `gorm:"type:varchar(500);not null;default:''"`
	OccurredAt        time.Time      `gorm:"type:timestamptz;not null;index:ix_evt_outbox_project_occurred,priority:2"`
	PublishedAt       *time.Time     `gorm:"type:timestamptz"`
	CreatedAt         time.Time      `gorm:"type:timestamptz;not null;index:ix_evt_outbox_pending,priority:3"`
	Workspace         Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (OutboxEvent) TableName() string { return "evt_outbox_events" }

type InboxEvent struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ConsumerGroup     string     `gorm:"type:varchar(150);not null;uniqueIndex:ux_evt_inbox_group_event,priority:1"`
	EventID           string     `gorm:"type:varchar(120);not null;uniqueIndex:ux_evt_inbox_group_event,priority:2"`
	EventType         string     `gorm:"type:varchar(100);not null"`
	WorkspaceID       uuid.UUID  `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID  `gorm:"type:uuid;not null;index:ix_evt_inbox_project_received,priority:1"`
	AggregateKind     string     `gorm:"type:varchar(80);not null"`
	AggregateID       string     `gorm:"type:varchar(120);not null"`
	AggregateRevision int64      `gorm:"not null;check:ck_evt_inbox_revision,aggregate_revision >= 1"`
	PayloadHash       string     `gorm:"type:char(64);not null;check:ck_evt_inbox_payload_hash,char_length(payload_hash) = 64"`
	OriginalTopic     string     `gorm:"type:varchar(249);not null"`
	SourcePartition   int32      `gorm:"not null"`
	SourceOffset      int64      `gorm:"not null;check:ck_evt_inbox_source_offset,source_offset >= 0"`
	Status            string     `gorm:"type:varchar(24);not null;check:ck_evt_inbox_status,status IN ('processing','retryable','processed','ignored_stale','dead_lettered')"`
	ClaimToken        string     `gorm:"type:varchar(120);not null;default:''"`
	ClaimExpiresAt    *time.Time `gorm:"type:timestamptz"`
	Attempts          int        `gorm:"not null;default:0;check:ck_evt_inbox_attempts,attempts >= 0"`
	LastError         string     `gorm:"type:varchar(500);not null;default:''"`
	ReceivedAt        time.Time  `gorm:"type:timestamptz;not null;index:ix_evt_inbox_project_received,priority:2"`
	ProcessedAt       *time.Time `gorm:"type:timestamptz"`
	Workspace         Workspace  `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project    `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (InboxEvent) TableName() string { return "evt_inbox_events" }

type EventCheckpoint struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey"`
	ConsumerGroup   string     `gorm:"type:varchar(150);not null;uniqueIndex:ux_evt_checkpoint_aggregate,priority:1"`
	WorkspaceID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:ux_evt_checkpoint_aggregate,priority:2"`
	AggregateKind   string     `gorm:"type:varchar(80);not null;uniqueIndex:ux_evt_checkpoint_aggregate,priority:3"`
	AggregateID     string     `gorm:"type:varchar(120);not null;uniqueIndex:ux_evt_checkpoint_aggregate,priority:4"`
	ProjectID       uuid.UUID  `gorm:"type:uuid;not null"`
	LastRevision    int64      `gorm:"not null;default:0;check:ck_evt_checkpoint_last_revision,last_revision >= 0"`
	LastEventID     string     `gorm:"type:varchar(120);not null;default:''"`
	PendingEventID  string     `gorm:"type:varchar(120);not null;default:''"`
	PendingRevision int64      `gorm:"not null;default:0;check:ck_evt_checkpoint_pending_revision,pending_revision >= 0"`
	ClaimToken      string     `gorm:"type:varchar(120);not null;default:''"`
	ClaimExpiresAt  *time.Time `gorm:"type:timestamptz"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null"`
	Workspace       Workspace  `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project         Project    `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (EventCheckpoint) TableName() string { return "evt_checkpoints" }

type DeadLetter struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ConsumerGroup     string         `gorm:"type:varchar(150);not null;uniqueIndex:ux_evt_dead_letter_group_event,priority:1"`
	EventID           string         `gorm:"type:varchar(200);not null;uniqueIndex:ux_evt_dead_letter_group_event,priority:2"`
	EventType         string         `gorm:"type:varchar(100);not null;default:''"`
	ProjectID         string         `gorm:"type:varchar(36);not null;default:'';index:ix_evt_dead_letter_replay,priority:2"`
	AggregateKind     string         `gorm:"type:varchar(80);not null;default:''"`
	AggregateID       string         `gorm:"type:varchar(120);not null;default:''"`
	AggregateRevision int64          `gorm:"not null;default:0;check:ck_evt_dead_letter_revision,aggregate_revision >= 0"`
	OriginalTopic     string         `gorm:"type:varchar(249);not null"`
	DLQTopic          string         `gorm:"type:varchar(249);not null"`
	SourcePartition   int32          `gorm:"not null"`
	SourceOffset      int64          `gorm:"not null;check:ck_evt_dead_letter_source_offset,source_offset >= 0"`
	PayloadHash       string         `gorm:"type:char(64);not null;check:ck_evt_dead_letter_payload_hash,char_length(payload_hash) = 64"`
	FailureCode       string         `gorm:"type:varchar(80);not null"`
	FailureMessage    string         `gorm:"type:varchar(500);not null"`
	Replayable        bool           `gorm:"not null;default:false"`
	Envelope          datatypes.JSON `gorm:"type:jsonb"`
	Status            string         `gorm:"type:varchar(24);not null;index:ix_evt_dead_letter_replay,priority:1;check:ck_evt_dead_letter_status,status IN ('ready','replay_claimed','replayed')"`
	ClaimToken        string         `gorm:"type:varchar(120);not null;default:''"`
	ClaimExpiresAt    *time.Time     `gorm:"type:timestamptz"`
	ReplayCount       int            `gorm:"not null;default:0;check:ck_evt_dead_letter_replay_count,replay_count >= 0"`
	FailedAt          time.Time      `gorm:"type:timestamptz;not null;index:ix_evt_dead_letter_replay,priority:3"`
	LastReplayedAt    *time.Time     `gorm:"type:timestamptz"`
	CreatedAt         time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time      `gorm:"type:timestamptz;not null"`
}

func (DeadLetter) TableName() string { return "evt_dead_letters" }
