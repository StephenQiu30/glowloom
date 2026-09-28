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

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/sse"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestBudgetChangeReachesSSEThroughRelay(t *testing.T) {
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

	orgID, actorID, projectID, budgetID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	stream := "project:" + projectID.String() + ":events"
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := redisConn.Client.Del(cleanupCtx, stream).Err(); err != nil {
			t.Errorf("remove budget replay stream: %v", err)
		}
	})
	for _, fixture := range []struct {
		name string
		sql  string
		args []any
	}{
		{"organization", `INSERT INTO workspace.organization(id, name) VALUES (?::uuid, ?)`,
			[]any{orgID.String(), "Budget relay test"}},
		{"actor", `INSERT INTO identity."user"
			(id, org_id, login_name, display_name, role, password_hash, must_change_password)
			VALUES (?::uuid, ?::uuid, ?, ?, 'producer', ?, false)`,
			[]any{actorID.String(), orgID.String(), "budget-relay-" + actorID.String(), "Budget relay actor", "unused-test-hash"}},
		{"project", `INSERT INTO workspace.project
			(id, org_id, name, aspect_ratio, style_type) VALUES (?::uuid, ?::uuid, ?, '16:9', 'realistic')`,
			[]any{projectID.String(), orgID.String(), "Budget relay project"}},
		{"budget", `INSERT INTO billing.budget
			(id, project_id, limit_micros, settled_micros) VALUES (?::uuid, ?::uuid, 500, 100)`,
			[]any{budgetID.String(), projectID.String()}},
	} {
		if err := dbConn.DB.WithContext(ctx).Exec(fixture.sql, fixture.args...).Error; err != nil {
			t.Fatalf("insert test %s: %v", fixture.name, err)
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
		kgo.ConsumeTopics(realtime.BudgetChangedTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("open budget Kafka observer: %v", err)
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
				t.Errorf("stop budget relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("budget relay did not stop after cancellation")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)

	actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: identitydomain.RoleProducer}
	changed, err := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(dbConn.DB), time.Now).Execute(
		ctx, actor, billingapp.ChangeBudgetInput{
			ProjectID: projectID, LimitMicros: 600, ExpectedRevision: 1, RequestID: uuid.NewString(),
		},
	)
	if err != nil || changed.ID != budgetID || changed.Revision != 2 || changed.LimitMicros != 600 {
		t.Fatalf("change budget command = %+v, error = %v", changed, err)
	}
	available, err := changed.AvailableMicros()
	if err != nil || available != 500 {
		t.Fatalf("changed available balance = %d, error = %v", available, err)
	}
	var events []struct {
		ID      uuid.UUID
		Topic   string
		Payload []byte
	}
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT id, topic, payload FROM infra.outbox WHERE partition_key = ?
	`, projectID.String()).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("budget Outbox events = %+v, error = %v; want change and audit", events, err)
	}
	var changeEvent, auditEvent uuid.UUID
	var changePayload []byte
	for _, event := range events {
		switch event.Topic {
		case realtime.BudgetChangedTopic:
			changeEvent, changePayload = event.ID, event.Payload
		case "lanverse.audit.recorded.v1":
			auditEvent = event.ID
		default:
			t.Fatalf("unexpected budget Outbox topic %q", event.Topic)
		}
	}
	if changeEvent == uuid.Nil || auditEvent == uuid.Nil {
		t.Fatalf("budget Outbox missing change or audit event: %+v", events)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published, realtimeProcessed, auditProcessed, audited bool
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT count(*) = 2 FROM infra.outbox
			WHERE id IN (?::uuid, ?::uuid) AND published_at IS NOT NULL
		`, changeEvent.String(), auditEvent.String()).Scan(&published).Error; err != nil {
			t.Fatalf("read budget Outbox delivery: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'realtime' AND event_id = ?::uuid)
		`, changeEvent.String()).Scan(&realtimeProcessed).Error; err != nil {
			t.Fatalf("read budget realtime marker: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'audit' AND event_id = ?::uuid)
		`, auditEvent.String()).Scan(&auditProcessed).Error; err != nil {
			t.Fatalf("read budget audit marker: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM audit.audit_log
			WHERE id = ?::uuid AND project_id = ?::uuid AND action = 'budget.changed')
		`, auditEvent.String(), projectID.String()).Scan(&audited).Error; err != nil {
			t.Fatalf("read budget audit row: %v", err)
		}
		if published && realtimeProcessed && auditProcessed && audited {
			break
		}
		select {
		case err := <-relayDone:
			t.Fatalf("relay exited before budget projection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for budget relay: %v (published %t, realtime %t, audit %t, audited %t)",
				ctx.Err(), published, realtimeProcessed, auditProcessed, audited)
		case <-ticker.C:
		}
	}

	var observed *kgo.Record
	for observed == nil && ctx.Err() == nil {
		fetches := observer.PollFetches(ctx)
		fetches.EachRecord(func(record *kgo.Record) {
			var envelope struct {
				EventID uuid.UUID `json:"event_id"`
			}
			if json.Unmarshal(record.Value, &envelope) == nil && envelope.EventID == changeEvent {
				observed = record
			}
		})
		for _, fetchErr := range fetches.Errors() {
			if ctx.Err() == nil {
				t.Fatalf("observe budget Kafka delivery: %v", fetchErr.Err)
			}
		}
	}
	if observed == nil {
		t.Fatalf("budget Kafka event %s not observed: %v", changeEvent, ctx.Err())
	}
	if observed.Topic != realtime.BudgetChangedTopic || string(observed.Key) != projectID.String() ||
		!bytes.Equal(observed.Value, changePayload) {
		t.Fatalf("budget Kafka route or payload differs from Outbox: topic=%q key=%q", observed.Topic, observed.Key)
	}
	entries, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(entries) != 1 || entries[0].Values["id"] != changeEvent.String() ||
		entries[0].Values["event"] != "budget.updated" {
		t.Fatalf("budget Redis projection = %+v, error = %v", entries, err)
	}
	streamData, ok := entries[0].Values["data"].(string)
	if !ok {
		t.Fatalf("budget Redis data = %T", entries[0].Values["data"])
	}
	assertRelayBudgetData(t, []byte(streamData), available)
	frame := nextNamedSSEFrame(t, reader)
	if !strings.Contains(frame, "id: "+changeEvent.String()+"\n") ||
		!strings.Contains(frame, "event: budget.updated\n") {
		t.Fatalf("budget SSE event = %q", frame)
	}
	var sseData string
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "data: ") {
			sseData = strings.TrimPrefix(line, "data: ")
		}
	}
	assertRelayBudgetData(t, []byte(sseData), available)

	// Reprocessing the same Kafka record must observe the durable consumer marker
	// and leave the project stream unchanged.
	handler := realtime.NewHandler(pginbox.NewStore(dbConn.DB), redisrealtime.NewSink(redisConn.Client))
	if err := handler.Handle(ctx, inbox.Record{
		Topic: observed.Topic, Key: observed.Key, Value: observed.Value,
	}); err != nil {
		t.Fatalf("reprocess delivered budget event: %v", err)
	}
	if length, err := redisConn.Client.XLen(ctx, stream).Result(); err != nil || length != 1 {
		t.Fatalf("budget replay stream after duplicate = %d, error = %v; want one", length, err)
	}
	var markerCount int64
	if err := dbConn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.processed_event
		WHERE consumer = 'realtime' AND event_id = ?::uuid
	`, changeEvent.String()).Scan(&markerCount).Error; err != nil || markerCount != 1 {
		t.Fatalf("budget realtime markers after duplicate = %d, error = %v", markerCount, err)
	}
}

func assertRelayBudgetData(t *testing.T, raw []byte, available int64) {
	t.Helper()
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil || len(data) != 1 {
		t.Fatalf("budget event data = %q, error = %v; want only available_micros", raw, err)
	}
	var got int64
	if err := json.Unmarshal(data["available_micros"], &got); err != nil || got != available {
		t.Fatalf("budget available_micros = %d, error = %v; want %d", got, err, available)
	}
}
