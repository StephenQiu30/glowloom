package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	identitygorm "github.com/StephenQiu30/lanverse/backend/internal/access/identity/adapter/gormdb"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/access/identity/adapter/httpapi"
	identityverification "github.com/StephenQiu30/lanverse/backend/internal/access/identity/adapter/verification"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/access/identity/application"
	agentclient "github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	agentgrant "github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoringdomain "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/config"
	mediagorm "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gormdb"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/httpapi"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	platformschema "github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	biblehttp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/httpapi"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planninghttp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/httpapi"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projecthttp "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/httpapi"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/mediareader"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardhttp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/httpapi"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowhttp "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/httpapi"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

const (
	shutdownTimeout   = 10 * time.Second
	schemaSyncTimeout = 2 * time.Minute
)

var (
	buildVersion = "development"
	buildCommit  = "unknown"
	buildTime    = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	configuration, err := config.Load()
	if err != nil {
		logger.Error("api configuration is invalid", "error", err)
		os.Exit(1)
	}
	connectContext, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := platformdatabase.Open(connectContext, configuration.DatabaseURL, os.Stderr)
	cancelConnect()
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := platformdatabase.Close(database); closeErr != nil {
			logger.Error("database close failed", "error", closeErr)
		}
	}()
	syncContext, cancelSync := context.WithTimeout(context.Background(), schemaSyncTimeout)
	if err = platformschema.Sync(syncContext, database); err != nil {
		cancelSync()
		logger.Error("database schema synchronization failed", "error", err)
		os.Exit(1)
	}
	systemNodeCatalog, err := authoringdomain.SystemCatalog()
	if err != nil {
		cancelSync()
		logger.Error("system node catalog is invalid", "error", err)
		os.Exit(1)
	}
	authoringStore := authoringgorm.New(database)
	if _, err = authoringStore.EnsureCatalog(syncContext, systemNodeCatalog, time.Now().UTC(), uuid.NewString); err != nil {
		cancelSync()
		logger.Error("system node catalog synchronization failed", "error", err)
		os.Exit(1)
	}
	cancelSync()
	logger.Info("database model and system node catalogs synchronized")
	objects, err := objectstore.Open(objectstore.Config{Endpoint: configuration.ObjectStoreEndpoint, PublicEndpoint: configuration.ObjectStorePublicEndpoint, AccessKey: configuration.ObjectStoreAccessKey, SecretKey: configuration.ObjectStoreSecretKey, Bucket: configuration.ObjectStoreBucket, Region: configuration.ObjectStoreRegion, Secure: configuration.ObjectStoreSecure, PublicSecure: configuration.ObjectStorePublicSecure})
	if err != nil {
		logger.Error("object storage configuration failed", "error", err)
		os.Exit(1)
	}
	objectContext, cancelObjects := context.WithTimeout(context.Background(), 15*time.Second)
	if err = objects.EnsureBucket(objectContext); err != nil {
		cancelObjects()
		logger.Error("object storage initialization failed", "error", err)
		os.Exit(1)
	}
	cancelObjects()
	logger.Info("object storage bucket ready")
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: configuration.TemporalAddress, Namespace: configuration.TemporalNamespace,
		TaskQueue: configuration.TemporalTaskQueue,
	})
	if err != nil {
		logger.Error("workflow Temporal connection failed", "error", err)
		os.Exit(1)
	}
	defer temporalRuntime.Close()
	logger.Info("workflow Temporal client ready", "namespace", configuration.TemporalNamespace)

	projectStore := projectgorm.New(database)
	projectService := projectapp.NewService(projectStore, func() time.Time { return time.Now().UTC() }, uuid.NewString)
	tokenVerifier := authentication.NewVerifier(configuration.JWTSecret, configuration.JWTIssuer, configuration.JWTAudience, func() time.Time { return time.Now().UTC() })
	tokenIssuer := authentication.NewIssuer(configuration.JWTSecret, configuration.JWTIssuer, configuration.JWTAudience, configuration.AccessTokenTTL, func() time.Time { return time.Now().UTC() }, uuid.NewString)
	verificationCode := authentication.RandomNumericCode
	verificationDeliveryEnabled := false
	if configuration.RegistrationVerificationCode != "" {
		verificationCode = func() string { return configuration.RegistrationVerificationCode }
		verificationDeliveryEnabled = true
	}
	identityStore := identitygorm.New(database)
	identityService, err := identityapp.NewService(identityStore, authentication.NewPasswordHasher(), tokenIssuer, identityverification.ConfiguredSender{Enabled: verificationDeliveryEnabled}, identityapp.Config{
		AccessTokenTTL:   configuration.AccessTokenTTL,
		SessionTTL:       configuration.SessionTTL,
		DigestSecret:     configuration.JWTSecret,
		Now:              func() time.Time { return time.Now().UTC() },
		NewID:            uuid.NewString,
		NewSecret:        authentication.RandomSecret,
		VerificationCode: verificationCode,
	})
	if err != nil {
		logger.Error("identity service initialization failed", "error", err)
		os.Exit(1)
	}
	identityHandler := identityhttp.New(identityService, tokenVerifier, configuration.SessionTTL, configuration.Environment == "production", uuid.NewString)
	mediaStore := mediagorm.New(database)
	mediaService := mediaapp.NewService(mediaStore, objects, mediaapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	mediaHandler := mediahttp.New(mediaService, tokenVerifier)
	scriptStore := scriptgorm.New(database)
	scriptService := scriptapp.NewService(scriptStore, mediareader.Reader{Service: mediaService}, scriptapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	scriptHandler := scripthttp.New(scriptService, tokenVerifier)
	agentSigner, err := agentgrant.NewSigner(configuration.AgentExecutionSecret, func() time.Time { return time.Now().UTC() })
	if err != nil {
		logger.Error("agent execution grant configuration failed", "error", err)
		os.Exit(1)
	}
	agentRuntime, err := agentclient.New(configuration.AgentURL, agentSigner, nil)
	if err != nil {
		logger.Error("agent runtime configuration failed", "error", err)
		os.Exit(1)
	}
	bibleStore := biblegorm.New(database)
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	bibleHandler := biblehttp.New(bibleService, tokenVerifier)
	bibleWorker := bibleapp.NewWorker(bibleStore, agentRuntime, func() time.Time { return time.Now().UTC() }, configuration.AgentPollInterval, configuration.AgentClaimLease, logger)
	planningStore := planninggorm.New(database)
	planningService := planningapp.NewService(planningStore, planningapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	planningHandler := planninghttp.New(planningService, tokenVerifier)
	storyboardStore := storyboardgorm.New(database)
	storyboardService := storyboardapp.NewService(storyboardStore, storyboardapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	storyboardHandler := storyboardhttp.New(storyboardService, tokenVerifier)
	storyboardWorker := storyboardapp.NewWorker(storyboardStore, agentRuntime, func() time.Time { return time.Now().UTC() }, configuration.AgentPollInterval, configuration.AgentClaimLease, logger)
	projectHandler := projecthttp.New(projectService, tokenVerifier)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	workflowStore := workflowgorm.New(database)
	workflowCompiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflowdomain.SystemCompilerContract(),
		workflowapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	workflowStartService := workflowapp.NewStartService(
		workflowCompiler, workflowStore, temporalRuntime,
		workflowapp.StartConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	workflowQueryService := workflowapp.NewQueryService(workflowStore)
	workflowControlService := workflowapp.NewControlService(
		workflowStore, temporalRuntime,
		workflowapp.ControlConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	workflowHandler := workflowhttp.New(workflowStartService, workflowQueryService, workflowControlService, tokenVerifier)
	httpMetrics := telemetry.NewHTTPMetrics()
	server := &http.Server{
		Addr: configuration.ListenAddress,
		Handler: bootstrap.NewAPIHandler(bootstrap.RuntimeOptions{
			Build:   bootstrap.BuildInfo{Service: "lanverse-api", Version: buildVersion, Commit: buildCommit, BuiltAt: buildTime},
			Metrics: httpMetrics,
			Ready: func(ctx context.Context) error {
				if readyErr := platformdatabase.Ping(ctx, database); readyErr != nil {
					return readyErr
				}
				if readyErr := objects.Ping(ctx); readyErr != nil {
					return readyErr
				}
				return temporalRuntime.Ping(ctx)
			},
			AllowedOrigins: configuration.AllowedOrigins,
			RegisterRoutes: func(mux *http.ServeMux) {
				identityHandler.Register(mux)
				mediaHandler.Register(mux)
				projectHandler.Register(mux)
				scriptHandler.Register(mux)
				bibleHandler.Register(mux)
				planningHandler.Register(mux)
				storyboardHandler.Register(mux)
				workflowHandler.Register(mux)
			},
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go bibleWorker.Run(shutdownSignal)
	go storyboardWorker.Run(shutdownSignal)
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("lanverse api started", "address", configuration.ListenAddress)
		serverErrors <- server.ListenAndServe()
	}()
	select {
	case <-shutdownSignal.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Error("api shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}
}
