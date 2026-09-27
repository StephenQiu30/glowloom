package realtime_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestOperationStatusWithHostPostgresAndRedis(t *testing.T) {
	dsn := os.Getenv("LV_TEST_INBOX_DB_DSN")
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if dsn == "" || url == "" {
		t.Skip("set LV_TEST_INBOX_DB_DSN and LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	eventID, projectID, operationID, targetID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	channel := "project:" + projectID
	stream := channel + ":events"
	t.Cleanup(func() {
		if err := dbConn.DB.Exec("DELETE FROM infra.processed_event WHERE consumer = ? AND event_id = ?::uuid", "realtime", eventID).Error; err != nil {
			t.Errorf("delete marker: %v", err)
		}
		if err := redisConn.Client.Del(context.Background(), stream).Err(); err != nil {
			t.Errorf("delete stream: %v", err)
		}
	})
	sub := redisConn.Client.Subscribe(ctx, channel)
	t.Cleanup(func() { _ = sub.Close() })
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	handler := realtime.NewOperationStatusHandler(pginbox.NewStore(dbConn.DB), redisrealtime.NewSink(redisConn.Client))
	record := inbox.Record{
		Topic: "lanverse.operation.status_changed.v1",
		Key:   []byte(projectID),
		Value: []byte(`{"event_id":"` + eventID + `","event_type":"lanverse.operation.status_changed.v1","project_id":"` + projectID + `","aggregate":{"type":"operation","id":"` + operationID + `"},"data":{"target_type":"shot_frame","target_id":"` + targetID + `","status":"submitted"}}`),
	}
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("handle event: %v", err)
	}
	message, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("receive project event: %v", err)
	}
	var published struct {
		ID    string          `json:"id"`
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(message.Payload), &published); err != nil || published.ID != eventID || published.Event != "operation.updated" {
		t.Fatalf("published event = %+v, error = %v", published, err)
	}
	items, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(items) != 1 || items[0].Values["id"] != eventID || items[0].Values["data"] != string(published.Data) {
		t.Fatalf("replay stream = %+v, error = %v", items, err)
	}
	if ttl := redisConn.Client.TTL(ctx, stream).Val(); ttl <= 0 || ttl > 10*time.Minute {
		t.Fatalf("stream TTL = %v", ttl)
	}
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("duplicate event: %v", err)
	}
	if length := redisConn.Client.XLen(ctx, stream).Val(); length != 1 {
		t.Fatalf("duplicate stream length = %d, want 1", length)
	}
	var markers int64
	if err := dbConn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = ? AND event_id = ?::uuid", "realtime", eventID).Scan(&markers).Error; err != nil || markers != 1 {
		t.Fatalf("processed markers = %d, error = %v", markers, err)
	}
}

func TestRedisReplayAfterLastEventAndResync(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	projectID := uuid.NewString()
	stream := "project:" + projectID + ":events"
	t.Cleanup(func() {
		if err := conn.Client.Del(context.Background(), stream).Err(); err != nil {
			t.Errorf("delete stream: %v", err)
		}
	})
	sink := redisrealtime.NewSink(conn.Client)
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for _, id := range ids {
		if err := sink.Publish(ctx, realtime.Event{ID: id, ProjectID: projectID, Type: "operation.updated", OperationID: uuid.NewString(), TargetType: "shot_frame", Status: "submitted"}); err != nil {
			t.Fatalf("publish test event: %v", err)
		}
	}
	// A database commit failure can replay the same external effect.
	if err := sink.Publish(ctx, realtime.Event{ID: ids[1], ProjectID: projectID, Type: "operation.updated", OperationID: uuid.NewString(), TargetType: "shot_frame", Status: "submitted"}); err != nil {
		t.Fatalf("publish duplicate effect: %v", err)
	}
	events, resync, err := sink.Replay(ctx, projectID, ids[0])
	if err != nil || resync || len(events) != 2 || events[0].ID != ids[1] || events[1].ID != ids[2] {
		t.Fatalf("replay = %+v, resync = %t, error = %v", events, resync, err)
	}
	if events, resync, err := sink.Replay(ctx, projectID, ids[1]); err != nil || resync || len(events) != 1 || events[0].ID != ids[2] {
		t.Fatalf("duplicate cursor = %+v, resync = %t, error = %v", events, resync, err)
	}
	if events, resync, err := sink.Replay(ctx, projectID, uuid.NewString()); err != nil || !resync || len(events) != 0 {
		t.Fatalf("expired cursor = %+v, resync = %t, error = %v", events, resync, err)
	}
	if events, resync, err := sink.Replay(ctx, projectID, ""); err != nil || resync || len(events) != 0 {
		t.Fatalf("initial subscription = %+v, resync = %t, error = %v", events, resync, err)
	}
}
