package model

import (
	"time"

	"github.com/google/uuid"
)

type QuotaPolicy struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_qta_policy_scope,priority:1"`
	ProjectID   uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_qta_policy_scope,priority:2;index"`
	Metric      string      `gorm:"type:varchar(80);not null;uniqueIndex:uq_qta_policy_scope,priority:3;check:ck_qta_policy_metric,metric = 'generation.image'"`
	WindowKind  string      `gorm:"type:varchar(20);not null;check:ck_qta_policy_window_kind,window_kind = 'UTC_DAY'"`
	LimitUnits  int64       `gorm:"not null;check:ck_qta_policy_limit,limit_units > 0"`
	Revision    int64       `gorm:"not null;check:ck_qta_policy_revision,revision >= 1"`
	ContentHash string      `gorm:"type:char(64);not null;check:ck_qta_policy_content_hash,char_length(content_hash) = 64"`
	CreatedBy   uuid.UUID   `gorm:"type:uuid;not null"`
	UpdatedBy   uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt   time.Time   `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time   `gorm:"type:timestamptz;not null"`
	Workspace   Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator     UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Updater     UserAccount `gorm:"foreignKey:UpdatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (QuotaPolicy) TableName() string { return "qta_policies" }

type QuotaCounter struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID   `gorm:"type:uuid;not null;index:ix_qta_counters_workspace_window,priority:1"`
	ProjectID      uuid.UUID   `gorm:"type:uuid;not null;index"`
	PolicyID       uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_qta_counter_window,priority:1"`
	Metric         string      `gorm:"type:varchar(80);not null;check:ck_qta_counter_metric,metric = 'generation.image'"`
	WindowStart    time.Time   `gorm:"type:timestamptz;not null;uniqueIndex:uq_qta_counter_window,priority:2;index:ix_qta_counters_workspace_window,priority:2"`
	WindowEnd      time.Time   `gorm:"type:timestamptz;not null;check:ck_qta_counter_window,window_end > window_start"`
	PolicyRevision int64       `gorm:"not null;check:ck_qta_counter_policy_revision,policy_revision >= 1"`
	LimitUnits     int64       `gorm:"not null;check:ck_qta_counter_limit,limit_units > 0"`
	ReservedUnits  int64       `gorm:"not null;check:ck_qta_counter_reserved,reserved_units >= 0"`
	ConsumedUnits  int64       `gorm:"not null;check:ck_qta_counter_consumed,consumed_units >= 0;check:ck_qta_counter_total,reserved_units + consumed_units <= limit_units"`
	Revision       int64       `gorm:"not null;check:ck_qta_counter_revision,revision >= 1"`
	CreatedAt      time.Time   `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time   `gorm:"type:timestamptz;not null"`
	Workspace      Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project        Project     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Policy         QuotaPolicy `gorm:"foreignKey:PolicyID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (QuotaCounter) TableName() string { return "qta_counters" }

type QuotaReservation struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey"`
	WorkspaceID    uuid.UUID    `gorm:"type:uuid;not null;index:ix_qta_reservations_workspace_status_updated,priority:1"`
	ProjectID      uuid.UUID    `gorm:"type:uuid;not null;index"`
	PolicyID       uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_qta_reservation_source,priority:1"`
	CounterID      uuid.UUID    `gorm:"type:uuid;not null;index"`
	Metric         string       `gorm:"type:varchar(80);not null;check:ck_qta_reservation_metric,metric = 'generation.image'"`
	SourceType     string       `gorm:"type:varchar(60);not null;uniqueIndex:uq_qta_reservation_source,priority:3;check:ck_qta_reservation_source_type,source_type = 'generation_intent'"`
	SourceID       uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:uq_qta_reservation_source,priority:4"`
	WindowStart    time.Time    `gorm:"type:timestamptz;not null;uniqueIndex:uq_qta_reservation_source,priority:2"`
	WindowEnd      time.Time    `gorm:"type:timestamptz;not null;check:ck_qta_reservation_window,window_end > window_start"`
	PolicyRevision int64        `gorm:"not null;check:ck_qta_reservation_policy_revision,policy_revision >= 1"`
	LimitUnits     int64        `gorm:"not null;check:ck_qta_reservation_limit,limit_units > 0"`
	Units          int64        `gorm:"not null;check:ck_qta_reservation_units,units > 0"`
	Status         string       `gorm:"type:varchar(20);not null;index:ix_qta_reservations_workspace_status_updated,priority:2;check:ck_qta_reservation_status,status IN ('RESERVED','CONSUMED','RELEASED')"`
	BindingHash    string       `gorm:"type:char(64);not null;check:ck_qta_reservation_binding_hash,char_length(binding_hash) = 64"`
	Revision       int64        `gorm:"not null;check:ck_qta_reservation_revision,revision >= 1"`
	CreatedBy      uuid.UUID    `gorm:"type:uuid;not null"`
	CreatedAt      time.Time    `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time    `gorm:"type:timestamptz;not null;index:ix_qta_reservations_workspace_status_updated,priority:3,sort:desc"`
	ConsumedAt     *time.Time   `gorm:"type:timestamptz"`
	ReleasedAt     *time.Time   `gorm:"type:timestamptz"`
	Workspace      Workspace    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project        Project      `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Policy         QuotaPolicy  `gorm:"foreignKey:PolicyID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Counter        QuotaCounter `gorm:"foreignKey:CounterID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator        UserAccount  `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (QuotaReservation) TableName() string { return "qta_reservations" }
