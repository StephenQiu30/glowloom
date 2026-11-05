package postgres

import (
	"time"

	"github.com/google/uuid"
)

type libraryIdentityRecord struct {
	ID              uuid.UUID
	Kind            string
	OrgID           uuid.UUID
	ProjectID       *uuid.UUID
	PersonalActorID *uuid.UUID
	Revision        int64
}

func (libraryIdentityRecord) TableName() string { return "media.library" }

type libraryCommandRecord struct {
	ID            uuid.UUID
	OrgID         uuid.UUID
	ActorID       uuid.UUID
	LibraryID     uuid.UUID
	IdemKey       uuid.UUID
	Action        string
	RequestSHA256 string
	ResponseBody  []byte
	CreateTime    time.Time
}

func (libraryCommandRecord) TableName() string { return "media.library_command" }

type libraryAuditRecord struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	ProjectID  *uuid.UUID
	ActorID    uuid.UUID
	ActorKind  string
	Action     string
	ObjectType string
	ObjectID   string
	After      jsonDocument
	CreateTime time.Time
	UpdateTime time.Time
}

func (libraryAuditRecord) TableName() string { return "audit.audit_log" }
