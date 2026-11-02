package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProjectBudgetView exposes current accounting facts without ledger identities.
type ProjectBudgetView struct {
	ProjectID       uuid.UUID `json:"project_id"`
	LimitMicros     int64     `json:"limit_micros"`
	ReservedMicros  int64     `json:"reserved_micros"`
	SettledMicros   int64     `json:"settled_micros"`
	AvailableMicros int64     `json:"available_micros"`
	Revision        int64     `json:"revision"`
}

// ProjectBudgetReader rechecks project and session rights before returning a budget.
type ProjectBudgetReader interface {
	FindBudget(context.Context, identity.Principal, uuid.UUID) (billing.Budget, error)
}

// ProjectBudgetQuery derives a validated, project-bound budget snapshot.
type ProjectBudgetQuery struct{ store ProjectBudgetReader }

// NewProjectBudgetQuery injects the existing authorized budget reader.
func NewProjectBudgetQuery(store ProjectBudgetReader) *ProjectBudgetQuery {
	return &ProjectBudgetQuery{store: store}
}

// Read validates arithmetic and binding; this is a snapshot, never a reservation.
func (q *ProjectBudgetQuery) Read(ctx context.Context, actor identity.Principal, id uuid.UUID) (ProjectBudgetView, error) {
	if id == uuid.Nil {
		return ProjectBudgetView{}, ErrInvalidProjectChange
	}
	budget, err := q.store.FindBudget(ctx, actor, id)
	if err != nil {
		return ProjectBudgetView{}, err
	}
	if budget.ProjectID != id {
		return ProjectBudgetView{}, ErrProjectNotFound
	}
	available, err := budget.AvailableMicros()
	if err != nil {
		return ProjectBudgetView{}, fmt.Errorf("read project budget: %w", err)
	}
	return ProjectBudgetView{ProjectID: id, LimitMicros: budget.LimitMicros, ReservedMicros: budget.ReservedMicros, SettledMicros: budget.SettledMicros, AvailableMicros: available, Revision: budget.Revision}, nil
}
