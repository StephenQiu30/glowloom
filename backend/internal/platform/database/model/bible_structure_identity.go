package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrImmutableStructureIdentitySetVersion = errors.New("StructureIdentitySetVersion is immutable")
	ErrImmutableStructureIdentityReceipt    = errors.New("StructureIdentityCollectionReceipt is immutable")
)

// StructureIdentitySetVersion is the narrow Gate 1 production snapshot. It
// stores accepted structure and identity decisions, never source or Candidate payloads.
type StructureIdentitySetVersion struct {
	ID                      uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID             uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID               uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_scr_structure_identity_project_version,priority:1"`
	Version                 int                          `gorm:"not null;uniqueIndex:uq_scr_structure_identity_project_version,priority:2;check:ck_scr_structure_identity_version,version >= 1"`
	ParentVersionID         *uuid.UUID                   `gorm:"type:uuid"`
	GateInputID             uuid.UUID                    `gorm:"type:uuid;not null"`
	GateInputHash           string                       `gorm:"type:char(64);not null;check:ck_scr_structure_identity_gate_hash,char_length(gate_input_hash) = 64"`
	ReviewDecisionID        uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectEpisodeReceiptID uuid.UUID                    `gorm:"type:uuid;not null"`
	DocumentRevisionID      uuid.UUID                    `gorm:"type:uuid;not null"`
	SpanIndexID             uuid.UUID                    `gorm:"type:uuid;not null"`
	CandidateRefs           datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_candidates,jsonb_typeof(candidate_refs) = 'array'"`
	EpisodeRefs             datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_episodes,jsonb_typeof(episode_refs) = 'array'"`
	SceneRefs               datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_scenes,jsonb_typeof(scene_refs) = 'array'"`
	Identities              datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_identities,jsonb_typeof(identities) = 'array'"`
	MentionMappings         datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_mentions,jsonb_typeof(mention_mappings) = 'array'"`
	Coverage                datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_coverage,jsonb_typeof(coverage) = 'object'"`
	ContentHash             string                       `gorm:"type:char(64);not null;uniqueIndex;check:ck_scr_structure_identity_hash,char_length(content_hash) = 64"`
	CreatedBy               uuid.UUID                    `gorm:"type:uuid;not null"`
	CreatedAt               time.Time                    `gorm:"type:timestamptz;not null"`
	Workspace               Workspace                    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                 Project                      `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ParentVersion           *StructureIdentitySetVersion `gorm:"foreignKey:ParentVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DocumentRevision        DocumentRevision             `gorm:"foreignKey:DocumentRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SpanIndex               SourceSpanIndexVersion       `gorm:"foreignKey:SpanIndexID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                 UserAccount                  `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StructureIdentitySetVersion) TableName() string { return "scr_structure_identity_set_versions" }
func (*StructureIdentitySetVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableStructureIdentitySetVersion
}
func (*StructureIdentitySetVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableStructureIdentitySetVersion
}

type StructureIdentityScopeHead struct {
	ProjectID          uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID                   `gorm:"type:uuid;not null"`
	ScopeKey           string                      `gorm:"type:varchar(160);not null"`
	ScopeRevision      int64                       `gorm:"not null;check:ck_scr_structure_identity_head_scope_revision,scope_revision >= 1"`
	ScopeContentHash   string                      `gorm:"type:char(64);not null"`
	MemberCount        int64                       `gorm:"not null;check:ck_scr_structure_identity_head_member_count,member_count = 1"`
	MembersHash        string                      `gorm:"type:char(64);not null"`
	CollectionRootHash string                      `gorm:"type:char(64);not null"`
	CurrentVersionRefs datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_head_refs,jsonb_typeof(current_version_refs) = 'array'"`
	CurrentVersionID   uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex"`
	HeadRevision       int64                       `gorm:"not null;check:ck_scr_structure_identity_head_revision,head_revision >= 1"`
	HeadContentHash    string                      `gorm:"column:head_hash;type:char(64);not null;check:ck_scr_structure_identity_head_hash,char_length(head_hash) = 64"`
	UpdatedAt          time.Time                   `gorm:"type:timestamptz;not null"`
	Workspace          Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project                     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentVersion     StructureIdentitySetVersion `gorm:"foreignKey:CurrentVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StructureIdentityScopeHead) TableName() string { return "scr_structure_identity_scope_heads" }

type StructureIdentityCollectionReceipt struct {
	ID                 uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	CommandID          uuid.UUID                   `gorm:"type:uuid;not null"`
	IdempotencyKey     string                      `gorm:"type:varchar(200);not null"`
	WorkspaceID        uuid.UUID                   `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:uq_scr_structure_identity_receipt,priority:1"`
	VersionID          uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:uq_scr_structure_identity_receipt,priority:2"`
	ReviewDecisionID   uuid.UUID                   `gorm:"type:uuid;not null"`
	CheckpointKey      string                      `gorm:"type:varchar(80);not null;check:ck_scr_structure_identity_checkpoint,checkpoint_key = 'gate_1_structure_identity'"`
	OwnerKind          string                      `gorm:"type:varchar(80);not null;check:ck_scr_structure_identity_owner,owner_kind = 'production/bible'"`
	CollectionFamily   string                      `gorm:"type:varchar(80);not null;check:ck_scr_structure_identity_family,collection_family = 'bible_structure_identity_set'"`
	ScopeKind          string                      `gorm:"type:varchar(40);not null;check:ck_scr_structure_identity_scope_kind,scope_kind = 'project'"`
	ScopeKey           string                      `gorm:"type:varchar(160);not null"`
	ScopeRevision      int64                       `gorm:"not null;check:ck_scr_structure_identity_scope_revision,scope_revision >= 1"`
	ScopeContentHash   string                      `gorm:"type:char(64);not null"`
	CoveredScopeKeys   datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_scopes,jsonb_typeof(covered_scope_keys) = 'array'"`
	Members            datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_members,jsonb_typeof(members) = 'array'"`
	MemberCount        int64                       `gorm:"not null;check:ck_scr_structure_identity_member_count,member_count = 1"`
	MembersHash        string                      `gorm:"type:char(64);not null"`
	CollectionRootHash string                      `gorm:"type:char(64);not null;uniqueIndex;check:ck_scr_structure_identity_root_hash,char_length(collection_root_hash) = 64"`
	CommittedOwnerRefs datatypes.JSON              `gorm:"type:jsonb;not null;check:ck_scr_structure_identity_committed_refs,jsonb_typeof(committed_owner_refs) = 'array'"`
	ReceiptContentHash string                      `gorm:"type:char(64);not null;uniqueIndex;check:ck_scr_structure_identity_receipt_hash,char_length(receipt_content_hash) = 64"`
	CommittedBy        uuid.UUID                   `gorm:"column:created_by;type:uuid;not null"`
	CommittedAt        time.Time                   `gorm:"column:created_at;type:timestamptz;not null"`
	Workspace          Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project                     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Version            StructureIdentitySetVersion `gorm:"foreignKey:VersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Committer          UserAccount                 `gorm:"foreignKey:CommittedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StructureIdentityCollectionReceipt) TableName() string {
	return "scr_structure_identity_collection_receipts"
}
func (*StructureIdentityCollectionReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableStructureIdentityReceipt
}
func (*StructureIdentityCollectionReceipt) BeforeDelete(*gorm.DB) error {
	return ErrImmutableStructureIdentityReceipt
}
