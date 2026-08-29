package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/config"
	eventinggorm "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/gormdb"
	eventingkafka "github.com/StephenQiu30/lanverse/backend/internal/eventing/adapter/kafka"
	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	searches "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/elasticsearch"
	searchgorm "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/gormdb"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

const (
	dependencyTimeout = 10 * time.Second
	publisherInterval = 500 * time.Millisecond
	eventLease        = 30 * time.Second
)

func RunEventWorker(ctx context.Context, logger *slog.Logger) error {
	configuration, err := config.Load()
	if err != nil {
		return fmt.Errorf("event worker configuration is invalid: %w", err)
	}
	connectContext, cancelConnect := context.WithTimeout(ctx, dependencyTimeout)
	database, err := platformdatabase.Open(connectContext, configuration.DatabaseURL, os.Stderr)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("event worker database connection failed: %w", err)
	}
	defer func() {
		if closeErr := platformdatabase.Close(database); closeErr != nil {
			logger.Error("event worker database close failed", "error", closeErr)
		}
	}()
	searchIndex, err := searches.New(searches.Config{
		Addresses: []string{configuration.ElasticsearchURL}, Username: configuration.ElasticsearchUsername,
		Password: configuration.ElasticsearchPassword, ScriptAlias: configuration.ElasticsearchScriptAlias,
		StoryGraphAlias: configuration.ElasticsearchStoryGraphAlias,
	})
	if err != nil {
		return fmt.Errorf("event worker Elasticsearch configuration failed: %w", err)
	}
	searchContext, cancelSearch := context.WithTimeout(ctx, dependencyTimeout)
	err = searchIndex.Ensure(searchContext)
	cancelSearch()
	if err != nil {
		return fmt.Errorf("event worker Elasticsearch initialization failed: %w", err)
	}
	searchSnapshots := searchgorm.New(database)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "replay":
			if err = runReplay(ctx, configuration, eventinggorm.New(database), os.Args[2:], logger); err != nil {
				return fmt.Errorf("event replay failed: %w", err)
			}
		case "reindex":
			if err = runReindex(ctx, searchSnapshots, searchIndex, os.Args[2:], logger); err != nil {
				return fmt.Errorf("search reindex failed: %w", err)
			}
		default:
			return fmt.Errorf("event worker command %q is invalid", os.Args[1])
		}
		return nil
	}
	kafkaClient, err := eventingkafka.New(eventingkafka.Config{
		Brokers: configuration.KafkaBrokers, ClientID: configuration.KafkaClientID,
		ConsumerGroup: configuration.KafkaConsumerGroup,
		Username:      configuration.KafkaUsername, Password: configuration.KafkaPassword,
		Topics: []string{configuration.KafkaScriptTopic, configuration.KafkaStoryGraphTopic},
	})
	if err != nil {
		return fmt.Errorf("event worker Kafka client configuration failed: %w", err)
	}
	defer kafkaClient.Close()
	pingContext, cancelPing := context.WithTimeout(ctx, dependencyTimeout)
	err = kafkaClient.Ping(pingContext)
	cancelPing()
	if err != nil {
		return fmt.Errorf("event worker Kafka connection failed: %w", err)
	}

	repository := eventinggorm.New(database)
	now := func() time.Time { return time.Now().UTC() }
	publisher := eventingapp.NewPublisher(repository, kafkaClient, eventingapp.StaticTopics{
		eventing.ScriptVersionPublished:     configuration.KafkaScriptTopic,
		eventing.StoryGraphVersionPublished: configuration.KafkaStoryGraphTopic,
	}, eventingapp.PublisherConfig{Now: now, NewID: uuid.NewString, Lease: eventLease, BatchSize: 100})
	projector := searchapp.NewProjector(searchSnapshots, searchIndex, now)
	consumer := eventingapp.NewConsumer(repository, projector, kafkaClient, eventingapp.ConsumerTopics{
		BusinessToDLQ: map[string]string{
			configuration.KafkaScriptTopic:     configuration.KafkaScriptDLQTopic,
			configuration.KafkaStoryGraphTopic: configuration.KafkaStoryGraphDLQTopic,
		},
	}, eventingapp.ConsumerConfig{
		Group: configuration.KafkaConsumerGroup, Now: now, NewID: uuid.NewString,
		Lease: eventLease, MaxAttempts: 3,
	})

	eventContext, stopEvent := context.WithCancel(ctx)
	defer stopEvent()
	server := &http.Server{
		Addr: configuration.EventWorkerListenAddress,
		Handler: healthHandler(
			func(ctx context.Context) error { return platformdatabase.Ping(ctx, database) }, kafkaClient, searchIndex,
		),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("lanverse event worker health server started", "address", configuration.EventWorkerListenAddress)
		serverErrors <- server.ListenAndServe()
	}()
	workerErrors := make(chan error, 1)
	go func() {
		workerErrors <- kafkaClient.Run(eventContext, consumer, func(retryErr error) {
			logger.Warn("event worker consumer will retry", "error", retryErr)
		})
	}()
	go runPublisher(eventContext, publisher, logger)
	logger.Info("lanverse event worker started", "consumer_group", configuration.KafkaConsumerGroup,
		"script_topic", configuration.KafkaScriptTopic, "storygraph_topic", configuration.KafkaStoryGraphTopic)

	var runtimeErr error
	select {
	case <-ctx.Done():
	case workerErr := <-workerErrors:
		if workerErr != nil {
			runtimeErr = fmt.Errorf("event worker consumer stopped: %w", workerErr)
		}
	case serverErr := <-serverErrors:
		if !errors.Is(serverErr, http.ErrServerClosed) {
			runtimeErr = fmt.Errorf("event worker health server stopped: %w", serverErr)
		}
	}
	stopEvent()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err = server.Shutdown(shutdownContext); err != nil {
		return errors.Join(runtimeErr, fmt.Errorf("event worker health server shutdown failed: %w", err))
	}
	logger.Info("lanverse event worker stopped")
	return runtimeErr
}

func runReplay(ctx context.Context, configuration config.Config, repository eventingapp.ReplayRepository, args []string, logger *slog.Logger) error {
	flags := flag.NewFlagSet("event-worker replay", flag.ContinueOnError)
	projectID := flags.String("project-id", "", "project UUID")
	eventType := flags.String("event-type", eventing.StoryGraphVersionPublished, "event type")
	failedAfter := flags.String("failed-after", "", "inclusive RFC3339 timestamp")
	failedBefore := flags.String("failed-before", "", "exclusive RFC3339 timestamp")
	limit := flags.Int("limit", 100, "maximum dead letters to replay")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := uuid.Parse(*projectID); err != nil ||
		(*eventType != eventing.ScriptVersionPublished && *eventType != eventing.StoryGraphVersionPublished) ||
		*limit < 1 || *limit > 200 {
		return errors.New("event replay arguments are invalid")
	}
	after, err := time.Parse(time.RFC3339, *failedAfter)
	if err != nil {
		return errors.New("failed-after must be an RFC3339 timestamp")
	}
	before, err := time.Parse(time.RFC3339, *failedBefore)
	if err != nil {
		return errors.New("failed-before must be an RFC3339 timestamp")
	}
	kafkaClient, err := eventingkafka.New(eventingkafka.Config{
		Brokers: configuration.KafkaBrokers, ClientID: configuration.KafkaClientID + "-replay",
		Username: configuration.KafkaUsername, Password: configuration.KafkaPassword,
	})
	if err != nil {
		return err
	}
	defer kafkaClient.Close()
	pingContext, cancelPing := context.WithTimeout(ctx, dependencyTimeout)
	err = kafkaClient.Ping(pingContext)
	cancelPing()
	if err != nil {
		return err
	}
	replayer := eventingapp.NewReplayer(repository, kafkaClient, eventingapp.ReplayConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		Lease: eventLease, BatchSize: *limit,
	})
	replayed, err := replayer.ReplayOnce(ctx, eventingapp.ReplayFilter{
		ConsumerGroup: configuration.KafkaConsumerGroup, ProjectID: *projectID,
		EventTypes: []string{*eventType}, FailedAfter: after, FailedBefore: before,
	})
	if err != nil {
		return err
	}
	logger.Info("event replay completed", "project_id", *projectID, "event_type", *eventType, "replayed", replayed)
	return nil
}

func runReindex(ctx context.Context, snapshots searchapp.SnapshotReader, index searchapp.Index, args []string, logger *slog.Logger) error {
	flags := flag.NewFlagSet("event-worker reindex", flag.ContinueOnError)
	kind := flags.String("kind", "", "search index kind: script or storygraph")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var searchKind search.Kind
	switch *kind {
	case string(search.KindScript):
		searchKind = search.KindScript
	case string(search.KindStoryGraph):
		searchKind = search.KindStoryGraph
	default:
		return errors.New("search reindex kind must be script or storygraph")
	}
	result, err := searchapp.NewReindexer(
		snapshots, index, func() time.Time { return time.Now().UTC() }, uuid.NewString,
	).Reindex(ctx, searchKind)
	if err != nil {
		return err
	}
	logger.Info("search reindex completed", "kind", result.Kind, "alias", result.Alias,
		"index_version", result.IndexVersion, "documents", result.Documents)
	return nil
}

func runPublisher(ctx context.Context, publisher *eventingapp.Publisher, logger *slog.Logger) {
	ticker := time.NewTicker(publisherInterval)
	defer ticker.Stop()
	for {
		if _, err := publisher.PublishOnce(ctx); err != nil && ctx.Err() == nil {
			logger.Error("event outbox publication failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type dependencyReady interface {
	Ping(context.Context) error
}

func healthHandler(databaseReady func(context.Context) error, kafkaClient *eventingkafka.Client, elasticsearch dependencyReady) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeHealth(writer, http.StatusOK, "ok", "")
	})
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, request *http.Request) {
		checkContext, cancel := context.WithTimeout(request.Context(), 3*time.Second)
		defer cancel()
		if err := databaseReady(checkContext); err != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "not_ready", "database_unavailable")
			return
		}
		if err := kafkaClient.Ping(checkContext); err != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "not_ready", "kafka_unavailable")
			return
		}
		if err := elasticsearch.Ping(checkContext); err != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "not_ready", "elasticsearch_unavailable")
			return
		}
		writeHealth(writer, http.StatusOK, "ready", "")
	})
	return mux
}

func writeHealth(writer http.ResponseWriter, status int, state, reason string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	response := map[string]string{"status": state, "service": "lanverse-event-worker"}
	if reason != "" {
		response["reason"] = reason
	}
	_ = json.NewEncoder(writer).Encode(response)
}
