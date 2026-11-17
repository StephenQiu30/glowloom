package aigateway

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"

	"github.com/google/uuid"

	pg "github.com/StephenQiu30/glowloom-server/backend/execution/gateway/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// RegisterWorker reuses Go Execution leases for dispatch and evidence-only settlement.
func (r *Runtime) RegisterWorker(handlers map[string]taskruntime.Handler) {
	handlers["ai.recover"] = func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil {
			return time.Time{}, err
		}
		return pg.New(r.DB, identity.Principal{}, nil).RecoverStale(ctx, id)
	}
	handlers["ai.deliver"] = func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil {
			return time.Time{}, err
		}
		if err = r.Deliver(ctx, id); err != nil {
			return time.Now().Add(5 * time.Second), err
		}
		return time.Time{}, nil
	}
	handlers["ai.poll"] = func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil {
			return time.Time{}, err
		}
		// Status reads are evidence recovery, independent of a user's later session revocation.
		store := pg.New(r.DB, identity.Principal{ID: uuid.NameSpaceOID}, func(context.Context, *gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, bool) error { return nil })
		d, err := store.PendingDispatch(ctx, id)
		if err != nil {
			return time.Now().Add(5 * time.Second), err
		}
		if !d.Send {
			if d.Record.State == "received" {
				_, err = store.Settle(ctx, id)
			}
			return time.Time{}, err
		}
		if r.Provider == nil {
			return time.Now().Add(5 * time.Second), domain.ErrUnavailable
		}
		receipt, err := r.Provider.Send(ctx, d, nil)
		if err != nil {
			_ = store.Unknown(ctx, id, d.Attempt)
			return time.Now().Add(30 * time.Second), nil
		}
		if receipt.Pending {
			return time.Now().Add(5 * time.Second), nil
		}
		if err = store.SaveReceipt(ctx, id, d.Attempt, receipt); err != nil {
			return time.Now().Add(5 * time.Second), err
		}
		_, err = store.Settle(ctx, id)
		if errors.Is(err, domain.ErrUnknown) {
			return time.Now().Add(5 * time.Second), nil
		}
		return time.Time{}, err
	}
	handlers[pg.ExecutionKind] = (Creation{Runtime: r}).Execute
	handlers["ai.settle"] = func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		id, err := uuid.Parse(job.BusinessID)
		if err != nil {
			return time.Time{}, err
		}
		_, err = pg.New(r.DB, identity.Principal{}, nil).Settle(ctx, id)
		if err != nil {
			return time.Now().Add(time.Second), err
		}
		return time.Time{}, nil
	}
}
