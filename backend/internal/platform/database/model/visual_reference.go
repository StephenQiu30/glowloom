package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableVisualReferenceFact = errors.New("Visual Foundation and Reference Plan facts are immutable")

type ProjectPresetBindingVersion struct {
	ID                  uuid.UUID                      `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID                      `gorm:"type:uuid;not null"`
	ProjectID           uuid.UUID                      `gorm:"type:uuid;not null;uniqueIndex:uq_pre_binding_revision,priority:1"`
	Revision            int64                          `gorm:"not null;uniqueIndex:uq_pre_binding_revision,priority:2;check:ck_pre_binding_revision,revision >= 1"`
	SelectionID         uuid.UUID                      `gorm:"type:uuid;not null"`
	CandidateRevisionID uuid.UUID                      `gorm:"type:uuid;not null"`
	ReviewDecisionID    uuid.UUID                      `gorm:"type:uuid;not null"`
	Content             datatypes.JSON                 `gorm:"type:jsonb;not null;check:ck_pre_binding_content,jsonb_typeof(content) = 'object'"`
	ContentHash         string                         `gorm:"type:char(64);not null;unique;check:ck_pre_binding_hash,char_length(content_hash) = 64"`
	CreatedBy           uuid.UUID                      `gorm:"type:uuid;not null"`
	CreatedAt           time.Time                      `gorm:"type:timestamptz;not null"`
	Workspace           Workspace                      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project             Project                        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Selection           ProjectPresetSelection         `gorm:"foreignKey:SelectionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Candidate           SceneAnalysisCandidateRevision `gorm:"foreignKey:CandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Decision            ReviewDecision                 `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount                    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectPresetBindingVersion) TableName() string { return "pre_project_preset_binding_versions" }
func (*ProjectPresetBindingVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}
func (*ProjectPresetBindingVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}

type EffectiveStyleSnapshot struct {
	ID                  uuid.UUID                      `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID                      `gorm:"type:uuid;not null"`
	ProjectID           uuid.UUID                      `gorm:"type:uuid;not null;index:ix_pre_style_project_revision,priority:1"`
	Revision            int64                          `gorm:"not null;index:ix_pre_style_project_revision,priority:2,sort:desc;check:ck_pre_style_revision,revision >= 1"`
	CandidateRevisionID uuid.UUID                      `gorm:"type:uuid;not null"`
	ReviewDecisionID    uuid.UUID                      `gorm:"type:uuid;not null"`
	Content             datatypes.JSON                 `gorm:"type:jsonb;not null;check:ck_pre_style_content,jsonb_typeof(content) = 'object'"`
	ContentHash         string                         `gorm:"type:char(64);not null;unique;check:ck_pre_style_hash,char_length(content_hash) = 64"`
	CreatedBy           uuid.UUID                      `gorm:"type:uuid;not null"`
	CreatedAt           time.Time                      `gorm:"type:timestamptz;not null"`
	Workspace           Workspace                      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project             Project                        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Candidate           SceneAnalysisCandidateRevision `gorm:"foreignKey:CandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Decision            ReviewDecision                 `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount                    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (EffectiveStyleSnapshot) TableName() string            { return "pre_effective_style_snapshots" }
func (*EffectiveStyleSnapshot) BeforeUpdate(*gorm.DB) error { return ErrImmutableVisualReferenceFact }
func (*EffectiveStyleSnapshot) BeforeDelete(*gorm.DB) error { return ErrImmutableVisualReferenceFact }

type EffectivePolicySnapshot struct {
	ID                       uuid.UUID              `gorm:"type:uuid;primaryKey"`
	WorkspaceID              uuid.UUID              `gorm:"type:uuid;not null"`
	ProjectID                uuid.UUID              `gorm:"type:uuid;not null;index:ix_pre_policy_project_revision,priority:1"`
	Revision                 int64                  `gorm:"not null;index:ix_pre_policy_project_revision,priority:2,sort:desc;check:ck_pre_policy_revision,revision >= 1"`
	EffectiveStyleSnapshotID uuid.UUID              `gorm:"type:uuid;not null"`
	ReviewDecisionID         uuid.UUID              `gorm:"type:uuid;not null"`
	Content                  datatypes.JSON         `gorm:"type:jsonb;not null;check:ck_pre_policy_content,jsonb_typeof(content) = 'object'"`
	ContentHash              string                 `gorm:"type:char(64);not null;unique;check:ck_pre_policy_hash,char_length(content_hash) = 64"`
	CreatedBy                uuid.UUID              `gorm:"type:uuid;not null"`
	CreatedAt                time.Time              `gorm:"type:timestamptz;not null"`
	Workspace                Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                  Project                `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Style                    EffectiveStyleSnapshot `gorm:"foreignKey:EffectiveStyleSnapshotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Decision                 ReviewDecision         `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                  UserAccount            `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (EffectivePolicySnapshot) TableName() string            { return "pre_effective_policy_snapshots" }
func (*EffectivePolicySnapshot) BeforeUpdate(*gorm.DB) error { return ErrImmutableVisualReferenceFact }
func (*EffectivePolicySnapshot) BeforeDelete(*gorm.DB) error { return ErrImmutableVisualReferenceFact }

type PresetEffectiveScopeHead struct {
	ProjectID          uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID                   `gorm:"type:uuid;not null"`
	HeadRevision       int64                       `gorm:"not null;check:ck_pre_effective_head_revision,head_revision >= 1"`
	BindingVersionID   uuid.UUID                   `gorm:"type:uuid;not null;unique"`
	StyleSnapshotID    uuid.UUID                   `gorm:"type:uuid;not null;unique"`
	PolicySnapshotID   uuid.UUID                   `gorm:"type:uuid;not null;unique"`
	CollectionRootHash string                      `gorm:"type:char(64);not null;check:ck_pre_effective_head_root,char_length(collection_root_hash) = 64"`
	HeadContentHash    string                      `gorm:"type:char(64);not null;check:ck_pre_effective_head_hash,char_length(head_content_hash) = 64"`
	UpdatedAt          time.Time                   `gorm:"type:timestamptz;not null"`
	Workspace          Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project                     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Binding            ProjectPresetBindingVersion `gorm:"foreignKey:BindingVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Style              EffectiveStyleSnapshot      `gorm:"foreignKey:StyleSnapshotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Policy             EffectivePolicySnapshot     `gorm:"foreignKey:PolicySnapshotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (PresetEffectiveScopeHead) TableName() string { return "pre_effective_scope_heads" }

type ApprovedReferencePlanVersion struct {
	ID                          uuid.UUID                      `gorm:"type:uuid;primaryKey"`
	WorkspaceID                 uuid.UUID                      `gorm:"type:uuid;not null"`
	ProjectID                   uuid.UUID                      `gorm:"type:uuid;not null;index:ix_ref_plan_project_revision,priority:1"`
	LogicalID                   uuid.UUID                      `gorm:"type:uuid;not null;uniqueIndex:uq_ref_plan_logical_revision,priority:1"`
	Revision                    int64                          `gorm:"not null;uniqueIndex:uq_ref_plan_logical_revision,priority:2;index:ix_ref_plan_project_revision,priority:2,sort:desc;check:ck_ref_plan_revision,revision >= 1"`
	CandidateRevisionID         uuid.UUID                      `gorm:"type:uuid;not null"`
	ProductionWorldOwnerSetHash string                         `gorm:"type:char(64);not null;check:ck_ref_plan_world_hash,char_length(production_world_owner_set_hash) = 64"`
	ExpectedTargetKeyRoot       string                         `gorm:"type:char(64);not null;check:ck_ref_plan_target_root,char_length(expected_target_key_root) = 64"`
	ReviewDecisionID            uuid.UUID                      `gorm:"type:uuid;not null"`
	Content                     datatypes.JSON                 `gorm:"type:jsonb;not null;check:ck_ref_plan_content,jsonb_typeof(content) = 'object'"`
	ContentHash                 string                         `gorm:"type:char(64);not null;unique;check:ck_ref_plan_hash,char_length(content_hash) = 64"`
	CreatedBy                   uuid.UUID                      `gorm:"type:uuid;not null"`
	CreatedAt                   time.Time                      `gorm:"type:timestamptz;not null"`
	Workspace                   Workspace                      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                     Project                        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Candidate                   SceneAnalysisCandidateRevision `gorm:"foreignKey:CandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Decision                    ReviewDecision                 `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                     UserAccount                    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ApprovedReferencePlanVersion) TableName() string { return "ref_approved_plan_versions" }
func (*ApprovedReferencePlanVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}
func (*ApprovedReferencePlanVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}

type ReferencePlanTargetVersion struct {
	ID                uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID         uuid.UUID                    `gorm:"type:uuid;not null"`
	PlanVersionID     uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_ref_target_business_key,priority:1"`
	Revision          int64                        `gorm:"not null;check:ck_ref_target_revision,revision >= 1"`
	TargetBusinessKey string                       `gorm:"type:text;not null;uniqueIndex:uq_ref_target_business_key,priority:2"`
	TargetKind        string                       `gorm:"type:varchar(40);not null;check:ck_ref_target_kind,target_kind IN ('character_appearance','character_identity_anchor','interaction_composition','location_board','prop_sheet','scene_composition')"`
	Fulfillment       string                       `gorm:"type:varchar(20);not null;check:ck_ref_target_fulfillment,fulfillment IN ('not_generated','optional','required')"`
	Content           datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_ref_target_content,jsonb_typeof(content) = 'object'"`
	ContentHash       string                       `gorm:"type:char(64);not null;unique;check:ck_ref_target_hash,char_length(content_hash) = 64"`
	CreatedBy         uuid.UUID                    `gorm:"type:uuid;not null"`
	CreatedAt         time.Time                    `gorm:"type:timestamptz;not null"`
	Workspace         Workspace                    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project           Project                      `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Plan              ApprovedReferencePlanVersion `gorm:"foreignKey:PlanVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator           UserAccount                  `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ReferencePlanTargetVersion) TableName() string { return "ref_plan_target_versions" }
func (*ReferencePlanTargetVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}
func (*ReferencePlanTargetVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}

type ReferencePlanScopeHead struct {
	PlanLogicalID          uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID            uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID              uuid.UUID                    `gorm:"type:uuid;not null"`
	HeadRevision           int64                        `gorm:"not null;check:ck_ref_scope_head_revision,head_revision >= 1"`
	CurrentPlanVersionID   uuid.UUID                    `gorm:"type:uuid;not null;unique"`
	CurrentPlanContentHash string                       `gorm:"type:char(64);not null;check:ck_ref_scope_head_plan_hash,char_length(current_plan_content_hash) = 64"`
	CollectionRootHash     string                       `gorm:"type:char(64);not null;check:ck_ref_scope_head_root,char_length(collection_root_hash) = 64"`
	HeadContentHash        string                       `gorm:"type:char(64);not null;check:ck_ref_scope_head_hash,char_length(head_content_hash) = 64"`
	UpdatedAt              time.Time                    `gorm:"type:timestamptz;not null"`
	Workspace              Workspace                    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                Project                      `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentPlan            ApprovedReferencePlanVersion `gorm:"foreignKey:CurrentPlanVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ReferencePlanScopeHead) TableName() string { return "ref_plan_scope_heads" }

type ProjectReferencePlanActivationHead struct {
	ProjectID              uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID            uuid.UUID                    `gorm:"type:uuid;not null"`
	HeadRevision           int64                        `gorm:"not null;check:ck_ref_activation_head_revision,head_revision >= 1"`
	CurrentPlanLogicalID   uuid.UUID                    `gorm:"type:uuid;not null"`
	CurrentPlanVersionID   uuid.UUID                    `gorm:"type:uuid;not null;unique"`
	CurrentPlanContentHash string                       `gorm:"type:char(64);not null;check:ck_ref_activation_plan_hash,char_length(current_plan_content_hash) = 64"`
	HeadContentHash        string                       `gorm:"type:char(64);not null;check:ck_ref_activation_head_hash,char_length(head_content_hash) = 64"`
	UpdatedAt              time.Time                    `gorm:"type:timestamptz;not null"`
	Workspace              Workspace                    `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                Project                      `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CurrentPlan            ApprovedReferencePlanVersion `gorm:"foreignKey:CurrentPlanVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectReferencePlanActivationHead) TableName() string { return "ref_project_activation_heads" }

type VisualFoundationScopeCollectionReceipt struct {
	ID                 uuid.UUID              `gorm:"type:uuid;primaryKey"`
	CommandID          uuid.UUID              `gorm:"type:uuid;not null"`
	WorkspaceID        uuid.UUID              `gorm:"type:uuid;not null"`
	ProjectID          uuid.UUID              `gorm:"type:uuid;not null"`
	GateInputID        uuid.UUID              `gorm:"type:uuid;not null"`
	ReviewDecisionID   uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_vfs_collection_receipt,priority:1"`
	OwnerKind          string                 `gorm:"type:varchar(80);not null;uniqueIndex:uq_vfs_collection_receipt,priority:2"`
	VersionFamily      string                 `gorm:"type:varchar(80);not null;uniqueIndex:uq_vfs_collection_receipt,priority:3"`
	Collection         datatypes.JSON         `gorm:"type:jsonb;not null;check:ck_vfs_collection_receipt,jsonb_typeof(collection) = 'object'"`
	CollectionRootHash string                 `gorm:"type:char(64);not null;check:ck_vfs_collection_root,char_length(collection_root_hash) = 64"`
	ReceiptContentHash string                 `gorm:"type:char(64);not null;unique;check:ck_vfs_receipt_hash,char_length(receipt_content_hash) = 64"`
	CommittedBy        uuid.UUID              `gorm:"type:uuid;not null"`
	CommittedAt        time.Time              `gorm:"type:timestamptz;not null"`
	Workspace          Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project            Project                `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	GateInput          WorkflowHumanGateInput `gorm:"foreignKey:GateInputID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Decision           ReviewDecision         `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Committer          UserAccount            `gorm:"foreignKey:CommittedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (VisualFoundationScopeCollectionReceipt) TableName() string {
	return "wrk_visual_foundation_scope_collection_receipts"
}
func (*VisualFoundationScopeCollectionReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}
func (*VisualFoundationScopeCollectionReceipt) BeforeDelete(*gorm.DB) error {
	return ErrImmutableVisualReferenceFact
}
