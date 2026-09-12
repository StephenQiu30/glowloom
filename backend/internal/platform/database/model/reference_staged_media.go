package model

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type GenerationReferenceStagedMedia struct {
	ID          uuid.UUID                       `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID                       `gorm:"type:uuid;not null;index:ix_gen_ref_staged_scope,priority:1"`
	ProjectID   uuid.UUID                       `gorm:"type:uuid;not null;index:ix_gen_ref_staged_scope,priority:2"`
	CallKey     string                          `gorm:"type:char(64);not null;uniqueIndex"`
	ReceiptHash string                          `gorm:"type:char(64);not null;check:ck_gen_ref_staged_receipt,char_length(receipt_hash) = 64"`
	State       string                          `gorm:"type:varchar(32);not null;check:ck_gen_ref_staged_state,state IN ('quarantined','ready_for_review','rejected')"`
	Revision    int64                           `gorm:"not null;check:ck_gen_ref_staged_revision,(state = 'quarantined' AND revision = 1) OR (state IN ('ready_for_review','rejected') AND revision = 2)"`
	ContentHash string                          `gorm:"type:char(64);not null;check:ck_gen_ref_staged_hash,char_length(content_hash) = 64"`
	Content     datatypes.JSON                  `gorm:"type:jsonb;not null"`
	Call        GenerationReferenceProviderCall `gorm:"belongsTo:Call;foreignKey:CallKey;references:CallKey;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationReferenceStagedMedia) TableName() string { return "gen_reference_staged_media" }
func (*GenerationReferenceStagedMedia) BeforeUpdate(*gorm.DB) error {
	return errors.New("staged media changes require owner CAS")
}
func (*GenerationReferenceStagedMedia) BeforeDelete(*gorm.DB) error {
	return errors.New("staged media identity must be retained")
}
