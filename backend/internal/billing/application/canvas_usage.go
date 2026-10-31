package application

import (
	"time"

	"github.com/google/uuid"
)

// CanvasCharge is a user settlement fact scoped by an operation's frozen canvas origin.
type CanvasCharge struct {
	ID, OperationID uuid.UUID
	AmountMicros    string
	CreatedAt       time.Time
	ModelKey        string
}

// CanvasUsage is the exact CNY total and bounded settlement page.
type CanvasUsage struct {
	SettledMicros, TaskCount string
	Entries                  []CanvasCharge
	NextPage                 *int
}
