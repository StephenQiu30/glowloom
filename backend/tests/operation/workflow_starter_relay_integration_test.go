package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"go.opentelemetry.io/otel/trace/noop"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	kafkaoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	operationevent "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/event"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/kafkaconn"
)

func TestConfirmedOutboxStartsOneWorkflowThroughLocalKafkaAndTemporal(t *testing.T) {
	brokers := os.Getenv("LV_TEST_KAFKA_BROKERS")
	addr := os.Getenv("LV_TEST_TEMPORAL_ADDR")
	namespace := os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if os.Getenv("LV_TEST_OPERATION_STARTER_DB_DSN") == "" || brokers == "" || addr == "" || namespace == "" {
		t.Skip("set dedicated LV_TEST_OPERATION_STARTER_DB_DSN, local Kafka, and local Temporal test variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	database := operationStarterDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	operationStore := pgoperation.NewStore(database)
	if err := operationStore.CreateQuoteSnapshot(ctx, actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	if _, err := operationStore.ConfirmSingleQuote(ctx, actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	var confirmedEvent struct {
		ID      string
		Payload []byte
	}
	if err := database.WithContext(ctx).Raw(`
		SELECT id::text, payload FROM infra.outbox
		WHERE topic = ? AND payload->'data'->>'operation_id' = ?
	`, operationevent.OperationConfirmedTopic, quoted.ID.String()).Scan(&confirmedEvent).Error; err != nil || confirmedEvent.ID == "" {
		t.Fatalf("read committed confirmation event: %v, id=%q", err, confirmedEvent.ID)
	}
	temporalClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(temporalClient.Close)
	workflowID := "operation/" + quoted.ID.String()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = temporalClient.TerminateWorkflow(cleanupCtx, workflowID, "", "test cleanup")
	})
	kafkaConn, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kafkaConn.Close)
	if err := kafkaConn.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	ensureWorkflowStarterTopic(ctx, t, kafkaConn.Client, operationevent.OperationConfirmedTopic)
	assigned := make(chan struct{})
	var assignedOnce sync.Once
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumerGroup("lanverse-workflow-e2e-"+uuid.NewString()),
		kgo.ConsumeTopics(operationevent.OperationConfirmedTopic),
		kgo.ConsumeStartOffset(kgo.NewOffset().AtEnd()),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.OnPartitionsAssigned(func(context.Context, *kgo.Client, map[string][]int32) {
			assignedOnce.Do(func() { close(assigned) })
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(consumer.Close)
	handler := operationevent.NewWorkflowStarterHandler(pginbox.NewStore(database), operationStore,
		operationflow.NewStarter(temporalClient))
	result := make(chan error, 1)
	go consumeConfirmation(ctx, consumer, confirmedEvent.ID, handler, result)
	select {
	case <-assigned:
	case err := <-result:
		t.Fatalf("consumer stopped before assignment: %v", err)
	case <-ctx.Done():
		t.Fatalf("wait for Kafka assignment: %v", ctx.Err())
	}
	relay := outboxapp.NewRelay(pgoutbox.NewStore(database), kafkaoutbox.NewPublisher(kafkaConn.Client))
	for published := false; !published; {
		sent, err := relay.RunOnce(ctx)
		if err != nil || !sent {
			t.Fatalf("publish confirmation Outbox: sent=%v, error=%v", sent, err)
		}
		if err := database.WithContext(ctx).Raw(`
			SELECT published_at IS NOT NULL FROM infra.outbox WHERE id = ?::uuid
		`, confirmedEvent.ID).Scan(&published).Error; err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatalf("wait for confirmed Kafka event: %v", ctx.Err())
	}
	var marked bool
	if err := database.WithContext(ctx).Raw(`
		SELECT EXISTS (SELECT 1 FROM infra.processed_event
		WHERE consumer = 'workflow-starter' AND event_id = ?::uuid)
	`, confirmedEvent.ID).Scan(&marked).Error; err != nil || !marked {
		t.Fatalf("durable workflow-start marker = %v, error=%v", marked, err)
	}
	first, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		t.Fatalf("describe started workflow: %v", err)
	}
	if err := handler.Handle(ctx, inbox.Record{
		Topic: operationevent.OperationConfirmedTopic,
		Key:   []byte(projectID.String()), Value: confirmedEvent.Payload,
	}); err != nil {
		t.Fatalf("replay confirmation: %v", err)
	}
	last, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		t.Fatalf("describe replayed workflow: %v", err)
	}
	if first.WorkflowExecutionInfo.Execution.RunId != last.WorkflowExecutionInfo.Execution.RunId {
		t.Fatal("replay changed workflow run")
	}
}

func TestProductionRelayStartsConfirmedBatchThroughLocalKafka(t *testing.T) {
	brokers, redisURL := os.Getenv("LV_TEST_KAFKA_BROKERS"), os.Getenv("LV_TEST_REDIS_URL")
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if os.Getenv("LV_TEST_OPERATION_STARTER_DB_DSN") == "" || brokers == "" || redisURL == "" ||
		addr == "" || namespace == "" {
		t.Skip("set isolated PostgreSQL, local Kafka, Redis, and Temporal test variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	database := operationStarterDB(t)
	actor, projectID, batchID, _ := seedConfirmableBatch(t, database, 1)
	store := pgoperation.NewStore(database)
	if _, err := store.ConfirmBatchQuote(ctx, actor, operationapp.ConfirmBatchQuoteInput{
		ProjectID: projectID, BatchID: batchID, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatalf("commit real batch confirmation: %v", err)
	}
	eligible, err := store.ConfirmedBatch(ctx, actor.OrgID, projectID, batchID)
	if err != nil || !eligible {
		t.Fatalf("confirmed batch is not eligible for workflow starter: %v, %v", eligible, err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE infra.outbox SET published_at = now()
		WHERE published_at IS NULL AND topic <> ?
	`, operationevent.BatchConfirmedTopic).Error; err != nil {
		t.Fatal(err)
	}
	var eventID string
	if err := database.WithContext(ctx).Raw(`
		SELECT id FROM infra.outbox
		WHERE topic = ? AND payload->'data'->>'batch_id' = ?
	`, operationevent.BatchConfirmedTopic, batchID.String()).Scan(&eventID).Error; err != nil || eventID == "" {
		t.Fatalf("read real batch confirmation event: %v, id=%s", err, eventID)
	}
	kafkaConn, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kafkaConn.Close)
	if err := kafkaConn.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	ensureWorkflowStarterTopic(ctx, t, kafkaConn.Client, operationevent.BatchConfirmedTopic)
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	workflowID := "batch/" + batchID.String()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = workflowClient.TerminateWorkflow(cleanupCtx, workflowID, "", "test cleanup")
	})
	relayCtx, stopRelay := context.WithCancel(context.Background())
	relayDone := make(chan error, 1)
	go func() {
		relayDone <- app.RunRelay(relayCtx, config.Config{
			DBDSN: os.Getenv("LV_TEST_OPERATION_STARTER_DB_DSN"), KafkaBrokers: brokers,
			RedisURL: redisURL, TemporalAddr: addr, TemporalNamespace: namespace,
			RelayHealthAddr: "127.0.0.1:0",
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stopRelay()
		select {
		case err := <-relayDone:
			if err != nil {
				t.Errorf("stop production relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("production relay did not stop")
		}
	})
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var processed bool
		if err := database.WithContext(ctx).Raw(`
			SELECT EXISTS (SELECT 1 FROM infra.processed_event
			WHERE consumer = 'workflow-starter' AND event_id = ?::uuid)
		`, eventID).Scan(&processed).Error; err != nil {
			t.Fatal(err)
		}
		if processed {
			break
		}
		select {
		case err := <-relayDone:
			t.Fatalf("relay stopped before batch start: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for batch Kafka confirmation: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	first, err := workflowClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		t.Fatalf("describe confirmed batch workflow: %v", err)
	}
	if err := operationflow.NewStarter(workflowClient).StartBatch(ctx, batchID); err != nil {
		t.Fatalf("replay batch workflow start: %v", err)
	}
	last, err := workflowClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil || first.WorkflowExecutionInfo.Execution.RunId != last.WorkflowExecutionInfo.Execution.RunId {
		t.Fatalf("batch replay changed Temporal run: %v", err)
	}
}

func TestProductionRelayRecoversStaleConfirmedBatch(t *testing.T) {
	brokers, redisURL := os.Getenv("LV_TEST_KAFKA_BROKERS"), os.Getenv("LV_TEST_REDIS_URL")
	addr, namespace := os.Getenv("LV_TEST_TEMPORAL_ADDR"), os.Getenv("LV_TEST_TEMPORAL_NAMESPACE")
	if os.Getenv("LV_TEST_OPERATION_STARTER_DB_DSN") == "" || brokers == "" || redisURL == "" ||
		addr == "" || namespace == "" {
		t.Skip("set isolated PostgreSQL, local Kafka, Redis, and Temporal test variables")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	database, _, batchID, _ := confirmedOneItemBatchOnDB(t, operationStarterDB(t))
	if err := database.WithContext(ctx).Exec(`
		UPDATE operation.batch
		SET status = 'confirmed', update_time = now() - interval '3 minutes'
		WHERE id = ?::uuid
	`, batchID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctx).Exec(`
		UPDATE infra.outbox SET published_at = now() WHERE published_at IS NULL
	`).Error; err != nil {
		t.Fatal(err)
	}
	kafkaConn, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(kafkaConn.Close)
	if err := kafkaConn.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	ensureWorkflowStarterTopic(ctx, t, kafkaConn.Client, operationevent.BatchConfirmedTopic)
	workflowClient, err := client.Dial(client.Options{HostPort: addr, Namespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(workflowClient.Close)
	workflowID := "batch/" + batchID.String()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = workflowClient.TerminateWorkflow(cleanupCtx, workflowID, "", "test cleanup")
	})
	relayCtx, stopRelay := context.WithCancel(context.Background())
	relayDone := make(chan error, 1)
	go func() {
		relayDone <- app.RunRelay(relayCtx, config.Config{
			DBDSN: os.Getenv("LV_TEST_OPERATION_STARTER_DB_DSN"), KafkaBrokers: brokers,
			RedisURL: redisURL, TemporalAddr: addr, TemporalNamespace: namespace,
			RelayHealthAddr: "127.0.0.1:0",
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stopRelay()
		select {
		case err := <-relayDone:
			if err != nil {
				t.Errorf("stop recovery relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("recovery relay did not stop")
		}
	})
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := workflowClient.DescribeWorkflowExecution(ctx, workflowID, "")
		if err == nil {
			break
		}
		select {
		case relayErr := <-relayDone:
			t.Fatalf("relay stopped before batch recovery: %v", relayErr)
		case <-ctx.Done():
			t.Fatalf("wait for stale batch recovery: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	var batchEvents int64
	if err := database.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox
		WHERE topic = ? AND payload->'data'->>'batch_id' = ?
	`, operationevent.BatchConfirmedTopic, batchID.String()).Scan(&batchEvents).Error; err != nil || batchEvents != 0 {
		t.Fatalf("batch recovery unexpectedly used confirmation event: %d, %v", batchEvents, err)
	}
}

func operationStarterDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, os.Getenv("LV_TEST_OPERATION_STARTER_DB_DSN"), noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open dedicated workflow starter database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB.WithContext(ctx)
}

func ensureWorkflowStarterTopic(ctx context.Context, t *testing.T, kafkaClient *kgo.Client, topicName string) {
	t.Helper()
	request := kmsg.NewPtrCreateTopicsRequest()
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic = topicName
	topic.NumPartitions = 1
	topic.ReplicationFactor = 1
	request.Topics = append(request.Topics, topic)
	response, err := request.RequestWith(ctx, kafkaClient)
	if err != nil {
		t.Fatalf("create workflow starter test topic: %v", err)
	}
	if len(response.Topics) != 1 {
		t.Fatalf("create workflow starter test topic returned %d results", len(response.Topics))
	}
	if topicErr := kerr.ErrorForCode(response.Topics[0].ErrorCode); topicErr != nil && !errors.Is(topicErr, kerr.TopicAlreadyExists) {
		t.Fatalf("create workflow starter test topic: %v", topicErr)
	}
}

func consumeConfirmation(ctx context.Context, consumer *kgo.Client, targetEventID string, handler *operationevent.WorkflowStarterHandler, result chan<- error) {
	for {
		fetches := consumer.PollRecords(ctx, 1)
		if err := ctx.Err(); err != nil {
			consumer.AllowRebalance()
			result <- err
			return
		}
		for _, fetchErr := range fetches.Errors() {
			consumer.AllowRebalance()
			result <- fmt.Errorf("poll confirmation topic: %w", fetchErr.Err)
			return
		}
		for _, record := range fetches.Records() {
			var event struct {
				EventID string `json:"event_id"`
			}
			if err := json.Unmarshal(record.Value, &event); err != nil {
				consumer.AllowRebalance()
				result <- fmt.Errorf("decode confirmation event: %w", err)
				return
			}
			if event.EventID == targetEventID {
				if err := handler.Handle(ctx, inbox.Record{Topic: record.Topic, Key: record.Key, Value: record.Value}); err != nil {
					consumer.AllowRebalance()
					result <- err
					return
				}
			}
			if err := consumer.CommitRecords(ctx, record); err != nil {
				consumer.AllowRebalance()
				result <- fmt.Errorf("commit confirmation record: %w", err)
				return
			}
			if event.EventID == targetEventID {
				consumer.AllowRebalance()
				result <- nil
				return
			}
		}
		consumer.AllowRebalance()
	}
}
