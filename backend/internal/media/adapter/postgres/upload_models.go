package postgres

import (
	"time"

	"github.com/google/uuid"
)

type uploadRequestRecord struct {
	ProjectID     *uuid.UUID
	PersonalOrgID *uuid.UUID
	PrincipalID   uuid.UUID
	RequestKey    uuid.UUID
	SHA256        string
	FileName      string
	ByteSize      int64
	AssetID       uuid.UUID
	Response      jsonDocument
}

func (uploadRequestRecord) TableName() string { return "media.upload_request" }

type uploadAuditRecord struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	ProjectID  *uuid.UUID
	ActorID    uuid.UUID
	ActorKind  string
	Action     string
	ObjectType string
	ObjectID   string
	After      jsonDocument
	RequestID  string
	CreateTime time.Time
	UpdateTime time.Time
}

func (uploadAuditRecord) TableName() string { return "audit.audit_log" }
