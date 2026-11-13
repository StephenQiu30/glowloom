// Package application coordinates live platform authority with safe diagnostic reads.
package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/domain"
)

// PlatformAccess retains current-session and platform-epoch authority for the read.
type PlatformAccess interface {
	ReadPlatform(context.Context, identity.Principal, func() error) error
}

// Source owns bounded safe request records only.
type Source interface {
	List(domain.Filter, uuid.UUID) (domain.Page, error)
	Overview(domain.Filter, uuid.UUID) (domain.Overview, error)
	Detail(uuid.UUID) (domain.Detail, error)
}

// Query requires authorization for every diagnostic query, including record identity reads.
type Query struct {
	access PlatformAccess
	source Source
	clock  func() time.Time
}

// NewQuery injects authority, records and time without business dependencies.
func NewQuery(access PlatformAccess, source Source, clock func() time.Time) *Query {
	return &Query{access, source, clock}
}

// List rechecks current platform authority even when continuing a signed cursor.
func (q *Query) List(ctx context.Context, a identity.Principal, f domain.Filter) (out domain.Page, err error) {
	if err = f.Validate(q.clock()); err != nil {
		return
	}
	err = q.access.ReadPlatform(ctx, a, func() error { var e error; out, e = q.source.List(f, a.ID); return e })
	return
}

// Overview uses the same safe population and filter contract as List.
func (q *Query) Overview(ctx context.Context, a identity.Principal, f domain.Filter) (out domain.Overview, err error) {
	if err = f.Validate(q.clock()); err != nil {
		return
	}
	err = q.access.ReadPlatform(ctx, a, func() error { var e error; out, e = q.source.Overview(f, a.ID); return e })
	return
}

// Detail does not expose whether an ID exists before checking current authority.
func (q *Query) Detail(ctx context.Context, a identity.Principal, id uuid.UUID) (out domain.Detail, err error) {
	if id == uuid.Nil {
		return out, domain.ErrInvalidFilter
	}
	err = q.access.ReadPlatform(ctx, a, func() error { var e error; out, e = q.source.Detail(id); return e })
	return
}
