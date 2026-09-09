package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableProductionWorldConfirmation = errors.New("Production World confirmation fact is immutable")

type ProductionWorldCommandDedup struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkspaceID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_prd_world_command_dedup,priority:1"`
	CommandContract string    `gorm:"type:varchar(80);not null;uniqueIndex:uq_prd_world_command_dedup,priority:2"`
	IdempotencyKey  string    `gorm:"type:varchar(200);not null;uniqueIndex:uq_prd_world_command_dedup,priority:3"`
	CommandID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	InputHash       string    `gorm:"type:char(64);not null;check:ck_prd_world_command_dedup_hash,char_length(input_hash) = 64"`
	CreatedAt       time.Time `gorm:"type:timestamptz;not null"`
	Workspace       Workspace `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldCommandDedup) TableName() string { return "prd_world_command_dedup" }
func (*ProductionWorldCommandDedup) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldConfirmation
}
func (*ProductionWorldCommandDedup) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldConfirmation
}

type ProductionWorldCollectionReceipt struct {
	ID                        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	CommandID                 uuid.UUID      `gorm:"type:uuid;not null"`
	IdempotencyKey            string         `gorm:"type:varchar(200);not null"`
	WorkspaceID               uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID                 uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_prd_world_collection_receipt,priority:2"`
	DecisionCheckpointID      uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_prd_world_collection_receipt,priority:1"`
	OwnerKind                 string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_prd_world_collection_receipt,priority:3"`
	VersionFamily             string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_prd_world_collection_receipt,priority:4"`
	ScopeKind                 string         `gorm:"type:varchar(40);not null;uniqueIndex:uq_prd_world_collection_receipt,priority:5"`
	ScopeKey                  string         `gorm:"type:varchar(160);not null;uniqueIndex:uq_prd_world_collection_receipt,priority:6"`
	ScopeRevision             int64          `gorm:"not null;check:ck_prd_world_collection_scope_revision,scope_revision >= 1"`
	ScopeContentHash          string         `gorm:"type:char(64);not null"`
	Members                   datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prd_world_collection_members,jsonb_typeof(members) = 'array'"`
	MemberCount               int            `gorm:"not null;check:ck_prd_world_collection_count,member_count >= 1"`
	MembersHash               string         `gorm:"type:char(64);not null"`
	CollectionRootHash        string         `gorm:"type:char(64);not null;uniqueIndex:uq_prd_world_collection_receipt,priority:7"`
	CoveredScopeKeys          datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prd_world_collection_scopes,jsonb_typeof(covered_scope_keys) = 'array'"`
	CommittedOwnerVersionRefs datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prd_world_collection_refs,jsonb_typeof(committed_owner_version_refs) = 'array'"`
	ReviewDecisionID          uuid.UUID      `gorm:"type:uuid;not null"`
	ReviewDecisionRevision    int64          `gorm:"not null;check:ck_prd_world_collection_decision_revision,review_decision_revision >= 1"`
	ReviewDecisionContentHash string         `gorm:"type:char(64);not null"`
	ReceiptContentHash        string         `gorm:"type:char(64);not null;uniqueIndex"`
	CommittedAt               time.Time      `gorm:"type:timestamptz;not null"`
	CommittedBy               uuid.UUID      `gorm:"type:uuid;not null"`
	Workspace                 Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                   Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Reviewer                  UserAccount    `gorm:"foreignKey:CommittedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldCollectionReceipt) TableName() string {
	return "prd_world_collection_receipts"
}
func (*ProductionWorldCollectionReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldConfirmation
}
func (*ProductionWorldCollectionReceipt) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldConfirmation
}

type ProductionWorldPlanningRebaseHead struct {
	ProjectID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID      `gorm:"type:uuid;not null"`
	ScopeKey           string         `gorm:"type:varchar(160);not null"`
	ScopeRevision      int64          `gorm:"not null;check:ck_prd_world_rebase_scope_revision,scope_revision >= 1"`
	ScopeContentHash   string         `gorm:"type:char(64);not null"`
	MemberCount        int            `gorm:"not null;check:ck_prd_world_rebase_member_count,member_count = 0"`
	MembersHash        string         `gorm:"type:char(64);not null"`
	CollectionRootHash string         `gorm:"type:char(64);not null"`
	CurrentRootRefs    datatypes.JSON `gorm:"type:jsonb;not null;check:ck_prd_world_rebase_refs,jsonb_typeof(current_root_refs) = 'array'"`
	HeadRevision       int64          `gorm:"not null;check:ck_prd_world_rebase_head_revision,head_revision >= 1"`
	HeadContentHash    string         `gorm:"type:char(64);not null"`
	UpdatedAt          time.Time      `gorm:"type:timestamptz;not null"`
	Workspace          Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningRebaseHead) TableName() string {
	return "pln_production_world_rebase_heads"
}
