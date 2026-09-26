package outbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestPostgresOutboxAcknowledgesOnlyAfterDelivery(t *testing.T) {
	dsn := os.Getenv("LV_TEST_OUTBOX_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_OUTBOX_DB_DSN to a disposable database with Outbox migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	id, projectID := uuid.NewString(), uuid.NewString()
	topic := "lanverse.operation.status_changed.v1"
	payload := `{"event_id":"` + id + `","event_type":"` + topic + `","project_id":"` + projectID + `"}`
	if err := conn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload, headers) VALUES (?::uuid, ?, ?, ?::jsonb, ?::jsonb)",
		id, topic, projectID, payload, `{"traceparent":"test-trace"}`,
	).Error; err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", id).Error; err != nil {
			t.Errorf("remove outbox event: %v", err)
		}
	})

	publisher := &recordingPublisher{err: errors.New("temporary broker failure")}
	relay := application.NewRelay(pgoutbox.NewStore(conn.DB), publisher)
	if sent, err := relay.RunOnce(ctx); sent || !errors.Is(err, publisher.err) {
		t.Fatalf("failed delivery = (%t, %v)", sent, err)
	}
	var pending bool
	if err := conn.DB.WithContext(ctx).Raw(
		"SELECT published_at IS NULL FROM infra.outbox WHERE id = ?::uuid", id,
	).Scan(&pending).Error; err != nil || !pending {
		t.Fatalf("failed delivery pending = %t, error = %v", pending, err)
	}

	publisher.err = nil
	if sent, err := relay.RunOnce(ctx); !sent || err != nil {
		t.Fatalf("successful delivery = (%t, %v)", sent, err)
	}
	if len(publisher.events) != 1 || publisher.events[0].ID != id ||
		publisher.events[0].Topic != topic || publisher.events[0].PartitionKey != projectID ||
		publisher.events[0].Headers["traceparent"] != "test-trace" {
		t.Fatalf("delivered event = %+v", publisher.events)
	}
	if err := conn.DB.WithContext(ctx).Raw(
		"SELECT published_at IS NULL FROM infra.outbox WHERE id = ?::uuid", id,
	).Scan(&pending).Error; err != nil || pending {
		t.Fatalf("successful delivery pending = %t, error = %v", pending, err)
	}
	if sent, err := relay.RunOnce(ctx); sent || err != nil {
		t.Fatalf("empty queue = (%t, %v)", sent, err)
	}
}
