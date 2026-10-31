package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CanvasCharge is one immutable user charge, never a reservation or provider overage.
type CanvasCharge struct {
	ID           uuid.UUID `json:"id"`
	OperationID  uuid.UUID `json:"operation_id"`
	AmountMicros string    `json:"amount_micros"`
	CreatedAt    time.Time `json:"created_at"`
	ModelKey     string    `json:"model_key"`
}

// CanvasUsage contains exact amounts and a bounded page of settled ledger facts.
type CanvasUsage struct {
	Currency      string         `json:"currency" enums:"CNY"`
	SettledMicros string         `json:"settled_micros"`
	TaskCount     string         `json:"task_count"`
	Entries       []CanvasCharge `json:"entries"`
	NextPage      *int           `json:"next_page"`
}

// CanvasUsageReader binds the billing owner to the authorized document transaction.
type CanvasUsageReader interface {
	ReadCanvasUsage(context.Context, identity.Principal, uuid.UUID, uuid.UUID, int) (CanvasUsage, error)
}

// CanvasInformation exposes authoritative metadata separately from the live graph.
type CanvasInformation struct {
	ID        uuid.UUID   `json:"id"`
	ProjectID uuid.UUID   `json:"project_id"`
	Revision  Revision    `json:"revision" swaggertype:"string"`
	OwnerName string      `json:"owner_name"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Usage     CanvasUsage `json:"usage"`
}

// InformationStore supports scoped metadata and accounting without expanding the document write port.
type InformationStore interface {
	Information(context.Context, identity.Principal, uuid.UUID, int) (CanvasInformation, error)
}

// Information validates paging before the authorized consistent read.
func (s *Service) Information(ctx context.Context, a identity.Principal, id uuid.UUID, page int) (CanvasInformation, error) {
	if id == uuid.Nil || page < 1 || page > 10000 {
		return CanvasInformation{}, domain.ErrInvalidCommand
	}
	store, ok := s.store.(InformationStore)
	if !ok {
		return CanvasInformation{}, ErrNotFound
	}
	return store.Information(ctx, a, id, page)
}
