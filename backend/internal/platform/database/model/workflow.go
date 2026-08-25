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
