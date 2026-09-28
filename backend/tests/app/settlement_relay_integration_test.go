package app_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/sse"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

const settlementBudgetLowTopic = "lanverse.billing.budget_low.v1"

func TestSettlementReachesBudgetSSEThroughRelay(t *testing.T) {
	dsn := os.Getenv("LV_TEST_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_RELAY_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_RELAY_REDIS_URL")
	if dsn == "" || brokers == "" || redisURL == "" {
		t.Skip("set disposable LV_TEST_RELAY_DB_DSN, LV_TEST_RELAY_KAFKA_BROKERS, and LV_TEST_RELAY_REDIS_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open disposable Redis database: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	var pending int64
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox WHERE published_at IS NULL AND NOT is_delete
	`).Scan(&pending).Error; err != nil {
		t.Fatalf("check disposable Outbox: %v", err)
	}
	if pending != 0 {
		t.Fatalf("test database has %d unrelated pending Outbox events; use an isolated disposable database", pending)
	}

	orgID, projectID, budgetID, operationID, reservationID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	stream := "project:" + projectID.String() + ":events"
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := redisConn.Client.Del(cleanupCtx, stream).Err(); err != nil {
			t.Errorf("remove settlement replay stream: %v", err)
		}
	})
	for _, fixture := range []struct {
		name string
		sql  string
		args []any
	}{
		{"organization", `INSERT INTO workspace.organization(id, name) VALUES (?::uuid, ?)`,
			[]any{orgID.String(), "Settlement relay test"}},
		{"project", `INSERT INTO workspace.project
			(id, org_id, name, aspect_ratio, style_type) VALUES (?::uuid, ?::uuid, ?, '16:9', 'realistic')`,
			[]any{projectID.String(), orgID.String(), "Settlement relay project"}},
		{"budget", `INSERT INTO billing.budget
			(id, project_id, limit_micros, reserved_micros) VALUES (?::uuid, ?::uuid, 100, 80)`,
			[]any{budgetID.String(), projectID.String()}},
		{"operation", `INSERT INTO operation.operation
			(id, project_id, capability, mode, input_hash, origin, status)
			VALUES (?::uuid, ?::uuid, 'image.generate', 'text_to_image', ?, 'upload', 'confirmed')`,
			[]any{operationID.String(), projectID.String(), "settlement-relay-" + operationID.String()}},
		{"reservation", `INSERT INTO billing.reservation
			(id, project_id, operation_id, amount_micros, status)
			VALUES (?::uuid, ?::uuid, ?::uuid, 80, 'held')`,
			[]any{reservationID.String(), projectID.String(), operationID.String()}},
		{"attach reservation", `UPDATE operation.operation SET reservation_id = ?::uuid WHERE id = ?::uuid`,
			[]any{reservationID.String(), operationID.String()}},
	} {
		if err := dbConn.DB.WithContext(ctx).Exec(fixture.sql, fixture.args...).Error; err != nil {
			t.Fatalf("create test %s: %v", fixture.name, err)
		}
	}

	streamHandler, err := sse.NewHandler(redisConn.Client, func(_ *http.Request, id string) bool {
		return id == projectID.String()
	}, zap.NewNop(), sse.Options{})
	if err != nil {
		t.Fatalf("create test-authorized SSE handler: %v", err)
	}
	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamHandler.ServeProject(w, r, projectID.String())
	}))
	t.Cleanup(streamServer.Close)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, streamServer.URL+"/events", nil)
	if err != nil {
		t.Fatalf("create SSE request: %v", err)
	}
	response, err := streamServer.Client().Do(request)
	if err != nil {
		t.Fatalf("connect project SSE: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("project SSE response = %d, %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("project SSE connection frame = %q, error = %v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("project SSE connection frame ending = %q, error = %v", line, err)
	}

	observer, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumeTopics(realtime.BillingSettledTopic, settlementBudgetLowTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("open settlement Kafka observer: %v", err)
	}
	t.Cleanup(observer.Close)
	runCtx, stopRelay := context.WithCancel(context.Background())
	relayDone := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		relayDone <- app.RunRelay(runCtx, config.Config{
			DBDSN: dsn, KafkaBrokers: brokers, RedisURL: redisURL,
			RelayHealthAddr: healthAddr,
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stopRelay()
		select {
		case err := <-relayDone:
			if err != nil {
				t.Errorf("stop settlement relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("settlement relay did not stop after cancellation")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)

	input := billingapp.SettleInput{
		ProjectID: projectID, OperationID: operationID, ActualCostMicros: 90,
		Capability: "image.generate", OccurredAt: time.Now().UTC(),
	}
	var settled billingapp.SettleResult
	if err := dbConn.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var settleErr error
		settled, settleErr = pgbilling.NewStore(dbConn.DB).SettleInTransaction(ctx, tx, input)
		return settleErr
	}); err != nil || settled.ChargeMicros != 90 || settled.ReleasedMicros != 0 || settled.BudgetOverrun {
		t.Fatalf("settle operation = %+v, error = %v", settled, err)
	}
	var budget struct {
		ReservedMicros int64
		SettledMicros  int64
		Revision       int64
	}
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT reserved_micros, settled_micros, revision FROM billing.budget WHERE id = ?::uuid
	`, budgetID.String()).Scan(&budget).Error; err != nil || budget.ReservedMicros != 0 ||
		budget.SettledMicros != 90 || budget.Revision != 2 {
		t.Fatalf("settled budget = %+v, error = %v", budget, err)
	}
	type outboxRow struct {
		ID      uuid.UUID
		Topic   string
		Payload []byte
	}
	var outbox []outboxRow
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT id, topic, payload FROM infra.outbox WHERE partition_key = ?
	`, projectID.String()).Scan(&outbox).Error; err != nil || len(outbox) != 2 {
		t.Fatalf("settlement Outbox = %+v, error = %v; want settled and budget_low", outbox, err)
	}
	events := make(map[string]outboxRow, len(outbox))
	for _, row := range outbox {
		if row.ID == uuid.Nil || events[row.Topic].ID != uuid.Nil {
			t.Fatalf("missing or duplicate settlement Outbox event: %+v", outbox)
		}
		events[row.Topic] = row
	}
	settlementEvent, settledOK := events[realtime.BillingSettledTopic]
	lowEvent, lowOK := events[settlementBudgetLowTopic]
	if !settledOK || !lowOK {
		t.Fatalf("settlement Outbox topics = %+v; want settled and budget_low", events)
	}
	var source struct {
		EventID uuid.UUID `json:"event_id"`
		Data    struct {
			AvailableMicros *int64 `json:"available_micros"`
		} `json:"data"`
	}
	if err := json.Unmarshal(settlementEvent.Payload, &source); err != nil || source.EventID != settlementEvent.ID ||
		source.Data.AvailableMicros == nil || *source.Data.AvailableMicros != 10 {
		t.Fatalf("source settlement event = %+v, error = %v", source, err)
	}
	var low struct {
		EventID   uuid.UUID `json:"event_id"`
		EventType string    `json:"event_type"`
		ProjectID uuid.UUID `json:"project_id"`
		Aggregate struct {
			Type     string    `json:"type"`
			ID       uuid.UUID `json:"id"`
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			LimitMicros     int64 `json:"limit_micros"`
			AvailableMicros int64 `json:"available_micros"`
			IsOverrun       bool  `json:"is_overrun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(lowEvent.Payload, &low); err != nil || low.EventID != lowEvent.ID ||
		low.EventType != settlementBudgetLowTopic || low.ProjectID != projectID ||
		low.Aggregate.Type != "budget" || low.Aggregate.ID != budgetID || low.Aggregate.Revision != 2 ||
		low.Data.LimitMicros != 100 || low.Data.AvailableMicros != 10 || low.Data.IsOverrun {
		t.Fatalf("source low-balance event = %+v, error = %v", low, err)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published, processed bool
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT count(*) = 2 FROM infra.outbox
			WHERE id IN (?::uuid, ?::uuid) AND published_at IS NOT NULL
		`, settlementEvent.ID.String(), lowEvent.ID.String()).Scan(&published).Error; err != nil {
			t.Fatalf("read settlement and low-balance publication: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'realtime' AND event_id = ?::uuid)
		`, settlementEvent.ID.String()).Scan(&processed).Error; err != nil {
			t.Fatalf("read settlement realtime marker: %v", err)
		}
		if published && processed {
			break
		}
		select {
		case err := <-relayDone:
			t.Fatalf("relay exited before settlement projection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for settlement relay: %v (published %t, processed %t)", ctx.Err(), published, processed)
		case <-ticker.C:
		}
	}
	observed := make(map[uuid.UUID]*kgo.Record, 2)
	for len(observed) < 2 && ctx.Err() == nil {
		fetches := observer.PollFetches(ctx)
		fetches.EachRecord(func(record *kgo.Record) {
			var envelope struct {
				EventID uuid.UUID `json:"event_id"`
			}
			if json.Unmarshal(record.Value, &envelope) == nil &&
				(envelope.EventID == settlementEvent.ID || envelope.EventID == lowEvent.ID) {
				observed[envelope.EventID] = record
			}
		})
		for _, fetchErr := range fetches.Errors() {
			if ctx.Err() == nil {
				t.Fatalf("observe settlement Kafka delivery: %v", fetchErr.Err)
			}
		}
	}
	if len(observed) != 2 {
		t.Fatalf("settlement Kafka events = %d, want two: %v", len(observed), ctx.Err())
	}
	for _, row := range []outboxRow{settlementEvent, lowEvent} {
		record := observed[row.ID]
		if record == nil || record.Topic != row.Topic || string(record.Key) != projectID.String() ||
			!bytes.Equal(record.Value, row.Payload) {
			t.Fatalf("settlement Kafka route or payload differs from Outbox: topic=%q record=%+v", row.Topic, record)
		}
	}
	var lowProcessed bool
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT EXISTS (SELECT 1 FROM infra.processed_event
		WHERE consumer = 'realtime' AND event_id = ?::uuid)
	`, lowEvent.ID.String()).Scan(&lowProcessed).Error; err != nil || lowProcessed {
		t.Fatalf("low-balance event was projected to realtime: processed=%t, error=%v", lowProcessed, err)
	}
	entries, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(entries) != 1 || entries[0].Values["id"] != settlementEvent.ID.String() ||
		entries[0].Values["event"] != "budget.updated" {
		t.Fatalf("settlement Redis projection = %+v, error = %v", entries, err)
	}
	streamData, ok := entries[0].Values["data"].(string)
	if !ok {
		t.Fatalf("settlement Redis data = %T", entries[0].Values["data"])
	}
	assertRelaySettlementData(t, []byte(streamData), 10)
	frame := nextNamedSSEFrame(t, reader)
	if !strings.Contains(frame, "id: "+settlementEvent.ID.String()+"\n") ||
		!strings.Contains(frame, "event: budget.updated\n") {
		t.Fatalf("settlement SSE event = %q", frame)
	}
	var sseData string
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "data: ") {
			sseData = strings.TrimPrefix(line, "data: ")
		}
	}
	assertRelaySettlementData(t, []byte(sseData), 10)

	handler := realtime.NewHandler(pginbox.NewStore(dbConn.DB), redisrealtime.NewSink(redisConn.Client))
	settledRecord := observed[settlementEvent.ID]
	if err := handler.Handle(ctx, inbox.Record{
		Topic: settledRecord.Topic, Key: settledRecord.Key, Value: settledRecord.Value,
	}); err != nil {
		t.Fatalf("reprocess delivered settlement event: %v", err)
	}
	if length, err := redisConn.Client.XLen(ctx, stream).Result(); err != nil || length != 1 {
		t.Fatalf("settlement replay stream after duplicate = %d, error = %v; want one", length, err)
	}
}

func assertRelaySettlementData(t *testing.T, raw []byte, available int64) {
	t.Helper()
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil || len(data) != 1 {
		t.Fatalf("settlement event data = %q, error = %v; want only available_micros", raw, err)
	}
	var got int64
	if err := json.Unmarshal(data["available_micros"], &got); err != nil || got != available {
		t.Fatalf("settlement available_micros = %d, error = %v; want %d", got, err, available)
	}
}
