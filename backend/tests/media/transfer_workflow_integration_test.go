package media_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	mediaevent "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/event"
	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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
	handler := mediaevent.NewTransferHandler(inboxpg.NewStore(f.db), f.repo, starter)
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

type isolatedTransferClient struct {
	client.Client
	queue string
}

func (c isolatedTransferClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, function any, args ...any) (client.WorkflowRun, error) {
	options.TaskQueue = c.queue
	args[1] = mediaflow.TransferWorkflowConfig{ActivityTaskQueue: c.queue}
	return c.Client.ExecuteWorkflow(ctx, options, function, args...)
}

type transferCancelWrite struct {
	mediaapp.ProjectCopyObjects
	once    sync.Once
	written chan struct{}
}

func (o *transferCancelWrite) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, digest string) error {
	if err := o.ProjectCopyObjects.PutIfAbsent(ctx, key, reader, size, mime, digest); err != nil {
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

func TestMediaTransferActualTemporalPrivateObjectsCancellationAndRecovery(t *testing.T) {
	address, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if address == "" || namespace == "" {
		t.Skip("set explicitly authorized local Temporal with an isolated task-owned queue")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	for _, cancelTransfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete private bytes", true: "cancel actual unknown write then explicit cleanup"}[cancelTransfer], func(t *testing.T) {
			f := newTransferRecoveryFixture(t, 1)
			queue := "mediaTransfer-test-" + f.job.ID.String()
			var objects mediaapp.ProjectCopyObjects = copyobjects.NewProjectCopyObjects(f.objects)
			blocking := &transferCancelWrite{ProjectCopyObjects: objects, written: make(chan struct{})}
			if cancelTransfer {
				objects = blocking
			}
			activities := mediaflow.NewTransferActivities(mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()), f.repo)
			w := worker.New(c, queue, worker.Options{})
			mediaflow.RegisterTransferWorkflow(w)
			mediaflow.RegisterTransferActivities(w, activities)
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			ctx, stop := context.WithTimeout(t.Context(), 50*time.Second)
			defer stop()
			starter := mediaflow.NewTransferStarter(isolatedTransferClient{Client: c, queue: queue})
			handler := mediaevent.NewTransferHandler(inboxpg.NewStore(f.db), f.repo, starter)
			record := transferEventRecord(t, f, f.input.Key)
			if err := handler.Handle(ctx, record); err != nil {
				t.Fatal(err)
			}
			if err := handler.Handle(ctx, record); err != nil {
				t.Fatal(err)
			}
			workflowID := "media-transfer/" + f.job.ID.String() + "/1"
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
				if err := handler.Handle(ctx, transferEventRecord(t, f, key)); err != nil {
					t.Fatal(err)
				}
				// The physical activity must return before the request may recover.
				_ = c.GetWorkflow(ctx, workflowID, "").Get(ctx, &result)
				current, err = f.repo.GetTransfer(ctx, f.actor, f.job.ID)
				if err != nil || current.Status != "needs_reconciliation" || current.ExecutionUnconfirmed || !current.CancellationRequested {
					t.Fatal("real stopped write lost cancellation or became published", current, err)
				}
				recoveryKey := uuid.New()
				recovery, err := f.repo.ControlTransfer(ctx, f.actor, current.ID, recoveryKey, current.Revision, "reconcile")
				if err != nil || !recovery.CancellationRequested {
					t.Fatal("explicit recovery lost cancellation intent", recovery, err)
				}
				if err := handler.Handle(ctx, transferEventRecord(t, f, recoveryKey)); err != nil {
					t.Fatal(err)
				}
				workflowID = "media-transfer/" + f.job.ID.String() + "/" + strconv.Itoa(recovery.Attempt) + "/reconcile/" + recoveryKey.String()
			}
			if err := c.GetWorkflow(ctx, workflowID, "").Get(ctx, &result); err != nil {
				t.Fatal("real Temporal physical work did not complete", err)
			}
			want := "succeeded"
			if cancelTransfer {
				want = "cancelled"
			}
			if result.Status != want || result.ExecutionUnconfirmed || result.NeedsReconciliation {
				t.Fatal("actual physical result", result)
			}
			info, err := c.DescribeWorkflowExecution(ctx, workflowID, "")
			if err != nil || info.WorkflowExecutionInfo.Status != enums.WORKFLOW_EXECUTION_STATUS_COMPLETED {
				t.Fatal("actual workflow is not completed", err)
			}
			history := c.GetWorkflowHistory(ctx, workflowID, info.WorkflowExecutionInfo.Execution.RunId, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
			activityCount := 0
			for history.HasNext() {
				event, err := history.Next()
				if err != nil {
					t.Fatal(err)
				}
				if event.EventType == enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED {
					activityCount++
				}
			}
			if activityCount != 1 {
				t.Fatal("physical activity was automatically retried", activityCount)
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
