package eventing_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	eventinggorm "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/gormdb"
	eventingkafka "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/kafka"
	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	"github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestRealKafkaCarriesTheExactEnvelopeAndCommitsOnlyAcknowledgedRecords(t *testing.T) {
	rawBrokers := os.Getenv("LANVERSE_TEST_KAFKA_BROKERS")
	if rawBrokers == "" {
		t.Skip("set LANVERSE_TEST_KAFKA_BROKERS to run the real Kafka journey")
	}
	topic := os.Getenv("LANVERSE_TEST_KAFKA_STORYGRAPH_TOPIC")
	if topic == "" {
		topic = "lanverse.business.storygraph-version.published"
	}
	client, err := eventingkafka.New(eventingkafka.Config{
		Brokers: strings.Split(rawBrokers, ","), ClientID: "lanverse-kafka-integration",
		ConsumerGroup: "lanverse-kafka-integration-" + uuid.NewString(), Topics: []string{topic},
		Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = client.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	envelope := eventingFixtureForProject(t, uuid.NewString(), uuid.NewString(), 1)
	encoded, err := domain.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	message := eventingapp.Message{
		Topic: topic, Key: envelope.EventID, Value: encoded,
		Headers: map[string]string{"lanverse-event-id": envelope.EventID, "lanverse-schema": envelope.Schema},
	}
	if err = client.Publish(ctx, message); err != nil {
		t.Fatal(err)
	}
	handler := &kafkaHandler{expectedKey: envelope.EventID}
	if err = consumeUntilHandled(ctx, client, handler); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 1 || handler.message.Topic != topic || handler.message.Key != envelope.EventID ||
		string(handler.message.Value) != string(encoded) || handler.message.Offset < 0 {
		t.Fatalf("Kafka delivery changed the envelope or coordinate: %#v", handler)
	}
}

func TestRealKafkaRunRetriesTheSameUnacknowledgedRecord(t *testing.T) {
	rawBrokers := os.Getenv("LANVERSE_TEST_KAFKA_BROKERS")
	if rawBrokers == "" {
		t.Skip("set LANVERSE_TEST_KAFKA_BROKERS to run the real Kafka journey")
	}
	topic := os.Getenv("LANVERSE_TEST_KAFKA_STORYGRAPH_TOPIC")
	if topic == "" {
		topic = "lanverse.business.storygraph-version.published"
	}
	client, err := eventingkafka.New(eventingkafka.Config{
		Brokers: strings.Split(rawBrokers, ","), ClientID: "lanverse-kafka-retry-integration",
		ConsumerGroup: "lanverse-kafka-retry-integration-" + uuid.NewString(), Topics: []string{topic},
		Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	envelope := eventingFixtureForProject(t, uuid.NewString(), uuid.NewString(), 1)
	publishEnvelope(t, ctx, client, topic, envelope)
	sentinel := eventingFixtureForProject(t, uuid.NewString(), uuid.NewString(), 1)
	publishEnvelope(t, ctx, client, topic, sentinel)
	handler := &retryingKafkaHandler{
		expectedKey: envelope.EventID, sentinelKey: sentinel.EventID, handled: make(chan struct{}),
	}
	retries := 0
	runResult := make(chan error, 1)
	go func() {
		runResult <- client.Run(ctx, handler, func(error) { retries++ })
	}()
	select {
	case <-handler.handled:
	case <-ctx.Done():
		t.Fatal("Kafka consumer did not retry the unacknowledged record")
	}
	cancel()
	if err = <-runResult; err != nil {
		t.Fatal(err)
	}
	if handler.calls != 2 || len(handler.offsets) != 2 || handler.offsets[0] != handler.offsets[1] || retries != 1 {
		t.Fatalf("consumer did not retry the same Kafka coordinate: handler=%#v retries=%d", handler, retries)
	}
}

func TestRealKafkaDuplicateOutOfOrderDLQAndReplayConvergeThroughPostgreSQL(t *testing.T) {
	rawBrokers := os.Getenv("LANVERSE_TEST_KAFKA_BROKERS")
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if rawBrokers == "" || databaseURL == "" {
		t.Skip("set LANVERSE_TEST_KAFKA_BROKERS and LANVERSE_TEST_DATABASE_URL to run the Kafka/PostgreSQL journey")
	}
	topic := os.Getenv("LANVERSE_TEST_KAFKA_STORYGRAPH_TOPIC")
	if topic == "" {
		topic = "lanverse.business.storygraph-version.published"
	}
	dlqTopic := os.Getenv("LANVERSE_TEST_KAFKA_STORYGRAPH_DLQ_TOPIC")
	if dlqTopic == "" {
		dlqTopic = "lanverse.business.storygraph-version.dead-letter"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	workspaceID, projectID := uuid.New(), uuid.New()
	now := time.Date(2026, time.August, 27, 13, 0, 0, 0, time.UTC)
	if err = database.Create(&model.Workspace{
		ID: workspaceID, Name: "Kafka Eventing", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "Kafka Eventing", AspectRatio: "16:9", Language: "zh-CN",
		TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	group := "lanverse.search-projector.integration." + uuid.NewString()
	client, err := eventingkafka.New(eventingkafka.Config{
		Brokers: strings.Split(rawBrokers, ","), ClientID: "lanverse-eventing-integration",
		ConsumerGroup: group, Topics: []string{topic},
		Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	repository := eventinggorm.New(database)
	processor := &revisionProcessor{poisonRevision: 3}
	consumer := eventingapp.NewConsumer(repository, processor, client, eventingapp.ConsumerTopics{
		BusinessToDLQ: map[string]string{topic: dlqTopic},
	}, eventingapp.ConsumerConfig{
		Group: group, Now: func() time.Time { return now }, NewID: uuid.NewString,
		Lease: time.Minute, MaxAttempts: 3,
	})
	handler := &projectHandler{projectID: projectID.String(), consumer: consumer}

	revision2 := eventingFixtureForProject(t, workspaceID.String(), projectID.String(), 2)
	publishEnvelope(t, ctx, client, topic, revision2)
	publishEnvelope(t, ctx, client, topic, revision2)
	if err = consumeUntilProjectCalls(ctx, client, handler, 2); err != nil {
		t.Fatal(err)
	}
	if processor.successful != 1 {
		t.Fatalf("duplicate Kafka event reached projector %d times", processor.successful)
	}
	revision1 := eventingFixtureForProject(t, workspaceID.String(), projectID.String(), 1)
	publishEnvelope(t, ctx, client, topic, revision1)
	if err = consumeUntilProjectCalls(ctx, client, handler, 3); err != nil {
		t.Fatal(err)
	}
	if processor.successful != 1 {
		t.Fatalf("out-of-order Kafka event reached projector: %d", processor.successful)
	}

	revision3 := eventingFixtureForProject(t, workspaceID.String(), projectID.String(), 3)
	publishEnvelope(t, ctx, client, topic, revision3)
	if err = consumeUntilProjectCalls(ctx, client, handler, 4); err != nil {
		t.Fatal(err)
	}
	var deadLetter model.DeadLetter
	if err = database.Where("consumer_group = ? AND event_id = ?", group, revision3.EventID).First(&deadLetter).Error; err != nil {
		t.Fatal(err)
	}
	dlqConsumer, err := eventingkafka.New(eventingkafka.Config{
		Brokers: strings.Split(rawBrokers, ","), ClientID: "lanverse-dlq-integration",
		ConsumerGroup: "lanverse.dlq.integration." + uuid.NewString(), Topics: []string{dlqTopic},
		Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dlqConsumer.Close)
	dlqHandler := &kafkaHandler{expectedKey: revision3.EventID}
	if err = consumeUntilHandled(ctx, dlqConsumer, dlqHandler); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dlqHandler.message.Value), revision3.EventID) {
		t.Fatalf("DLQ record lost the original event identity: %s", dlqHandler.message.Value)
	}

	replayer := eventingapp.NewReplayer(repository, client, eventingapp.ReplayConfig{
		Now: func() time.Time { return now.Add(time.Minute) }, NewID: uuid.NewString,
		Lease: time.Minute, BatchSize: 10,
	})
	replayed, err := replayer.ReplayOnce(ctx, eventingapp.ReplayFilter{
		ConsumerGroup: group, ProjectID: projectID.String(),
		EventTypes:  []string{domain.StoryGraphVersionPublished},
		FailedAfter: now.Add(-time.Second), FailedBefore: now.Add(time.Minute),
	})
	if err != nil || replayed != 1 {
		t.Fatalf("Kafka replay failed: replayed=%d error=%v", replayed, err)
	}
	if err = consumeUntilProjectCalls(ctx, client, handler, 5); err != nil {
		t.Fatal(err)
	}
	if processor.successful != 2 || processor.poisoned != 1 {
		t.Fatalf("replay did not converge exactly once: %#v", processor)
	}
}

func TestRealKafkaOutboxSurvivesDisconnectedBrokerWithSameEventID(t *testing.T) {
	rawBrokers := os.Getenv("LANVERSE_TEST_KAFKA_BROKERS")
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if rawBrokers == "" || databaseURL == "" {
		t.Skip("set LANVERSE_TEST_KAFKA_BROKERS and LANVERSE_TEST_DATABASE_URL to run the Kafka outage journey")
	}
	topic := os.Getenv("LANVERSE_TEST_KAFKA_STORYGRAPH_TOPIC")
	if topic == "" {
		topic = "lanverse.business.storygraph-version.published"
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 27, 14, 0, 0, 0, time.UTC)
	workspaceID, projectID := uuid.New(), uuid.New()
	if err = database.Create(&model.Workspace{
		ID: workspaceID, Name: "Kafka Outage", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "Kafka Outage", AspectRatio: "16:9", Language: "zh-CN",
		TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	eventID, versionID := uuid.New(), uuid.New()
	payload := json.RawMessage(`{"content_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","owner_set_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","topology_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","version_id":"` + versionID.String() + `","version_no":1}`)
	payloadHash, err := domain.HashPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	outbox := model.OutboxEvent{
		ID: eventID, EventType: domain.StoryGraphVersionPublished, EventVersion: 1,
		WorkspaceID: workspaceID, ProjectID: projectID, AggregateKind: "storygraph",
		AggregateID: projectID.String(), AggregateRevision: 1, SourceReceiptID: uuid.New(),
		PayloadHash: payloadHash, Status: "pending", OccurredAt: now, CreatedAt: now,
	}
	outbox.Payload = append(outbox.Payload, payload...)
	if err = database.Create(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	repository := eventinggorm.New(database)
	disconnected, err := eventingkafka.New(eventingkafka.Config{Brokers: []string{"127.0.0.1:1"}, ClientID: "lanverse-disconnected", Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	disconnectedPublisher := eventingapp.NewPublisher(repository, disconnected, eventingapp.StaticTopics{
		domain.StoryGraphVersionPublished: topic,
	}, eventingapp.PublisherConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, Lease: time.Minute, BatchSize: 10})
	disconnectedContext, cancelDisconnected := context.WithTimeout(ctx, 750*time.Millisecond)
	_, publishErr := disconnectedPublisher.PublishOnce(disconnectedContext)
	cancelDisconnected()
	disconnected.Close()
	if publishErr == nil {
		t.Fatal("disconnected Kafka broker was reported as a successful publication")
	}
	var pending model.OutboxEvent
	if err = database.First(&pending, "id = ?", eventID).Error; err != nil || pending.Status != "pending" || pending.Attempts != 1 {
		t.Fatalf("broker outage changed the owner outbox fact: %#v error=%v", pending, err)
	}
	realClient, err := eventingkafka.New(eventingkafka.Config{Brokers: strings.Split(rawBrokers, ","), ClientID: "lanverse-recovered", Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(realClient.Close)
	recoveredPublisher := eventingapp.NewPublisher(repository, realClient, eventingapp.StaticTopics{
		domain.StoryGraphVersionPublished: topic,
	}, eventingapp.PublisherConfig{Now: func() time.Time { return now.Add(2 * time.Minute) }, NewID: uuid.NewString, Lease: time.Minute, BatchSize: 10})
	if published, recoverErr := recoveredPublisher.PublishOnce(ctx); recoverErr != nil || published < 1 {
		t.Fatalf("outbox did not recover through the real broker: published=%d error=%v", published, recoverErr)
	}
	consumer, err := eventingkafka.New(eventingkafka.Config{
		Brokers: strings.Split(rawBrokers, ","), ClientID: "lanverse-recovered-consumer",
		ConsumerGroup: "lanverse.recovered." + uuid.NewString(), Topics: []string{topic},
		Username: os.Getenv("LANVERSE_TEST_KAFKA_USERNAME"), Password: os.Getenv("LANVERSE_TEST_KAFKA_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(consumer.Close)
	waitContext, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()
	handler := &kafkaHandler{expectedKey: eventID.String()}
	if err = consumeUntilHandled(waitContext, consumer, handler); err != nil {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeEnvelope(handler.message.Value)
	if err != nil || decoded.EventID != eventID.String() {
		t.Fatalf("broker recovery changed the event identity: %#v error=%v", decoded, err)
	}
	if err = database.First(&pending, "id = ?", eventID).Error; err != nil || pending.Status != "published" || pending.Attempts != 2 {
		t.Fatalf("recovered outbox state is wrong: %#v error=%v", pending, err)
	}
}

type kafkaHandler struct {
	expectedKey string
	calls       int
	message     eventingapp.IncomingMessage
}

type retryingKafkaHandler struct {
	expectedKey string
	sentinelKey string
	calls       int
	offsets     []int64
	handled     chan struct{}
}

func (handler *retryingKafkaHandler) Handle(_ context.Context, message eventingapp.IncomingMessage) (eventingapp.HandleResult, error) {
	if message.Key == handler.sentinelKey {
		close(handler.handled)
		return eventingapp.HandleResult{Ack: true}, nil
	}
	if message.Key != handler.expectedKey {
		return eventingapp.HandleResult{Ack: true}, nil
	}
	handler.calls++
	handler.offsets = append(handler.offsets, message.Offset)
	if handler.calls == 1 {
		return eventingapp.HandleResult{}, errors.New("temporary projector failure")
	}
	return eventingapp.HandleResult{Ack: true}, nil
}

func (handler *kafkaHandler) Handle(_ context.Context, message eventingapp.IncomingMessage) (eventingapp.HandleResult, error) {
	if message.Key != handler.expectedKey {
		return eventingapp.HandleResult{Ack: true}, nil
	}
	handler.calls++
	handler.message = message
	return eventingapp.HandleResult{Ack: true}, nil
}

type projectHandler struct {
	projectID string
	consumer  *eventingapp.Consumer
	calls     int
}

func (handler *projectHandler) Handle(ctx context.Context, message eventingapp.IncomingMessage) (eventingapp.HandleResult, error) {
	envelope, err := domain.DecodeEnvelope(message.Value)
	if err != nil || envelope.ProjectID != handler.projectID {
		return eventingapp.HandleResult{Ack: true}, nil
	}
	handler.calls++
	return handler.consumer.Handle(ctx, message)
}

type revisionProcessor struct {
	poisonRevision int64
	poisoned       int
	successful     int
}

func (processor *revisionProcessor) Process(_ context.Context, envelope domain.Envelope) error {
	if envelope.AggregateRevision == processor.poisonRevision && processor.poisoned == 0 {
		processor.poisoned++
		return eventingapp.Permanent(context.Canceled)
	}
	processor.successful++
	return nil
}

func publishEnvelope(t *testing.T, ctx context.Context, client *eventingkafka.Client, topic string, envelope domain.Envelope) {
	t.Helper()
	encoded, err := domain.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Publish(ctx, eventingapp.Message{Topic: topic, Key: envelope.EventID, Value: encoded}); err != nil {
		t.Fatal(err)
	}
}

func consumeUntilHandled(ctx context.Context, client *eventingkafka.Client, handler *kafkaHandler) error {
	for handler.calls == 0 && ctx.Err() == nil {
		if err := client.ConsumeOnce(ctx, handler); err != nil {
			return err
		}
	}
	if handler.calls == 0 {
		return context.DeadlineExceeded
	}
	return nil
}

func consumeUntilProjectCalls(ctx context.Context, client *eventingkafka.Client, handler *projectHandler, expected int) error {
	for handler.calls < expected && ctx.Err() == nil {
		if err := client.ConsumeOnce(ctx, handler); err != nil {
			return err
		}
	}
	if handler.calls < expected {
		return context.DeadlineExceeded
	}
	return nil
}
