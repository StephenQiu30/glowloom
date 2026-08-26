package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type GenerationCandidate struct {
	ID               uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID      uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_gen_candidate_source_output,priority:1;uniqueIndex:uq_gen_candidate_artifact,priority:1;index:ix_gen_candidates_workspace_status,priority:1"`
	ProjectID        uuid.UUID   `gorm:"type:uuid;not null;index"`
	ProviderJobID    uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_gen_candidate_source_output,priority:2"`
	OutputKey        string      `gorm:"type:varchar(120);not null;uniqueIndex:uq_gen_candidate_source_output,priority:3"`
	ArtifactID       uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_gen_candidate_artifact,priority:2"`
	ArtifactRevision int         `gorm:"not null;check:ck_gen_candidate_artifact_revision,artifact_revision >= 1"`
	ArtifactSHA256   string      `gorm:"type:char(64);not null;check:ck_gen_candidate_artifact_sha256,char_length(artifact_sha256) = 64"`
	MediaType        string      `gorm:"type:varchar(120);not null;check:ck_gen_candidate_media_type,media_type IN ('image/png','image/jpeg')"`
	Width            int         `gorm:"not null;check:ck_gen_candidate_width,width > 0"`
	Height           int         `gorm:"not null;check:ck_gen_candidate_height,height > 0"`
	Status           string      `gorm:"type:varchar(20);not null;index:ix_gen_candidates_workspace_status,priority:2;check:ck_gen_candidate_status,status IN ('QC_PASSED','QC_FAILED')"`
	Revision         int         `gorm:"not null;check:ck_gen_candidate_revision,revision >= 1"`
	CreatedBy        uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt        time.Time   `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time   `gorm:"type:timestamptz;not null"`
	Workspace        Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project          Project     `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Artifact         Artifact    `gorm:"foreignKey:ArtifactID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator          UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationCandidate) TableName() string { return "gen_candidates" }

type GenerationQCReport struct {
	ID            uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	WorkspaceID   uuid.UUID                   `gorm:"type:uuid;not null;index:ix_gen_qc_reports_workspace_created,priority:1"`
	CandidateID   uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:uq_gen_qc_report_candidate"`
	PolicyVersion string                      `gorm:"type:varchar(80);not null"`
	PolicyHash    string                      `gorm:"type:char(64);not null;check:ck_gen_qc_report_policy_hash,char_length(policy_hash) = 64"`
	Policy        datatypes.JSON              `gorm:"type:jsonb;not null"`
	Status        string                      `gorm:"type:varchar(12);not null;check:ck_gen_qc_report_status,status IN ('PASSED','FAILED')"`
	FailureCodes  datatypes.JSONSlice[string] `gorm:"type:jsonb;not null"`
	ReportHash    string                      `gorm:"type:char(64);not null;index;check:ck_gen_qc_report_hash,char_length(report_hash) = 64"`
	CreatedAt     time.Time                   `gorm:"type:timestamptz;not null;index:ix_gen_qc_reports_workspace_created,priority:2,sort:desc"`
	Workspace     Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Candidate     GenerationCandidate         `gorm:"foreignKey:CandidateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (GenerationQCReport) TableName() string { return "gen_qc_reports" }
