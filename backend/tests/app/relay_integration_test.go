package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestRelayRolePublishesAndProjectsOnLocalServices(t *testing.T) {
	dsn := os.Getenv("LV_TEST_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_RELAY_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_RELAY_REDIS_URL")
	if dsn == "" || brokers == "" || redisURL == "" {
		t.Skip("set disposable LV_TEST_RELAY_DB_DSN, LV_TEST_RELAY_KAFKA_BROKERS, and LV_TEST_RELAY_REDIS_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open test Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	eventID, projectID, operationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	stream := "project:" + projectID + ":events"
	t.Cleanup(func() { _ = redisConn.Client.Del(context.Background(), stream).Err() })
	payload := `{"event_id":"` + eventID + `","event_type":"` + realtime.OperationStatusTopic +
		`","project_id":"` + projectID + `","aggregate":{"type":"operation","id":"` + operationID +
		`"},"data":{"target_type":"shot_frame","status":"submitted"}}`
	if err := dbConn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload) VALUES (?::uuid, ?, ?, ?::jsonb)",
		eventID, realtime.OperationStatusTopic, projectID, payload,
	).Error; err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", eventID).Error })
	workerCtx, stopRelay := context.WithCancel(context.Background())
	relayDone := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		relayDone <- app.RunRelay(workerCtx, config.Config{
			DBDSN: dsn, KafkaBrokers: brokers, RedisURL: redisURL,
			RelayHealthAddr: healthAddr,
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stopRelay()
		select {
		case err := <-relayDone:
			if err != nil {
				t.Errorf("stop relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("relay did not stop after cancellation")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published, processed bool
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT published_at IS NOT NULL FROM infra.outbox WHERE id = ?::uuid", eventID,
		).Scan(&published).Error; err != nil {
			t.Fatalf("read Outbox delivery: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT EXISTS (SELECT 1 FROM infra.processed_event WHERE consumer = 'realtime' AND event_id = ?::uuid)", eventID,
		).Scan(&processed).Error; err != nil {
			t.Fatalf("read consumer marker: %v", err)
		}
		if published && processed {
			entries, err := redisConn.Client.XRange(ctx, stream, "-", "+").Result()
			if err != nil || len(entries) != 1 || entries[0].Values["id"] != eventID {
				t.Fatalf("Redis projected entries = %+v, error = %v", entries, err)
			}
			return
		}
		select {
		case err := <-relayDone:
			t.Fatalf("relay exited before projection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for relay delivery: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestRelayRoleConsumesAuditOnLocalServices(t *testing.T) {
	dsn := os.Getenv("LV_TEST_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_RELAY_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_RELAY_REDIS_URL")
	if dsn == "" || brokers == "" || redisURL == "" {
		t.Skip("set disposable LV_TEST_RELAY_DB_DSN, LV_TEST_RELAY_KAFKA_BROKERS, and LV_TEST_RELAY_REDIS_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	eventID, orgID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	const topic = "lanverse.audit.recorded.v1"
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": topic, "occurred_at": time.Now().UTC(), "org_id": orgID,
		"actor":     map[string]any{"kind": "user", "id": actorID},
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"data": map[string]any{
			"action": "auth.login_succeeded", "object": map[string]any{"type": "user", "id": actorID},
			"request_id": "relay-audit-test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dbConn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload) VALUES (?::uuid, ?, ?, ?::jsonb)",
		eventID, topic, orgID, string(payload),
	).Error; err != nil {
		t.Fatalf("insert audit Outbox event: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", eventID).Error })
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		done <- app.RunRelay(runCtx, config.Config{
			DBDSN: dsn, KafkaBrokers: brokers, RedisURL: redisURL,
			RelayHealthAddr: healthAddr,
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stop audit relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("audit relay did not stop after cancellation")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published, recorded, processed bool
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT published_at IS NOT NULL FROM infra.outbox WHERE id = ?::uuid", eventID,
		).Scan(&published).Error; err != nil {
			t.Fatalf("read audit Outbox delivery: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT EXISTS (SELECT 1 FROM audit.audit_log WHERE id = ?::uuid AND org_id = ?::uuid AND action = 'auth.login_succeeded')", eventID, orgID,
		).Scan(&recorded).Error; err != nil {
			t.Fatalf("read audit row: %v", err)
		}
		if err := dbConn.DB.WithContext(ctx).Raw(
			"SELECT EXISTS (SELECT 1 FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid)", eventID,
		).Scan(&processed).Error; err != nil {
			t.Fatalf("read audit consumer marker: %v", err)
		}
		if published && recorded && processed {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("audit relay exited early: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for audit delivery: %v (published %t, recorded %t, processed %t)", ctx.Err(), published, recorded, processed)
		case <-ticker.C:
		}
	}
}

func TestRelayRoleStopsOnInvalidOutboxEvent(t *testing.T) {
	dsn := os.Getenv("LV_TEST_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_RELAY_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_RELAY_REDIS_URL")
	if dsn == "" || brokers == "" || redisURL == "" {
		t.Skip("set disposable LV_TEST_RELAY_DB_DSN, LV_TEST_RELAY_KAFKA_BROKERS, and LV_TEST_RELAY_REDIS_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	eventID := uuid.NewString()
	if err := dbConn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload) VALUES (?::uuid, ?, ?, ?::jsonb)",
		eventID, realtime.OperationStatusTopic, uuid.NewString(),
		`{"event_id":"`+eventID+`","event_type":"wrong.topic"}`,
	).Error; err != nil {
		t.Fatalf("insert invalid Outbox event: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", eventID).Error })
	err = app.RunRelay(ctx, config.Config{
		DBDSN: dsn, KafkaBrokers: brokers, RedisURL: redisURL,
		RelayHealthAddr: "127.0.0.1:0",
	}, zap.NewNop())
	if !errors.Is(err, outboxapp.ErrInvalidEnvelope) {
		t.Fatalf("RunRelay error = %v, want invalid Outbox envelope", err)
	}
}
