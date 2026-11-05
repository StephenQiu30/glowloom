package workspace_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"

	copytask "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	outkafka "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/kafka"
	outpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/postgres"
	outapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/kafkaconn"
	copyevent "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/event"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// This fixture contains zero media objects. Any unexpected object access fails.
type noCopyObjects struct{}

func (noCopyObjects) Get(context.Context, mediadomain.ObjectRef) (io.ReadCloser, error) {
	return nil, errors.New("unexpected media object")
}
func (noCopyObjects) Exists(context.Context, mediadomain.ObjectRef) (bool, error) {
	return false, errors.New("unexpected media object")
}
func (noCopyObjects) PutIfAbsent(context.Context, mediadomain.ObjectRef, io.Reader, int64, string, string) error {
	return errors.New("unexpected media object")
}
func (noCopyObjects) Remove(context.Context, mediadomain.ObjectRef) error {
	return errors.New("unexpected media object")
}

var _ mediaapp.PhysicalTransferObjects = noCopyObjects{}

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

func TestProjectCopyKafkaOutboxCompleteAndRecover(t *testing.T) {
	dsn, brokers := os.Getenv("GLOWLOOM_TEST_COPY_KAFKA_DB_DSN"), os.Getenv("GLOWLOOM_TEST_KAFKA_BROKERS")
	if dsn == "" || brokers == "" {
		t.Skip("set isolated COPY_KAFKA_DB_DSN, KAFKA_BROKERS")
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
	taskfixture.RunWork(t, db, copytask.Copy, map[string]any{"JobID": job.ID}, copytask.CopyHandler(actualCopyWorker(t, db, noCopyObjects{}), store))
	finished, err := store.FindForWorker(ctx, actor, job.ID)
	if err != nil || finished.Status != "succeeded" || finished.Attempt != 1 {
		t.Fatal("copy did not finish once", finished, err)
	}
	var primary uuid.UUID
	if err := db.Raw("SELECT initial_canvas_id FROM workspace.project WHERE id=? AND status='active'", job.TargetProjectID).Scan(&primary).Error; err != nil || primary == uuid.Nil || primary == doc.ID {
		t.Fatal("copy primary identity", primary, err)
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=? AND epoch=1 AND completed_at IS NOT NULL", copytask.Copy, event).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("lost broker acknowledgement duplicated the copy", count, err)
	}
}
