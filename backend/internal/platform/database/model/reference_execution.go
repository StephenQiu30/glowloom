package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableReferenceExecution = errors.New("ReferenceExecution is immutable")

type GenerationReferenceExecution struct {
	ID          uuid.UUID        `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID        `gorm:"type:uuid;not null;index:ix_gen_ref_exec_scope,priority:1"`
	ProjectID   uuid.UUID        `gorm:"type:uuid;not null;index:ix_gen_ref_exec_scope,priority:2"`
	TargetID    uuid.UUID        `gorm:"type:uuid;not null;index:ix_gen_ref_exec_scope,priority:3"`
	TargetHash  string           `gorm:"type:char(64);not null;check:ck_gen_ref_exec_target_hash,char_length(target_hash) = 64"`
	Revision    int64            `gorm:"not null;check:ck_gen_ref_exec_revision,revision = 1"`
	ContentHash string           `gorm:"type:char(64);not null;check:ck_gen_ref_exec_hash,char_length(content_hash) = 64"`
	Content     datatypes.JSON   `gorm:"type:jsonb;not null"`
	CreatedBy   uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt   time.Time        `gorm:"type:timestamptz;not null"`
	Target      GenerationTarget `gorm:"foreignKey:TargetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project          `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator     UserAccount      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceExecution) TableName() string { return "gen_reference_executions" }
func (*GenerationReferenceExecution) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableReferenceExecution
}
func (*GenerationReferenceExecution) BeforeDelete(*gorm.DB) error {
	return ErrImmutableReferenceExecution
}

type GenerationReferenceExecutionHead struct {
	WorkspaceID          uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	ProjectID            uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	TargetID             uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	TargetHash           string                       `gorm:"type:char(64);not null;check:ck_gen_ref_exec_head_target_hash,char_length(target_hash) = 64"`
	CurrentExecutionID   uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex"`
	CurrentExecutionHash string                       `gorm:"type:char(64);not null;check:ck_gen_ref_exec_head_hash,char_length(current_execution_hash) = 64"`
	Revision             int64                        `gorm:"not null;check:ck_gen_ref_exec_head_revision,revision >= 1"`
	Target               GenerationTarget             `gorm:"foreignKey:TargetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentExecution     GenerationReferenceExecution `gorm:"foreignKey:CurrentExecutionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceExecutionHead) TableName() string { return "gen_reference_execution_heads" }
