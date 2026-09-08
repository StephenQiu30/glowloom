package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableTextWorldVersion = errors.New("TextWorldVersion is immutable")

// TextWorldVersion is a formal, immutable Bible record built from reviewed text.
type TextWorldVersion struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey"`
	RunID            uuid.UUID        `gorm:"type:uuid;not null;index"`
	ProposalID       uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex"`
	DecisionID       uuid.UUID        `gorm:"type:uuid;not null"`
	WorkspaceID      uuid.UUID        `gorm:"type:uuid;not null"`
	ProjectID        uuid.UUID        `gorm:"type:uuid;not null;index"`
	SourceRevisionID uuid.UUID        `gorm:"type:uuid;not null"`
	SourceHash       string           `gorm:"type:char(64);not null"`
	Revision         int              `gorm:"not null;check:ck_bib_text_world_revision,revision = 1"`
	ContentHash      string           `gorm:"type:char(64);not null"`
	Body             datatypes.JSON   `gorm:"type:jsonb;not null"`
	CreatedBy        uuid.UUID        `gorm:"type:uuid;not null"`
	CreatedAt        time.Time        `gorm:"type:timestamptz;not null"`
	Project          Project          `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	SourceRevision   DocumentRevision `gorm:"foreignKey:SourceRevisionID;constraint:OnDelete:RESTRICT"`
	Decision         ReviewDecision   `gorm:"foreignKey:DecisionID;constraint:OnDelete:RESTRICT"`
}

func (TextWorldVersion) TableName() string { return "bib_text_world_versions" }

func (*TextWorldVersion) BeforeUpdate(*gorm.DB) error { return ErrImmutableTextWorldVersion }
func (*TextWorldVersion) BeforeDelete(*gorm.DB) error { return ErrImmutableTextWorldVersion }
