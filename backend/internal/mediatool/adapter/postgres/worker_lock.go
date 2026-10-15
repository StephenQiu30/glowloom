package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

// lockWorkerJob follows the HTTP order: current identity/project authority,
// then the job row. The unlocked candidate supplies no publishing authority;
// its ownership binding is checked again under the job lock.
func (s *Store) lockWorkerJob(ctx context.Context, tx *gorm.DB, id uuid.UUID, runningOnly bool) (jobRow, error) {
	candidate, err := readJob(tx, uuid.Nil, id, false)
	if err != nil {
		return jobRow{}, err
	}
	authorized := !runningOnly || candidate.Status == "running"
	if authorized {
		if err := s.authorize(ctx, tx, candidate.actor(), candidate.ProjectID, true); err != nil {
			return jobRow{}, err
		}
	}
	row, err := readJob(tx, candidate.ProjectID, id, true)
	if err != nil {
		return jobRow{}, err
	}
	if row.ActorID != candidate.ActorID || row.OrgID != candidate.OrgID || row.ProjectID != candidate.ProjectID || !authorized && row.Status == "running" {
		return jobRow{}, application.ErrConflict
	}
	return row, nil
}
