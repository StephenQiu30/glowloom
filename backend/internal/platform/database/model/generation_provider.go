package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrImmutableProviderCredentialVersion       = errors.New("ProviderCredentialVersion is immutable")
	ErrImmutableProviderConnectionVersion       = errors.New("ProviderConnectionVersion is immutable")
	ErrImmutableProviderModelProfileVersion     = errors.New("ProviderModelProfileVersion is immutable")
	ErrImmutableProjectProviderBindingVersion   = errors.New("ProjectProviderBindingVersion is immutable")
	ErrImmutableGenerationRequest               = errors.New("GenerationRequest is immutable")
	ErrImmutableGenerationProviderResultReceipt = errors.New("GenerationProviderResultReceipt is immutable")
)

type ProviderCredentialVersion struct {
	ID                uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID       uuid.UUID   `gorm:"type:uuid;not null;index;uniqueIndex:uq_gen_provider_credential_revision,priority:1"`
	ConnectionKey     string      `gorm:"type:varchar(80);not null;uniqueIndex:uq_gen_provider_credential_revision,priority:2"`
	Revision          int64       `gorm:"not null;uniqueIndex:uq_gen_provider_credential_revision,priority:3;check:ck_gen_provider_credential_revision,revision >= 1"`
	ProviderKey       string      `gorm:"type:varchar(80);not null"`
	CipherSuite       string      `gorm:"type:varchar(32);not null;check:ck_gen_provider_credential_cipher,cipher_suite = 'aes-256-gcm'"`
	KeyID             string      `gorm:"type:varchar(64);not null"`
	Nonce             []byte      `gorm:"type:bytea;not null"`
	Ciphertext        []byte      `gorm:"type:bytea;not null"`
	SecretFingerprint string      `gorm:"type:char(64);not null;check:ck_gen_provider_credential_fingerprint,char_length(secret_fingerprint) = 64"`
	CreatedBy         uuid.UUID   `gorm:"type:uuid;not null"`
	CreatedAt         time.Time   `gorm:"type:timestamptz;not null"`
	Workspace         Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator           UserAccount `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProviderCredentialVersion) TableName() string { return "gen_provider_credential_versions" }

func (*ProviderCredentialVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProviderCredentialVersion
}

func (*ProviderCredentialVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProviderCredentialVersion
}

type ProviderConnectionVersion struct {
	ID                     uuid.UUID                 `gorm:"type:uuid;primaryKey"`
	WorkspaceID            uuid.UUID                 `gorm:"type:uuid;not null;index;uniqueIndex:uq_gen_provider_connection_revision,priority:1"`
	ConnectionKey          string                    `gorm:"type:varchar(80);not null;uniqueIndex:uq_gen_provider_connection_revision,priority:2"`
	Revision               int64                     `gorm:"not null;uniqueIndex:uq_gen_provider_connection_revision,priority:3;check:ck_gen_provider_connection_revision,revision >= 1"`
	SourcePresetKey        string                    `gorm:"type:varchar(120);not null"`
	SourcePresetVersion    int64                     `gorm:"not null;check:ck_gen_provider_connection_preset_revision,source_preset_version >= 1"`
	PresetSnapshotHash     string                    `gorm:"type:char(64);not null;check:ck_gen_provider_connection_preset_hash,char_length(preset_snapshot_hash) = 64"`
	ProviderKey            string                    `gorm:"type:varchar(80);not null"`
	DisplayName            string                    `gorm:"type:varchar(120);not null"`
	CredentialVersionID    uuid.UUID                 `gorm:"type:uuid;not null;index"`
	ResolvedConfig         datatypes.JSON            `gorm:"type:jsonb;not null"`
	State                  string                    `gorm:"type:varchar(20);not null;check:ck_gen_provider_connection_state,state IN ('enabled','disabled')"`
	AdapterContractVersion string                    `gorm:"type:varchar(120);not null"`
	ContentHash            string                    `gorm:"type:char(64);not null;check:ck_gen_provider_connection_hash,char_length(content_hash) = 64"`
	CreatedBy              uuid.UUID                 `gorm:"type:uuid;not null"`
	CreatedAt              time.Time                 `gorm:"type:timestamptz;not null"`
	Workspace              Workspace                 `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Credential             ProviderCredentialVersion `gorm:"foreignKey:CredentialVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                UserAccount               `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProviderConnectionVersion) TableName() string { return "gen_provider_connection_versions" }

func (*ProviderConnectionVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProviderConnectionVersion
}

func (*ProviderConnectionVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProviderConnectionVersion
}

type ProviderModelProfileVersion struct {
	ID                       uuid.UUID      `gorm:"type:uuid;primaryKey"`
	WorkspaceID              uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:uq_gen_provider_profile_revision,priority:1"`
	ProfileKey               string         `gorm:"type:varchar(80);not null;uniqueIndex:uq_gen_provider_profile_revision,priority:2"`
	Revision                 int64          `gorm:"not null;uniqueIndex:uq_gen_provider_profile_revision,priority:3;check:ck_gen_provider_profile_revision,revision >= 1"`
	CreationSource           datatypes.JSON `gorm:"type:jsonb;not null"`
	ConnectionKey            string         `gorm:"type:varchar(80);not null;index"`
	ProviderKey              string         `gorm:"type:varchar(80);not null"`
	ExternalModelID          string         `gorm:"type:varchar(180);not null"`
	Modality                 string         `gorm:"type:varchar(20);not null;check:ck_gen_provider_profile_modality,modality IN ('image','video')"`
	Family                   string         `gorm:"type:varchar(80);not null"`
	AdapterTransportContract string         `gorm:"type:varchar(120);not null"`
	CapabilitySchemaVersion  string         `gorm:"type:varchar(120);not null"`
	BillingMetric            string         `gorm:"type:varchar(80);not null;check:ck_gen_provider_profile_metric,billing_metric IN ('generation.image.call','generation.video.call')"`
	Defaults                 datatypes.JSON `gorm:"type:jsonb;not null"`
	State                    string         `gorm:"type:varchar(20);not null;check:ck_gen_provider_profile_state,state IN ('enabled','disabled')"`
	ContentHash              string         `gorm:"type:char(64);not null;check:ck_gen_provider_profile_hash,char_length(content_hash) = 64"`
	CreatedBy                uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt                time.Time      `gorm:"type:timestamptz;not null"`
	Workspace                Workspace      `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                  UserAccount    `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProviderModelProfileVersion) TableName() string { return "gen_provider_model_profile_versions" }

func (*ProviderModelProfileVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProviderModelProfileVersion
}

func (*ProviderModelProfileVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProviderModelProfileVersion
}

type ProjectProviderBindingVersion struct {
	ID                     uuid.UUID                   `gorm:"type:uuid;primaryKey"`
	WorkspaceID            uuid.UUID                   `gorm:"type:uuid;not null;index"`
	ProjectID              uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:uq_gen_project_provider_binding_revision,priority:1"`
	Purpose                string                      `gorm:"type:varchar(32);not null;uniqueIndex:uq_gen_project_provider_binding_revision,priority:2;check:ck_gen_project_provider_binding_purpose,purpose IN ('reference_asset','shot_frame','shot_video')"`
	Revision               int64                       `gorm:"not null;uniqueIndex:uq_gen_project_provider_binding_revision,priority:3;check:ck_gen_project_provider_binding_revision,revision >= 1"`
	ConnectionVersionID    uuid.UUID                   `gorm:"type:uuid;not null;index"`
	CredentialVersionID    uuid.UUID                   `gorm:"type:uuid;not null;index"`
	ModelProfileVersionID  uuid.UUID                   `gorm:"type:uuid;not null;index"`
	ProviderKey            string                      `gorm:"type:varchar(80);not null"`
	Modality               string                      `gorm:"type:varchar(20);not null;check:ck_gen_project_provider_binding_modality,modality IN ('image','video')"`
	AdapterContractVersion string                      `gorm:"type:varchar(120);not null"`
	ContentHash            string                      `gorm:"type:char(64);not null;check:ck_gen_project_provider_binding_hash,char_length(content_hash) = 64"`
	CreatedBy              uuid.UUID                   `gorm:"type:uuid;not null"`
	CreatedAt              time.Time                   `gorm:"type:timestamptz;not null"`
	Workspace              Workspace                   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                Project                     `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Connection             ProviderConnectionVersion   `gorm:"foreignKey:ConnectionVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Credential             ProviderCredentialVersion   `gorm:"foreignKey:CredentialVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ModelProfile           ProviderModelProfileVersion `gorm:"foreignKey:ModelProfileVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                UserAccount                 `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ProjectProviderBindingVersion) TableName() string {
	return "gen_project_provider_binding_versions"
}

func (*ProjectProviderBindingVersion) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableProjectProviderBindingVersion
}

func (*ProjectProviderBindingVersion) BeforeDelete(*gorm.DB) error {
	return ErrImmutableProjectProviderBindingVersion
}

type GenerationRequest struct {
	ID                      uuid.UUID                     `gorm:"type:uuid;primaryKey"`
	WorkspaceID             uuid.UUID                     `gorm:"type:uuid;not null;index"`
	ProjectID               uuid.UUID                     `gorm:"type:uuid;not null;index"`
	IntentID                uuid.UUID                     `gorm:"type:uuid;not null;uniqueIndex:uq_gen_request_intent"`
	TargetID                uuid.UUID                     `gorm:"type:uuid;not null;index"`
	BindingID               uuid.UUID                     `gorm:"type:uuid;not null;index"`
	BindingRevision         int64                         `gorm:"not null;check:ck_gen_request_binding_revision,binding_revision >= 1"`
	BindingContentHash      string                        `gorm:"type:char(64);not null;check:ck_gen_request_binding_hash,char_length(binding_content_hash) = 64"`
	Purpose                 string                        `gorm:"type:varchar(32);not null;check:ck_gen_request_purpose,purpose IN ('reference_asset','shot_frame','shot_video')"`
	ProviderKey             string                        `gorm:"type:varchar(80);not null"`
	ExternalModelID         string                        `gorm:"type:varchar(180);not null"`
	ConnectionVersionID     uuid.UUID                     `gorm:"type:uuid;not null;index"`
	CredentialVersionID     uuid.UUID                     `gorm:"type:uuid;not null;index"`
	ModelProfileVersionID   uuid.UUID                     `gorm:"type:uuid;not null;index"`
	ModelProfileRevision    int64                         `gorm:"not null;check:ck_gen_request_profile_revision,model_profile_revision >= 1"`
	ModelProfileContentHash string                        `gorm:"type:char(64);not null;check:ck_gen_request_profile_hash,char_length(model_profile_content_hash) = 64"`
	PriceQuoteID            uuid.UUID                     `gorm:"type:uuid;not null;index"`
	PriceQuoteRevision      int64                         `gorm:"not null;check:ck_gen_request_price_revision,price_quote_revision >= 1"`
	PriceQuoteContentHash   string                        `gorm:"type:char(64);not null;check:ck_gen_request_price_hash,char_length(price_quote_content_hash) = 64"`
	BillingMetric           string                        `gorm:"type:varchar(80);not null;check:ck_gen_request_billing_metric,billing_metric IN ('generation.image.call','generation.video.call')"`
	RequestKey              string                        `gorm:"type:varchar(96);not null;uniqueIndex:uq_gen_request_key"`
	TargetHash              string                        `gorm:"type:char(64);not null;check:ck_gen_request_target_hash,char_length(target_hash) = 64"`
	EstimatedUnits          int64                         `gorm:"not null;check:ck_gen_request_estimated_units,estimated_units > 0"`
	ContentHash             string                        `gorm:"type:char(64);not null;check:ck_gen_request_hash,char_length(content_hash) = 64"`
	CreatedBy               uuid.UUID                     `gorm:"type:uuid;not null"`
	CreatedAt               time.Time                     `gorm:"type:timestamptz;not null"`
	Workspace               Workspace                     `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                 Project                       `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Intent                  GenerationIntent              `gorm:"foreignKey:IntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Target                  GenerationTarget              `gorm:"foreignKey:TargetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Binding                 ProjectProviderBindingVersion `gorm:"foreignKey:BindingID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Connection              ProviderConnectionVersion     `gorm:"foreignKey:ConnectionVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Credential              ProviderCredentialVersion     `gorm:"foreignKey:CredentialVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	ModelProfile            ProviderModelProfileVersion   `gorm:"foreignKey:ModelProfileVersionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	PriceQuote              CostPriceQuote                `gorm:"foreignKey:PriceQuoteID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Creator                 UserAccount                   `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationRequest) TableName() string { return "gen_requests" }

func (*GenerationRequest) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableGenerationRequest
}

func (*GenerationRequest) BeforeDelete(*gorm.DB) error {
	return ErrImmutableGenerationRequest
}

type GenerationProviderJob struct {
	ID                  uuid.UUID         `gorm:"type:uuid;primaryKey"`
	WorkspaceID         uuid.UUID         `gorm:"type:uuid;not null;index"`
	ProjectID           uuid.UUID         `gorm:"type:uuid;not null;index"`
	IntentID            uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_job_intent"`
	RequestID           uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_job_request"`
	ProviderKey         string            `gorm:"type:varchar(80);not null"`
	RequestKey          string            `gorm:"type:varchar(96);not null;uniqueIndex:uq_gen_provider_job_request_key"`
	Status              string            `gorm:"type:varchar(24);not null;index;check:ck_gen_provider_job_state,(status = 'PENDING' AND dispatched_call_count = 0 AND succeeded_call_count = 0 AND failed_call_count = 0) OR (status = 'RUNNING' AND succeeded_call_count + failed_call_count < call_count) OR (status = 'OUTCOME_UNKNOWN' AND dispatched_call_count > 0 AND succeeded_call_count + failed_call_count < call_count) OR (status = 'SUCCEEDED' AND succeeded_call_count = call_count AND failed_call_count = 0) OR (status = 'PARTIAL_SUCCEEDED' AND succeeded_call_count > 0 AND failed_call_count > 0 AND succeeded_call_count + failed_call_count = call_count) OR (status = 'FAILED' AND succeeded_call_count = 0 AND failed_call_count = call_count)"`
	CallCount           int               `gorm:"not null;check:ck_gen_provider_job_call_count,call_count > 0"`
	DispatchedCallCount int               `gorm:"not null;check:ck_gen_provider_job_dispatched_calls,dispatched_call_count >= 0 AND dispatched_call_count <= call_count"`
	SucceededCallCount  int               `gorm:"not null;check:ck_gen_provider_job_succeeded_calls,succeeded_call_count >= 0 AND succeeded_call_count <= call_count"`
	FailedCallCount     int               `gorm:"not null;check:ck_gen_provider_job_failed_calls,failed_call_count >= 0 AND failed_call_count <= call_count"`
	CallSetHash         string            `gorm:"type:char(64);not null;check:ck_gen_provider_job_call_set_hash,char_length(call_set_hash) = 64"`
	Revision            int64             `gorm:"not null;check:ck_gen_provider_job_revision,revision >= 1"`
	ContentHash         string            `gorm:"type:char(64);not null;check:ck_gen_provider_job_hash,char_length(content_hash) = 64"`
	CreatedAt           time.Time         `gorm:"type:timestamptz;not null"`
	UpdatedAt           time.Time         `gorm:"type:timestamptz;not null"`
	Workspace           Workspace         `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project             Project           `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Intent              GenerationIntent  `gorm:"foreignKey:IntentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Request             GenerationRequest `gorm:"foreignKey:RequestID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationProviderJob) TableName() string { return "gen_provider_jobs" }

type GenerationProviderCall struct {
	ID                        uuid.UUID             `gorm:"type:uuid;primaryKey"`
	WorkspaceID               uuid.UUID             `gorm:"type:uuid;not null;index"`
	ProjectID                 uuid.UUID             `gorm:"type:uuid;not null;index"`
	JobID                     uuid.UUID             `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_call_candidate,priority:1"`
	CandidateIndex            int                   `gorm:"not null;uniqueIndex:uq_gen_provider_call_candidate,priority:2;check:ck_gen_provider_call_candidate_index,candidate_index > 0"`
	CallKey                   string                `gorm:"type:varchar(180);not null;uniqueIndex:uq_gen_provider_call_key"`
	RequestHash               string                `gorm:"type:char(64);not null;check:ck_gen_provider_call_request_hash,char_length(request_hash) = 64"`
	RequestedOutputCount      int                   `gorm:"not null;check:ck_gen_provider_call_output_count,requested_output_count = 1"`
	Status                    string                `gorm:"type:varchar(24);not null;index;check:ck_gen_provider_call_state,status IN ('PENDING','DISPATCHING','SUBMITTED','RUNNING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN')"`
	LocalFailureCode          *string               `gorm:"type:varchar(120)"`
	RemoteRequestID           *string               `gorm:"type:varchar(180)"`
	RemoteJobID               *string               `gorm:"type:varchar(180)"`
	DispatchBoundaryEnteredAt *time.Time            `gorm:"type:timestamptz;check:ck_gen_provider_call_boundary,(status = 'PENDING' AND dispatch_boundary_entered_at IS NULL AND query_deadline_at IS NULL AND remote_expires_at IS NULL AND local_failure_code IS NULL AND remote_request_id IS NULL AND remote_job_id IS NULL) OR (status = 'DISPATCHING' AND dispatch_boundary_entered_at IS NOT NULL AND query_deadline_at IS NULL AND remote_expires_at IS NULL AND local_failure_code IS NULL AND remote_request_id IS NULL AND remote_job_id IS NULL) OR (status IN ('SUBMITTED','RUNNING') AND dispatch_boundary_entered_at IS NOT NULL AND query_deadline_at IS NOT NULL AND remote_expires_at IS NOT NULL AND dispatch_boundary_entered_at < query_deadline_at AND query_deadline_at < remote_expires_at AND local_failure_code IS NULL AND (remote_request_id IS NOT NULL OR remote_job_id IS NOT NULL)) OR (status IN ('SUCCEEDED','OUTCOME_UNKNOWN') AND dispatch_boundary_entered_at IS NOT NULL AND local_failure_code IS NULL AND ((query_deadline_at IS NULL AND remote_expires_at IS NULL) OR (query_deadline_at IS NOT NULL AND remote_expires_at IS NOT NULL AND dispatch_boundary_entered_at < query_deadline_at AND query_deadline_at < remote_expires_at))) OR (status = 'FAILED' AND ((dispatch_boundary_entered_at IS NULL AND query_deadline_at IS NULL AND remote_expires_at IS NULL AND local_failure_code IS NOT NULL AND remote_request_id IS NULL AND remote_job_id IS NULL) OR (dispatch_boundary_entered_at IS NOT NULL AND local_failure_code IS NULL AND ((query_deadline_at IS NULL AND remote_expires_at IS NULL) OR (query_deadline_at IS NOT NULL AND remote_expires_at IS NOT NULL AND dispatch_boundary_entered_at < query_deadline_at AND query_deadline_at < remote_expires_at)))))"`
	QueryDeadlineAt           *time.Time            `gorm:"type:timestamptz"`
	RemoteExpiresAt           *time.Time            `gorm:"type:timestamptz"`
	Revision                  int64                 `gorm:"not null;check:ck_gen_provider_call_revision,revision >= 1"`
	ContentHash               string                `gorm:"type:char(64);not null;check:ck_gen_provider_call_hash,char_length(content_hash) = 64"`
	CreatedAt                 time.Time             `gorm:"type:timestamptz;not null"`
	UpdatedAt                 time.Time             `gorm:"type:timestamptz;not null"`
	Workspace                 Workspace             `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                   Project               `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Job                       GenerationProviderJob `gorm:"foreignKey:JobID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationProviderCall) TableName() string { return "gen_provider_calls" }

type GenerationProviderResultReceipt struct {
	ID                       uuid.UUID              `gorm:"type:uuid;primaryKey"`
	WorkspaceID              uuid.UUID              `gorm:"type:uuid;not null;index"`
	ProjectID                uuid.UUID              `gorm:"type:uuid;not null;index"`
	CallID                   uuid.UUID              `gorm:"type:uuid;not null;uniqueIndex:uq_gen_provider_receipt_call"`
	ProviderEventID          *string                `gorm:"type:varchar(180)"`
	Status                   string                 `gorm:"type:varchar(20);not null;check:ck_gen_provider_receipt_status,status IN ('SUCCEEDED','FAILED')"`
	OutputCount              int                    `gorm:"not null;check:ck_gen_provider_receipt_output_count,(status = 'SUCCEEDED' AND output_count = 1) OR (status = 'FAILED' AND output_count = 0)"`
	Output                   datatypes.JSON         `gorm:"type:jsonb;not null"`
	FailureCode              *string                `gorm:"type:varchar(120);check:ck_gen_provider_receipt_result,(status = 'SUCCEEDED' AND failure_code IS NULL) OR (status = 'FAILED' AND failure_code IS NOT NULL)"`
	ProviderUsageObservation datatypes.JSON         `gorm:"type:jsonb;not null"`
	ProviderUsageHash        string                 `gorm:"type:char(64);not null;check:ck_gen_provider_usage_hash,char_length(provider_usage_hash) = 64"`
	ContentHash              string                 `gorm:"type:char(64);not null;check:ck_gen_provider_receipt_hash,char_length(content_hash) = 64"`
	OccurredAt               time.Time              `gorm:"type:timestamptz;not null"`
	ReceivedAt               time.Time              `gorm:"type:timestamptz;not null"`
	Workspace                Workspace              `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Project                  Project                `gorm:"foreignKey:ProjectID,WorkspaceID;references:ID,WorkspaceID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Call                     GenerationProviderCall `gorm:"foreignKey:CallID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (GenerationProviderResultReceipt) TableName() string { return "gen_provider_result_receipts" }

func (*GenerationProviderResultReceipt) BeforeUpdate(*gorm.DB) error {
	return ErrImmutableGenerationProviderResultReceipt
}

func (*GenerationProviderResultReceipt) BeforeDelete(*gorm.DB) error {
	return ErrImmutableGenerationProviderResultReceipt
}
