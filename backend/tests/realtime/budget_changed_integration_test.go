package realtime_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestBudgetChangedWithHostRedis(t *testing.T) {
	url := os.Getenv("LV_TEST_REALTIME_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_REALTIME_REDIS_URL to a disposable Redis database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("open Redis: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	envelope := newBudgetChangedEnvelope()
	channel := "project:" + envelope.ProjectID
	stream := channel + ":events"
	t.Cleanup(func() {
		if err := conn.Client.Del(context.Background(), stream).Err(); err != nil {
			t.Errorf("remove isolated replay stream: %v", err)
		}
	})
	sub := conn.Client.Subscribe(ctx, channel)
	t.Cleanup(func() { _ = sub.Close() })
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe project: %v", err)
	}
	handler := realtime.NewBudgetChangedHandler(
		&processedStore{seen: make(map[string]bool)}, redisrealtime.NewSink(conn.Client),
	)
	record := budgetRecord(t, envelope)
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("project budget change: %v", err)
	}
	message, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("receive budget event: %v", err)
	}
	assertBudgetRealtimeMessage(t, []byte(message.Payload), envelope.EventID, envelope.Data.AvailableMicros)
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("repeat budget change: %v", err)
	}
	if length := conn.Client.XLen(ctx, stream).Val(); length != 1 {
		t.Fatalf("replay stream length = %d, want 1", length)
	}
	replayed, resync, err := redisrealtime.NewSink(conn.Client).Replay(ctx, envelope.ProjectID, envelope.EventID)
	if err != nil || resync || len(replayed) != 0 {
		t.Fatalf("replay after event = %+v, resync = %t, error = %v", replayed, resync, err)
	}
	items, err := conn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(items) != 1 {
		t.Fatalf("budget replay entries = %+v, error = %v", items, err)
	}
	if items[0].Values["event"] != "budget.updated" || items[0].Values["id"] != envelope.EventID {
		t.Fatalf("budget replay entry = %+v", items[0].Values)
	}
	data, ok := items[0].Values["data"].(string)
	if !ok {
		t.Fatalf("budget replay data = %T", items[0].Values["data"])
	}
	assertBudgetRealtimeData(t, []byte(data), envelope.Data.AvailableMicros)
}

func assertBudgetRealtimeMessage(t *testing.T, raw []byte, eventID string, available int64) {
	t.Helper()
	var message struct {
		ID    string          `json:"id"`
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("decode budget realtime event: %v", err)
	}
	if message.ID != eventID || message.Event != "budget.updated" {
		t.Fatalf("budget realtime envelope = %+v", message)
	}
	assertBudgetRealtimeData(t, message.Data, available)
}

func assertBudgetRealtimeData(t *testing.T, raw []byte, available int64) {
	t.Helper()
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode budget realtime data: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("budget realtime data = %v, want only available_micros", data)
	}
	var got int64
	if err := json.Unmarshal(data["available_micros"], &got); err != nil || got != available {
		t.Fatalf("available_micros = %d, error = %v, want %d", got, err, available)
	}
}
