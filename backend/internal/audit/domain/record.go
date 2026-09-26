// Package domain defines the immutable audit facts and scoped query values.
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Record is an append-only summary of one action.
type Record struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	ProjectID  *uuid.UUID
	ActorID    *uuid.UUID
	ActorKind  string
	Action     string
	ObjectType string
	ObjectID   string
	Before     json.RawMessage
	After      json.RawMessage
	RequestID  string
	TraceID    string
	IP         string
	CreateTime time.Time
}

// Cursor identifies the last record of a page in newest-first order.
type Cursor struct {
	CreateTime time.Time
	ID         uuid.UUID
}

// Filter narrows an organization-scoped audit query.
type Filter struct {
	From       time.Time
	To         time.Time
	ActorID    *uuid.UUID
	ProjectID  *uuid.UUID
	ObjectType string
	Action     string
	Before     *Cursor
	Limit      int
}

// Page contains one page of records and a cursor when more records exist.
type Page struct {
	Items []Record
	Next  *Cursor
}
