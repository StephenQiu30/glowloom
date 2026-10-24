package workspace_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	inkafka "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/kafka"
	inpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	outkafka "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/kafka"
	outpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/postgres"
	outapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/kafkaconn"
	copyevent "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/event"
	copyflow "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/workflow"
	copyapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	copydomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// This fixture contains zero media objects. Any unexpected object access fails.
type noCopyObjects struct{}

func (noCopyObjects) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("unexpected media object")
}
func (noCopyObjects) Exists(context.Context, string) (bool, error) {
	return false, errors.New("unexpected media object")
}
func (noCopyObjects) PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error {
	return errors.New("unexpected media object")
}
func (noCopyObjects) Remove(context.Context, string) error {
	return errors.New("unexpected media object")
}

var _ mediaapp.ProjectCopyObjects = noCopyObjects{}

var errLostCopyAcknowledgment = errors.New("injected failure after real external acceptance")

type loseCopyPublishAck struct {
	outapp.Publisher
	event string
}

func (p *loseCopyPublishAck) Publish(ctx context.Context, e outapp.Event) error {
	if err := p.Publisher.Publish(ctx, e); err != nil {
		return err
	}
	if e.ID == p.event {
		p.event = ""
		return errLostCopyAcknowledgment
	}
	return nil
}

type loseCopyStartAck struct {
	copyevent.ProjectCopyStarter
	calls int
}

func (s *loseCopyStartAck) Deliver(ctx context.Context, d copyapp.ProjectCopyDelivery) error {
	s.calls++
	if err := s.ProjectCopyStarter.Deliver(ctx, d); err != nil {
		return err
	}
	if s.calls == 1 {
		return errLostCopyAcknowledgment
	}
	return nil
}

// This test requires a dedicated copy database/topic and Temporal namespace.
// Prior completed fixture records may remain: the real handler ignores terminal jobs.
// Both fault wrappers fail only AFTER the real Kafka/Temporal acknowledgment.
func TestProjectCopyKafkaOutboxInboxTemporalCompleteAndRecover(t *testing.T) {
	dsn, brokers := os.Getenv("GLOWLOOM_TEST_COPY_KAFKA_DB_DSN"), os.Getenv("GLOWLOOM_TEST_KAFKA_BROKERS")
	address, namespace := os.Getenv("GLOWLOOM_TEST_TEMPORAL_ADDR"), os.Getenv("GLOWLOOM_TEST_TEMPORAL_NAMESPACE")
	if dsn == "" || brokers == "" || address == "" || namespace == "" {
		t.Skip("set isolated COPY_KAFKA_DB_DSN, KAFKA_BROKERS, TEMPORAL_ADDR and TEMPORAL_NAMESPACE")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	conn, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db := conn.DB
	var database string
	if err := db.Raw(`SELECT current_database()`).Scan(&database).Error; err != nil || database != "glowloom_kafka_copy" {
		t.Fatal("dedicated empty glowloom_kafka_copy database required", err)
	}
	actor, source := lifecycleActorProject(ctx, t, db)
	doc, err := canvasfixture.NewService(workspaceCanvasStore(db, nil)).Create(ctx, actor, source, uuid.NewString(), canvasapp.CreateInput{Name: "Kafka完整副本"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc.ID, source)
	store := projectCopyStore(db)
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var event string
	if err := db.Raw(`SELECT id::text FROM infra.outbox WHERE topic=? AND partition_key=?`, copyevent.ProjectCopyTopic, job.ID.String()).Scan(&event).Error; err != nil || event == "" {
		t.Fatal("committed copy event", err)
	}
	broker, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	publisher := &loseCopyPublishAck{Publisher: outkafka.NewPublisher(broker.Client), event: event}
	relay := outapp.NewRelay(outpg.NewStore(db), publisher)
	for {
		sent, err := relay.RunOnce(ctx)
		if errors.Is(err, errLostCopyAcknowledgment) {
			break
		}
		if err != nil || !sent {
			t.Fatal("real relay did not reach copy event", sent, err)
		}
	}
	var pending bool
	if err := db.Raw(`SELECT published_at IS NULL FROM infra.outbox WHERE id=?`, event).Scan(&pending).Error; err != nil || !pending {
		t.Fatal("lost broker acknowledgment prematurely marked published", err)
	}
	if sent, err := relay.RunOnce(ctx); err != nil || !sent {
		t.Fatal("retry original outbox event", err)
	}
	temporal, err := client.Dial(client.Options{HostPort: address, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(temporal.Close)
	starter := &loseCopyStartAck{ProjectCopyStarter: copyflow.NewProjectCopyStarter(temporal)}
	handler := copyevent.NewProjectCopyHandler(inpg.NewStore(db), store, starter)
	group := "copy-recovery-" + job.ID.String()
	consumer, err := inkafka.NewConsumer(brokers, group, []string{copyevent.ProjectCopyTopic}, handler)
	if err != nil {
		t.Fatal(err)
	}
	for {
		applied, err := consumer.RunOnce(ctx)
		if errors.Is(err, errLostCopyAcknowledgment) && !applied {
			break
		}
		if err != nil || !applied {
			consumer.Close()
			t.Fatal("consume until lost Temporal acknowledgment", applied, err)
		}
	}
	consumer.Close()
	workflowID := "project-copy/" + job.ID.String()
	first, err := temporal.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		t.Fatal(err)
	}
	var markers int
	if err := db.Raw(`SELECT count(*) FROM infra.processed_event WHERE event_id=? AND consumer='project-copy-command'`, event).Scan(&markers).Error; err != nil || markers != 0 {
		t.Fatal("external failure committed inbox", markers, err)
	}
	restarted, err := inkafka.NewConsumer(brokers, group, []string{copyevent.ProjectCopyTopic}, handler)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // The original Kafka record and the real duplicate publish.
		if applied, err := restarted.RunOnce(ctx); !applied || err != nil {
			restarted.Close()
			t.Fatal("same group retry/deduplication", applied, err)
		}
	}
	restarted.Close()
	if starter.calls != 2 {
		t.Fatalf("duplicate event delivered again: calls=%d", starter.calls)
	}
	flow := worker.New(temporal, "flow", worker.Options{})
	copyflow.RegisterProjectCopyWorkflow(flow)
	media := worker.New(temporal, "media", worker.Options{})
	copyflow.RegisterProjectCopyActivities(media, copyflow.NewProjectCopyActivities(actualCopyWorker(t, db, noCopyObjects{}), store))
	for _, w := range []worker.Worker{flow, media} {
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Stop)
	}
	var finished copydomain.ProjectCopyJob
	if err := temporal.GetWorkflow(ctx, workflowID, "").Get(ctx, &finished); err != nil || finished.Status != "succeeded" || finished.Attempt != 1 {
		t.Fatal("actual copy workflow did not finish once", finished.Status, finished.Attempt, err)
	}
	latest, err := temporal.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil || latest.WorkflowExecutionInfo.Execution.RunId != first.WorkflowExecutionInfo.Execution.RunId {
		t.Fatal("retry created another workflow run", err)
	}
	history := temporal.GetWorkflowHistory(ctx, workflowID, "", false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	starts := 0
	for history.HasNext() {
		e, err := history.Next()
		if err != nil {
			t.Fatal(err)
		}
		if e.EventType == enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED {
			starts++
		}
	}
	var primary struct{ ID uuid.UUID }
	if err := db.Raw(`SELECT initial_canvas_id AS id FROM workspace.project WHERE id=? AND status='active'`, job.TargetProjectID).Scan(&primary).Error; err != nil || primary.ID == uuid.Nil || primary.ID == doc.ID || starts != 1 {
		t.Fatal("copy primary identity or workflow history", primary, starts, err)
	}
	if err := db.Raw(`SELECT count(*) FROM infra.processed_event WHERE event_id=? AND consumer='project-copy-command'`, event).Scan(&markers).Error; err != nil || markers != 1 {
		t.Fatal("durable inbox marker", markers, err)
	}
	verified, err := inkafka.NewConsumer(brokers, group, []string{copyevent.ProjectCopyTopic}, handler)
	if err != nil {
		t.Fatal(err)
	}
	defer verified.Close()
	short, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	if applied, err := verified.RunOnce(short); applied || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("committed group offset replayed records", applied, err)
	}
	t.Logf("event=%s group=%s job=%s target=%s canvas=%s run=%s starts=%d inbox=%d", event, group, job.ID, job.TargetProjectID, primary.ID, first.WorkflowExecutionInfo.Execution.RunId, starts, markers)
}
