package application

import (
	"context"

	"github.com/google/uuid"
)

// ExecutionAvailability reads current catalog permission through the quote transaction.
// It excludes user-specific budget and input checks, which operation still owns.
type ExecutionAvailability interface {
	AvailableForNewExecution(context.Context, uuid.UUID, uuid.UUID, string) (bool, error)
}
