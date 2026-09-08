package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableTextIntentVersion = errors.New("TextIntentVersion is immutable")

type TextIntentVersion struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey"`
	RunID            uuid.UUID        `gorm:"type:uuid;not null;index"`
	ProposalID       uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex"`
	DecisionID       uuid.UUID        `gorm:"type:uuid;not null"`
	WorkspaceID      uuid.UUID        `gorm:"type:uuid;not null"`
	ProjectID        uuid.UUID        `gorm:"type:uuid;not null;index"`
	SourceRevisionID uuid.UUID        `gorm:"type:uuid;not null"`
	SourceHash       string           `gorm:"type:char(64);not null"`
	EpisodeID        uuid.UUID        `gorm:"type:uuid;not null"`
	StructureID      uuid.UUID        `gorm:"type:uuid;not null"`
	SceneID          uuid.UUID        `gorm:"type:uuid;not null;index"`
	WorldVersionID   uuid.UUID        `gorm:"type:uuid;not null"`
	Revision         int              `gorm:"not null;check:ck_stb_text_intent_revision,revision = 1"`
	ContentHash      string           `gorm:"type:char(64);not null"`
	AssetReadiness   string           `gorm:"type:varchar(20);not null;check:ck_stb_text_intent_readiness,asset_readiness = 'needs_asset'"`
	Body             datatypes.JSON   `gorm:"type:jsonb;not null"`
	CreatedBy        uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt        time.Time        `gorm:"type:timestamptz;not null"`
	Project          Project          `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	SourceRevision   DocumentRevision `gorm:"foreignKey:SourceRevisionID;constraint:OnDelete:RESTRICT"`
	Decision         ReviewDecision   `gorm:"foreignKey:DecisionID;constraint:OnDelete:RESTRICT"`
	Structure        EpisodeStructure `gorm:"foreignKey:StructureID;constraint:OnDelete:RESTRICT"`
	WorldVersion     TextWorldVersion `gorm:"foreignKey:WorldVersionID;constraint:OnDelete:RESTRICT"`
}

func (TextIntentVersion) TableName() string { return "stb_text_intent_versions" }

func (*TextIntentVersion) BeforeUpdate(*gorm.DB) error { return ErrImmutableTextIntentVersion }
func (*TextIntentVersion) BeforeDelete(*gorm.DB) error { return ErrImmutableTextIntentVersion }
