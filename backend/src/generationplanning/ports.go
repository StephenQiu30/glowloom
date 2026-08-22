package generationplanning

import (
	"context"

	"github.com/google/uuid"
)

type GenerationPlanStore interface {
	CreatePlan(context.Context, Plan, []Item) error
	GetPlan(context.Context, uuid.UUID) (Plan, []Item, error)
	Preflight(context.Context, uuid.UUID) (Plan, []Item, error)
	Approve(context.Context, uuid.UUID, string, []uuid.UUID) (Plan, []Item, error)
}
