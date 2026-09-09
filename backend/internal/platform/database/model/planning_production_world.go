package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var ErrImmutableProductionWorldPlanningFact = errors.New("Production World Planning fact is immutable")

type ProductionWorldPlanningScene struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID           uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_pln_scene_revision,priority:1;uniqueIndex:uq_pln_scene_content,priority:1"`
	EpisodeID           uuid.UUID      `gorm:"type:uuid;not null;index:ix_pln_scene_episode"`
	CreatedBy           uuid.UUID      `gorm:"type:uuid;not null"`
	BusinessKey         string         `gorm:"type:varchar(220);not null;uniqueIndex:uq_pln_scene_revision,priority:2;uniqueIndex:uq_pln_scene_content,priority:2"`
	SceneScopeKey       string         `gorm:"type:varchar(160);not null"`
	SceneOwnerLogicalID string         `gorm:"type:varchar(160);not null"`
	StoryTimeKey        string         `gorm:"type:varchar(160);not null"`
	Revision            int            `gorm:"not null;uniqueIndex:uq_pln_scene_revision,priority:3;check:ck_pln_scene_revision,revision >= 1"`
	Payload             datatypes.JSON `gorm:"type:jsonb;not null;check:ck_pln_scene_payload,jsonb_typeof(payload) = 'object'"`
	ContentHash         string         `gorm:"type:char(64);not null;uniqueIndex:uq_pln_scene_content,priority:3"`
	CreatedAt           time.Time      `gorm:"type:timestamptz;not null"`
	Workspace           Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project             Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode             Episode        `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningScene) TableName() string { return "pln_production_world_scenes" }
func (*ProductionWorldPlanningScene) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}
func (*ProductionWorldPlanningScene) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}

type ProductionWorldPlanningDialogue struct {
	ID          uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID   uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_pln_dialogue_revision,priority:1;uniqueIndex:uq_pln_dialogue_content,priority:1"`
	EpisodeID   uuid.UUID                    `gorm:"type:uuid;not null"`
	SceneID     uuid.UUID                    `gorm:"type:uuid;not null;index:ix_pln_dialogue_scene"`
	CreatedBy   uuid.UUID                    `gorm:"type:uuid;not null"`
	BusinessKey string                       `gorm:"type:varchar(220);not null;uniqueIndex:uq_pln_dialogue_revision,priority:2;uniqueIndex:uq_pln_dialogue_content,priority:2"`
	Revision    int                          `gorm:"not null;uniqueIndex:uq_pln_dialogue_revision,priority:3;check:ck_pln_dialogue_revision,revision >= 1"`
	SequenceKey int                          `gorm:"not null;check:ck_pln_dialogue_sequence,sequence_key >= 1"`
	Payload     datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_pln_dialogue_payload,jsonb_typeof(payload) = 'object'"`
	ContentHash string                       `gorm:"type:char(64);not null;uniqueIndex:uq_pln_dialogue_content,priority:3"`
	CreatedAt   time.Time                    `gorm:"type:timestamptz;not null"`
	Scene       ProductionWorldPlanningScene `gorm:"foreignKey:SceneID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningDialogue) TableName() string { return "pln_production_world_dialogues" }
func (*ProductionWorldPlanningDialogue) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}
func (*ProductionWorldPlanningDialogue) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}

type ProductionWorldPlanningBeat struct {
	ID          uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID   uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_pln_beat_revision,priority:1;uniqueIndex:uq_pln_beat_content,priority:1"`
	EpisodeID   uuid.UUID                    `gorm:"type:uuid;not null"`
	SceneID     uuid.UUID                    `gorm:"type:uuid;not null;index:ix_pln_beat_scene"`
	CreatedBy   uuid.UUID                    `gorm:"type:uuid;not null"`
	BusinessKey string                       `gorm:"type:varchar(220);not null;uniqueIndex:uq_pln_beat_revision,priority:2;uniqueIndex:uq_pln_beat_content,priority:2"`
	Revision    int                          `gorm:"not null;uniqueIndex:uq_pln_beat_revision,priority:3;check:ck_pln_beat_revision,revision >= 1"`
	SequenceKey int                          `gorm:"not null;check:ck_pln_beat_sequence,sequence_key >= 1"`
	Payload     datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_pln_beat_payload,jsonb_typeof(payload) = 'object'"`
	ContentHash string                       `gorm:"type:char(64);not null;uniqueIndex:uq_pln_beat_content,priority:3"`
	CreatedAt   time.Time                    `gorm:"type:timestamptz;not null"`
	Scene       ProductionWorldPlanningScene `gorm:"foreignKey:SceneID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningBeat) TableName() string { return "pln_production_world_beats" }
func (*ProductionWorldPlanningBeat) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}
func (*ProductionWorldPlanningBeat) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}

type ProductionWorldPlanningOccurrence struct {
	ID                                   uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID                          uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID                            uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_pln_occurrence_revision,priority:1;uniqueIndex:uq_pln_occurrence_content,priority:1"`
	EpisodeID                            uuid.UUID                    `gorm:"type:uuid;not null"`
	SceneID, AssetID, AssetStateID       uuid.UUID                    `gorm:"type:uuid;not null"`
	SpecificationID, ProductionBindingID uuid.UUID                    `gorm:"type:uuid;not null"`
	CreatedBy                            uuid.UUID                    `gorm:"type:uuid;not null"`
	BusinessKey                          string                       `gorm:"type:varchar(220);not null;uniqueIndex:uq_pln_occurrence_revision,priority:2;uniqueIndex:uq_pln_occurrence_content,priority:2"`
	Revision                             int                          `gorm:"not null;uniqueIndex:uq_pln_occurrence_revision,priority:3;check:ck_pln_occurrence_revision,revision >= 1"`
	SequenceKey                          int                          `gorm:"not null;check:ck_pln_occurrence_sequence,sequence_key >= 1"`
	AssetContentHash, StateContentHash   string                       `gorm:"type:char(64);not null"`
	SpecificationHash, BindingHash       string                       `gorm:"type:char(64);not null"`
	Payload                              datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_pln_occurrence_payload,jsonb_typeof(payload) = 'object'"`
	ContentHash                          string                       `gorm:"type:char(64);not null;uniqueIndex:uq_pln_occurrence_content,priority:3"`
	CreatedAt                            time.Time                    `gorm:"type:timestamptz;not null"`
	Scene                                ProductionWorldPlanningScene `gorm:"foreignKey:SceneID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Asset                                Asset                        `gorm:"foreignKey:AssetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	AssetState                           AssetState                   `gorm:"foreignKey:AssetStateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Specification                        ProductionWorldSpecification `gorm:"foreignKey:SpecificationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ProductionBinding                    ProductionWorldBinding       `gorm:"foreignKey:ProductionBindingID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningOccurrence) TableName() string {
	return "pln_production_world_occurrences"
}
func (*ProductionWorldPlanningOccurrence) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}
func (*ProductionWorldPlanningOccurrence) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}

type ProductionWorldPlanningClaim struct {
	ID                           uuid.UUID                    `gorm:"type:uuid;primaryKey"`
	WorkspaceID                  uuid.UUID                    `gorm:"type:uuid;not null"`
	ProjectID                    uuid.UUID                    `gorm:"type:uuid;not null;uniqueIndex:uq_pln_claim_revision,priority:1;uniqueIndex:uq_pln_claim_content,priority:1"`
	EpisodeID                    uuid.UUID                    `gorm:"type:uuid;not null"`
	SourceSceneID, TargetSceneID uuid.UUID                    `gorm:"type:uuid;not null"`
	CreatedBy                    uuid.UUID                    `gorm:"type:uuid;not null"`
	BusinessKey                  string                       `gorm:"type:varchar(220);not null;uniqueIndex:uq_pln_claim_revision,priority:2;uniqueIndex:uq_pln_claim_content,priority:2"`
	ClaimType                    string                       `gorm:"type:varchar(20);not null;check:ck_pln_claim_type,claim_type IN ('interaction','continuity')"`
	Revision                     int                          `gorm:"not null;uniqueIndex:uq_pln_claim_revision,priority:3;check:ck_pln_claim_revision,revision >= 1"`
	StoryTimeKey                 string                       `gorm:"type:varchar(160);not null"`
	Payload                      datatypes.JSON               `gorm:"type:jsonb;not null;check:ck_pln_claim_payload,jsonb_typeof(payload) = 'object'"`
	ContentHash                  string                       `gorm:"type:char(64);not null;uniqueIndex:uq_pln_claim_content,priority:3"`
	CreatedAt                    time.Time                    `gorm:"type:timestamptz;not null"`
	SourceScene                  ProductionWorldPlanningScene `gorm:"foreignKey:SourceSceneID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	TargetScene                  ProductionWorldPlanningScene `gorm:"foreignKey:TargetSceneID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningClaim) TableName() string { return "pln_production_world_claims" }
func (*ProductionWorldPlanningClaim) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}
func (*ProductionWorldPlanningClaim) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}

type ProductionWorldPlanningMembership struct {
	ID                          uuid.UUID `gorm:"type:uuid;primaryKey"`
	WorkspaceID                 uuid.UUID `gorm:"type:uuid;not null"`
	ProjectID                   uuid.UUID `gorm:"type:uuid;not null"`
	EpisodeID                   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_pln_member_position,priority:1;uniqueIndex:uq_pln_member_fact,priority:1"`
	ScopeRevision               int64     `gorm:"not null;uniqueIndex:uq_pln_member_position,priority:2;uniqueIndex:uq_pln_member_fact,priority:2"`
	Position                    int       `gorm:"not null;uniqueIndex:uq_pln_member_position,priority:3"`
	FactID                      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_pln_member_fact,priority:4"`
	FactKind, BusinessKey       string    `gorm:"type:varchar(220);not null"`
	FactRevision                int       `gorm:"not null"`
	FactContentHash, MemberHash string    `gorm:"type:char(64);not null"`
	CreatedAt                   time.Time `gorm:"type:timestamptz;not null"`
}

func (ProductionWorldPlanningMembership) TableName() string {
	return "pln_production_world_memberships"
}
func (*ProductionWorldPlanningMembership) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}
func (*ProductionWorldPlanningMembership) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProductionWorldPlanningFact
}

type ProductionWorldPlanningEpisodeHead struct {
	EpisodeID                                        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID, ProjectID                           uuid.UUID      `gorm:"type:uuid;not null"`
	ScopeRevision                                    int64          `gorm:"not null;check:ck_pln_head_scope_revision,scope_revision >= 1"`
	HeadRevision                                     int64          `gorm:"not null;check:ck_pln_head_revision,head_revision >= 1"`
	MemberCount                                      int            `gorm:"not null;check:ck_pln_head_members,member_count >= 1"`
	MembersHash, CollectionRootHash, HeadContentHash string         `gorm:"type:char(64);not null"`
	CurrentRootRefs                                  datatypes.JSON `gorm:"type:jsonb;not null;check:ck_pln_head_refs,jsonb_typeof(current_root_refs) = 'array'"`
	UpdatedAt                                        time.Time      `gorm:"type:timestamptz;not null"`
	Episode                                          Episode        `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProductionWorldPlanningEpisodeHead) TableName() string {
	return "pln_production_world_episode_heads"
}
