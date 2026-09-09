package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableAssetIdentityStateMembership = errors.New("AssetIdentityStateMembership is immutable")

type AssetIdentityStateMembership struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID  `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_ast_identity_state_member_position,priority:1;uniqueIndex:uq_ast_identity_state_member_state,priority:1"`
	ScopeRevision     int64      `gorm:"not null;uniqueIndex:uq_ast_identity_state_member_position,priority:2;uniqueIndex:uq_ast_identity_state_member_state,priority:2;check:ck_ast_identity_state_member_revision,scope_revision >= 1"`
	Position          int        `gorm:"not null;uniqueIndex:uq_ast_identity_state_member_position,priority:3;check:ck_ast_identity_state_member_position,position >= 1"`
	AssetID           uuid.UUID  `gorm:"type:uuid;not null"`
	AssetStateID      uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_ast_identity_state_member_state,priority:3"`
	IdentityKey       string     `gorm:"type:varchar(100);not null"`
	StateKey          string     `gorm:"type:varchar(80);not null"`
	AssetContentHash  string     `gorm:"type:char(64);not null;check:ck_ast_identity_state_asset_hash,char_length(asset_content_hash) = 64"`
	StateContentHash  string     `gorm:"type:char(64);not null;check:ck_ast_identity_state_state_hash,char_length(state_content_hash) = 64"`
	MemberContentHash string     `gorm:"type:char(64);not null;index:ix_ast_identity_state_member_hash;check:ck_ast_identity_state_member_hash,char_length(member_content_hash) = 64"`
	CreatedAt         time.Time  `gorm:"type:timestamptz;not null"`
	Workspace         Workspace  `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project    `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Asset             Asset      `gorm:"foreignKey:AssetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	AssetState        AssetState `gorm:"foreignKey:AssetStateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AssetIdentityStateMembership) TableName() string { return "ast_asset_identity_state_memberships" }
func (*AssetIdentityStateMembership) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableAssetIdentityStateMembership
}
func (*AssetIdentityStateMembership) BeforeDelete(*gorm.DB) error {
	return ErrImmutableAssetIdentityStateMembership
}

type AssetIdentityStateScopeHead struct {
	ProjectID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID      `gorm:"type:uuid;not null;index:ix_ast_identity_state_head_workspace"`
	ScopeRevision      int64          `gorm:"not null;check:ck_ast_identity_state_scope_revision,scope_revision >= 1"`
	ScopeContentHash   string         `gorm:"type:char(64);not null;check:ck_ast_identity_state_scope_hash,char_length(scope_content_hash) = 64"`
	MemberCount        int            `gorm:"not null;check:ck_ast_identity_state_member_count,member_count >= 1"`
	MembersHash        string         `gorm:"type:char(64);not null;check:ck_ast_identity_state_members_hash,char_length(members_hash) = 64"`
	CollectionRootHash string         `gorm:"type:char(64);not null;uniqueIndex;check:ck_ast_identity_state_collection_hash,char_length(collection_root_hash) = 64"`
	CurrentRootRefs    datatypes.JSON `gorm:"type:jsonb;not null;check:ck_ast_identity_state_root_refs,jsonb_typeof(current_root_refs) = 'array'"`
	HeadRevision       int64          `gorm:"not null;check:ck_ast_identity_state_head_revision,head_revision >= 1"`
	HeadContentHash    string         `gorm:"type:char(64);not null;check:ck_ast_identity_state_head_hash,char_length(head_content_hash) = 64"`
	UpdatedAt          time.Time      `gorm:"type:timestamptz;not null"`
	Workspace          Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (AssetIdentityStateScopeHead) TableName() string { return "ast_identity_state_scope_heads" }
