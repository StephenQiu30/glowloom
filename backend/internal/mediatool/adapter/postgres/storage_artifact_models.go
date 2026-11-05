package postgres

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
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
		return nil, nil
	}
	return string(j), nil
}

func (jsonDocument) GormDataType() string { return "jsonb" }

type depthResultIntentRecord struct {
	JobID          uuid.UUID `gorm:"primaryKey"`
	Attempt        int       `gorm:"primaryKey"`
	AssetID        uuid.UUID
	OutputSHA256   string
	ArtifactSHA256 string
	Artifact       jsonDocument
	CreatedAt      time.Time
}

func (depthResultIntentRecord) TableName() string {
	return "mediatool.depth_result_intent"
}

type depthObjectRecord struct {
	JobID            uuid.UUID                   `gorm:"primaryKey"`
	Attempt          int                         `gorm:"primaryKey"`
	Kind             application.DepthObjectKind `gorm:"primaryKey"`
	ObjectKey        string
	SHA256           string
	ByteSize         int64
	MIMEType         string
	WriteStarted     bool
	DeleteStarted    bool
	Status           application.DepthObjectStatus
	UpdatedAt        time.Time
	ObjectLocationID *uuid.UUID
}

func (depthObjectRecord) TableName() string { return "mediatool.depth_object" }

func (r depthObjectRecord) object() application.DepthObject {
	return application.DepthObject{
		Kind: r.Kind, ObjectKey: r.ObjectKey, SHA256: r.SHA256, ByteSize: r.ByteSize,
		MIMEType: r.MIMEType, WriteStarted: r.WriteStarted, DeleteStarted: r.DeleteStarted,
		Status: r.Status, ObjectLocationID: r.ObjectLocationID,
	}
}
