package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type HumanTask struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID      `gorm:"type:uuid;not null;index:ix_rev_tasks_workspace_status_updated,priority:1;uniqueIndex:uq_rev_task_node,priority:1"`
	ProjectID        uuid.UUID      `gorm:"type:uuid;not null;index:ix_rev_tasks_project_status_updated,priority:1"`
	WorkflowRunID    uuid.UUID      `gorm:"type:uuid;not null;index:ix_rev_tasks_run"`
	NodeRunID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_rev_task_node,priority:2"`
	SubjectType      string         `gorm:"type:varchar(80);not null"`
	SubjectID        uuid.UUID      `gorm:"type:uuid;not null"`
	SubjectRevision  int            `gorm:"not null;check:ck_rev_task_subject_revision,subject_revision >= 1"`
	SubjectHash      string         `gorm:"type:char(64);not null;check:ck_rev_task_subject_hash,char_length(subject_hash) = 64"`
	CandidateIDs     datatypes.JSON `gorm:"type:jsonb;not null"`
	RubricVersion    string         `gorm:"type:varchar(80);not null"`
	AllowedDecisions datatypes.JSON `gorm:"type:jsonb;not null"`
	Status           string         `gorm:"type:varchar(20);not null;index:ix_rev_tasks_workspace_status_updated,priority:2;index:ix_rev_tasks_project_status_updated,priority:2;check:ck_rev_task_status,status IN ('OPEN','CLAIMED','COMPLETED','CANCELLED','STALE')"`
	ClaimedBy        *uuid.UUID     `gorm:"type:uuid"`
	ClaimToken       *uuid.UUID     `gorm:"type:uuid"`
	ClaimExpiresAt   *time.Time     `gorm:"type:timestamptz;index:ix_rev_tasks_claim_expires"`
	Revision         int            `gorm:"not null;check:ck_rev_task_revision,revision >= 1"`
	CreatedAt        time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time      `gorm:"type:timestamptz;not null;index:ix_rev_tasks_workspace_status_updated,priority:3,sort:desc;index:ix_rev_tasks_project_status_updated,priority:3,sort:desc"`
	Workspace        Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project          Project        `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Claimer          *UserAccount   `gorm:"foreignKey:ClaimedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (HumanTask) TableName() string { return "rev_human_tasks" }

type ReviewDecision struct {
	ID                  uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID   `gorm:"type:uuid;not null;index:ix_rev_decisions_workspace_created,priority:1"`
	HumanTaskID         uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_rev_decision_task"`
	Decision            string      `gorm:"type:varchar(30);not null;check:ck_rev_decision_value,decision IN ('approved','rejected','changes_requested','selected')"`
	SubjectRevision     int         `gorm:"not null;check:ck_rev_decision_subject_revision,subject_revision >= 1"`
	SubjectHash         string      `gorm:"type:char(64);not null;check:ck_rev_decision_subject_hash,char_length(subject_hash) = 64"`
	SelectedCandidateID *uuid.UUID  `gorm:"type:uuid"`
	CreatedBy           uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt           time.Time   `gorm:"type:timestamptz;not null;index:ix_rev_decisions_workspace_created,priority:2,sort:desc"`
	Workspace           Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	HumanTask           HumanTask   `gorm:"foreignKey:HumanTaskID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator             UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ReviewDecision) TableName() string { return "rev_review_decisions" }
