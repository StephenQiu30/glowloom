package postgres

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// WorkerPermit snapshots a caller-owned authorization after durable funds admission.
type WorkerPermit func(context.Context, *gorm.DB, domain.Call) (json.RawMessage, error)

// WithWorkerPermit adds a frozen worker proof, without conferring scope or funds authority.
func (s *Store) WithWorkerPermit(permit WorkerPermit) *Store {
	bound := *s
	bound.workerPermit = permit
	return &bound
}
