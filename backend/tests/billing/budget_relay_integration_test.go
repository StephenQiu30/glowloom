package billing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace/noop"

	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	billingapp "github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	kafkaoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/kafkaconn"
)

const budgetLowTopic = "lanverse.billing.budget_low.v1"

func TestBudgetThresholdEventReachesKafkaThroughOutboxRelay(t *testing.T) {
	dsn := os.Getenv("LV_TEST_BILLING_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_BILLING_RELAY_KAFKA_BROKERS")
	if dsn == "" || brokers == "" {
		t.Skip("set LV_TEST_BILLING_RELAY_DB_DSN to a migrated disposable database and LV_TEST_BILLING_RELAY_KAFKA_BROKERS to local Kafka with billing, audit, and budget_low topics")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	connection, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open budget relay test database: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	database := connection.DB.WithContext(ctx)
	var pending int64
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE published_at IS NULL AND NOT is_delete`).Scan(&pending).Error; err != nil {
		t.Fatalf("check isolated Outbox: %v", err)
	}
	if pending != 0 {
		t.Fatalf("budget relay test database has %d pending events; use an isolated disposable database", pending)
	}

	kafkaConn, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatalf("open Kafka producer: %v", err)
	}
	t.Cleanup(kafkaConn.Close)
	reader, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumeTopics(budgetLowTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("open budget low Kafka reader: %v", err)
	}
	t.Cleanup(reader.Close)

	actor, projectID := billingProject(t, database)
	budgetID := uuid.New()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		cleanupDB := connection.DB.WithContext(cleanupCtx)
		for _, row := range []struct {
			statement string
			id        uuid.UUID
		}{
			{`DELETE FROM infra.outbox WHERE partition_key = ?`, projectID},
			{`UPDATE billing.ledger_entry SET is_delete = true WHERE project_id = ?::uuid AND NOT is_delete`, projectID},
			{`DELETE FROM billing.budget WHERE project_id = ?::uuid`, projectID},
			{`DELETE FROM workspace.project WHERE id = ?::uuid`, projectID},
			{`DELETE FROM identity."user" WHERE id = ?::uuid`, actor.ID},
			{`DELETE FROM workspace.organization WHERE id = ?::uuid`, actor.OrgID},
		} {
			if err := cleanupDB.Exec(row.statement, row.id.String()).Error; err != nil {
				t.Errorf("clean budget relay fixture: %v", err)
			}
		}
	})
	if err := database.Exec(`
		INSERT INTO billing.budget (id, project_id, limit_micros, settled_micros)
		VALUES (?::uuid, ?::uuid, 1000, 750)
	`, budgetID.String(), projectID.String()).Error; err != nil {
		t.Fatalf("create budget at 25 percent available: %v", err)
	}
	changed, err := billingapp.NewChangeBudgetCommand(pgbilling.NewStore(database), time.Now).Execute(ctx, actor, billingapp.ChangeBudgetInput{
		ProjectID: projectID, LimitMicros: 900, ExpectedRevision: 1, RequestID: uuid.NewString(),
	})
	if err != nil || changed.LimitMicros != 900 || changed.SettledMicros != 750 || changed.Revision != 2 {
		t.Fatalf("change budget across 20 percent threshold: %+v, error = %v", changed, err)
	}
	var lowRows []struct {
		ID      uuid.UUID
		Payload []byte
	}
	if err := database.Raw(`
		SELECT id, payload FROM infra.outbox
		WHERE partition_key = ? AND topic = ?
	`, projectID.String(), budgetLowTopic).Scan(&lowRows).Error; err != nil {
		t.Fatalf("read budget low Outbox row: %v", err)
	}
	if len(lowRows) != 1 {
		t.Fatalf("budget low Outbox rows = %d, want 1", len(lowRows))
	}
	var eventCount int64
	if err := database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, projectID.String()).Scan(&eventCount).Error; err != nil || eventCount != 3 {
		t.Fatalf("budget Outbox event count = %d, error = %v; want changed, audit, and low", eventCount, err)
	}

	relay := outboxapp.NewRelay(pgoutbox.NewStore(database), kafkaoutbox.NewPublisher(kafkaConn.Client))
	var published bool
	for range 3 {
		sent, err := relay.RunOnce(ctx)
		if err != nil || !sent {
			t.Fatalf("relay budget event = (%t, %v)", sent, err)
		}
		if err := database.Raw(`SELECT published_at IS NOT NULL FROM infra.outbox WHERE id = ?::uuid`, lowRows[0].ID.String()).Scan(&published).Error; err != nil {
			t.Fatalf("read budget low publication time: %v", err)
		}
		if published {
			break
		}
	}
	if !published {
		t.Fatal("budget low Outbox row was not acknowledged after relaying three events")
	}

	var got *kgo.Record
	for got == nil && ctx.Err() == nil {
		fetches := reader.PollFetches(ctx)
		fetches.EachRecord(func(record *kgo.Record) {
			var envelope struct {
				EventID uuid.UUID `json:"event_id"`
			}
			if json.Unmarshal(record.Value, &envelope) == nil && envelope.EventID == lowRows[0].ID {
				got = record
			}
		})
		for _, fetchErr := range fetches.Errors() {
			if ctx.Err() == nil {
				t.Fatalf("consume budget low Kafka event: %v", fetchErr.Err)
			}
		}
	}
	if got == nil {
		t.Fatalf("budget low event %s was not consumed: %v", lowRows[0].ID, ctx.Err())
	}
	if got.Topic != budgetLowTopic || string(got.Key) != projectID.String() || !bytes.Equal(got.Value, lowRows[0].Payload) {
		t.Fatalf("Kafka budget low route or payload differs from Outbox: topic=%q key=%q payload=%q", got.Topic, got.Key, got.Value)
	}
	var envelope struct {
		EventID    uuid.UUID `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      uuid.UUID `json:"org_id"`
		ProjectID  uuid.UUID `json:"project_id"`
		Aggregate  struct {
			Type     string    `json:"type"`
			ID       uuid.UUID `json:"id"`
			Revision int64     `json:"revision"`
		} `json:"aggregate"`
		Data struct {
			LimitMicros     int64 `json:"limit_micros"`
			AvailableMicros int64 `json:"available_micros"`
		} `json:"data"`
	}
	if err := json.Unmarshal(got.Value, &envelope); err != nil ||
		envelope.EventID != lowRows[0].ID || envelope.EventType != budgetLowTopic ||
		envelope.OccurredAt.IsZero() || envelope.OrgID != actor.OrgID ||
		envelope.ProjectID != projectID || envelope.Aggregate.Type != "budget" ||
		envelope.Aggregate.ID != budgetID || envelope.Aggregate.Revision != 2 ||
		envelope.Data.LimitMicros != 900 || envelope.Data.AvailableMicros != 150 {
		t.Fatalf("relayed budget low envelope = %+v, error = %v", envelope, err)
	}
}
