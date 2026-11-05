package postgres

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

const (
	scriptCommandPending   = domain.CommandStatusPending
	scriptCommandCompleted = domain.CommandStatusCompleted
	scriptCommandCancelled = domain.CommandStatusCancelled
)

const (
	scriptIOIdle    = domain.CommandIOStateIdle
	scriptIORunning = domain.CommandIOStateRunning
	scriptIOEnded   = domain.CommandIOStateEnded
)

type scriptJSON []byte

func (j *scriptJSON) Scan(value any) error {
	switch value := value.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], value...)
	case string:
		*j = append((*j)[:0], value...)
	default:
		return fmt.Errorf("scan JSON document from %T", value)
	}
	return nil
}

func (j scriptJSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "null", nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid JSON document")
	}
	return string(j), nil
}

func (scriptJSON) GormDataType() string { return "jsonb" }

type intArray []int

func (a intArray) Value() (driver.Value, error) {
	values := make([]string, len(a))
	for i, value := range a {
		values[i] = strconv.Itoa(value)
	}
	return "{" + strings.Join(values, ",") + "}", nil
}

func (a *intArray) Scan(value any) error {
	var raw string
	switch value := value.(type) {
	case nil:
		*a = nil
		return nil
	case []byte:
		raw = string(value)
	case string:
		raw = value
	default:
		return fmt.Errorf("scan integer array from %T", value)
	}
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return fmt.Errorf("invalid integer array")
	}
	contents := raw[1 : len(raw)-1]
	if contents == "" {
		*a = intArray{}
		return nil
	}
	parts := strings.Split(contents, ",")
	result := make(intArray, len(parts))
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid integer array: %w", err)
		}
		result[i] = value
	}
	*a = result
	return nil
}

func (intArray) GormDataType() string { return "integer[]" }

type scriptObjectIntentRow struct {
	ActorID          uuid.UUID `gorm:"column:actor_id"`
	RequestID        uuid.UUID `gorm:"column:request_id"`
	OrgID            uuid.UUID `gorm:"column:org_id"`
	ProjectID        uuid.UUID `gorm:"column:project_id"`
	ObjectKey        string    `gorm:"column:object_key;primaryKey"`
	SHA256           string    `gorm:"column:sha256"`
	ByteSize         int64     `gorm:"column:byte_size"`
	MIME             string    `gorm:"column:mime"`
	ObjectLocationID uuid.UUID `gorm:"column:object_location_id;not null"`
}

func (scriptObjectIntentRow) TableName() string { return "script.object_intent" }

type scriptObjectStateRow struct {
	ObjectKey        string    `gorm:"column:object_key;primaryKey"`
	PutStarted       bool      `gorm:"column:put_started"`
	Confirmed        bool      `gorm:"column:confirmed"`
	Removed          bool      `gorm:"column:removed"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
	ObjectLocationID uuid.UUID `gorm:"column:object_location_id;not null"`
}

func (scriptObjectStateRow) TableName() string { return "script.object_state" }

type scriptCopyObjectIntentRow struct {
	TargetKey        string    `gorm:"column:target_key;primaryKey"`
	SnapshotID       uuid.UUID `gorm:"column:snapshot_id"`
	SourceKey        string    `gorm:"column:source_key"`
	SHA256           string    `gorm:"column:sha256"`
	ByteSize         int64     `gorm:"column:byte_size"`
	MIME             string    `gorm:"column:mime"`
	SourceLocationID uuid.UUID `gorm:"column:source_location_id;not null"`
	TargetLocationID uuid.UUID `gorm:"column:target_location_id;not null"`
}

func (scriptCopyObjectIntentRow) TableName() string { return "script.copy_object_intent" }

type scriptCopyObjectStateRow struct {
	TargetKey        string    `gorm:"column:target_key;primaryKey"`
	PutStarted       bool      `gorm:"column:put_started"`
	Confirmed        bool      `gorm:"column:confirmed"`
	Removed          bool      `gorm:"column:removed"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
	TargetLocationID uuid.UUID `gorm:"column:target_location_id;not null"`
}

func (scriptCopyObjectStateRow) TableName() string { return "script.copy_object_state" }

type scriptCopySnapshotRow struct {
	ID              uuid.UUID  `gorm:"column:id;primaryKey"`
	JobID           uuid.UUID  `gorm:"column:job_id"`
	OrgID           uuid.UUID  `gorm:"column:org_id"`
	SourceProjectID uuid.UUID  `gorm:"column:source_project_id"`
	TargetProjectID uuid.UUID  `gorm:"column:target_project_id"`
	Manifest        scriptJSON `gorm:"column:manifest"`
	ManifestSHA256  string     `gorm:"column:manifest_sha256"`
	ContentSHA256   string     `gorm:"column:content_sha256"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	StorageIntentID *uuid.UUID `gorm:"column:storage_intent_id"`
}

func (scriptCopySnapshotRow) TableName() string { return "script.copy_snapshot" }

type scriptCopyReceiptRow struct {
	SnapshotID uuid.UUID  `gorm:"column:snapshot_id;primaryKey"`
	Receipt    scriptJSON `gorm:"column:receipt"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (scriptCopyReceiptRow) TableName() string { return "script.copy_receipt" }

type scriptCommandRow struct {
	ActorID     uuid.UUID  `gorm:"column:actor_id"`
	RequestID   uuid.UUID  `gorm:"column:request_id"`
	OrgID       uuid.UUID  `gorm:"column:org_id"`
	ProjectID   uuid.UUID  `gorm:"column:project_id"`
	Action      string     `gorm:"column:action"`
	RequestHash string     `gorm:"column:request_hash"`
	Plan        scriptJSON `gorm:"column:plan"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

func (scriptCommandRow) TableName() string { return "script.command" }

type scriptRequestRow struct {
	ActorID     uuid.UUID `gorm:"column:actor_id;primaryKey"`
	RequestID   uuid.UUID `gorm:"column:request_id;primaryKey"`
	OrgID       uuid.UUID `gorm:"column:org_id"`
	ProjectID   uuid.UUID `gorm:"column:project_id"`
	Action      string    `gorm:"column:action"`
	RequestHash string    `gorm:"column:request_hash"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (scriptRequestRow) TableName() string { return "script.request" }

type scriptCommandResultRow struct {
	ActorID   uuid.UUID  `gorm:"column:actor_id;primaryKey"`
	RequestID uuid.UUID  `gorm:"column:request_id;primaryKey"`
	Response  scriptJSON `gorm:"column:response"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (scriptCommandResultRow) TableName() string { return "script.command_result" }

type scriptCommandStateRow struct {
	ActorID               uuid.UUID             `gorm:"column:actor_id"`
	RequestID             uuid.UUID             `gorm:"column:request_id"`
	ID                    uuid.UUID             `gorm:"column:id;primaryKey"`
	Revision              int64                 `gorm:"column:revision"`
	Status                domain.CommandStatus  `gorm:"column:status"`
	FailureCode           string                `gorm:"column:failure_code"`
	CancellationRequested bool                  `gorm:"column:cancellation_requested"`
	NeedsReconciliation   bool                  `gorm:"column:needs_reconciliation"`
	IOOwnerID             *uuid.UUID            `gorm:"column:io_owner_id"`
	IOState               domain.CommandIOState `gorm:"column:io_state"`
	UpdatedAt             time.Time             `gorm:"column:updated_at"`
}

func (scriptCommandStateRow) TableName() string { return "script.command_state" }

type scriptSourceControlRow struct {
	ActorID          uuid.UUID  `gorm:"column:actor_id"`
	RequestID        uuid.UUID  `gorm:"column:request_id"`
	OrgID            uuid.UUID  `gorm:"column:org_id"`
	ProjectID        uuid.UUID  `gorm:"column:project_id"`
	IntentID         uuid.UUID  `gorm:"column:intent_id"`
	Action           string     `gorm:"column:action"`
	ExpectedRevision int64      `gorm:"column:expected_revision"`
	RequestHash      string     `gorm:"column:request_hash"`
	Response         scriptJSON `gorm:"column:response"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (scriptSourceControlRow) TableName() string { return "script.source_control" }

type scriptImportJobRow struct {
	ID                     uuid.UUID  `gorm:"column:id;primaryKey"`
	ActorID                uuid.UUID  `gorm:"column:actor_id"`
	OrgID                  uuid.UUID  `gorm:"column:org_id"`
	ProjectID              uuid.UUID  `gorm:"column:project_id"`
	ActorRole              *string    `gorm:"column:actor_role"`
	RequestID              uuid.UUID  `gorm:"column:request_id"`
	RequestHash            string     `gorm:"column:request_hash"`
	ExpectedScriptRevision int64      `gorm:"column:expected_script_revision"`
	BaseVersionID          *uuid.UUID `gorm:"column:base_version_id"`
	RightsConfirmedAt      time.Time  `gorm:"column:rights_confirmed_at"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
}

func (scriptImportJobRow) TableName() string { return "script.import_job" }

type scriptImportAttemptRow struct {
	JobID                  uuid.UUID  `gorm:"column:job_id;primaryKey"`
	Attempt                int        `gorm:"column:attempt;primaryKey"`
	ExpectedScriptRevision int64      `gorm:"column:expected_script_revision"`
	BaseVersionID          *uuid.UUID `gorm:"column:base_version_id"`
	PublicationKey         uuid.UUID  `gorm:"column:publication_key"`
	Positions              intArray   `gorm:"column:positions"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
}

func (scriptImportAttemptRow) TableName() string { return "script.import_attempt" }

type scriptImportStateRow struct {
	JobID                   uuid.UUID            `gorm:"column:job_id;primaryKey"`
	Revision                int64                `gorm:"column:revision"`
	Attempt                 int                  `gorm:"column:attempt"`
	Status                  domain.ImportStatus  `gorm:"column:status"`
	Stage                   domain.ImportStage   `gorm:"column:stage"`
	LatestScriptRevision    int64                `gorm:"column:latest_script_revision"`
	LatestVersionID         *uuid.UUID           `gorm:"column:latest_version_id"`
	CancellationRequested   bool                 `gorm:"column:cancellation_requested"`
	ReconciliationRequested bool                 `gorm:"column:reconciliation_requested"`
	NeedsReconciliation     bool                 `gorm:"column:needs_reconciliation"`
	FailureCode             string               `gorm:"column:failure_code"`
	IOOwnerID               *uuid.UUID           `gorm:"column:io_owner_id"`
	IOState                 domain.ImportIOState `gorm:"column:io_state"`
	UpdatedAt               time.Time            `gorm:"column:updated_at"`
}

func (scriptImportStateRow) TableName() string { return "script.import_state" }

type scriptImportFileRow struct {
	JobID         uuid.UUID `gorm:"column:job_id;primaryKey"`
	Position      int       `gorm:"column:position;primaryKey"`
	SourceID      uuid.UUID `gorm:"column:source_id"`
	AssetID       uuid.UUID `gorm:"column:asset_id"`
	ProjectID     uuid.UUID `gorm:"column:project_id"`
	MediaRevision int64     `gorm:"column:media_revision"`
	SHA256        string    `gorm:"column:sha256"`
	ByteSize      int64     `gorm:"column:byte_size"`
	MIME          string    `gorm:"column:mime"`
	FileName      string    `gorm:"column:file_name"`
}

func (scriptImportFileRow) TableName() string { return "script.import_file" }

type scriptImportFileResultRow struct {
	JobID     uuid.UUID  `gorm:"column:job_id;primaryKey"`
	Attempt   int        `gorm:"column:attempt;primaryKey"`
	Position  int        `gorm:"column:position;primaryKey"`
	Result    scriptJSON `gorm:"column:result"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (scriptImportFileResultRow) TableName() string { return "script.import_file_result" }

type scriptImportPublicationRow struct {
	JobID     uuid.UUID  `gorm:"column:job_id;primaryKey"`
	Attempt   int        `gorm:"column:attempt;primaryKey"`
	Response  scriptJSON `gorm:"column:response"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (scriptImportPublicationRow) TableName() string { return "script.import_publication" }

type scriptImportCommandRow struct {
	ActorID          uuid.UUID  `gorm:"column:actor_id;primaryKey"`
	RequestID        uuid.UUID  `gorm:"column:request_id;primaryKey"`
	OrgID            uuid.UUID  `gorm:"column:org_id"`
	ProjectID        uuid.UUID  `gorm:"column:project_id"`
	JobID            uuid.UUID  `gorm:"column:job_id"`
	Action           string     `gorm:"column:action"`
	RequestHash      string     `gorm:"column:request_hash"`
	ExpectedRevision int64      `gorm:"column:expected_revision"`
	EventID          uuid.UUID  `gorm:"column:event_id"`
	EventAction      string     `gorm:"column:event_action"`
	EventAttempt     int        `gorm:"column:event_attempt"`
	Response         scriptJSON `gorm:"column:response"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (scriptImportCommandRow) TableName() string { return "script.import_command" }

type scriptOutboxRow struct {
	ID           uuid.UUID  `gorm:"column:id;primaryKey"`
	Topic        string     `gorm:"column:topic"`
	PartitionKey string     `gorm:"column:partition_key"`
	Payload      scriptJSON `gorm:"column:payload"`
}

func (scriptOutboxRow) TableName() string { return "infra.outbox" }

type scriptReviewCommandRow struct {
	ActorID     uuid.UUID  `gorm:"column:actor_id;primaryKey"`
	RequestID   uuid.UUID  `gorm:"column:request_id;primaryKey"`
	OrgID       uuid.UUID  `gorm:"column:org_id"`
	ProjectID   uuid.UUID  `gorm:"column:project_id"`
	Action      string     `gorm:"column:action"`
	RequestHash string     `gorm:"column:request_hash"`
	Response    scriptJSON `gorm:"column:response"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

func (scriptReviewCommandRow) TableName() string { return "script.review_command" }
