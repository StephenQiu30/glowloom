package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	catalogwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/catalog"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	copywire "github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopy"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"

	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	auditevent "github.com/StephenQiu30/glowloom-server/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	catalogevent "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/event"
	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/workflow"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	kafkainbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/kafka"
	pginbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	maintenanceflow "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/adapter/temporal"
	maintenanceapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/maintenance/application"
	kafkaoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/postgres"
	outboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
	redisrealtime "github.com/StephenQiu30/glowloom-server/backend/internal/infra/realtime/adapter/redis"
	realtime "github.com/StephenQiu30/glowloom-server/backend/internal/infra/realtime/application"
	mediaevent "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/event"
	mediastaged "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/staged"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	toolevent "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/event"
	toolff "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/ffmpeg"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	operationevent "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/event"
	managedprovider "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	operationstaging "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/staging"
	operationflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/kafkaconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/redisconn"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/temporalconn"
	scriptevent "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/event"
	scriptflow "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/workflow"
	workspaceevent "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/event"
	workspaceflow "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/workflow"
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

func provideQuoteExpiryScheduleInstaller(ctx context.Context, temporalConn *temporalconn.Connection, prefix string) (*operationflow.QuoteExpiryScheduleInstaller, error) {
	if err := temporalConn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect setup Temporal: %w", err)
	}
	return operationflow.NewQuoteExpiryScheduleInstaller(temporalConn.Client.ScheduleClient(), prefix), nil
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
	case "provider":
		if err := catalogwire.RegisterProviderActivities(queueWorker, dbConn.DB, cfg); err != nil {
			return nil, err
		}
	case "flow":
		toolflow.RegisterWorkflow(queueWorker)
		toolflow.RegisterTranscriptionWorkflow(queueWorker)
		toolflow.RegisterDepthWorkflow(queueWorker)
		workspaceflow.RegisterProjectCopyWorkflow(queueWorker)
		scriptflow.RegisterImportWorkflow(queueWorker)
		mediaflow.RegisterTransferWorkflow(queueWorker)
		mediaflow.RegisterPurgeWorkflow(queueWorker)
		service := maintenanceapp.NewService(pgoutbox.NewPartitionStore(dbConn.DB), pginbox.NewStore(dbConn.DB))
		maintenanceflow.Register(queueWorker, maintenanceflow.NewActivities(service, 500))
		catalogflow.Register(queueWorker, catalogflow.NewActivities(catalogapp.NewCredentialTestService(operationwire.NewCatalogStore(dbConn.DB)), operationwire.NewCatalogStore(dbConn.DB)))
		operationStore := operationwire.ProvideOperationStore(dbConn.DB)
		finalizer := operationwire.NewOperationFinalizer(dbConn.DB, operationStore, pgbilling.NewStore(dbConn.DB))
		var recovery operationflow.ProviderImageRecoverer
		if cfg.ObjectStorageEndpoint != "" {
			storage, err := objectstorage.Open(cfg.ObjectStorageEndpoint, cfg.ObjectStorageBucket,
				cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageRegion)
			if err != nil {
				return nil, fmt.Errorf("configure flow receipt storage: %w", err)
			}
			if err := storage.Ping(ctx); err != nil {
				return nil, fmt.Errorf("connect flow receipt storage: %w", err)
			}
			recovery = operationapp.NewProviderStageService(operationstaging.NewObjects(storage), operationStore)
		}
		operationflow.Register(queueWorker, operationflow.NewActivitiesWithImageRecovery(operationStore, finalizer, operationStore, recovery))
		operationflow.RegisterBatch(queueWorker, operationflow.NewBatchActivities(operationStore, finalizer))
		operationflow.RegisterQuoteExpiry(queueWorker,
			operationflow.NewQuoteExpiryActivities(operationapp.NewQuoteExpiryService(operationStore, 500)))
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
		receiptService := operationapp.NewProviderStageService(operationstaging.NewObjects(storage), operationwire.ProvideOperationStore(dbConn.DB))
		activities, err := mediaflow.NewScopedActivitiesWithStaging(dbConn.DB, storage, mediaflow.DownloadPolicy{
			AllowedOrigins: origins, AllowTestLoopbackTLS: cfg.MediaAllowTestLoopbackTLS,
		}, mediastaged.NewReader(receiptService), mediawire.NewMediaWorkerScopeAccess)
		if err != nil {
			return nil, fmt.Errorf("configure media activities: %w", err)
		}

		managedResults := operationapp.NewManagedResults(operationstaging.NewObjects(storage), operationwire.ProvideOperationStore(dbConn.DB))
		managedIngest := managedprovider.NewScopedIngestActivities(dbConn.DB, storage, managedResults, mediawire.NewMediaWorkerScopeAccess, operationwire.ProvideOperationStore(dbConn.DB))
		queueWorker.RegisterActivityWithOptions(managedIngest.Ingest, activity.RegisterOptions{Name: "media.IngestManaged"})
		mediaflow.Register(queueWorker, activities)
		exportRenderer, err := toolff.NewRenderer(mediaflow.FFProber{})
		if err != nil {
			return nil, fmt.Errorf("configure media export renderer: %w", err)
		}
		exports := mediawire.ProvideMediaExportStoreWithAccess(dbConn.DB, mediawire.NewMediaWorkerScopeAccess)
		exportWorker := toolapp.NewWorker(exports, toolflow.NewObjects(storage), exportRenderer, mediaflow.FFProber{}, mediaflow.FFUploadRenderer{})
		toolflow.RegisterActivities(queueWorker, toolflow.NewActivities(exportWorker, exports, exports))
		transcriber, err := mediawire.ProvideTranscriber(cfg)
		if err != nil {
			return nil, err
		}
		audioPreprocessor, err := toolff.NewAudioPreprocessor(mediaflow.FFProber{})
		if err != nil {
			return nil, fmt.Errorf("configure speech audio preparation: %w", err)
		}
		transcriptions := mediawire.ProvideMediaTranscriptionStoreWithAccess(dbConn.DB, transcriber != nil, mediawire.NewMediaWorkerScopeAccess)
		transcriptionWorker := toolapp.NewTranscriptionWorker(transcriptions, toolflow.NewObjects(storage), audioPreprocessor, transcriber)
		toolflow.RegisterTranscriptionActivities(queueWorker, toolflow.NewTranscriptionActivities(transcriptionWorker, transcriptions))
		depthWorker, depths, err := mediawire.ProvideMediaDepthWorker(dbConn.DB, storage, cfg)
		if err != nil {
			return nil, err
		}
		toolflow.RegisterDepthActivities(queueWorker, toolflow.NewDepthActivities(depthWorker, depths))
		copyWorker, copies := copywire.ProvideProjectCopyWorker(dbConn.DB, storage)
		workspaceflow.RegisterProjectCopyActivities(queueWorker, workspaceflow.NewProjectCopyActivities(copyWorker, copies))
		importWorker, imports := scriptwire.ProvideScriptImportWorker(dbConn.DB, storage)
		scriptflow.RegisterImportActivities(queueWorker, scriptflow.NewImportActivities(importWorker, imports))
		transferWorker, transfers := mediawire.ProvideMediaTransferWorker(dbConn.DB, storage)
		mediaflow.RegisterTransferActivities(queueWorker, mediaflow.NewTransferActivities(transferWorker, transfers))
		purgeWorker, purges := mediawire.NewMediaPurgeWorker(dbConn.DB, storage)
		mediaflow.RegisterPurgeActivities(queueWorker, mediaflow.NewPurgeActivities(purgeWorker, purges))
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

func provideRelayRuntime(ctx context.Context, cfg config.Config, dbConn *db.Connection, kafkaConn *kafkaconn.Connection, redisConn *redisconn.Connection, logger *zap.Logger, tracerProvider trace.TracerProvider) (*relayRuntime, func(), error) {
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
		[]string{"lanverse.audit.recorded.v1"},
		auditevent.NewHandler(processed, auditapp.NewRecordedActionParser()))
	if err != nil {
		realtimeConsumer.Close()
		return nil, nil, fmt.Errorf("configure relay audit consumer: %w", err)
	}
	relay := outboxapp.NewRelay(pgoutbox.NewStore(dbConn.DB), kafkaoutbox.NewPublisher(kafkaConn.Client))
	runtime := &relayRuntime{outbox: relay, realtime: realtimeConsumer, audit: auditConsumer}
	var temporalConn *temporalconn.Connection
	if cfg.TemporalAddr != "" || cfg.TemporalNamespace != "" {
		temporalConn, err = temporalconn.Open(cfg.TemporalAddr, cfg.TemporalNamespace, logger, tracerProvider)
		if err != nil {
			auditConsumer.Close()
			realtimeConsumer.Close()
			return nil, nil, fmt.Errorf("configure relay Temporal: %w", err)
		}
		starterHandler := operationevent.NewWorkflowStarterHandler(processed,
			operationwire.ProvideOperationStore(dbConn.DB), operationflow.NewStarter(temporalConn.Client))
		controlHandler := operationevent.NewWorkflowControlHandler(processed,
			operationwire.ProvideOperationStore(dbConn.DB), operationflow.NewControl(temporalConn.Client))
		credentialHandler := catalogevent.NewCredentialTestHandler(processed,
			operationwire.NewCatalogStore(dbConn.DB), catalogflow.NewTestStarter(temporalConn.Client))
		exportHandler := toolevent.NewHandler(processed, mediawire.ProvideMediaExportStoreWithAccess(dbConn.DB, mediawire.NewMediaWorkerScopeAccess), toolflow.NewStarter(temporalConn.Client))
		transcriptionHandler := toolevent.NewTranscriptionHandler(processed, mediawire.ProvideMediaTranscriptionStoreWithAccess(dbConn.DB, false, mediawire.NewMediaWorkerScopeAccess), toolflow.NewTranscriptionStarter(temporalConn.Client))
		depthHandler := toolevent.NewDepthHandler(processed, mediawire.ProvideMediaDepthStoreWithAccess(dbConn.DB, false, mediawire.NewMediaWorkerScopeAccess), toolflow.NewDepthStarter(temporalConn.Client))
		copyHandler := workspaceevent.NewProjectCopyHandler(processed, copywire.ProvideProjectCopyStoreWithAccess(dbConn.DB, mediawire.NewMediaWorkerScopeAccess), workspaceflow.NewProjectCopyStarter(temporalConn.Client))
		importHandler := scriptevent.NewImportHandler(processed, scriptwire.ProvideScriptImportStore(dbConn.DB), scriptflow.NewImportStarter(temporalConn.Client))
		transferHandler := mediaevent.NewTransferHandler(processed, mediawire.ProvideMediaTransferWorkerStore(dbConn.DB), mediaflow.NewTransferStarter(temporalConn.Client))
		purgeHandler := mediaevent.NewPurgeHandler(processed, mediawire.ProvideMediaPurgeWorkerStore(dbConn.DB), mediaflow.NewPurgeStarter(temporalConn.Client))
		runtime.starter, err = kafkainbox.NewConsumer(cfg.KafkaBrokers, workflowStarterGroup,
			[]string{operationevent.OperationConfirmedTopic, operationevent.BatchConfirmedTopic, operationevent.WorkflowControlTopic, catalogevent.CredentialTestTopic, toolevent.Topic, toolevent.TranscriptionTopic, toolevent.DepthTopic, workspaceevent.ProjectCopyTopic, scriptevent.ImportTopic, mediaevent.TransferTopic, mediaevent.PurgeTopic}, workflowDelivery{
				operations: operationevent.NewWorkflowEventHandler(starterHandler, controlHandler), credentials: credentialHandler,
				exports: exportHandler, transcriptions: transcriptionHandler, depths: depthHandler, copies: copyHandler, imports: importHandler, transfers: transferHandler, purges: purgeHandler,
			})
		if err != nil {
			temporalConn.Close()
			auditConsumer.Close()
			realtimeConsumer.Close()
			return nil, nil, fmt.Errorf("configure workflow starter consumer: %w", err)
		}
		runtime.sweep = func(ctx context.Context) error {
			return runStarterSweep(ctx, func(ctx context.Context, now time.Time) error {
				if err := starterHandler.SweepOnce(ctx, now); err != nil {
					return err
				}
				return starterHandler.SweepBatchesOnce(ctx, now)
			})
		}
	}
	return runtime, func() {
		if runtime.starter != nil {
			runtime.starter.Close()
			temporalConn.Close()
		}
		auditConsumer.Close()
		realtimeConsumer.Close()
	}, nil
}

// workflowDelivery dispatches distinct durable requests on the existing owned consumer.
type workflowDelivery struct {
	operations     inboxapp.Handler
	credentials    inboxapp.Handler
	exports        inboxapp.Handler
	transcriptions inboxapp.Handler
	depths         inboxapp.Handler
	copies         inboxapp.Handler
	imports        inboxapp.Handler
	transfers      inboxapp.Handler
	purges         inboxapp.Handler
}

func (h workflowDelivery) Handle(ctx context.Context, record inboxapp.Record) error {
	if record.Topic == mediaevent.PurgeTopic {
		return h.purges.Handle(ctx, record)
	}
	if record.Topic == mediaevent.TransferTopic {
		return h.transfers.Handle(ctx, record)
	}
	if record.Topic == scriptevent.ImportTopic {
		return h.imports.Handle(ctx, record)
	}
	if record.Topic == workspaceevent.ProjectCopyTopic {
		return h.copies.Handle(ctx, record)
	}
	if record.Topic == catalogevent.CredentialTestTopic {
		return h.credentials.Handle(ctx, record)
	}
	if record.Topic == toolevent.Topic {
		return h.exports.Handle(ctx, record)
	}
	if record.Topic == toolevent.TranscriptionTopic {
		return h.transcriptions.Handle(ctx, record)
	}
	if record.Topic == toolevent.DepthTopic {
		return h.depths.Handle(ctx, record)
	}
	return h.operations.Handle(ctx, record)
}
