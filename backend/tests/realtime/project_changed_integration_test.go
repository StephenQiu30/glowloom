package realtime_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace/noop"

	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestProjectChangedWithHostPostgresAndRedis(t *testing.T) {
	dsn := os.Getenv("LV_TEST_INBOX_DB_DSN")
	redisURL := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("set LV_TEST_INBOX_DB_DSN and LV_TEST_REALTIME_REDIS_URL for disposable integration data")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })

	projectID, orgID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	firstID, secondID := uuid.NewString(), uuid.NewString()
	channel := "project:" + projectID
	stream := channel + ":events"
	t.Cleanup(func() {
		if err := dbConn.DB.Exec(
			"DELETE FROM infra.processed_event WHERE consumer = ? AND event_id IN (?::uuid, ?::uuid)",
			"realtime", firstID, secondID,
		).Error; err != nil {
			t.Errorf("delete processed markers: %v", err)
		}
		if err := redisConn.Client.Del(context.Background(), stream).Err(); err != nil {
			t.Errorf("delete replay stream: %v", err)
		}
	})
	sub := redisConn.Client.Subscribe(ctx, channel)
	t.Cleanup(func() { _ = sub.Close() })
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe to project channel: %v", err)
	}

	handler := realtime.NewProjectChangedHandler(pginbox.NewStore(dbConn.DB), redisrealtime.NewSink(redisConn.Client))
	first := projectChangedRecord(t, firstID, projectID, orgID, actorID, 1, "created")
	if err := handler.Handle(ctx, first); err != nil {
		t.Fatalf("handle created event: %v", err)
	}
	assertProjectChangedPublication(ctx, t, sub, firstID, projectID, 1, "created")
	if err := handler.Handle(ctx, first); err != nil {
		t.Fatalf("handle duplicate event: %v", err)
	}
	if length := redisConn.Client.XLen(ctx, stream).Val(); length != 1 {
		t.Fatalf("stream length after duplicate = %d, want 1", length)
	}

	second := projectChangedRecord(t, secondID, projectID, orgID, actorID, 2, "updated")
	if err := handler.Handle(ctx, second); err != nil {
		t.Fatalf("handle updated event: %v", err)
	}
	assertProjectChangedPublication(ctx, t, sub, secondID, projectID, 2, "updated")
	items, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(items) != 2 {
		t.Fatalf("replay stream entries = %+v, error = %v", items, err)
	}
	for i, want := range []struct {
		id       string
		revision int64
		change   string
	}{{firstID, 1, "created"}, {secondID, 2, "updated"}} {
		if items[i].Values["id"] != want.id || items[i].Values["event"] != "project.updated" {
			t.Fatalf("stream entry %d routing = %+v", i, items[i].Values)
		}
		data, ok := items[i].Values["data"].(string)
		if !ok {
			t.Fatalf("stream entry %d data = %T", i, items[i].Values["data"])
		}
		assertProjectChangedData(t, []byte(data), projectID, want.revision, want.change)
	}
	replayed, resync, err := redisrealtime.NewSink(redisConn.Client).Replay(ctx, projectID, firstID)
	if err != nil || resync || len(replayed) != 1 {
		t.Fatalf("replay after first = %+v, resync = %t, error = %v", replayed, resync, err)
	}
	if replayed[0].ID != secondID || replayed[0].Event != "project.updated" {
		t.Fatalf("replayed event = %+v", replayed[0])
	}
	assertProjectChangedData(t, replayed[0].Data, projectID, 2, "updated")

	var markers int64
	if err := dbConn.DB.WithContext(ctx).Raw(
		"SELECT count(*) FROM infra.processed_event WHERE consumer = ? AND event_id IN (?::uuid, ?::uuid)",
		"realtime", firstID, secondID,
	).Scan(&markers).Error; err != nil || markers != 2 {
		t.Fatalf("processed markers = %d, error = %v", markers, err)
	}
}

func projectChangedRecord(t *testing.T, eventID, projectID, orgID, actorID string, revision int64, change string) inbox.Record {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": "lanverse.workspace.project_changed.v1",
		"occurred_at": time.Now().UTC(), "org_id": orgID, "project_id": projectID,
		"actor":     map[string]any{"kind": "user", "id": actorID},
		"aggregate": map[string]any{"type": "project", "id": projectID, "revision": revision},
		"data":      map[string]any{"change": change},
	})
	if err != nil {
		t.Fatalf("encode project event: %v", err)
	}
	return inbox.Record{Topic: "lanverse.workspace.project_changed.v1", Key: []byte(projectID), Value: payload}
}

func assertProjectChangedPublication(ctx context.Context, t *testing.T, sub *redisclient.PubSub, eventID, projectID string, revision int64, change string) {
	t.Helper()
	message, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("receive project event: %v", err)
	}
	var published struct {
		ID    string          `json:"id"`
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(message.Payload), &published); err != nil {
		t.Fatalf("decode published event: %v", err)
	}
	if published.ID != eventID || published.Event != "project.updated" {
		t.Fatalf("published event = %+v", published)
	}
	assertProjectChangedData(t, published.Data, projectID, revision, change)
}

func assertProjectChangedData(t *testing.T, raw []byte, projectID string, revision int64, change string) {
	t.Helper()
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode realtime data: %v", err)
	}
	if len(data) != 3 {
		t.Fatalf("realtime data fields = %v, want only project_id, revision, change", data)
	}
	var gotProjectID, gotChange string
	var gotRevision int64
	if err := json.Unmarshal(data["project_id"], &gotProjectID); err != nil {
		t.Fatalf("decode project_id: %v", err)
	}
	if err := json.Unmarshal(data["revision"], &gotRevision); err != nil {
		t.Fatalf("decode revision: %v", err)
	}
	if err := json.Unmarshal(data["change"], &gotChange); err != nil {
		t.Fatalf("decode change: %v", err)
	}
	if gotProjectID != projectID || gotRevision != revision || gotChange != change {
		t.Fatalf("realtime data = (%q, %d, %q), want (%q, %d, %q)", gotProjectID, gotRevision, gotChange, projectID, revision, change)
	}
}
