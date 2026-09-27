package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	pgbilling "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogflow "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/workflow"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	kafkainbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/kafka"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	maintenanceapp "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/application"
	kafkaoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/kafkaconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/temporalconn"
)

func provideWorkerTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "worker")
}

func provideRelayTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "relay")
}

func provideSetupTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "setup")
}

func provideCleanupScheduleInstaller(ctx context.Context, temporalConn *temporalconn.Connection, prefix string) (*maintenanceflow.ScheduleInstaller, error) {
	if err := temporalConn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect setup Temporal: %w", err)
	}
	return maintenanceflow.NewScheduleInstaller(temporalConn.Client.ScheduleClient(), prefix), nil
}

func provideBackendWorker(ctx context.Context, cfg config.Config, dbConn *db.Connection, temporalConn *temporalconn.Connection, queue string) (worker.Worker, error) {
	if err := temporalConn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect worker Temporal: %w", err)
	}
	options := worker.Options{
		MaxConcurrentWorkflowTaskExecutionSize: 200,
		MaxConcurrentActivityExecutionSize:     100,
		WorkerStopTimeout:                      5 * time.Minute,
	}
	if queue == "media" {
		// This queue registers Activities only; zero lets the SDK use its
		// default workflow-task setting instead of rejecting a value of one.
		options.MaxConcurrentWorkflowTaskExecutionSize = 0
		options.MaxConcurrentActivityExecutionSize = 4
	}
	queueWorker := worker.New(temporalConn.Client, queue, options)
	switch queue {
	case "flow":
		service := maintenanceapp.NewService(pgoutbox.NewPartitionStore(dbConn.DB), pginbox.NewStore(dbConn.DB))
		maintenanceflow.Register(queueWorker, maintenanceflow.NewActivities(service, 500))
		catalogflow.Register(queueWorker, catalogflow.NewActivities(catalogapp.NewCredentialTestService(pgcatalog.NewStore(dbConn.DB))))
		operationStore := pgoperation.NewStore(dbConn.DB)
		finalizer := NewOperationFinalizer(dbConn.DB, operationStore, pgbilling.NewStore(dbConn.DB))
		operationflow.Register(queueWorker, operationflow.NewActivities(operationStore, finalizer))
	case "media":
		storage, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
			cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
		if err != nil {
			return nil, fmt.Errorf("configure media object storage: %w", err)
		}
		if err := storage.Ping(ctx); err != nil {
			return nil, fmt.Errorf("connect media object storage: %w", err)
		}
		var origins []string
		for _, raw := range strings.Split(cfg.MediaResultAllowedOrigins, ",") {
			if origin := strings.TrimSpace(raw); origin != "" {
				origins = append(origins, origin)
			}
		}
		activities, err := mediaflow.NewActivities(dbConn.DB, storage, mediaflow.DownloadPolicy{
			AllowedOrigins: origins, AllowTestLoopbackTLS: cfg.MediaAllowTestLoopbackTLS,
		})
		if err != nil {
			return nil, fmt.Errorf("configure media activities: %w", err)
		}
		mediaflow.Register(queueWorker, activities)
	default:
		return nil, fmt.Errorf("%w: worker queue %q", ErrRoleNotAvailable, queue)
	}
	return queueWorker, nil
}

func provideKafka(ctx context.Context, cfg config.Config) (*kafkaconn.Connection, func(), error) {
	conn, err := kafkaconn.Open(cfg.KafkaBrokers)
	if err != nil {
		return nil, nil, fmt.Errorf("configure relay Kafka: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("connect relay Kafka: %w", err)
	}
	return conn, conn.Close, nil
}

func provideRelayRuntime(ctx context.Context, cfg config.Config, dbConn *db.Connection, kafkaConn *kafkaconn.Connection, redisConn *redisconn.Connection) (*relayRuntime, func(), error) {
	if err := redisConn.Ping(ctx); err != nil {
		return nil, nil, fmt.Errorf("connect relay Redis: %w", err)
	}
	processed := pginbox.NewStore(dbConn.DB)
	realtimeConsumer, err := kafkainbox.NewConsumer(cfg.KafkaBrokers, realtimeConsumerGroup,
		[]string{realtime.OperationStatusTopic, realtime.ProjectChangedTopic},
		realtime.NewHandler(processed, redisrealtime.NewSink(redisConn.Client)))
	if err != nil {
		return nil, nil, fmt.Errorf("configure relay realtime consumer: %w", err)
	}
	auditConsumer, err := kafkainbox.NewConsumer(cfg.KafkaBrokers, auditConsumerGroup,
		[]string{"lanverse.audit.recorded.v1"},
		auditevent.NewHandler(processed, auditapp.NewRecordedActionParser()))
	if err != nil {
		realtimeConsumer.Close()
		return nil, nil, fmt.Errorf("configure relay audit consumer: %w", err)
	}
	relay := outboxapp.NewRelay(pgoutbox.NewStore(dbConn.DB), kafkaoutbox.NewPublisher(kafkaConn.Client))
	return &relayRuntime{outbox: relay, realtime: realtimeConsumer, audit: auditConsumer}, func() {
		auditConsumer.Close()
		realtimeConsumer.Close()
	}, nil
}
