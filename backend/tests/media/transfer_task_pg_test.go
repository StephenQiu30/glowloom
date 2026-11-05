package media_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	currenttask "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"

	inboxpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	mediaevent "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/event"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type transferDeliveryRecorder struct {
	calls int
	fail  bool
}

func (s *transferDeliveryRecorder) Deliver(context.Context, mediaapp.TransferDelivery) error {
	s.calls++
	if s.fail {
		return errors.New("controlled external scheduling failure")
	}
	return nil
}

func transferEventRecord(t *testing.T, f transferRecoveryFixture, key uuid.UUID) inboxapp.Record {
	t.Helper()
	var row struct {
		Payload      []byte
		PartitionKey string
	}
	if err := f.db.Raw(`SELECT o.payload,o.partition_key FROM media.transfer_command c JOIN infra.outbox o ON o.id=c.event_id WHERE c.actor_id=? AND c.idem_key=?`, f.actor.ID, key).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	return inboxapp.Record{Topic: mediaevent.TransferTopic, Key: []byte(row.PartitionKey), Value: row.Payload}
}

func TestMediaTransferPGPermanentInboxRetriesOnlyFailedExternalDelivery(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	starter := &transferDeliveryRecorder{fail: true}
	handler := mediaevent.NewTransferHandler(inboxpg.NewStore(f.db), f.repo.Worker, starter)
	record := transferEventRecord(t, f, f.input.Key)
	forged := record
	forged.Key = []byte(uuid.NewString())
	if err := handler.Handle(t.Context(), forged); err == nil || starter.calls != 0 {
		t.Fatal("forged event was dispatched or acknowledged", err, starter.calls)
	}
	if err := handler.Handle(t.Context(), record); err == nil || starter.calls != 1 {
		t.Fatal("failed external delivery was acknowledged", err, starter.calls)
	}
	starter.fail = false
	if err := handler.Handle(t.Context(), record); err != nil || starter.calls != 2 {
		t.Fatal("failed delivery could not recover its original identity", err, starter.calls)
	}
	if err := handler.Handle(t.Context(), record); err != nil || starter.calls != 2 {
		t.Fatal("permanent inbox duplicated accepted external effect", err, starter.calls)
	}
}

type transferCancelWrite struct {
	mediaapp.PhysicalTransferObjects
	once    sync.Once
	written chan struct{}
}

func (o *transferCancelWrite) PutIfAbsent(ctx context.Context, key domain.ObjectRef, reader io.Reader, size int64, mime, digest string) error {
	if err := o.PhysicalTransferObjects.PutIfAbsent(ctx, key, reader, size, mime, digest); err != nil {
		return err
	}
	first := false
	o.once.Do(func() { first = true; close(o.written) })
	if first {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func TestMediaTransferActualDatabasePrivateObjectsCancellationAndRecovery(t *testing.T) {
	for _, cancelTransfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete private bytes", true: "cancel actual unknown write then explicit cleanup"}[cancelTransfer], func(t *testing.T) {
			f := newTransferRecoveryFixture(t, 1)
			var objects mediaapp.PhysicalTransferObjects = mediaFixtureCopyObjects(t, f.db)
			blocking := &transferCancelWrite{PhysicalTransferObjects: objects, written: make(chan struct{})}
			if cancelTransfer {
				objects = blocking
			}
			realRun := currenttask.TransferHandler(mediaapp.NewRoutedTransferWorker(f.repo.Worker, objects, t.TempDir()), f.repo.Worker)
			run := func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
				bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				return realRun(bounded, job)
			}
			ctx, stop := context.WithTimeout(t.Context(), 50*time.Second)
			defer stop()
			active := taskfixture.StartWork(t, f.db, currenttask.Transfer, map[string]any{"job_id": f.job.ID}, run)
			var result domain.TransferJob
			if cancelTransfer {
				select {
				case <-blocking.written:
				case <-ctx.Done():
					t.Fatal("actual conditional write did not run", ctx.Err())
				}
				current, err := f.repo.GetTransfer(ctx, f.actor, f.job.ID)
				if err != nil {
					t.Fatal(err)
				}
				key := uuid.New()
				requested, err := f.repo.ControlTransfer(ctx, f.actor, current.ID, key, current.Revision, "cancel")
				if err != nil || !requested.CancellationRequested {
					t.Fatal(requested, err)
				}
				taskfixture.WaitWork(t, f.db, currenttask.Transfer, active, 20*time.Second)
				current, err = f.repo.GetTransfer(ctx, f.actor, f.job.ID)
				if err != nil || current.Status != "needs_reconciliation" || current.ExecutionUnconfirmed || !current.CancellationRequested {
					t.Fatal("real stopped write lost cancellation or became published", current, err)
				}
				drainFixtureTransfer(t, libraryOwnerDB(t), f.job.ID)
				recoveryKey := uuid.New()
				recovery, err := f.repo.ControlTransfer(ctx, f.actor, current.ID, recoveryKey, current.Revision, "reconcile")
				if err != nil || !recovery.CancellationRequested {
					t.Fatal("explicit recovery lost cancellation intent", recovery, err)
				}
				active = taskfixture.StartWork(t, f.db, currenttask.Transfer, map[string]any{"job_id": f.job.ID, "attempt": recovery.Attempt}, run)
			}
			taskfixture.WaitWork(t, f.db, currenttask.Transfer, active, 30*time.Second)
			result, err := f.repo.GetTransfer(ctx, f.actor, f.job.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := domain.TransferStatus("succeeded")
			if cancelTransfer {
				want = "cancelled"
			}
			if result.Status != want || result.ExecutionUnconfirmed || result.NeedsReconciliation {
				t.Fatal("actual physical result", result)
			}
			var count int
			if err := f.db.Raw("SELECT epoch FROM infra.task_work WHERE kind=? AND business_id=?", currenttask.Transfer, active).Scan(&count).Error; err != nil || count != 1 {
				t.Fatal("unexpected duplicate physical attempt", err, count)
			}
			var rows []struct{ TargetObjectKey string }
			if err := f.db.Raw(`SELECT target_object_key FROM media.transfer_object WHERE job_id=?`, f.job.ID).Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				exists, err := f.objects.Exists(ctx, row.TargetObjectKey)
				if err != nil || exists == cancelTransfer {
					t.Fatal("actual private object outcome disagrees with completed job", exists, err)
				}
			}
		})
	}
}
