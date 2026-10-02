package script_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	inboxpg "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	scriptevent "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/event"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptflow "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/workflow"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

type isolatedImportClient struct {
	client.Client
	queue string
}

func (c isolatedImportClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, w any, args ...any) (client.WorkflowRun, error) {
	options.TaskQueue = c.queue
	return c.Client.ExecuteWorkflow(ctx, options, w, args...)
}

func TestScriptFileImportRealTemporalExistingRunReceivesSavedControls(t *testing.T) {
	address, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if address == "" || namespace == "" {
		t.Skip("set actual authorized Temporal address/namespace; queue is unique and unpolled")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	job := uuid.New()
	id := "script-import/" + job.String()
	t.Cleanup(func() {
		end, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := c.TerminateWorkflow(end, id, "", "task-owned import starter test finished"); err != nil {
			t.Error(err)
		}
	})
	starter := scriptflow.NewImportStarter(isolatedImportClient{Client: c, queue: "script-import-starter-test-" + job.String()})
	d := app.ImportDelivery{ImportWork: app.ImportWork{JobID: job, Attempt: 1}, EventID: uuid.New(), RequestID: uuid.New(), OrgID: uuid.New(), ProjectID: uuid.New(), ActorID: uuid.New(), Action: "start"}
	if err := starter.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	first, err := c.DescribeWorkflowExecution(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	d.Action = "cancel"
	d.RequestID = uuid.New()
	d.EventID = uuid.New()
	if err := starter.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Action = "reconcile"
	d.RequestID = uuid.New()
	d.EventID = uuid.New()
	if err := starter.Deliver(ctx, d); err != nil {
		t.Fatal(err)
	}
	same, err := c.DescribeWorkflowExecution(ctx, id, "")
	if err != nil || same.WorkflowExecutionInfo.Execution.RunId != first.WorkflowExecutionInfo.Execution.RunId {
		t.Fatal("new execution replaced existing", err)
	}
	starts, signals := 0, 0
	history := c.GetWorkflowHistory(ctx, id, first.WorkflowExecutionInfo.Execution.RunId, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for history.HasNext() {
		event, err := history.Next()
		if err != nil {
			t.Fatal(err)
		}
		switch event.EventType {
		case enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			starts++
		case enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED:
			signals++
			if event.GetWorkflowExecutionSignaledEventAttributes().SignalName != scriptflow.ImportCommandSignal {
				t.Fatal("wrong control signal")
			}
		}
	}
	if starts != 1 || signals != 2 {
		t.Fatal("SDK silently swallowed current controls", starts, signals)
	}
}

// The test interceptor changes only scheduling queues. Production workflow code,
// permanent command data and all worker/media/object consumers remain unchanged.
type importQueueInterceptor struct {
	interceptor.WorkerInterceptorBase
	queue string
}

func (i *importQueueInterceptor) InterceptWorkflow(_ workflow.Context, next interceptor.WorkflowInboundInterceptor) interceptor.WorkflowInboundInterceptor {
	return &importQueueInbound{WorkflowInboundInterceptorBase: interceptor.WorkflowInboundInterceptorBase{Next: next}, queue: i.queue}
}

type importQueueInbound struct {
	interceptor.WorkflowInboundInterceptorBase
	queue string
}

func (i *importQueueInbound) Init(out interceptor.WorkflowOutboundInterceptor) error {
	return i.Next.Init(&importQueueOutbound{WorkflowOutboundInterceptorBase: interceptor.WorkflowOutboundInterceptorBase{Next: out}, queue: i.queue})
}

type importQueueOutbound struct {
	interceptor.WorkflowOutboundInterceptorBase
	queue string
}

func (i *importQueueOutbound) ExecuteActivity(ctx workflow.Context, activityType string, args ...any) workflow.Future {
	options := workflow.GetActivityOptions(ctx)
	options.TaskQueue = i.queue
	return i.Next.ExecuteActivity(workflow.WithActivityOptions(ctx, options), activityType, args...)
}

func TestScriptFileImportRealTemporalPGInboxActualPrivateOriginalToVersion(t *testing.T) {
	address, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if address == "" || namespace == "" {
		t.Skip("set authorized local Temporal address/namespace; all worker queues are task unique")
	}
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	file := scriptDocument(t, db, storage, actor, pid, "真实任务.txt", []byte("Temporal 原文😀"))
	store := scriptImportStore(db, storage)
	service := app.NewImportService(store, time.Now)
	in := app.ImportCommand{ProjectID: pid, Key: uuid.New(), RequestID: uuid.New(), AssetIDs: []uuid.UUID{file}, RightsConfirmed: true}
	accepted, err := service.Create(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	queue := "script-import-execute-test-" + uuid.NewString()
	w := worker.New(c, queue, worker.Options{Interceptors: []interceptor.WorkerInterceptor{&importQueueInterceptor{queue: queue}}})
	scriptflow.RegisterImportWorkflow(w)
	scriptflow.RegisterImportActivities(w, scriptflow.NewImportActivities(scriptImportWorker(db, storage, extract.NewExtractor()), store))
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	id := "script-import/" + accepted.ID.String()
	t.Cleanup(func() {
		end, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		err := c.TerminateWorkflow(end, id, "", "task-owned actual import verification finished")
		if err != nil {
			var missing *serviceerror.NotFound
			if !errors.As(err, &missing) {
				t.Error(err)
			}
		}
	})
	var outbox struct {
		ID      uuid.UUID
		Payload string
	}
	if err := owner.Raw(`SELECT o.id,o.payload::text FROM infra.outbox o JOIN script.import_command c ON c.event_id=o.id WHERE c.actor_id=? AND c.request_id=?`, actor.ID, in.Key).Scan(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	handler := scriptevent.NewImportHandler(inboxpg.NewStore(db), store, scriptflow.NewImportStarter(isolatedImportClient{Client: c, queue: queue}))
	record := inboxapp.Record{Topic: scriptevent.ImportTopic, Key: []byte(pid.String()), Value: []byte(outbox.Payload)}
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatal("real owning inbox delivery", err)
	}
	var result app.ImportJob
	if err := c.GetWorkflow(ctx, id, "").Get(ctx, &result); err != nil {
		t.Fatal("real production workflow", err)
	}
	current, err := service.Get(ctx, actor, pid, accepted.ID)
	if err != nil || result.Status != "succeeded" || current.Status != "succeeded" || current.LatestVersionID == nil || current.ActiveIO || current.NeedsReconciliation {
		t.Fatal("actual workflow durable state", result, current, err)
	}
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatal("duplicate inbox", err)
	}
	var markers int64
	if err := owner.Raw(`SELECT count(*) FROM infra.processed_event WHERE event_id=? AND consumer='script-import-command'`, outbox.ID).Scan(&markers).Error; err != nil || markers != 1 {
		t.Fatal("inbox first receipt", markers, err)
	}
}
