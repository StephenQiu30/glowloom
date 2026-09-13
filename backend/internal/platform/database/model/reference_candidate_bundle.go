package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type GenerationReferenceCandidateBundle struct {
	ID                uuid.UUID                      `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID                      `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID                      `gorm:"type:uuid;not null;index"`
	ExecutionID       uuid.UUID                      `gorm:"type:uuid;not null;index"`
	VisionCandidateID uuid.UUID                      `gorm:"type:uuid;not null;uniqueIndex"`
	ContentHash       string                         `gorm:"type:char(64);not null;check:ck_gen_ref_bundle_hash,char_length(content_hash) = 64"`
	Content           datatypes.JSON                 `gorm:"type:jsonb;not null"`
	CreatedAt         time.Time                      `gorm:"type:timestamptz;not null"`
	Project           Project                        `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Execution         GenerationReferenceExecution   `gorm:"foreignKey:ExecutionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	VisionCandidate   SceneAnalysisCandidateRevision `gorm:"foreignKey:VisionCandidateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceCandidateBundle) TableName() string {
	return "gen_reference_candidate_bundles"
}
func (*GenerationReferenceCandidateBundle) BeforeUpdate(*gorm.DB) error {
	return errors.New("Reference Candidate Bundle is immutable")
}
func (*GenerationReferenceCandidateBundle) BeforeDelete(*gorm.DB) error {
	return errors.New("Reference Candidate Bundle is immutable")
}
