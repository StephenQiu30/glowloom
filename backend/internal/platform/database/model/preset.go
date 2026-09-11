package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableProjectPresetSelection = errors.New("ProjectPresetSelection is immutable")

type ProjectPresetSelection struct {
	ID                uuid.UUID               `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID               `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID               `gorm:"type:uuid;not null;uniqueIndex:uq_pre_project_selection_revision,priority:1"`
	Revision          int64                   `gorm:"not null;uniqueIndex:uq_pre_project_selection_revision,priority:2;check:ck_pre_project_selection_revision,revision >= 1"`
	ParentSelectionID *uuid.UUID              `gorm:"type:uuid"`
	ParentContentHash *string                 `gorm:"type:char(64);check:ck_pre_project_selection_parent_hash,parent_content_hash IS NULL OR char_length(parent_content_hash) = 64"`
	PresetKey         string                  `gorm:"type:varchar(64);not null"`
	PresetRelease     string                  `gorm:"type:varchar(10);not null"`
	PresetContentHash string                  `gorm:"type:char(64);not null;check:ck_pre_project_selection_preset_hash,char_length(preset_content_hash) = 64"`
	ApplicationMode   string                  `gorm:"type:varchar(24);not null;check:ck_pre_project_selection_mode,application_mode IN ('faithful','world_adaptation')"`
	Selection         datatypes.JSON          `gorm:"type:jsonb;not null;check:ck_pre_project_selection_json,jsonb_typeof(selection) = 'object'"`
	ContentHash       string                  `gorm:"type:char(64);not null;unique;check:ck_pre_project_selection_hash,char_length(content_hash) = 64"`
	SelectedBy        uuid.UUID               `gorm:"type:uuid;not null"`
	SelectedAt        time.Time               `gorm:"type:timestamptz;not null"`
	CreatedAt         time.Time               `gorm:"type:timestamptz;not null"`
	Workspace         Workspace               `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project                 `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ParentSelection   *ProjectPresetSelection `gorm:"foreignKey:ParentSelectionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Selector          UserAccount             `gorm:"foreignKey:SelectedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectPresetSelection) TableName() string { return "pre_project_preset_selections" }
func (*ProjectPresetSelection) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProjectPresetSelection
}
func (*ProjectPresetSelection) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProjectPresetSelection
}

type ProjectPresetSelectionHead struct {
	WorkspaceID        uuid.UUID              `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID              `gorm:"type:uuid;primaryKey"`
	CurrentSelectionID uuid.UUID              `gorm:"type:uuid;not null;unique"`
	CurrentContentHash string                 `gorm:"type:char(64);not null;check:ck_pre_project_selection_head_hash,char_length(current_content_hash) = 64"`
	Revision           int64                  `gorm:"not null;check:ck_pre_project_selection_head_revision,revision >= 1"`
	UpdatedAt          time.Time              `gorm:"type:timestamptz;not null"`
	Workspace          Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project                `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentSelection   ProjectPresetSelection `gorm:"foreignKey:CurrentSelectionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectPresetSelectionHead) TableName() string { return "pre_project_preset_selection_heads" }
