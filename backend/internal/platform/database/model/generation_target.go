package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableGenerationTarget = errors.New("GenerationTarget is immutable")

// GenerationTarget stores the canonical Backend-owned input snapshot for one
// image generation. Payload is a strict union decoded by the generation
// adapter; PostgreSQL remains the only SQL fact source.
type GenerationTarget struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null;index:ix_gen_targets_workspace_hash,priority:1"`
	ProjectID         uuid.UUID      `gorm:"type:uuid;not null;index:ix_gen_targets_workspace_hash,priority:2"`
	Kind              string         `gorm:"type:varchar(32);not null;index;check:ck_gen_target_kind,kind IN ('reference_asset','shot_frame')"`
	SourceOwnerRef    datatypes.JSON `gorm:"type:jsonb;not null"`
	SourceContentHash string         `gorm:"type:char(64);not null;check:ck_gen_target_source_hash,char_length(source_content_hash) = 64"`
	PolicySnapshotRef datatypes.JSON `gorm:"type:jsonb;not null"`
	PolicyContentHash string         `gorm:"type:char(64);not null;check:ck_gen_target_policy_hash,char_length(policy_content_hash) = 64"`
	Payload           datatypes.JSON `gorm:"type:jsonb;not null"`
	TargetHash        string         `gorm:"type:char(64);not null;index:ix_gen_targets_workspace_hash,priority:3;check:ck_gen_target_hash,char_length(target_hash) = 64"`
	Revision          int64          `gorm:"not null;check:ck_gen_target_revision,revision = 1"`
	CreatedBy         uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt         time.Time      `gorm:"type:timestamptz;not null"`
	Workspace         Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project        `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator           UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationTarget) TableName() string { return "gen_targets" }

func (*GenerationTarget) BeforeUpdate(*gorm.DB) error { return ErrImmutableGenerationTarget }
func (*GenerationTarget) BeforeDelete(*gorm.DB) error { return ErrImmutableGenerationTarget }
