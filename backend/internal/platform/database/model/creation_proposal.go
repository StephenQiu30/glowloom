package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// CreationProposal pins an external immutable draft for a platform review. The
// authoritative draft remains Python-owned; only exact submitted bytes are copied.
type CreationProposal struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	RunID             uuid.UUID      `gorm:"type:uuid;not null;index"`
	StepID            uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"`
	Stage             string         `gorm:"type:varchar(40);not null"`
	ResultHash        string         `gorm:"type:char(64);not null"`
	Draft             datatypes.JSON `gorm:"type:jsonb;not null"`
	HumanTaskID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"`
	Acceptance        datatypes.JSON `gorm:"type:jsonb"`
	AdoptionInputHash string         `gorm:"type:varchar(64);not null;default:''"`
	AdoptionKey       string         `gorm:"type:varchar(200);not null;default:''"`
	ProjectRevision   int            `gorm:"not null"`
	CreatedAt         time.Time      `gorm:"type:timestamptz;not null"`
	Run               CreationRun    `gorm:"foreignKey:RunID;constraint:OnDelete:RESTRICT"`
	HumanTask         HumanTask      `gorm:"foreignKey:HumanTaskID;constraint:OnDelete:RESTRICT"`
}

func (CreationProposal) TableName() string { return "crn_proposals" }
