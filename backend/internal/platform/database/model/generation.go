package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type GenerationIntent struct {
	ID                        uuid.UUID         `gorm:"type:uuid;primaryKey"`
	WorkspaceID               uuid.UUID         `gorm:"type:uuid;not null;index"`
	ProjectID                 uuid.UUID         `gorm:"type:uuid;not null;index"`
	WorkflowRunID             uuid.UUID         `gorm:"type:uuid;not null;index"`
	NodeRunID                 uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_gen_intent_node_run"`
	TargetID                  uuid.UUID         `gorm:"type:uuid;not null;index"`
	Metric                    string            `gorm:"type:varchar(64);not null;check:ck_gen_intent_metric,metric = 'generation.image'"`
	TargetHash                string            `gorm:"type:char(64);not null;check:ck_gen_intent_target_hash,char_length(target_hash) = 64"`
	Units                     int64             `gorm:"not null;check:ck_gen_intent_units,units > 0"`
	CostEstimateID            *uuid.UUID        `gorm:"type:uuid"`
	CostReservationID         *uuid.UUID        `gorm:"type:uuid"`
	QuotaReservationID        *uuid.UUID        `gorm:"type:uuid"`
	CostEstimateReceiptID     *uuid.UUID        `gorm:"type:uuid"`
	CostReservationReceiptID  *uuid.UUID        `gorm:"type:uuid"`
	QuotaReservationReceiptID *uuid.UUID        `gorm:"type:uuid"`
	CostReleaseReceiptID      *uuid.UUID        `gorm:"type:uuid"`
	QuotaReleaseReceiptID     *uuid.UUID        `gorm:"type:uuid"`
	CostSettlementReceiptID   *uuid.UUID        `gorm:"type:uuid"`
	QuotaConsumptionReceiptID *uuid.UUID        `gorm:"type:uuid"`
	GenerationRequestID       *uuid.UUID        `gorm:"type:uuid"`
	ProviderJobID             *uuid.UUID        `gorm:"type:uuid"`
	ProviderReceiptID         *uuid.UUID        `gorm:"type:uuid"`
	Status                    string            `gorm:"type:varchar(20);not null;index;check:ck_gen_intent_state,(status = 'PREPARING' AND revision = 1 AND cost_estimate_id IS NULL AND cost_reservation_id IS NULL AND quota_reservation_id IS NULL AND cost_estimate_receipt_id IS NULL AND cost_reservation_receipt_id IS NULL AND quota_reservation_receipt_id IS NULL AND cost_release_receipt_id IS NULL AND quota_release_receipt_id IS NULL AND cost_settlement_receipt_id IS NULL AND quota_consumption_receipt_id IS NULL AND generation_request_id IS NULL AND provider_job_id IS NULL AND provider_receipt_id IS NULL) OR (status = 'PREPARED' AND revision = 2 AND cost_estimate_id IS NOT NULL AND cost_reservation_id IS NOT NULL AND quota_reservation_id IS NOT NULL AND cost_estimate_receipt_id IS NOT NULL AND cost_reservation_receipt_id IS NOT NULL AND quota_reservation_receipt_id IS NOT NULL AND cost_release_receipt_id IS NULL AND quota_release_receipt_id IS NULL AND cost_settlement_receipt_id IS NULL AND quota_consumption_receipt_id IS NULL AND generation_request_id IS NULL AND provider_job_id IS NULL AND provider_receipt_id IS NULL) OR (status = 'CLAIMED' AND revision = 3 AND cost_estimate_id IS NOT NULL AND cost_reservation_id IS NOT NULL AND quota_reservation_id IS NOT NULL AND cost_estimate_receipt_id IS NOT NULL AND cost_reservation_receipt_id IS NOT NULL AND quota_reservation_receipt_id IS NOT NULL AND cost_release_receipt_id IS NULL AND quota_release_receipt_id IS NULL AND cost_settlement_receipt_id IS NULL AND quota_consumption_receipt_id IS NULL AND generation_request_id IS NULL AND provider_job_id IS NULL AND provider_receipt_id IS NULL) OR (status IN ('DISPATCHING','SUBMITTED','OUTCOME_UNKNOWN') AND revision >= 4 AND cost_estimate_id IS NOT NULL AND cost_reservation_id IS NOT NULL AND quota_reservation_id IS NOT NULL AND cost_estimate_receipt_id IS NOT NULL AND cost_reservation_receipt_id IS NOT NULL AND quota_reservation_receipt_id IS NOT NULL AND cost_release_receipt_id IS NULL AND quota_release_receipt_id IS NULL AND cost_settlement_receipt_id IS NULL AND quota_consumption_receipt_id IS NULL AND generation_request_id IS NOT NULL AND provider_job_id IS NOT NULL AND provider_receipt_id IS NULL) OR (status = 'SUCCEEDED' AND revision >= 5 AND cost_estimate_id IS NOT NULL AND cost_reservation_id IS NOT NULL AND quota_reservation_id IS NOT NULL AND cost_estimate_receipt_id IS NOT NULL AND cost_reservation_receipt_id IS NOT NULL AND quota_reservation_receipt_id IS NOT NULL AND cost_release_receipt_id IS NULL AND quota_release_receipt_id IS NULL AND cost_settlement_receipt_id IS NOT NULL AND quota_consumption_receipt_id IS NOT NULL AND generation_request_id IS NOT NULL AND provider_job_id IS NOT NULL AND provider_receipt_id IS NOT NULL) OR (status = 'FAILED' AND revision >= 5 AND cost_estimate_id IS NOT NULL AND cost_reservation_id IS NOT NULL AND quota_reservation_id IS NOT NULL AND cost_estimate_receipt_id IS NOT NULL AND cost_reservation_receipt_id IS NOT NULL AND quota_reservation_receipt_id IS NOT NULL AND cost_release_receipt_id IS NOT NULL AND quota_release_receipt_id IS NOT NULL AND cost_settlement_receipt_id IS NULL AND quota_consumption_receipt_id IS NULL AND generation_request_id IS NOT NULL AND provider_job_id IS NOT NULL AND provider_receipt_id IS NOT NULL) OR (status = 'CANCELLED' AND revision = 3 AND cost_estimate_id IS NOT NULL AND cost_reservation_id IS NOT NULL AND quota_reservation_id IS NOT NULL AND cost_estimate_receipt_id IS NOT NULL AND cost_reservation_receipt_id IS NOT NULL AND quota_reservation_receipt_id IS NOT NULL AND cost_release_receipt_id IS NOT NULL AND quota_release_receipt_id IS NOT NULL AND cost_settlement_receipt_id IS NULL AND quota_consumption_receipt_id IS NULL AND generation_request_id IS NULL AND provider_job_id IS NULL AND provider_receipt_id IS NULL)"`
	Claimant                  *string           `gorm:"type:varchar(120)"`
	ClaimToken                *uuid.UUID        `gorm:"type:uuid;uniqueIndex"`
	ClaimExpiresAt            *time.Time        `gorm:"type:timestamptz;index"`
	ClaimFencingVersion       int64             `gorm:"not null;check:ck_gen_intent_claim_fields,(status IN ('CLAIMED','DISPATCHING','SUBMITTED','OUTCOME_UNKNOWN','SUCCEEDED','FAILED') AND claimant IS NOT NULL AND claim_token IS NOT NULL AND claim_expires_at IS NOT NULL AND claim_fencing_version = 1) OR (status IN ('PREPARING','PREPARED','CANCELLED') AND claimant IS NULL AND claim_token IS NULL AND claim_expires_at IS NULL AND claim_fencing_version = 0)"`
	CancelledAt               *time.Time        `gorm:"type:timestamptz;check:ck_gen_intent_cancelled_at,(status = 'CANCELLED') = (cancelled_at IS NOT NULL)"`
	Revision                  int64             `gorm:"not null;check:ck_gen_intent_revision,revision >= 1"`
	ContentHash               string            `gorm:"type:char(64);not null;check:ck_gen_intent_content_hash,char_length(content_hash) = 64"`
	CreatedBy                 uuid.UUID         `gorm:"type:uuid;not null"`
	InitiatorTokenVersion     int               `gorm:"not null;check:ck_gen_intent_token_version,initiator_token_version >= 1"`
	CreatedAt                 time.Time         `gorm:"type:timestamptz;not null"`
	UpdatedAt                 time.Time         `gorm:"type:timestamptz;not null"`
	Workspace                 Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                   Project           `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	WorkflowRun               WorkflowRun       `gorm:"foreignKey:WorkflowRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	NodeRun                   NodeRunProjection `gorm:"foreignKey:NodeRunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Target                    GenerationTarget  `gorm:"foreignKey:TargetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CostEstimate              *CostEstimate     `gorm:"foreignKey:CostEstimateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CostReservation           *CostReservation  `gorm:"foreignKey:CostReservationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	QuotaReservation          *QuotaReservation `gorm:"foreignKey:QuotaReservationID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CostEstimateReceipt       *CommandReceipt   `gorm:"foreignKey:CostEstimateReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CostReservationReceipt    *CommandReceipt   `gorm:"foreignKey:CostReservationReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	QuotaReservationReceipt   *CommandReceipt   `gorm:"foreignKey:QuotaReservationReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CostReleaseReceipt        *CommandReceipt   `gorm:"foreignKey:CostReleaseReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	QuotaReleaseReceipt       *CommandReceipt   `gorm:"foreignKey:QuotaReleaseReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	CostSettlementReceipt     *CommandReceipt   `gorm:"foreignKey:CostSettlementReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	QuotaConsumptionReceipt   *CommandReceipt   `gorm:"foreignKey:QuotaConsumptionReceiptID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                   UserAccount       `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationIntent) TableName() string { return "gen_intents" }

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

type GenerationCandidateSelection struct {
	ID                     uuid.UUID           `gorm:"type:uuid;primaryKey"`
	WorkspaceID            uuid.UUID           `gorm:"type:uuid;not null;index:ix_gen_selections_workspace_created,priority:1"`
	ProjectID              uuid.UUID           `gorm:"type:uuid;not null;index"`
	WorkflowRunID          uuid.UUID           `gorm:"type:uuid;not null;index"`
	NodeRunID              uuid.UUID           `gorm:"type:uuid;not null;index"`
	HumanTaskID            uuid.UUID           `gorm:"type:uuid;not null;uniqueIndex:uq_gen_selection_task"`
	ReviewDecisionID       uuid.UUID           `gorm:"type:uuid;not null;uniqueIndex:uq_gen_selection_decision"`
	SubjectType            string              `gorm:"type:varchar(80);not null"`
	SubjectID              uuid.UUID           `gorm:"type:uuid;not null"`
	SubjectRevision        int                 `gorm:"not null;check:ck_gen_selection_subject_revision,subject_revision >= 1"`
	Candidates             datatypes.JSON      `gorm:"type:jsonb;not null"`
	CandidateSetHash       string              `gorm:"type:char(64);not null;check:ck_gen_selection_candidate_set_hash,char_length(candidate_set_hash) = 64"`
	SelectedCandidateID    uuid.UUID           `gorm:"type:uuid;not null;index"`
	SelectedArtifactID     uuid.UUID           `gorm:"type:uuid;not null"`
	SelectedArtifactSHA256 string              `gorm:"type:char(64);not null;check:ck_gen_selection_artifact_sha256,char_length(selected_artifact_sha256) = 64"`
	ReviewerID             uuid.UUID           `gorm:"type:uuid;not null"`
	ContentHash            string              `gorm:"type:char(64);not null;check:ck_gen_selection_content_hash,char_length(content_hash) = 64"`
	Revision               int                 `gorm:"not null;check:ck_gen_selection_revision,revision = 1"`
	CreatedBy              uuid.UUID           `gorm:"type:uuid;not null"`
	CreatedAt              time.Time           `gorm:"type:timestamptz;not null;index:ix_gen_selections_workspace_created,priority:2,sort:desc"`
	Workspace              Workspace           `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                Project             `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	HumanTask              HumanTask           `gorm:"foreignKey:HumanTaskID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ReviewDecision         ReviewDecision      `gorm:"foreignKey:ReviewDecisionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SelectedCandidate      GenerationCandidate `gorm:"foreignKey:SelectedCandidateID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	SelectedArtifact       Artifact            `gorm:"foreignKey:SelectedArtifactID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Reviewer               UserAccount         `gorm:"foreignKey:ReviewerID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                UserAccount         `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationCandidateSelection) TableName() string { return "gen_candidate_selections" }
