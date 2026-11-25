package app

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// initializeAPI explicitly wires the HTTP API server and its runtime dependencies.
func initializeAPI(ctx context.Context, cfg config.Config, logger *zap.Logger) (*http.Server, func(), error) {
	tracerProvider, cleanupTrace, err := provideTrace(ctx, cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	connection, cleanupDB, err := provideDB(ctx, cfg, logger, tracerProvider)
	if err != nil {
		cleanupTrace()
		return nil, nil, err
	}
	redisConn, cleanupRedis, err := provideRedis(cfg, logger)
	if err != nil {
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	registry, cleanupStorage, err := provideObjectStorage(cfg)
	if err != nil {
		cleanupRedis()
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	readyCheck := provideReadyCheck(cfg, connection, redisConn, registry)
	server, cleanupServer, err := provideAPIServer(ctx, cfg, logger, readyCheck, tracerProvider, connection, registry, redisConn)
	if err != nil {
		cleanupStorage()
		cleanupRedis()
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	return server, func() {
		cleanupServer()
		cleanupStorage()
		cleanupRedis()
		cleanupDB()
		cleanupTrace()
	}, nil
}

// initializeWorker explicitly wires a background task worker for the selected queue.
func initializeWorker(ctx context.Context, cfg config.Config, logger *zap.Logger, queue string) (*taskruntime.Worker, func(), error) {
	tracerProvider, cleanupTrace, err := provideWorkerTrace(ctx, cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	connection, cleanupDB, err := provideDB(ctx, cfg, logger, tracerProvider)
	if err != nil {
		cleanupTrace()
		return nil, nil, err
	}
	worker, cleanupWorker, err := provideBackendWorker(ctx, cfg, connection, queue, logger)
	if err != nil {
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	return worker, func() {
		cleanupWorker()
		cleanupDB()
		cleanupTrace()
	}, nil
}

// initializeRelay explicitly wires the Kafka outbox/inbox async event relay runtime.
func initializeRelay(ctx context.Context, cfg config.Config, logger *zap.Logger) (*relayRuntime, func(), error) {
	tracerProvider, cleanupTrace, err := provideRelayTrace(ctx, cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	connection, cleanupDB, err := provideDB(ctx, cfg, logger, tracerProvider)
	if err != nil {
		cleanupTrace()
		return nil, nil, err
	}
	kafkaConn, cleanupKafka, err := provideKafka(ctx, cfg)
	if err != nil {
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	redisConn, cleanupRedis, err := provideRedis(cfg, logger)
	if err != nil {
		cleanupKafka()
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	relay, cleanupRelay, err := provideRelayRuntime(ctx, cfg, connection, kafkaConn, redisConn, logger, tracerProvider)
	if err != nil {
		cleanupRedis()
		cleanupKafka()
		cleanupDB()
		cleanupTrace()
		return nil, nil, err
	}
	return relay, func() {
		cleanupRelay()
		cleanupRedis()
		cleanupKafka()
		cleanupDB()
		cleanupTrace()
	}, nil
}
