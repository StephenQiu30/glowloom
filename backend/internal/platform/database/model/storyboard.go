package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type StoryboardDraftSet struct {
	ID                    uuid.UUID               `gorm:"type:uuid;primaryKey"`
	WorkspaceID           uuid.UUID               `gorm:"type:uuid;not null"`
	ProjectID             uuid.UUID               `gorm:"type:uuid;not null"`
	WorkflowRunID         uuid.UUID               `gorm:"type:uuid;not null;index"`
	NodeRunID             uuid.UUID               `gorm:"type:uuid;not null;uniqueIndex"`
	GraphVersionID        uuid.UUID               `gorm:"type:uuid;not null;index"`
	GraphVersionNo        int64                   `gorm:"not null;check:ck_stb_draft_set_graph_version,graph_version_no >= 1"`
	GraphContentHash      string                  `gorm:"type:char(64);not null;check:ck_stb_draft_set_graph_hash,char_length(graph_content_hash) = 64"`
	ManifestID            uuid.UUID               `gorm:"type:uuid;not null"`
	ManifestVersion       int64                   `gorm:"not null;check:ck_stb_draft_set_manifest_version,manifest_version >= 1"`
	ManifestHash          string                  `gorm:"type:char(64);not null;check:ck_stb_draft_set_manifest_hash,char_length(manifest_hash) = 64"`
	Status                string                  `gorm:"type:varchar(20);not null;check:ck_stb_draft_set_status,status IN ('queued','needs_asset','intent_frozen','failed','unknown','cancelled')"`
	InputHash             string                  `gorm:"type:char(64);not null;check:ck_stb_draft_set_input_hash,char_length(input_hash) = 64"`
	ResultHash            *string                 `gorm:"type:char(64);check:ck_stb_draft_set_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	CandidateRevisionID   *uuid.UUID              `gorm:"type:uuid"`
	CandidateRevisionHash *string                 `gorm:"type:char(64);check:ck_stb_draft_set_candidate_hash,candidate_revision_hash IS NULL OR char_length(candidate_revision_hash) = 64"`
	Batches               datatypes.JSON          `gorm:"type:jsonb;not null"`
	Revision              int                     `gorm:"not null;check:ck_stb_draft_set_revision,revision >= 1"`
	CreatedBy             uuid.UUID               `gorm:"type:uuid;not null"`
	CreatedAt             time.Time               `gorm:"type:timestamptz;not null"`
	UpdatedAt             time.Time               `gorm:"type:timestamptz;not null"`
	Workspace             Workspace               `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project               Project                 `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun           WorkflowRun             `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun               NodeRunProjection       `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	GraphVersion          StoryGraphVersion       `gorm:"foreignKey:GraphVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Manifest              ShardManifest           `gorm:"foreignKey:ManifestID,ManifestVersion;references:ID,Version;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CandidateRevision     *StageCandidateRevision `gorm:"foreignKey:CandidateRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator               UserAccount             `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardDraftSet) TableName() string { return "stb_draft_sets" }

type StoryboardDraftBatch struct {
	ID                    uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID           uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID             uuid.UUID            `gorm:"type:uuid;not null"`
	EpisodeID             uuid.UUID            `gorm:"type:uuid;not null;index:ix_stb_batches_episode_created,priority:1"`
	StructureID           uuid.UUID            `gorm:"type:uuid;not null"`
	ScriptVersionID       uuid.UUID            `gorm:"type:uuid;not null"`
	WorkflowRunID         uuid.UUID            `gorm:"type:uuid;not null;index"`
	NodeRunID             uuid.UUID            `gorm:"type:uuid;not null;index;uniqueIndex:uq_stb_batch_node_scene,priority:1"`
	ManifestID            uuid.UUID            `gorm:"type:uuid;not null"`
	ManifestVersion       int64                `gorm:"not null;check:ck_stb_batch_manifest_version,manifest_version >= 1"`
	GraphVersionID        uuid.UUID            `gorm:"type:uuid;not null"`
	GraphVersionNo        int64                `gorm:"not null;check:ck_stb_batch_graph_version,graph_version_no >= 1"`
	SceneStoryNodeKey     string               `gorm:"type:varchar(68);not null;uniqueIndex:uq_stb_batch_node_scene,priority:2"`
	Status                string               `gorm:"type:varchar(20);not null;check:ck_stb_batch_status,status IN ('queued','running','ready','needs_asset','failed','unknown','cancelled')"`
	InputHash             string               `gorm:"type:char(64);not null;check:ck_stb_batch_input_hash,char_length(input_hash) = 64"`
	ResultHash            *string              `gorm:"type:char(64);check:ck_stb_batch_result_hash,result_hash IS NULL OR char_length(result_hash) = 64"`
	CandidateRevisionID   *uuid.UUID           `gorm:"type:uuid"`
	CandidateRevisionHash *string              `gorm:"type:char(64);check:ck_stb_batch_candidate_hash,candidate_revision_hash IS NULL OR char_length(candidate_revision_hash) = 64"`
	Candidate             datatypes.JSON       `gorm:"type:jsonb;not null"`
	Decisions             datatypes.JSON       `gorm:"type:jsonb;not null"`
	Error                 datatypes.JSON       `gorm:"type:jsonb"`
	Revision              int                  `gorm:"not null;check:ck_stb_batch_revision,revision >= 1"`
	ApprovedBy            *uuid.UUID           `gorm:"type:uuid"`
	ApprovedAt            *time.Time           `gorm:"type:timestamptz"`
	AppliedAt             *time.Time           `gorm:"type:timestamptz"`
	CreatedBy             uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt             time.Time            `gorm:"type:timestamptz;not null;index:ix_stb_batches_episode_created,priority:2,sort:desc"`
	UpdatedAt             time.Time            `gorm:"type:timestamptz;not null"`
	Workspace             Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project               Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode               Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Structure             EpisodeStructure     `gorm:"foreignKey:StructureID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ScriptVersion         EpisodeScriptVersion `gorm:"foreignKey:ScriptVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator               UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Approver              *UserAccount         `gorm:"foreignKey:ApprovedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardDraftBatch) TableName() string { return "stb_draft_batches" }

type StoryboardShot struct {
	ID               uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID        uuid.UUID            `gorm:"type:uuid;not null"`
	EpisodeID        uuid.UUID            `gorm:"type:uuid;not null;index:ix_stb_shots_episode_status,priority:1"`
	BatchID          uuid.UUID            `gorm:"type:uuid;not null;uniqueIndex:uq_stb_shot_batch_position,priority:1"`
	ProposalKey      string               `gorm:"type:varchar(120);not null"`
	Position         int                  `gorm:"not null;uniqueIndex:uq_stb_shot_batch_position,priority:2;check:ck_stb_shot_position,position >= 1"`
	Title            string               `gorm:"type:varchar(200);not null"`
	NarrativeUnitIDs datatypes.JSON       `gorm:"type:jsonb;not null"`
	Spec             datatypes.JSON       `gorm:"type:jsonb;not null"`
	ContentHash      string               `gorm:"type:char(64);not null;check:ck_stb_shot_hash,char_length(content_hash) = 64"`
	Status           string               `gorm:"type:varchar(20);not null;index:ix_stb_shots_episode_status,priority:2;check:ck_stb_shot_status,status IN ('active','archived')"`
	Revision         int                  `gorm:"not null;check:ck_stb_shot_revision,revision >= 1"`
	CreatedBy        uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt        time.Time            `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time            `gorm:"type:timestamptz;not null"`
	Workspace        Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project          Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode          Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Batch            StoryboardDraftBatch `gorm:"foreignKey:BatchID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator          UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardShot) TableName() string { return "stb_shots" }

type StoryboardShotImageBindingVersion struct {
	ID                            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID                   uuid.UUID      `gorm:"type:uuid;not null"`
	ProjectID                     uuid.UUID      `gorm:"type:uuid;not null"`
	EpisodeID                     uuid.UUID      `gorm:"type:uuid;not null"`
	ShotID                        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_stb_shot_image_binding_revision,priority:1;index:ix_stb_shot_image_bindings_current,priority:1"`
	ShotRevision                  int            `gorm:"not null;check:ck_stb_shot_image_binding_shot_revision,shot_revision >= 1"`
	ShotContentHash               string         `gorm:"type:char(64);not null;check:ck_stb_shot_image_binding_shot_hash,char_length(shot_content_hash) = 64"`
	CandidateSelectionID          uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_stb_shot_image_binding_selection"`
	CandidateSelectionRevision    int            `gorm:"not null;check:ck_stb_shot_image_binding_selection_revision,candidate_selection_revision >= 1"`
	CandidateSelectionContentHash string         `gorm:"type:char(64);not null;check:ck_stb_shot_image_binding_selection_hash,char_length(candidate_selection_content_hash) = 64"`
	CandidateID                   uuid.UUID      `gorm:"type:uuid;not null"`
	CandidateRevision             int            `gorm:"not null;check:ck_stb_shot_image_binding_candidate_revision,candidate_revision >= 1"`
	ArtifactID                    uuid.UUID      `gorm:"type:uuid;not null"`
	ArtifactRevision              int            `gorm:"not null;check:ck_stb_shot_image_binding_artifact_revision,artifact_revision >= 1"`
	ArtifactSHA256                string         `gorm:"type:char(64);not null;check:ck_stb_shot_image_binding_artifact_sha256,char_length(artifact_sha256) = 64"`
	BindingRevision               int            `gorm:"not null;uniqueIndex:uq_stb_shot_image_binding_revision,priority:2;index:ix_stb_shot_image_bindings_current,priority:2,sort:desc;check:ck_stb_shot_image_binding_revision,binding_revision >= 1"`
	ContentHash                   string         `gorm:"type:char(64);not null;check:ck_stb_shot_image_binding_hash,char_length(content_hash) = 64"`
	CreatedBy                     uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt                     time.Time      `gorm:"type:timestamptz;not null"`
	Workspace                     Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                       Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode                       Episode        `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Shot                          StoryboardShot `gorm:"foreignKey:ShotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                       UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardShotImageBindingVersion) TableName() string {
	return "stb_shot_image_binding_versions"
}

type StoryboardExportSet struct {
	ID               uuid.UUID          `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID          `gorm:"type:uuid;not null"`
	ProjectID        uuid.UUID          `gorm:"type:uuid;not null"`
	DraftSetID       uuid.UUID          `gorm:"type:uuid;not null;index:ix_stb_export_sets_draft_created,priority:1"`
	DraftSetRevision int                `gorm:"not null;check:ck_stb_export_set_draft_revision,draft_set_revision >= 1"`
	Status           string             `gorm:"type:varchar(20);not null;check:ck_stb_export_set_status,status IN ('succeeded')"`
	InputHash        string             `gorm:"type:char(64);not null;check:ck_stb_export_set_input_hash,char_length(input_hash) = 64"`
	ContentHash      string             `gorm:"type:char(64);not null;check:ck_stb_export_set_content_hash,char_length(content_hash) = 64"`
	Exports          datatypes.JSON     `gorm:"type:jsonb;not null"`
	Revision         int                `gorm:"not null;check:ck_stb_export_set_revision,revision >= 1"`
	CreatedBy        uuid.UUID          `gorm:"type:uuid;not null"`
	CreatedAt        time.Time          `gorm:"type:timestamptz;not null;index:ix_stb_export_sets_draft_created,priority:2,sort:desc"`
	UpdatedAt        time.Time          `gorm:"type:timestamptz;not null"`
	Workspace        Workspace          `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project          Project            `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	DraftSet         StoryboardDraftSet `gorm:"foreignKey:DraftSetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator          UserAccount        `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardExportSet) TableName() string { return "stb_export_sets" }

type StoryboardExport struct {
	ID          uuid.UUID            `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID            `gorm:"type:uuid;not null"`
	ProjectID   uuid.UUID            `gorm:"type:uuid;not null"`
	ExportSetID *uuid.UUID           `gorm:"type:uuid;uniqueIndex:uq_stb_export_set_episode,priority:1"`
	EpisodeID   uuid.UUID            `gorm:"type:uuid;not null;index:ix_stb_exports_episode_created,priority:1;uniqueIndex:uq_stb_export_set_episode,priority:2"`
	Status      string               `gorm:"type:varchar(20);not null;check:ck_stb_export_status,status IN ('succeeded','failed')"`
	InputHash   string               `gorm:"type:char(64);not null;check:ck_stb_export_input_hash,char_length(input_hash) = 64"`
	ContentHash string               `gorm:"type:char(64);not null;check:ck_stb_export_content_hash,char_length(content_hash) = 64"`
	Manifest    datatypes.JSON       `gorm:"type:jsonb;not null"`
	Files       datatypes.JSON       `gorm:"type:jsonb;not null"`
	Package     []byte               `gorm:"type:bytea;not null"`
	Revision    int                  `gorm:"not null;check:ck_stb_export_revision,revision >= 1"`
	CreatedBy   uuid.UUID            `gorm:"type:uuid;not null"`
	CreatedAt   time.Time            `gorm:"type:timestamptz;not null;index:ix_stb_exports_episode_created,priority:2,sort:desc"`
	UpdatedAt   time.Time            `gorm:"type:timestamptz;not null"`
	Workspace   Workspace            `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project     Project              `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ExportSet   *StoryboardExportSet `gorm:"foreignKey:ExportSetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Episode     Episode              `gorm:"foreignKey:EpisodeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Creator     UserAccount          `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (StoryboardExport) TableName() string { return "stb_exports" }
