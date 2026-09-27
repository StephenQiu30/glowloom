package domain

import "github.com/google/uuid"

// Budget is the project's current spending envelope.
type Budget struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	LimitMicros    int64
	ReservedMicros int64
	SettledMicros  int64
	IsOverrun      bool
	Revision       int64
}
