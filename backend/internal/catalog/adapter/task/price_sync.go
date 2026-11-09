package task

import (
	"context"
	"errors"
	"time"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// PriceSync is the durable periodic public-price synchronization task kind.
const PriceSync = "catalog.price_sync"

// PriceCandidateSynchronizer performs one bounded public catalog sweep.
type PriceCandidateSynchronizer interface {
	Synchronize(context.Context) (catalogapp.PriceCandidateSyncResult, error)
}

// PriceSyncHandler runs only the persisted periodic cursor and never publishes prices.
func PriceSyncHandler(syncer PriceCandidateSynchronizer) taskruntime.Handler {
	return func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		if syncer == nil || job.BusinessID != "periodic" {
			return time.Time{}, errors.New("invalid periodic price sync task")
		}
		if _, err := syncer.Synchronize(ctx); err != nil {
			return time.Time{}, err
		}
		return nextPriceSyncUTC(time.Now().UTC()), nil
	}
}

func nextPriceSyncUTC(now time.Time) time.Time {
	next := now.UTC().Truncate(24 * time.Hour).Add(3 * time.Hour)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
