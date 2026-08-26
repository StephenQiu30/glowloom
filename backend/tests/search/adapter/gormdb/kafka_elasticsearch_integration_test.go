package gormdb_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	eventinggorm "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/gormdb"
	eventingkafka "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/kafka"
	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	searches "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/elasticsearch"
	searchgorm "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/gormdb"
	searchproject "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/projectaccess"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

func TestRealKafkaProjectsPostgreSQLOwnersIntoElasticsearchWithDeepLinks(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	elasticsearchURL := os.Getenv("LANVERSE_TEST_ELASTICSEARCH_URL")
	kafkaBrokers := os.Getenv("LANVERSE_TEST_KAFKA_BROKERS")
	if databaseURL == "" || elasticsearchURL == "" || kafkaBrokers == "" {
		t.Skip("set PostgreSQL, Elasticsearch, and Kafka test endpoints to run the Search event journey")
	}
	scriptTopic := environmentOr("LANVERSE_TEST_KAFKA_SCRIPT_TOPIC", "lanverse.business.script-version.v1")
	storyGraphTopic := environmentOr("LANVERSE_TEST_KAFKA_STORYGRAPH_TOPIC", "lanverse.business.storygraph-version.v1")
	scriptDLQ := environmentOr("LANVERSE_TEST_KAFKA_SCRIPT_DLQ_TOPIC", "lanverse.business.script-version.dlq.v1")
	storyGraphDLQ := environmentOr("LANVERSE_TEST_KAFKA_STORYGRAPH_DLQ_TOPIC", "lanverse.business.storygraph-version.dlq.v1")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	fixture := seedSearchOwners(t, func(value any) error { return database.Create(value).Error })

	prefix := "lanverse-kafka-search-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	index, err := searches.New(searches.Config{
		Addresses: []string{elasticsearchURL}, ScriptAlias: prefix + "-script-v1", StoryGraphAlias: prefix + "-storygraph-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteSearchIndices(elasticsearchURL, prefix) })

	group := "lanverse.search.integration." + uuid.NewString()
	client, err := eventingkafka.New(eventingkafka.Config{
		Brokers: strings.Split(kafkaBrokers, ","), ClientID: "lanverse-search-integration-" + uuid.NewString(),
		ConsumerGroup: group, Topics: []string{scriptTopic, storyGraphTopic},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err = client.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 22, 0, 0, 0, time.UTC)
	snapshots := searchgorm.New(database)
	projector := searchapp.NewProjector(snapshots, index, func() time.Time { return now })
	consumer := eventingapp.NewConsumer(eventinggorm.New(database), projector, client, eventingapp.ConsumerTopics{
		BusinessToDLQ: map[string]string{scriptTopic: scriptDLQ, storyGraphTopic: storyGraphDLQ},
	}, eventingapp.ConsumerConfig{
		Group: group, Now: func() time.Time { return now }, NewID: uuid.NewString,
		Lease: time.Minute, MaxAttempts: 3,
	})
	handler := &projectSearchHandler{
		projectID: fixture.projectID.String(), consumer: consumer, projected: make(map[string]bool), done: make(chan struct{}),
	}
	scriptEvent := newScriptEnvelope(t, fixture, now)
	storyGraphEvent := newStoryGraphEnvelope(t, fixture, now)
	publishSearchEnvelope(t, ctx, client, scriptTopic, scriptEvent)
	publishSearchEnvelope(t, ctx, client, storyGraphTopic, storyGraphEvent)
	consumerContext, stopConsumer := context.WithCancel(ctx)
	consumerResult := make(chan error, 1)
	go func() { consumerResult <- client.Run(consumerContext, handler, nil) }()
	select {
	case <-handler.done:
	case <-ctx.Done():
		t.Fatal("Kafka events were not projected before the deadline")
	}
	stopConsumer()
	if err = <-consumerResult; err != nil {
		t.Fatal(err)
	}
	if !handler.projected[scriptEvent.EventID] || !handler.projected[storyGraphEvent.EventID] {
		t.Fatalf("Kafka events did not project both Owner snapshots: %#v", handler.projected)
	}

	projects := projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString)
	service := searchapp.NewService(searchproject.New(projects), snapshots, index)
	actor := searchapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	scriptResult, err := service.SearchScripts(ctx, actor, searchapp.Query{ProjectID: fixture.projectID.String(), Text: "雨夜", Limit: 10})
	if err != nil || scriptResult.Status != search.StatusFresh || len(scriptResult.Hits) != 1 ||
		scriptResult.Source == nil || scriptResult.Source.ID != scriptEvent.EventID || scriptResult.Hits[0].Evidence[0].Href == "" {
		t.Fatalf("Kafka Script projection is not queryable with provenance: %#v err=%v", scriptResult, err)
	}
	graphResult, err := service.SearchStoryGraph(ctx, actor, searchapp.Query{ProjectID: fixture.projectID.String(), Text: "码头", Limit: 10})
	if err != nil || graphResult.Status != search.StatusFresh || len(graphResult.Hits) != 1 ||
		graphResult.Source == nil || graphResult.Source.ID != storyGraphEvent.EventID || !strings.Contains(graphResult.Hits[0].OwnerHref, fixture.storyNodeKey) {
		t.Fatalf("Kafka StoryGraph projection is not queryable with provenance: %#v err=%v", graphResult, err)
	}
}

type projectSearchHandler struct {
	projectID string
	consumer  *eventingapp.Consumer
	projected map[string]bool
	done      chan struct{}
	doneOnce  sync.Once
}

func (handler *projectSearchHandler) Handle(ctx context.Context, message eventingapp.IncomingMessage) (eventingapp.HandleResult, error) {
	envelope, err := eventing.DecodeEnvelope(message.Value)
	if err != nil || envelope.ProjectID != handler.projectID {
		return eventingapp.HandleResult{Ack: true}, nil
	}
	result, err := handler.consumer.Handle(ctx, message)
	if err == nil && result.Ack {
		handler.projected[envelope.EventID] = true
		if len(handler.projected) == 2 {
			handler.doneOnce.Do(func() { close(handler.done) })
		}
	}
	return result, err
}

func newScriptEnvelope(t *testing.T, fixture searchOwnerFixture, occurredAt time.Time) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"script_version_id": fixture.scriptVersionID.String(), "episode_id": fixture.episodeID.String(), "version_no": int64(1),
		"document_revision_id": fixture.revisionID.String(), "content_hash": searchHashText("雨夜码头，角色发现关键线索。"),
		"source_start": 0, "source_end": len([]rune("雨夜码头，角色发现关键线索。")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return newSearchEnvelope(t, eventing.OutboxEvent{
		ID: uuid.NewString(), EventType: eventing.ScriptVersionPublished, EventVersion: 1,
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(), AggregateKind: "episode_script",
		AggregateID: fixture.episodeID.String(), AggregateRevision: 1, SourceReceiptID: uuid.NewString(), Payload: payload, OccurredAt: occurredAt,
	})
}

func newStoryGraphEnvelope(t *testing.T, fixture searchOwnerFixture, occurredAt time.Time) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"version_id": uuid.NewString(), "version_no": int64(1), "owner_set_hash": searchHashText("owners"),
		"topology_hash": searchHashText("topology"), "content_hash": searchHashText("graph"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return newSearchEnvelope(t, eventing.OutboxEvent{
		ID: uuid.NewString(), EventType: eventing.StoryGraphVersionPublished, EventVersion: 1,
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(), AggregateKind: "storygraph",
		AggregateID: fixture.projectID.String(), AggregateRevision: 1, SourceReceiptID: uuid.NewString(), Payload: payload, OccurredAt: occurredAt,
	})
}

func newSearchEnvelope(t *testing.T, value eventing.OutboxEvent) eventing.Envelope {
	t.Helper()
	envelope, err := eventing.NewEnvelope(value, eventing.TraceContext{RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func publishSearchEnvelope(t *testing.T, ctx context.Context, client *eventingkafka.Client, topic string, envelope eventing.Envelope) {
	t.Helper()
	encoded, err := eventing.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Publish(ctx, eventingapp.Message{
		Topic: topic, Key: envelope.EventID, Value: encoded,
		Headers: map[string]string{"lanverse-event-id": envelope.EventID, "lanverse-schema": envelope.Schema},
	}); err != nil {
		t.Fatal(err)
	}
}

func environmentOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func deleteSearchIndices(elasticsearchURL, prefix string) {
	request, err := http.NewRequest(http.MethodDelete, strings.TrimRight(elasticsearchURL, "/")+"/"+prefix+"-*", nil)
	if err == nil {
		_, _ = http.DefaultClient.Do(request)
	}
}
