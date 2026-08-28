package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type GenerationProviderBindingVersion struct {
	ID            uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID   uuid.UUID   `gorm:"type:uuid;not null;index"`
	ProjectID     uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_binding_revision,priority:1"`
	Capability    string      `gorm:"type:varchar(64);not null;uniqueIndex:uq_gen_provider_binding_revision,priority:2;check:ck_gen_provider_binding_capability,capability = 'generation.image'"`
	Revision      int64       `gorm:"not null;uniqueIndex:uq_gen_provider_binding_revision,priority:3;check:ck_gen_provider_binding_revision,revision >= 1"`
	ProviderKey   string      `gorm:"type:varchar(80);not null"`
	ModelKey      string      `gorm:"type:varchar(120);not null"`
	CredentialRef string      `gorm:"type:varchar(160);not null"`
	ContentHash   string      `gorm:"type:char(64);not null;check:ck_gen_provider_binding_hash,char_length(content_hash) = 64"`
	CreatedBy     uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt     time.Time   `gorm:"type:timestamptz;not null"`
	Workspace     Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project       Project     `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator       UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationProviderBindingVersion) TableName() string { return "gen_provider_binding_versions" }

type GenerationRequest struct {
	ID              uuid.UUID                        `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID                        `gorm:"type:uuid;not null;index"`
	ProjectID       uuid.UUID                        `gorm:"type:uuid;not null;index"`
	IntentID        uuid.UUID                        `gorm:"type:uuid;not null;uniqueIndex:uq_gen_request_intent"`
	TargetID        uuid.UUID                        `gorm:"type:uuid;not null;index"`
	BindingID       uuid.UUID                        `gorm:"type:uuid;not null;index"`
	BindingRevision int64                            `gorm:"not null;check:ck_gen_request_binding_revision,binding_revision >= 1"`
	Capability      string                           `gorm:"type:varchar(64);not null;check:ck_gen_request_capability,capability = 'generation.image'"`
	ProviderKey     string                           `gorm:"type:varchar(80);not null"`
	ModelKey        string                           `gorm:"type:varchar(120);not null"`
	CredentialRef   string                           `gorm:"type:varchar(160);not null"`
	RequestKey      string                           `gorm:"type:varchar(96);not null;uniqueIndex:uq_gen_request_key"`
	TargetHash      string                           `gorm:"type:char(64);not null;check:ck_gen_request_target_hash,char_length(target_hash) = 64"`
	Units           int64                            `gorm:"not null;check:ck_gen_request_units,units > 0"`
	ContentHash     string                           `gorm:"type:char(64);not null;check:ck_gen_request_hash,char_length(content_hash) = 64"`
	CreatedBy       uuid.UUID                        `gorm:"type:uuid;not null"`
	CreatedAt       time.Time                        `gorm:"type:timestamptz;not null"`
	Workspace       Workspace                        `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project         Project                          `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Intent          GenerationIntent                 `gorm:"foreignKey:IntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Target          GenerationTarget                 `gorm:"foreignKey:TargetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Binding         GenerationProviderBindingVersion `gorm:"foreignKey:BindingID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator         UserAccount                      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationRequest) TableName() string { return "gen_requests" }

type GenerationProviderJob struct {
	ID                uuid.UUID         `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID         `gorm:"type:uuid;not null;index"`
	ProjectID         uuid.UUID         `gorm:"type:uuid;not null;index"`
	IntentID          uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_job_intent"`
	RequestID         uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_job_request"`
	ProviderKey       string            `gorm:"type:varchar(80);not null;uniqueIndex:uq_gen_provider_external_job,priority:1"`
	RequestKey        string            `gorm:"type:varchar(96);not null;uniqueIndex:uq_gen_provider_job_request_key"`
	ProviderJobKey    *string           `gorm:"type:varchar(180);uniqueIndex:uq_gen_provider_external_job,priority:2"`
	Status            string            `gorm:"type:varchar(20);not null;index;check:ck_gen_provider_job_state,status IN ('DISPATCHING','RUNNING','UNKNOWN','SUCCEEDED','FAILED')"`
	ProviderReceiptID *uuid.UUID        `gorm:"type:uuid;check:ck_gen_provider_job_terminal,(status IN ('DISPATCHING','RUNNING','UNKNOWN') AND provider_receipt_id IS NULL) OR (status IN ('SUCCEEDED','FAILED') AND provider_receipt_id IS NOT NULL)"`
	Revision          int64             `gorm:"not null;check:ck_gen_provider_job_revision,revision >= 1"`
	ContentHash       string            `gorm:"type:char(64);not null;check:ck_gen_provider_job_hash,char_length(content_hash) = 64"`
	CreatedAt         time.Time         `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time         `gorm:"type:timestamptz;not null"`
	Workspace         Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project           `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Intent            GenerationIntent  `gorm:"foreignKey:IntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Request           GenerationRequest `gorm:"foreignKey:RequestID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationProviderJob) TableName() string { return "gen_provider_jobs" }

type GenerationProviderResultReceipt struct {
	ID              uuid.UUID             `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID             `gorm:"type:uuid;not null;index"`
	ProjectID       uuid.UUID             `gorm:"type:uuid;not null;index"`
	JobID           uuid.UUID             `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_receipt_job"`
	RequestID       uuid.UUID             `gorm:"type:uuid;not null;index"`
	ProviderKey     string                `gorm:"type:varchar(80);not null;uniqueIndex:uq_gen_provider_receipt_event,priority:1"`
	ProviderJobKey  *string               `gorm:"type:varchar(180)"`
	ProviderEventID string                `gorm:"type:varchar(180);not null;uniqueIndex:uq_gen_provider_receipt_event,priority:2"`
	Status          string                `gorm:"type:varchar(20);not null;check:ck_gen_provider_receipt_status,status IN ('SUCCEEDED','FAILED')"`
	ActualUnits     int64                 `gorm:"not null;check:ck_gen_provider_receipt_units,(status = 'SUCCEEDED' AND actual_units > 0) OR (status = 'FAILED' AND actual_units = 0)"`
	Outputs         datatypes.JSON        `gorm:"type:jsonb;not null"`
	FailureCode     *string               `gorm:"type:varchar(120);check:ck_gen_provider_receipt_result,(status = 'SUCCEEDED' AND failure_code IS NULL) OR (status = 'FAILED' AND failure_code IS NOT NULL)"`
	ContentHash     string                `gorm:"type:char(64);not null;check:ck_gen_provider_receipt_hash,char_length(content_hash) = 64"`
	OccurredAt      time.Time             `gorm:"type:timestamptz;not null"`
	ReceivedAt      time.Time             `gorm:"type:timestamptz;not null"`
	Workspace       Workspace             `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project         Project               `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Job             GenerationProviderJob `gorm:"foreignKey:JobID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Request         GenerationRequest     `gorm:"foreignKey:RequestID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationProviderResultReceipt) TableName() string { return "gen_provider_result_receipts" }
