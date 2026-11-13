// Package application coordinates authenticated read-only creation analysis.
package application

import (
	"context"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/analytics/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Store owns current-session authorization and a transaction-consistent projection read.
type Store interface {
	ReadCreation(context.Context, identity.Principal, domain.Filter, func() time.Time) ([]domain.Fact, bool, error)
	CheckSession(context.Context, identity.Principal, func() time.Time) error
}

// Query provides bounded analysis without writing business or billing facts.
type Query struct {
	store Store
	clock func() time.Time
}

// NewQuery injects its source and clock.
func NewQuery(store Store, clock func() time.Time) *Query { return &Query{store, clock} }

// Creation requires a fresh source authorization on every request.
func (q *Query) Creation(ctx context.Context, actor identity.Principal, f domain.Filter) (domain.Creation, error) {
	now := q.clock().UTC()
	if err := f.Validate(now); err != nil {
		return domain.Creation{}, err
	}
	facts, truncated, err := q.store.ReadCreation(ctx, actor, f, q.clock)
	if err != nil {
		return domain.Creation{}, err
	}
	return domain.Project(facts, f, now, truncated), nil
}

// Business rechecks the session but exposes no financial detail without finance authority.
func (q *Query) Business(ctx context.Context, actor identity.Principal) (domain.Business, error) {
	if err := q.store.CheckSession(ctx, actor, q.clock); err != nil {
		return domain.Business{}, err
	}
	return domain.UnavailableBusiness(q.clock()), nil
}
