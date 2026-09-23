package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type CanvasDocument struct {
	ProjectID   uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID      `gorm:"type:uuid;not null;index:ix_can_document_workspace_updated,priority:1"`
	Revision    int            `gorm:"not null;check:ck_can_document_revision,revision >= 1"`
	Content     datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null;index:ix_can_document_workspace_updated,priority:2,sort:desc"`
	Project     Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (CanvasDocument) TableName() string { return "can_documents" }
