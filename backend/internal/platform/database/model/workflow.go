package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type WorkflowDefinitionVersion struct {
	ID                     uuid.UUID          `gorm:"type:uuid;primaryKey"`
	WorkspaceID            uuid.UUID          `gorm:"type:uuid;not null;index:ix_wrk_definitions_workspace_created,priority:1"`
	ProjectID              uuid.UUID          `gorm:"type:uuid;not null;index:ix_wrk_definitions_project_created,priority:1"`
	AuthoringRevisionID    uuid.UUID          `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_definition_source_compiler,priority:1"`
	NodeCatalogVersionID   uuid.UUID          `gorm:"type:uuid;not null"`
	CompilerVersion        string             `gorm:"type:varchar(40);not null;uniqueIndex:uq_wrk_definition_source_compiler,priority:2"`
	WorkflowType           string             `gorm:"type:varchar(120);not null"`
	WorkflowTypeVersion    string             `gorm:"type:varchar(40);not null"`
	RuntimeContractVersion string             `gorm:"type:varchar(40);not null"`
	Definition             datatypes.JSON     `gorm:"type:jsonb;not null"`
	ContentHash            string             `gorm:"type:char(64);not null;index:ix_wrk_definitions_content_hash;check:ck_wrk_definition_content_hash,char_length(content_hash) = 64"`
	CreatedBy              uuid.UUID          `gorm:"type:uuid;not null"`
	CreatedAt              time.Time          `gorm:"type:timestamptz;not null;index:ix_wrk_definitions_workspace_created,priority:2,sort:desc;index:ix_wrk_definitions_project_created,priority:2,sort:desc"`
	Workspace              Workspace          `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                Project            `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	AuthoringRevision      AuthoringRevision  `gorm:"foreignKey:AuthoringRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeCatalogVersion     NodeCatalogVersion `gorm:"foreignKey:NodeCatalogVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                UserAccount        `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowDefinitionVersion) TableName() string { return "wrk_definition_versions" }

type RunInputSnapshot struct {
	ID                          uuid.UUID                 `gorm:"type:uuid;primaryKey"`
	WorkspaceID                 uuid.UUID                 `gorm:"type:uuid;not null;index:ix_wrk_input_snapshots_workspace_created,priority:1"`
	ProjectID                   uuid.UUID                 `gorm:"type:uuid;not null;index:ix_wrk_input_snapshots_project_created,priority:1"`
	WorkflowDefinitionVersionID uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_input_snapshot_definition"`
	AuthoringRevisionID         uuid.UUID                 `gorm:"type:uuid;not null"`
	Snapshot                    datatypes.JSON            `gorm:"type:jsonb;not null"`
	ContentHash                 string                    `gorm:"type:char(64);not null;index:ix_wrk_input_snapshots_content_hash;check:ck_wrk_input_snapshot_content_hash,char_length(content_hash) = 64"`
	CreatedBy                   uuid.UUID                 `gorm:"type:uuid;not null"`
	CreatedAt                   time.Time                 `gorm:"type:timestamptz;not null;index:ix_wrk_input_snapshots_workspace_created,priority:2,sort:desc;index:ix_wrk_input_snapshots_project_created,priority:2,sort:desc"`
	Workspace                   Workspace                 `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                     Project                   `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowDefinitionVersion   WorkflowDefinitionVersion `gorm:"foreignKey:WorkflowDefinitionVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	AuthoringRevision           AuthoringRevision         `gorm:"foreignKey:AuthoringRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                     UserAccount               `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (RunInputSnapshot) TableName() string { return "wrk_run_input_snapshots" }

type WorkflowRun struct {
	ID                          uuid.UUID                 `gorm:"type:uuid;primaryKey"`
	WorkspaceID                 uuid.UUID                 `gorm:"type:uuid;not null;index:ix_wrk_runs_workspace_updated,priority:1"`
	ProjectID                   uuid.UUID                 `gorm:"type:uuid;not null;index:ix_wrk_runs_project_updated,priority:1"`
	AuthoringRevisionID         uuid.UUID                 `gorm:"type:uuid;not null;index:ix_wrk_runs_authoring_revision"`
	WorkflowDefinitionVersionID uuid.UUID                 `gorm:"type:uuid;not null"`
	RunInputSnapshotID          uuid.UUID                 `gorm:"type:uuid;not null"`
	TemporalWorkflowID          string                    `gorm:"type:varchar(220);not null;uniqueIndex:uq_wrk_run_temporal_workflow"`
	StartInputHash              string                    `gorm:"type:char(64);not null;check:ck_wrk_run_start_hash,char_length(start_input_hash) = 64"`
	Status                      string                    `gorm:"type:varchar(30);not null;index:ix_wrk_runs_status_updated,priority:1;check:ck_wrk_run_status,status IN ('QUEUED','RUNNING','WAITING_HUMAN','RETRYING','PAUSED','SUCCEEDED','FAILED','CANCELLED','NEEDS_ATTENTION')"`
	ProgressStage               string                    `gorm:"type:varchar(80);not null"`
	NextAction                  *string                   `gorm:"type:varchar(80)"`
	Error                       datatypes.JSON            `gorm:"type:jsonb"`
	PausedFromStatus            *string                   `gorm:"type:varchar(30);check:ck_wrk_run_paused_from_status,paused_from_status IS NULL OR paused_from_status IN ('RUNNING','RETRYING')"`
	PausedFromProgressStage     *string                   `gorm:"type:varchar(80);check:ck_wrk_run_pause_source_pair,(paused_from_status IS NULL) = (paused_from_progress_stage IS NULL)"`
	Revision                    int                       `gorm:"not null;check:ck_wrk_run_revision,revision >= 1"`
	CreatedBy                   uuid.UUID                 `gorm:"type:uuid;not null"`
	CreatedAt                   time.Time                 `gorm:"type:timestamptz;not null"`
	UpdatedAt                   time.Time                 `gorm:"type:timestamptz;not null;index:ix_wrk_runs_workspace_updated,priority:2,sort:desc;index:ix_wrk_runs_project_updated,priority:2,sort:desc;index:ix_wrk_runs_status_updated,priority:2,sort:desc"`
	Workspace                   Workspace                 `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                     Project                   `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	AuthoringRevision           AuthoringRevision         `gorm:"foreignKey:AuthoringRevisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowDefinitionVersion   WorkflowDefinitionVersion `gorm:"foreignKey:WorkflowDefinitionVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	RunInputSnapshot            RunInputSnapshot          `gorm:"foreignKey:RunInputSnapshotID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                     UserAccount               `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowRun) TableName() string { return "wrk_runs" }

type NodeRunProjection struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID      `gorm:"type:uuid;not null;index:ix_wrk_node_runs_workspace_updated,priority:1"`
	WorkflowRunID     uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_node_run_identity,priority:1"`
	NodeID            string         `gorm:"type:varchar(100);not null;uniqueIndex:uq_wrk_node_run_identity,priority:2"`
	DefinitionKey     string         `gorm:"type:varchar(100);not null"`
	DefinitionVersion string         `gorm:"type:varchar(40);not null"`
	Executor          string         `gorm:"type:varchar(120);not null"`
	RiskLevel         string         `gorm:"type:varchar(30);not null;check:ck_wrk_node_run_risk,risk_level IN ('low','external_ai','human_gate')"`
	Status            string         `gorm:"type:varchar(30);not null;index:ix_wrk_node_runs_status_updated,priority:1;check:ck_wrk_node_run_status,status IN ('QUEUED','RUNNING','WAITING_HUMAN','RETRYING','SUCCEEDED','FAILED','CANCELLED','SKIPPED','CACHED')"`
	Attempt           int            `gorm:"not null;check:ck_wrk_node_run_attempt,attempt >= 0"`
	ActiveClaimToken  *uuid.UUID     `gorm:"type:uuid"`
	Input             datatypes.JSON `gorm:"type:jsonb"`
	InputHash         *string        `gorm:"type:char(64);check:ck_wrk_node_run_input_hash,input_hash IS NULL OR char_length(input_hash) = 64"`
	CacheKey          *string        `gorm:"type:char(64);check:ck_wrk_node_run_cache_key,cache_key IS NULL OR char_length(cache_key) = 64"`
	Output            datatypes.JSON `gorm:"type:jsonb"`
	OutputHash        *string        `gorm:"type:char(64);check:ck_wrk_node_run_output_hash,output_hash IS NULL OR char_length(output_hash) = 64"`
	Revision          int            `gorm:"not null;check:ck_wrk_node_run_revision,revision >= 1"`
	CreatedAt         time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time      `gorm:"type:timestamptz;not null;index:ix_wrk_node_runs_workspace_updated,priority:2,sort:desc;index:ix_wrk_node_runs_status_updated,priority:2,sort:desc"`
	Workspace         Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun       WorkflowRun    `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (NodeRunProjection) TableName() string { return "wrk_node_run_projections" }

type NodeCacheEntry struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_node_cache_key,priority:1"`
	CacheKey            string         `gorm:"type:char(64);not null;uniqueIndex:uq_wrk_node_cache_key,priority:2;check:ck_wrk_node_cache_key,char_length(cache_key) = 64"`
	KeyMaterial         datatypes.JSON `gorm:"type:jsonb;not null"`
	Output              datatypes.JSON `gorm:"type:jsonb;not null"`
	OutputHash          string         `gorm:"type:char(64);not null;check:ck_wrk_node_cache_output_hash,char_length(output_hash) = 64"`
	SourceWorkflowRunID uuid.UUID      `gorm:"type:uuid;not null;index:ix_wrk_node_cache_source,priority:1"`
	SourceNodeRunID     uuid.UUID      `gorm:"type:uuid;not null;index:ix_wrk_node_cache_source,priority:2"`
	CreatedAt           time.Time      `gorm:"type:timestamptz;not null"`
	Workspace           Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (NodeCacheEntry) TableName() string { return "wrk_node_cache_entries" }

type WorkflowStartIntent struct {
	ID                uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_start_intent_key,priority:1"`
	WorkflowRunID     uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_start_intent_run"`
	IdempotencyKey    string      `gorm:"type:varchar(200);not null;uniqueIndex:uq_wrk_start_intent_key,priority:2"`
	CommandInputHash  string      `gorm:"type:char(64);not null;check:ck_wrk_start_command_hash,char_length(command_input_hash) = 64"`
	TemporalInputHash string      `gorm:"type:char(64);not null;check:ck_wrk_start_temporal_hash,char_length(temporal_input_hash) = 64"`
	Status            string      `gorm:"type:varchar(20);not null;check:ck_wrk_start_intent_status,status IN ('pending','completed','unknown','conflict')"`
	AttemptNo         int         `gorm:"not null;check:ck_wrk_start_attempt,attempt_no >= 0"`
	Revision          int         `gorm:"not null;check:ck_wrk_start_intent_revision,revision >= 1"`
	CreatedBy         uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt         time.Time   `gorm:"type:timestamptz;not null"`
	UpdatedAt         time.Time   `gorm:"type:timestamptz;not null"`
	Workspace         Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun       WorkflowRun `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator           UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowStartIntent) TableName() string { return "wrk_start_intents" }

type WorkflowStartReceipt struct {
	ID                 uuid.UUID           `gorm:"type:uuid;primaryKey"`
	WorkspaceID        uuid.UUID           `gorm:"type:uuid;not null;index:ix_wrk_start_receipts_workspace_created,priority:1"`
	StartIntentID      uuid.UUID           `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_start_receipt_attempt,priority:1"`
	WorkflowRunID      uuid.UUID           `gorm:"type:uuid;not null"`
	AttemptNo          int                 `gorm:"not null;uniqueIndex:uq_wrk_start_receipt_attempt,priority:2;check:ck_wrk_start_receipt_attempt,attempt_no >= 1"`
	Outcome            string              `gorm:"type:varchar(30);not null;check:ck_wrk_start_receipt_outcome,outcome IN ('started','already_started','unknown','conflict')"`
	TemporalWorkflowID string              `gorm:"type:varchar(220);not null"`
	ExpectedInputHash  string              `gorm:"type:char(64);not null;check:ck_wrk_start_expected_hash,char_length(expected_input_hash) = 64"`
	ObservedInputHash  *string             `gorm:"type:char(64);check:ck_wrk_start_observed_hash,observed_input_hash IS NULL OR char_length(observed_input_hash) = 64"`
	CreatedAt          time.Time           `gorm:"type:timestamptz;not null;index:ix_wrk_start_receipts_workspace_created,priority:2,sort:desc"`
	Workspace          Workspace           `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	StartIntent        WorkflowStartIntent `gorm:"foreignKey:StartIntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun        WorkflowRun         `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowStartReceipt) TableName() string { return "wrk_start_receipts" }

type WorkflowTask struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID   uuid.UUID      `gorm:"type:uuid;not null;index:ix_wrk_tasks_workspace_updated,priority:1"`
	TaskType      string         `gorm:"type:varchar(60);not null;index:ix_wrk_tasks_type_status,priority:1"`
	RequestType   string         `gorm:"type:varchar(60);not null;uniqueIndex:uq_wrk_task_request,priority:1"`
	RequestID     uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_wrk_task_request,priority:2"`
	Scope         datatypes.JSON `gorm:"type:jsonb;not null"`
	Status        string         `gorm:"type:varchar(30);not null;index:ix_wrk_tasks_type_status,priority:2"`
	ProgressStage string         `gorm:"type:varchar(80);not null"`
	Error         datatypes.JSON `gorm:"type:jsonb"`
	NextAction    *string        `gorm:"type:varchar(80)"`
	CancelStatus  string         `gorm:"type:varchar(20);not null"`
	Revision      int            `gorm:"not null;check:ck_wrk_task_revision,revision >= 1"`
	CreatedAt     time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time      `gorm:"type:timestamptz;not null;index:ix_wrk_tasks_workspace_updated,priority:2,sort:desc"`
	Workspace     Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (WorkflowTask) TableName() string { return "wrk_tasks" }
