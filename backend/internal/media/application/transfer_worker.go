package application

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// TransferLease is private execution authority and the actual frozen item set.
type TransferLease struct {
	Work      TransferWorkID
	Fence     uuid.UUID
	Actor     identityapp.Principal
	Job       domain.TransferJob
	ItemCount int
	Done      bool
}

// TransferWorkerRepository keeps physical receipts under one live SQL fence.
type TransferWorkerRepository interface {
	ClaimTransfer(context.Context, TransferWorkID) (TransferLease, error)
	HeartbeatTransfer(context.Context, TransferLease) error
	StartTransferItem(context.Context, TransferLease, int) (bool, error)
	TransferObjects(context.Context, TransferLease, int) ([]ProjectCopyObject, error)
	RecordTransferDigest(context.Context, TransferLease, int, string, string, int64) error
	BeginTransferWrite(context.Context, TransferLease, int, string) error
	ConfirmTransferObject(context.Context, TransferLease, int, string, string, int64) error
	AuthorizeTransferRemoval(context.Context, TransferLease, int, string) error
	ConfirmTransferRemoval(context.Context, TransferLease, int, string) error
	PublishTransferItem(context.Context, TransferLease, int) error
	FailTransferItem(context.Context, TransferLease, int, string, bool) error
	EndTransferPhysical(context.Context, TransferLease) error
	FinishTransfer(context.Context, TransferLease) (domain.TransferJob, error)
}

// TransferItemError supplies a closed safe failure code without source content.
type TransferItemError struct {
	Code  string
	Cause error
}

func (e *TransferItemError) Error() string { return "media transfer item: " + e.Code }
func (e *TransferItemError) Unwrap() error { return e.Cause }

// TransferWorker copies real private objects and publishes each complete item.
// Its heartbeat has explicit cancellation, ownership and a join before end proof.
type TransferWorker struct {
	repo    TransferWorkerRepository
	objects ProjectCopyObjects
	tempDir string
}

// NewTransferWorker injects fenced SQL, immutable object I/O and temporary files.
func NewTransferWorker(repo TransferWorkerRepository, objects ProjectCopyObjects, tempDir string) *TransferWorker {
	return &TransferWorker{repo: repo, objects: objects, tempDir: tempDir}
}

func (w *TransferWorker) copyItem(ctx context.Context, lease TransferLease, index int) error {
	objects, err := w.repo.TransferObjects(ctx, lease, index)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := transferPrivateObject(ctx, w.objects, w.tempDir, object,
			func(sha string, size int64) error {
				return w.repo.RecordTransferDigest(ctx, lease, index, object.RenditionKind, sha, size)
			},
			func() error { return w.repo.BeginTransferWrite(ctx, lease, index, object.RenditionKind) },
			func(sha string, size int64) error {
				return w.repo.ConfirmTransferObject(ctx, lease, index, object.RenditionKind, sha, size)
			},
		); err != nil {
			return err
		}
	}
	return w.repo.PublishTransferItem(ctx, lease, index)
}

func (w *TransferWorker) cleanupItem(ctx context.Context, lease TransferLease, index int) error {
	objects, err := w.repo.TransferObjects(ctx, lease, index)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := w.repo.AuthorizeTransferRemoval(ctx, lease, index, object.RenditionKind); err != nil {
			return err
		}
		exists, err := w.objects.Exists(ctx, object.TargetObjectKey)
		if err != nil {
			return err
		}
		if !exists && object.WriteStarted && object.Status != "removed" {
			return &ProjectCopyTransferError{Code: "object_absence_unknown", NeedsReconciliation: true, Cause: ErrObjectMismatch}
		}
		if exists {
			if err := verifyTransferredObject(ctx, w.objects, object); err != nil {
				return err
			}
			if err := w.objects.Remove(ctx, object.TargetObjectKey); err != nil {
				return err
			}
			if exists, err := w.objects.Exists(ctx, object.TargetObjectKey); err != nil || exists {
				return errors.Join(ErrObjectMismatch, err)
			}
		}
		if err := w.repo.ConfirmTransferRemoval(ctx, lease, index, object.RenditionKind); err != nil {
			return err
		}
	}
	return nil
}

func transferFailure(err error) (string, bool) {
	var item *TransferItemError
	if errors.As(err, &item) {
		return item.Code, false
	}
	var unknown *ProjectCopyTransferError
	if errors.As(err, &unknown) && unknown.NeedsReconciliation {
		if unknown.Code == "object_receipt_unknown" {
			return "object_receipt_unknown", true
		}
		return "object_write_unknown", true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrTransferCancelled) {
		return "cancelled", false
	}
	if errors.Is(err, ErrObjectMismatch) {
		return "object_mismatch", false
	}
	return "source_unavailable", false
}

// Execute waits for every physical call and owned heartbeat before recording
// cessation. Unknown remote object outcomes remain durable reconciliation work.
func (w *TransferWorker) Execute(ctx context.Context, id TransferWorkID) (domain.TransferJob, error) {
	if w == nil || w.repo == nil || w.objects == nil {
		return domain.TransferJob{}, ErrUnavailable
	}
	lease, err := w.repo.ClaimTransfer(ctx, id)
	if err != nil || lease.Done {
		return lease.Job, err
	}
	run, cancel := context.WithCancel(ctx)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var heartbeatErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-run.Done():
				return
			case <-ticker.C:
				if err := w.repo.HeartbeatTransfer(run, lease); err != nil {
					heartbeatErr = err
					cancel()
					return
				}
			}
		}
	}()
	for index := range lease.ItemCount {
		// Published rows are immutable outcomes of the saved claim. Cancellation
		// cleans only unpublished rows and must never revisit their objects.
		if lease.Job.Items[index].Status == "succeeded" {
			continue
		}
		if err := w.repo.HeartbeatTransfer(run, lease); err != nil {
			cancel()
		}
		done, startErr := w.repo.StartTransferItem(run, lease, index)
		if done {
			continue
		}
		itemErr := startErr
		if itemErr == nil {
			itemErr = w.copyItem(run, lease, index)
		}
		if itemErr == nil {
			continue
		}
		code, unknown := transferFailure(itemErr)
		cleanup, end := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		if !unknown {
			if err := w.cleanupItem(cleanup, lease, index); err != nil {
				code, unknown = "object_cleanup_unknown", true
			}
		}
		markErr := w.repo.FailTransferItem(cleanup, lease, index, code, unknown)
		end()
		if unknown || markErr != nil || run.Err() != nil {
			break
		}
	}
	close(stop)
	cancel()
	wg.Wait()
	cleanup, end := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer end()
	if err := w.repo.EndTransferPhysical(cleanup, lease); err != nil {
		return domain.TransferJob{}, errors.Join(err, heartbeatErr)
	}
	result, err := w.repo.FinishTransfer(cleanup, lease)
	return result, errors.Join(err, heartbeatErr)
}
