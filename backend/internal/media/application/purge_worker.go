package application

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// PurgeWorkerRepository binds every read/receipt to one actual physical fence.
type PurgeWorkerRepository interface {
	ClaimPurge(context.Context, PurgeWorkID) (PurgeLease, error)
	HeartbeatPurge(context.Context, PurgeLease) error
	StartPurgeItem(context.Context, PurgeLease, int) (bool, error)
	PurgeObjects(context.Context, PurgeLease, int) ([]PurgeObject, error)
	RecordPurgeDigest(context.Context, PurgeLease, int, string, string, int64) error
	BeginPurgeRemoval(context.Context, PurgeLease, int, string) error
	ConfirmPurgeRemoval(context.Context, PurgeLease, int, string) error
	CompletePurgeItem(context.Context, PurgeLease, int) error
	FailPurgeItem(context.Context, PurgeLease, int, string) error
	EndPurgePhysical(context.Context, PurgeLease) error
	FinishPurge(context.Context, PurgeLease) (domain.PurgeJob, error)
}

// PurgeWorker executes only exact media-owned reservations. Its heartbeat and
// object calls are cancelled and joined before any physical end is recorded.
type PurgeWorker struct {
	repo    PurgeWorkerRepository
	objects PurgeObjects
}

// NewPurgeWorker explicitly injects durable owning state and private object I/O.
func NewPurgeWorker(repo PurgeWorkerRepository, objects PurgeObjects) *PurgeWorker {
	return &PurgeWorker{repo: repo, objects: objects}
}

type purgeItemIntents struct {
	repo  PurgeWorkerRepository
	lease PurgeLease
	index int
}

func (p purgeItemIntents) RecordPurgeDigest(ctx context.Context, key, sha string, size int64) error {
	return p.repo.RecordPurgeDigest(ctx, p.lease, p.index, key, sha, size)
}
func (p purgeItemIntents) BeginPurgeRemoval(ctx context.Context, key string) error {
	return p.repo.BeginPurgeRemoval(ctx, p.lease, p.index, key)
}
func (p purgeItemIntents) ConfirmPurgeRemoval(ctx context.Context, key string) error {
	return p.repo.ConfirmPurgeRemoval(ctx, p.lease, p.index, key)
}

// Execute leaves unknown outcomes occupied under their original identities.
// Reconciliation never invents a new digest for an already started removal.
func (w *PurgeWorker) Execute(ctx context.Context, work PurgeWorkID) (domain.PurgeJob, error) {
	if w == nil || w.repo == nil || w.objects == nil {
		return domain.PurgeJob{}, ErrUnavailable
	}
	lease, err := w.repo.ClaimPurge(ctx, work)
	if err != nil || lease.Done {
		return lease.Job, err
	}
	physical, cancel := context.WithCancel(ctx)
	var heartbeatError error
	var receiptError error
	var mu sync.Mutex
	var joined sync.WaitGroup
	joined.Add(1)
	go func() {
		defer joined.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-physical.Done():
				return
			case <-ticker.C:
				if err := w.repo.HeartbeatPurge(physical, lease); err != nil {
					mu.Lock()
					heartbeatError = err
					mu.Unlock()
					cancel()
					return
				}
			}
		}
	}()
	for index, item := range lease.Job.Items {
		if item.Status == "blocked" || item.Status == "succeeded" || item.Status == "cancelled" {
			continue
		}
		done, err := w.repo.StartPurgeItem(physical, lease, index)
		if err == nil && !done {
			var objects []PurgeObject
			objects, err = w.repo.PurgeObjects(physical, lease, index)
			if err == nil && len(objects) > 0 {
				err = RemovePurgeObjects(physical, w.objects, purgeItemIntents{w.repo, lease, index}, objects)
			}
			if err == nil {
				err = w.repo.CompletePurgeItem(physical, lease, index)
			}
		}
		if err != nil {
			code := "source_unavailable"
			if errors.Is(err, ErrObjectMismatch) {
				code = "object_mismatch"
			}
			if errors.Is(err, ErrPurgeUnknown) {
				code = "object_remove_unknown"
			}
			if physical.Err() != nil {
				code = "worker_interrupted"
			}
			if errors.Is(err, domain.ErrPurgeCancelled) {
				code = "cancelled"
			}
			recordCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			recordErr := w.repo.FailPurgeItem(recordCtx, lease, index, code)
			stop()
			if recordErr != nil {
				receiptError = errors.Join(receiptError, err, recordErr)
				cancel()
				break
			}
			if physical.Err() != nil {
				break
			}
		}
	}
	cancel()
	joined.Wait()
	mu.Lock()
	heartbeatErr := heartbeatError
	mu.Unlock()
	finishCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer stop()
	if err := w.repo.EndPurgePhysical(finishCtx, lease); err != nil {
		return domain.PurgeJob{}, errors.Join(err, heartbeatErr, receiptError)
	}
	job, err := w.repo.FinishPurge(finishCtx, lease)
	return job, errors.Join(err, heartbeatErr, receiptError)
}
