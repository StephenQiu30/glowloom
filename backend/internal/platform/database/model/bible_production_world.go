package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableProductionWorldBibleFact = errors.New("Production World Bible fact is immutable")

type ProductionWorldEvidence struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID   uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_scr_world_evidence_revision,priority:1;uniqueIndex:uq_scr_world_evidence_content,priority:1"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null"`
	SubjectKey  string         `gorm:"type:varchar(100);not null;uniqueIndex:uq_scr_world_evidence_revision,priority:2;uniqueIndex:uq_scr_world_evidence_content,priority:2"`
	Revision    int            `gorm:"not null;uniqueIndex:uq_scr_world_evidence_revision,priority:3;check:ck_scr_world_evidence_revision,revision >= 1"`
	Basis       datatypes.JSON `gorm:"type:jsonb;not null;check:ck_scr_world_evidence_basis,jsonb_typeof(basis) = 'object'"`
	ContentHash string         `gorm:"type:char(64);not null;uniqueIndex:uq_scr_world_evidence_content,priority:3;check:ck_scr_world_evidence_hash,char_length(content_hash) = 64"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator     UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldEvidence) TableName() string { return "scr_production_world_evidence" }
func (*ProductionWorldEvidence) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}
func (*ProductionWorldEvidence) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}

type ProductionWorldSpecification struct {
	ID               uuid.UUID               `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID               `gorm:"type:uuid;not null"`
	ProjectID        uuid.UUID               `gorm:"type:uuid;not null;uniqueIndex:uq_scr_world_spec_revision,priority:1;uniqueIndex:uq_scr_world_spec_content,priority:1"`
	AssetID          uuid.UUID               `gorm:"type:uuid;not null"`
	EvidenceID       uuid.UUID               `gorm:"type:uuid;not null"`
	CreatedBy        uuid.UUID               `gorm:"type:uuid;not null"`
	SpecificationKey string                  `gorm:"type:varchar(100);not null;uniqueIndex:uq_scr_world_spec_revision,priority:2;uniqueIndex:uq_scr_world_spec_content,priority:2"`
	IdentityKey      string                  `gorm:"type:varchar(100);not null"`
	Kind             string                  `gorm:"type:varchar(20);not null;check:ck_scr_world_spec_kind,kind IN ('character','location','prop')"`
	Revision         int                     `gorm:"not null;uniqueIndex:uq_scr_world_spec_revision,priority:3;check:ck_scr_world_spec_revision,revision >= 1"`
	AssetContentHash string                  `gorm:"type:char(64);not null"`
	EvidenceHash     string                  `gorm:"type:char(64);not null"`
	Slots            datatypes.JSON          `gorm:"type:jsonb;not null;check:ck_scr_world_spec_slots,jsonb_typeof(slots) = 'array'"`
	ContentHash      string                  `gorm:"type:char(64);not null;uniqueIndex:uq_scr_world_spec_content,priority:3"`
	CreatedAt        time.Time               `gorm:"type:timestamptz;not null"`
	Workspace        Workspace               `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project          Project                 `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Asset            Asset                   `gorm:"foreignKey:AssetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Evidence         ProductionWorldEvidence `gorm:"foreignKey:EvidenceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator          UserAccount             `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldSpecification) TableName() string { return "scr_production_world_specifications" }
func (*ProductionWorldSpecification) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}
func (*ProductionWorldSpecification) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}

type ProductionWorldClaim struct {
	ID           uuid.UUID               `gorm:"type:uuid;primaryKey"`
	WorkspaceID  uuid.UUID               `gorm:"type:uuid;not null"`
	ProjectID    uuid.UUID               `gorm:"type:uuid;not null;uniqueIndex:uq_scr_world_claim_revision,priority:1;uniqueIndex:uq_scr_world_claim_content,priority:1"`
	EvidenceID   uuid.UUID               `gorm:"type:uuid;not null"`
	CreatedBy    uuid.UUID               `gorm:"type:uuid;not null"`
	ClaimKey     string                  `gorm:"type:varchar(100);not null;uniqueIndex:uq_scr_world_claim_revision,priority:2;uniqueIndex:uq_scr_world_claim_content,priority:2"`
	ClaimType    string                  `gorm:"type:varchar(24);not null;check:ck_scr_world_claim_type,claim_type IN ('world_rule','relationship','foreshadowing','payoff','story_arc','plot_thread')"`
	Statement    string                  `gorm:"type:text;not null"`
	Revision     int                     `gorm:"not null;uniqueIndex:uq_scr_world_claim_revision,priority:3;check:ck_scr_world_claim_revision,revision >= 1"`
	Participants datatypes.JSON          `gorm:"column:subjects;type:jsonb;not null;check:ck_scr_world_claim_subjects,jsonb_typeof(subjects) = 'array'"`
	Narrative    datatypes.JSON          `gorm:"type:jsonb;check:ck_scr_world_claim_narrative,narrative IS NULL OR narrative = 'null'::jsonb OR jsonb_typeof(narrative) = 'object'"`
	EvidenceHash string                  `gorm:"type:char(64);not null"`
	ContentHash  string                  `gorm:"type:char(64);not null;uniqueIndex:uq_scr_world_claim_content,priority:3"`
	CreatedAt    time.Time               `gorm:"type:timestamptz;not null"`
	Workspace    Workspace               `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project      Project                 `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Evidence     ProductionWorldEvidence `gorm:"foreignKey:EvidenceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator      UserAccount             `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldClaim) TableName() string { return "scr_production_world_claims" }
func (*ProductionWorldClaim) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}
func (*ProductionWorldClaim) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}

type ProductionWorldBinding struct {
	ID                       uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID              uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID                uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_scr_world_binding_revision,priority:1;uniqueIndex:uq_scr_world_binding_content,priority:1"`
	AssetID                  uuid.UUID                    `gorm:"type:uuid;not null"`
	SpecificationID          uuid.UUID                    `gorm:"type:uuid;not null"`
	CreatedBy                uuid.UUID                    `gorm:"type:uuid;not null"`
	IdentityKey              string                       `gorm:"type:varchar(100);not null;uniqueIndex:uq_scr_world_binding_revision,priority:2;uniqueIndex:uq_scr_world_binding_content,priority:2"`
	Revision                 int                          `gorm:"not null;uniqueIndex:uq_scr_world_binding_revision,priority:3;check:ck_scr_world_binding_revision,revision >= 1"`
	AssetContentHash         string                       `gorm:"type:char(64);not null"`
	SpecificationContentHash string                       `gorm:"type:char(64);not null"`
	ContentHash              string                       `gorm:"type:char(64);not null;uniqueIndex:uq_scr_world_binding_content,priority:3"`
	CreatedAt                time.Time                    `gorm:"type:timestamptz;not null"`
	Workspace                Workspace                    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                  Project                      `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Asset                    Asset                        `gorm:"foreignKey:AssetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Specification            ProductionWorldSpecification `gorm:"foreignKey:SpecificationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                  UserAccount                  `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldBinding) TableName() string { return "scr_production_world_bindings" }
func (*ProductionWorldBinding) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}
func (*ProductionWorldBinding) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}

type ProductionWorldBindingState struct {
	BindingID    uuid.UUID              `gorm:"type:uuid;primaryKey;uniqueIndex:uq_scr_world_binding_state_position,priority:1"`
	AssetStateID uuid.UUID              `gorm:"type:uuid;primaryKey"`
	Position     int                    `gorm:"not null;uniqueIndex:uq_scr_world_binding_state_position,priority:2;check:ck_scr_world_binding_state_position,position >= 1"`
	StateKey     string                 `gorm:"type:varchar(80);not null"`
	Revision     int                    `gorm:"not null;check:ck_scr_world_binding_state_revision,revision >= 1"`
	ContentHash  string                 `gorm:"type:char(64);not null"`
	Binding      ProductionWorldBinding `gorm:"foreignKey:BindingID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	AssetState   AssetState             `gorm:"foreignKey:AssetStateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldBindingState) TableName() string { return "scr_production_world_binding_states" }
func (*ProductionWorldBindingState) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}
func (*ProductionWorldBindingState) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}

type ProductionWorldBibleVersion struct {
	ID                                                                              uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	WorkspaceID                                                                     uuid.UUID                   `gorm:"type:uuid;not null"`
	ProjectID                                                                       uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:uq_scr_world_bible_revision,priority:1"`
	StructureIdentitySetVersionID                                                   uuid.UUID                   `gorm:"type:uuid;not null"`
	CandidateRevisionID                                                             uuid.UUID                   `gorm:"type:uuid;not null"`
	ReviewDecisionID                                                                uuid.UUID                   `gorm:"type:uuid;not null"`
	CreatedBy                                                                       uuid.UUID                   `gorm:"type:uuid;not null"`
	Revision                                                                        int64                       `gorm:"not null;uniqueIndex:uq_scr_world_bible_revision,priority:2;check:ck_scr_world_bible_revision,revision >= 1"`
	StructureIdentitySetRevision                                                    int64                       `gorm:"not null"`
	CandidateRevisionNo                                                             int64                       `gorm:"not null;check:ck_scr_world_bible_candidate_revision,candidate_revision_no >= 1"`
	StructureIdentitySetHash, CandidateRevisionHash, PartitionHash, BusinessKeyRoot string                      `gorm:"type:char(64);not null"`
	EvidenceRefs                                                                    datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_world_bible_evidence_refs,jsonb_typeof(evidence_refs) = 'array'"`
	SpecificationRefs                                                               datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_world_bible_spec_refs,jsonb_typeof(specification_refs) = 'array'"`
	ClaimRefs                                                                       datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_world_bible_claim_refs,jsonb_typeof(claim_refs) = 'array'"`
	BindingRefs                                                                     datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_world_bible_binding_refs,jsonb_typeof(binding_refs) = 'array'"`
	ContentHash                                                                     string                      `gorm:"type:char(64);not null;uniqueIndex"`
	CreatedAt                                                                       time.Time                   `gorm:"type:timestamptz;not null"`
	Workspace                                                                       Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                                                                         Project                     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	StructureIdentitySet                                                            StructureIdentitySetVersion `gorm:"foreignKey:StructureIdentitySetVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CandidateRevision                                                               StageCandidateRevision      `gorm:"foreignKey:CandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ReviewDecision                                                                  ReviewDecision              `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                                                                         UserAccount                 `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldBibleVersion) TableName() string { return "scr_production_world_bible_versions" }
func (*ProductionWorldBibleVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}
func (*ProductionWorldBibleVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldBibleFact
}

type ProductionWorldBibleScopeHead struct {
	ProjectID                                        uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	WorkspaceID                                      uuid.UUID                   `gorm:"type:uuid;not null"`
	CurrentVersionID                                 uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex"`
	ScopeKey                                         string                      `gorm:"type:varchar(160);not null"`
	ScopeRevision                                    int64                       `gorm:"not null;check:ck_scr_world_bible_scope_revision,scope_revision >= 1"`
	HeadRevision                                     int64                       `gorm:"not null;check:ck_scr_world_bible_head_revision,head_revision >= 1"`
	MemberCount                                      int                         `gorm:"not null;check:ck_scr_world_bible_member_count,member_count = 1"`
	VersionContentHash, ScopeContentHash             string                      `gorm:"type:char(64);not null"`
	MembersHash, CollectionRootHash, HeadContentHash string                      `gorm:"type:char(64);not null"`
	CurrentRootRefs                                  datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_world_bible_root_refs,jsonb_typeof(current_root_refs) = 'array'"`
	UpdatedAt                                        time.Time                   `gorm:"type:timestamptz;not null"`
	Workspace                                        Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                                          Project                     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentVersion                                   ProductionWorldBibleVersion `gorm:"foreignKey:CurrentVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldBibleScopeHead) TableName() string {
	return "scr_production_world_bible_scope_heads"
}
