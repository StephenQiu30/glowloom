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

func TestLegacyBillingSettlementResyncWithHostRedis(t *testing.T) {
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
	envelope := newSettlementEnvelope()
	record := settlementRecord(t, envelope)
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatalf("decode settlement: %v", err)
	}
	delete(body["data"].(map[string]any), "available_micros")
	record.Value, err = json.Marshal(body)
	if err != nil {
		t.Fatalf("encode legacy settlement: %v", err)
	}
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
	handler := realtime.NewBillingSettledHandler(
		&processedStore{seen: make(map[string]bool)}, redisrealtime.NewSink(conn.Client),
	)
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("project legacy settlement: %v", err)
	}
	message, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("receive project resync: %v", err)
	}
	var published struct {
		ID    string          `json:"id"`
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(message.Payload), &published); err != nil {
		t.Fatalf("decode project resync: %v", err)
	}
	if published.ID != envelope.EventID || published.Event != "resync" || string(published.Data) != "{}" {
		t.Fatalf("project resync = %+v", published)
	}
	items, err := conn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(items) != 1 {
		t.Fatalf("project replay entries = %+v, error = %v", items, err)
	}
	if items[0].Values["id"] != envelope.EventID || items[0].Values["event"] != "resync" || items[0].Values["data"] != "{}" {
		t.Fatalf("project replay entry = %+v", items[0].Values)
	}
}
