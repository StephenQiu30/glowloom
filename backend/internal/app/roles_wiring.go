package app

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	kafkainbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/kafka"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	maintenanceflow "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/adapter/temporal"
	maintenanceapp "github.com/StephenQiu30/lanverse/backend/internal/infra/maintenance/application"
	kafkaoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	outboxapp "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	redisrealtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/kafkaconn"
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

func provideMaintenanceWorker(ctx context.Context, dbConn *db.Connection, temporalConn *temporalconn.Connection, queue string) (worker.Worker, error) {
	if err := temporalConn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect worker Temporal: %w", err)
	}
	flowWorker := worker.New(temporalConn.Client, queue, worker.Options{
		MaxConcurrentWorkflowTaskExecutionSize: 200,
		MaxConcurrentActivityExecutionSize:     100,
		WorkerStopTimeout:                      5 * time.Minute,
	})
	service := maintenanceapp.NewService(pgoutbox.NewPartitionStore(dbConn.DB), pginbox.NewStore(dbConn.DB))
	maintenanceflow.Register(flowWorker, maintenanceflow.NewActivities(service, 500))
	return flowWorker, nil
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
		[]string{realtime.OperationStatusTopic},
		realtime.NewOperationStatusHandler(processed, redisrealtime.NewSink(redisConn.Client)))
	if err != nil {
		return nil, nil, fmt.Errorf("configure relay realtime consumer: %w", err)
	}
	auditConsumer, err := kafkainbox.NewConsumer(cfg.KafkaBrokers, auditConsumerGroup,
		[]string{"lanverse.audit.recorded.v1"},
		auditevent.NewHandler(processed, auditapp.NewIdentityActionParser()))
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
