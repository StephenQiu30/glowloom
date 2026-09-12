package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableReferenceProviderIdentity = errors.New("Reference Provider invocation identity is immutable")

type GenerationReferenceProviderJob struct {
	ExecutionID   uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID   uuid.UUID                    `gorm:"type:uuid;not null;index:ix_gen_ref_job_scope,priority:1"`
	ProjectID     uuid.UUID                    `gorm:"type:uuid;not null;index:ix_gen_ref_job_scope,priority:2"`
	ExecutionHash string                       `gorm:"type:char(64);not null;check:ck_gen_ref_job_execution_hash,char_length(execution_hash) = 64"`
	CallSetRoot   string                       `gorm:"type:char(64);not null;check:ck_gen_ref_job_root,char_length(call_set_root) = 64"`
	ContentHash   string                       `gorm:"type:char(64);not null;check:ck_gen_ref_job_hash,char_length(content_hash) = 64"`
	Content       datatypes.JSON               `gorm:"type:jsonb;not null"`
	Execution     GenerationReferenceExecution `gorm:"foreignKey:ExecutionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceProviderJob) TableName() string { return "gen_reference_provider_jobs" }
func (*GenerationReferenceProviderJob) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableReferenceProviderIdentity
}
func (*GenerationReferenceProviderJob) BeforeDelete(*gorm.DB) error {
	return ErrImmutableReferenceProviderIdentity
}

type GenerationReferenceProviderCall struct {
	CallKey             string                         `gorm:"type:char(64);primaryKey;check:ck_gen_ref_call_key,char_length(call_key) = 64"`
	ExecutionID         uuid.UUID                      `gorm:"type:uuid;not null;uniqueIndex:ux_gen_ref_call_slot,priority:1"`
	WorkspaceID         uuid.UUID                      `gorm:"type:uuid;not null;index:ix_gen_ref_call_scope,priority:1"`
	ProjectID           uuid.UUID                      `gorm:"type:uuid;not null;index:ix_gen_ref_call_scope,priority:2"`
	BundleIndex         int                            `gorm:"not null;uniqueIndex:ux_gen_ref_call_slot,priority:2;check:ck_gen_ref_call_bundle,bundle_index >= 0 AND bundle_index < 4"`
	SlotKey             string                         `gorm:"type:varchar(120);not null;uniqueIndex:ux_gen_ref_call_slot,priority:3"`
	CompiledRequestHash string                         `gorm:"type:char(64);not null;check:ck_gen_ref_call_request_hash,char_length(compiled_request_hash) = 64"`
	Content             datatypes.JSON                 `gorm:"type:jsonb;not null"`
	Status              string                         `gorm:"type:varchar(32);not null;check:ck_gen_ref_call_status,status IN ('PENDING','DISPATCHING','OUTCOME_UNKNOWN')"`
	Revision            int64                          `gorm:"not null;check:ck_gen_ref_call_revision,revision >= 1 AND revision <= 3"`
	StateContent        datatypes.JSON                 `gorm:"type:jsonb;not null"`
	StateHash           string                         `gorm:"type:char(64);not null;check:ck_gen_ref_call_state_hash,char_length(state_hash) = 64"`
	SubmissionToken     *uuid.UUID                     `gorm:"type:uuid;uniqueIndex;check:ck_gen_ref_call_dispatch_metadata,(status = 'PENDING' AND submission_token IS NULL AND dispatched_at IS NULL) OR (status IN ('DISPATCHING','OUTCOME_UNKNOWN') AND submission_token IS NOT NULL AND dispatched_at IS NOT NULL)"`
	DispatchedAt        *time.Time                     `gorm:"type:timestamptz"`
	Job                 GenerationReferenceProviderJob `gorm:"belongsTo:Job;foreignKey:ExecutionID;references:ExecutionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceProviderCall) TableName() string { return "gen_reference_provider_calls" }
func (*GenerationReferenceProviderCall) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableReferenceProviderIdentity
}
func (*GenerationReferenceProviderCall) BeforeDelete(*gorm.DB) error {
	return ErrImmutableReferenceProviderIdentity
}
