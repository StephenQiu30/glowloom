package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	maintenancetask "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/task"
	mediatask "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/task"
	tooltask "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/task"
	operationtask "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	scripttask "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/task"
	workspacetask "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/task"

	codexwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/codexruntime"

	catalogwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/catalog"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	copywire "github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopy"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	auditevent "github.com/StephenQiu30/glowloom-server/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	billingevent "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/event"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingredis "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/redis"
	kafkainbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/kafka"
	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	maintenanceapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/application"
	kafkaoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/postgres"
	outboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
	redisrealtime "github.com/StephenQiu30/glowloom-server/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/glowloom-server/backend/internal/infra/realtime/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediastaged "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/staged"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	toolproject "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	managedprovider "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationstaging "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/kafkaconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/redisconn"
)

func provideWorkerTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "worker")
}

func provideRelayTrace(ctx context.Context, cfg config.Config, logger *zap.Logger) (trace.TracerProvider, func(), error) {
	return provideNamedTrace(ctx, cfg, logger, "relay")
}

func provideBackendWorker(ctx context.Context, cfg config.Config, dbConn *db.Connection, queue string, _ *zap.Logger) (*taskruntime.Worker, func(), error) {
	owned, cancel := context.WithCancel(ctx)
	worker, err := buildBackendWorker(owned, cfg, dbConn, queue)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	stop, err := startStorageRuntime(owned, cfg, true, dbConn.DB)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return worker, func() { cancel(); stop() }, nil
}
func buildBackendWorker(ctx context.Context, cfg config.Config, dbConn *db.Connection, queue string) (*taskruntime.Worker, error) {
	handlers := map[string]taskruntime.Handler{}
	concurrency := 4
	switch queue {
	case "agent.codex":
		concurrency = 1
		if err := codexwire.Register(ctx, handlers, dbConn.DB, cfg); err != nil {
			return nil, err
		}
	case "provider":
		if err := catalogwire.RegisterProvider(ctx, handlers, dbConn.DB, cfg); err != nil {
			return nil, err
		}
	case "flow":
		service := maintenanceapp.NewService(pgoutbox.NewPartitionStore(dbConn.DB), pginbox.NewStore(dbConn.DB))
		handlers = maintenancetask.Handlers(service)
		handlers[workspacetask.Retention] = workspacetask.RetentionHandler(NewProjectRetention(dbConn.DB))
		operationStore := operationwire.ProvideOperationStore(dbConn.DB)
		finalizer := operationwire.NewOperationFinalizer(dbConn.DB, operationStore, pgbilling.NewStore(dbConn.DB))
		var recovery *operationapp.ProviderStageService
		objects, err := subjectwire.OpenObjects(cfg)
		if err != nil {
			return nil, err
		}
		context.AfterFunc(ctx, objects.Close)
		if objects.Registry() != nil {
			recovery = operationapp.NewRoutedProviderStageService(operationstaging.NewRoutedFactory(objects.Registry(), pgmedia.NewStorageRoutes(dbConn.DB)), operationStore)
		}
		operationtask.RegisterFlow(handlers, operationStore, finalizer, recovery)
		expiry := operationapp.NewQuoteExpiryService(operationStore, 500)
		handlers["operation.quote_expiry"] = operationtask.QuoteExpiryHandler(expiry)
	case "media":
		objects, err := subjectwire.OpenObjects(cfg)
		if err != nil {
			return nil, err
		}
		context.AfterFunc(ctx, objects.Close)
		registry := objects.Registry()
		var origins []string
		for _, raw := range strings.Split(cfg.MediaResultAllowedOrigins, ",") {
			if origin := strings.TrimSpace(raw); origin != "" {
				origins = append(origins, origin)
			}
		}
		receiptService := operationapp.NewRoutedProviderStageService(operationstaging.NewRoutedFactory(registry, pgmedia.NewStorageRoutes(dbConn.DB)), operationwire.ProvideOperationStore(dbConn.DB))
		activities, err := mediaflow.NewRoutedScopedActivitiesWithStaging(dbConn.DB, registry, mediaflow.DownloadPolicy{AllowedOrigins: origins, AllowTestLoopbackTLS: cfg.MediaAllowTestLoopbackTLS}, mediastaged.NewReader(receiptService), mediawire.NewMediaWorkerScopeAccess)
		if err != nil {
			return nil, fmt.Errorf("configure media activities: %w", err)
		}

		managedResults := operationapp.NewRoutedManagedResults(operationstaging.NewRoutedFactory(registry, pgmedia.NewStorageRoutes(dbConn.DB)), operationwire.ProvideOperationStore(dbConn.DB))
		managedIngest := managedprovider.NewRoutedIngestActivities(dbConn.DB, registry, managedResults, mediawire.NewMediaWorkerScopeAccess, operationwire.ProvideOperationStore(dbConn.DB))
		operationtask.RegisterMedia(handlers, operationwire.ProvideOperationStore(dbConn.DB), activities, managedIngest)
		exportRenderer, err := toolff.NewRenderer(mediaflow.FFProber{})
		if err != nil {
			return nil, fmt.Errorf("configure media export renderer: %w", err)
		}
		exports := mediawire.ProvideRoutedMediaExportStore(dbConn.DB, mediawire.NewMediaWorkerScopeAccess)
		exportWorker := toolapp.NewRoutedWorker(exports, mediawire.ToolObjects(dbConn.DB, registry), toolproject.NewRenderer(exportRenderer), mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
		handlers[tooltask.Export] = tooltask.ExportHandler(exportWorker, exports)
		transcriber, err := mediawire.ProvideTranscriber(cfg)
		if err != nil {
			return nil, err
		}
		audioPreprocessor, err := toolff.NewAudioPreprocessor(mediaflow.FFProber{})
		if err != nil {
			return nil, fmt.Errorf("configure speech audio preparation: %w", err)
		}
		transcriptions := mediawire.ProvideRoutedMediaTranscriptionStore(dbConn.DB, transcriber != nil, mediawire.NewMediaWorkerScopeAccess)
		transcriptionWorker := toolapp.NewRoutedTranscriptionWorker(transcriptions, mediawire.ToolObjects(dbConn.DB, registry), audioPreprocessor, transcriber)
		handlers[tooltask.Transcription] = tooltask.TranscriptionHandler(transcriptionWorker, transcriptions)
		depthWorker, depths, err := mediawire.ProvideMediaDepthWorker(dbConn.DB, registry, cfg)
		if err != nil {
			return nil, err
		}
		handlers[tooltask.Depth] = tooltask.DepthHandler(depthWorker, depths)
		copyWorker, copies := copywire.ProvideProjectCopyWorker(dbConn.DB, registry)
		handlers[workspacetask.Copy] = workspacetask.CopyHandler(copyWorker, copies)
		importWorker, imports := scriptwire.ProvideScriptImportWorkerWithObjects(dbConn.DB, registry)
		handlers[scripttask.Import] = scripttask.ImportHandler(importWorker, imports)
		transferWorker, transfers := mediawire.ProvideMediaTransferWorker(dbConn.DB, registry)
		handlers[mediatask.Transfer] = mediatask.TransferHandler(transferWorker, transfers)
		purgeWorker, purges := mediawire.NewRoutedMediaPurgeWorker(dbConn.DB, registry)
		handlers[mediatask.Purge] = mediatask.PurgeHandler(purgeWorker, purges)
	default:
		return nil, fmt.Errorf("%w: worker queue %q", ErrRoleNotAvailable, queue)
	}
	return &taskruntime.Worker{Store: taskruntime.NewStore(dbConn.DB), Handlers: handlers, Concurrency: concurrency, Lease: 30 * time.Second, Poll: 250 * time.Millisecond}, nil
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

func provideRelayRuntime(ctx context.Context, cfg config.Config, dbConn *db.Connection, kafkaConn *kafkaconn.Connection, redisConn *redisconn.Connection, _ *zap.Logger, _ trace.TracerProvider) (*relayRuntime, func(), error) {
	if err := redisConn.Ping(ctx); err != nil {
		return nil, nil, fmt.Errorf("connect relay Redis: %w", err)
	}
	processed := pginbox.NewStore(dbConn.DB)
	realtimeConsumer, err := kafkainbox.NewConsumer(cfg.KafkaBrokers, realtimeConsumerGroup,
		[]string{realtime.OperationStatusTopic, realtime.ProjectChangedTopic, realtime.BudgetChangedTopic, realtime.BillingSettledTopic},
		realtime.NewHandler(processed, redisrealtime.NewSink(redisConn.Client)))
	if err != nil {
		return nil, nil, fmt.Errorf("configure relay realtime consumer: %w", err)
	}
	auditConsumer, err := kafkainbox.NewConsumer(cfg.KafkaBrokers, auditConsumerGroup,
		[]string{"glowloom.audit.recorded.v1"},
		auditevent.NewHandler(processed, auditapp.NewRecordedActionParser()))
	if err != nil {
		realtimeConsumer.Close()
		return nil, nil, fmt.Errorf("configure relay audit consumer: %w", err)
	}
	creditConsumer, err := kafkainbox.NewRetryingConsumer(cfg.KafkaBrokers, creditConsumerGroup,
		[]string{billingevent.CreditCommandTopic, billingevent.CreditBalanceChangedTopic},
		billingevent.NewCreditEventHandler(pgbilling.NewStore(dbConn.DB), billingredis.NewWalletBalanceCache(redisConn.Client)),
		kafkainbox.FailurePolicy{
			MaxAttempts: 12, DeadLetterTopic: billingevent.CreditDeadLetterTopic,
			RetryBase: 250 * time.Millisecond, RetryCap: 5 * time.Second,
		})
	if err != nil {
		auditConsumer.Close()
		realtimeConsumer.Close()
		return nil, nil, fmt.Errorf("configure relay credit consumer: %w", err)
	}
	relay := outboxapp.NewRelay(pgoutbox.NewStore(dbConn.DB), kafkaoutbox.NewPublisher(kafkaConn.Client))
	runtime := &relayRuntime{outbox: relay, realtime: realtimeConsumer, audit: auditConsumer, credit: creditConsumer}
	stopStorageRuntime, err := startStorageRuntime(ctx, cfg, false, dbConn.DB)
	if err != nil {
		creditConsumer.Close()
		auditConsumer.Close()
		realtimeConsumer.Close()
		return nil, nil, err
	}
	return runtime, func() {
		stopStorageRuntime()
		creditConsumer.Close()
		auditConsumer.Close()
		realtimeConsumer.Close()
	}, nil
}
