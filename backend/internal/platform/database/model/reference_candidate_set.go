package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type GenerationReferenceCandidateSet struct {
	ID          uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID   uuid.UUID                    `gorm:"type:uuid;not null;index"`
	ExecutionID uuid.UUID                    `gorm:"type:uuid;not null;index"`
	ContentHash string                       `gorm:"type:char(64);not null;uniqueIndex;check:ck_gen_ref_set_hash,char_length(content_hash) = 64"`
	Content     datatypes.JSON               `gorm:"type:jsonb;not null"`
	CreatedAt   time.Time                    `gorm:"type:timestamptz;not null"`
	Project     Project                      `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Execution   GenerationReferenceExecution `gorm:"foreignKey:ExecutionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceCandidateSet) TableName() string { return "gen_reference_candidate_sets" }
func (*GenerationReferenceCandidateSet) BeforeUpdate(*gorm.DB) error {
	return errors.New("Reference Candidate Set is immutable")
}
func (*GenerationReferenceCandidateSet) BeforeDelete(*gorm.DB) error {
	return errors.New("Reference Candidate Set is immutable")
}
