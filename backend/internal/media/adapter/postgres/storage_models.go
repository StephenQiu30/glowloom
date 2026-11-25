package postgres

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type jsonDocument []byte

func (j *jsonDocument) Scan(value any) error {
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

func (j jsonDocument) Value() (driver.Value, error) {
	if j == nil {
		return "null", nil
	}
	return string(j), nil
}

func (jsonDocument) GormDataType() string { return "jsonb" }

type storageProbeEvidenceRecord struct {
	ID               uuid.UUID `gorm:"primaryKey"`
	ProfileID        string
	ResourceIdentity string
	ConfigSHA256     string
	EvidenceSHA256   string
	ExpiresAt        time.Time
	RecordedAt       time.Time
}

func (storageProbeEvidenceRecord) TableName() string { return "media.storage_probe_evidence" }

type storageBucketEvidenceRecord struct {
	ID               uuid.UUID `gorm:"primaryKey"`
	ProfileID        string
	ResourceIdentity string
	ConfigSHA256     string
	EvidenceSHA256   string
	Contract         string
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

func (storageBucketEvidenceRecord) TableName() string { return "media.storage_bucket_evidence" }

type storageProbeIntentRecord struct {
	ID             uuid.UUID `gorm:"primaryKey"`
	ProfileID      string
	Bucket         string
	ConfigSHA256   string
	SentinelKey    string
	RemovalKey     *string
	Mode           string
	State          application.StorageProbeState
	EvidenceSHA256 *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (storageProbeIntentRecord) TableName() string { return "media.storage_probe_intent" }

type storageEmptyDeploymentEvidenceRecord struct {
	ConfigSHA256   string `gorm:"primaryKey"`
	EvidenceSHA256 string
	Inventory      jsonDocument
	RecordedAt     time.Time
}

func (storageEmptyDeploymentEvidenceRecord) TableName() string {
	return "media.storage_empty_deployment_evidence"
}

const storageSchemaVersionCurrent = 2

type storageSchemaVersionRecord struct {
	Version   int `gorm:"primaryKey"`
	AppliedAt time.Time
}

func (storageSchemaVersionRecord) TableName() string { return "media.storage_schema_version" }

type storageIdempotencyLockRecord struct {
	Namespace  string    `gorm:"primaryKey"`
	ActorID    uuid.UUID `gorm:"primaryKey"`
	RequestKey uuid.UUID `gorm:"primaryKey"`
	CreatedAt  time.Time
}

func (storageIdempotencyLockRecord) TableName() string { return "media.storage_idempotency_lock" }

type storageConfigRevisionRecord struct {
	Revision          string `gorm:"primaryKey"`
	ConfigSHA256      string
	ProfileID         string
	UploadPolicy      jsonDocument
	SessionTTLSeconds int64
	CreatedAt         time.Time
}

func (storageConfigRevisionRecord) TableName() string { return "media.storage_config_revision" }
