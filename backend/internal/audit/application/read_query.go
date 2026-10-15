package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/audit/domain"
)

var (
	// ErrForbidden means the actor cannot read organization audit records.
	ErrForbidden = errors.New("audit read forbidden")
	// ErrInvalidRead means the requested record or page filter is invalid.
	ErrInvalidRead = errors.New("invalid audit read query")
)

// ReadStore checks current administrator rights in the same transaction as the read.
type ReadStore interface {
	ListForAdmin(context.Context, identityapp.Principal, uuid.UUID, domain.Filter) (domain.Page, error)
	GetForAdmin(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.Record, error)
}

// ReadQuery exposes bounded, organization-scoped audit reads to adapters.
type ReadQuery struct{ store ReadStore }

// NewReadQuery injects the administrator-scoped reader.
func NewReadQuery(store ReadStore) *ReadQuery { return &ReadQuery{store: store} }

// List returns a keyset page after the store rechecks current administrator rights.
func (q *ReadQuery) List(ctx context.Context, actor identityapp.Principal, orgID uuid.UUID, filter domain.Filter) (domain.Page, error) {
	if actor.ID == uuid.Nil || orgID == uuid.Nil {
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
	page, err := q.store.ListForAdmin(ctx, actor, orgID, filter)
	if err != nil {
		return domain.Page{}, fmt.Errorf("list audit records: %w", err)
	}
	return page, nil
}

// Get returns one organization record only after current authorization.
func (q *ReadQuery) Get(ctx context.Context, actor identityapp.Principal, orgID, recordID uuid.UUID) (domain.Record, error) {
	if actor.ID == uuid.Nil || orgID == uuid.Nil {
		return domain.Record{}, ErrForbidden
	}
	if q == nil || q.store == nil || recordID == uuid.Nil {
		return domain.Record{}, ErrInvalidRead
	}
	record, err := q.store.GetForAdmin(ctx, actor, orgID, recordID)
	if err != nil {
		return domain.Record{}, fmt.Errorf("get audit record: %w", err)
	}
	return record, nil
}

// ActorAccess verifies the exact platform session before reading metadata.
type ActorAccess interface {
	CheckPlatformSession(context.Context, identityapp.Principal, time.Time) error
}
