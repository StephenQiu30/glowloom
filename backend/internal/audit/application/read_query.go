package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
)

var (
	// ErrForbidden means the actor cannot read organization audit records.
	ErrForbidden = errors.New("audit read forbidden")
	// ErrInvalidRead means the requested record or page filter is invalid.
	ErrInvalidRead = errors.New("invalid audit read query")
)

// ReadStore checks current administrator rights in the same transaction as the read.
type ReadStore interface {
	ListForAdmin(context.Context, uuid.UUID, uuid.UUID, domain.Filter) (domain.Page, error)
	GetForAdmin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (domain.Record, error)
}

// ReadQuery exposes bounded, organization-scoped audit reads to adapters.
type ReadQuery struct{ store ReadStore }

// NewReadQuery injects the administrator-scoped reader.
func NewReadQuery(store ReadStore) *ReadQuery { return &ReadQuery{store: store} }

// List returns a keyset page after the store rechecks current administrator rights.
func (q *ReadQuery) List(ctx context.Context, actorID, orgID uuid.UUID, filter domain.Filter) (domain.Page, error) {
	if actorID == uuid.Nil || orgID == uuid.Nil {
		return domain.Page{}, ErrForbidden
	}
	if q == nil || q.store == nil || filter.Limit < 0 || filter.Limit > 100 ||
		(!filter.From.IsZero() && !filter.To.IsZero() && !filter.From.Before(filter.To)) ||
		(filter.Before != nil && (filter.Before.ID == uuid.Nil || filter.Before.CreateTime.IsZero())) {
		return domain.Page{}, ErrInvalidRead
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	page, err := q.store.ListForAdmin(ctx, actorID, orgID, filter)
	if err != nil {
		return domain.Page{}, fmt.Errorf("list audit records: %w", err)
	}
	return page, nil
}

// Get returns one organization record only after current authorization.
func (q *ReadQuery) Get(ctx context.Context, actorID, orgID, recordID uuid.UUID) (domain.Record, error) {
	if actorID == uuid.Nil || orgID == uuid.Nil {
		return domain.Record{}, ErrForbidden
	}
	if q == nil || q.store == nil || recordID == uuid.Nil {
		return domain.Record{}, ErrInvalidRead
	}
	record, err := q.store.GetForAdmin(ctx, actorID, orgID, recordID)
	if err != nil {
		return domain.Record{}, fmt.Errorf("get audit record: %w", err)
	}
	return record, nil
}
