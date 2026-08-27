package main

import (
	"context"
	"errors"
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
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	agentgrant "github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoringdomain "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/config"
	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costhttp "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/httpapi"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	generationasset "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/asset"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationreview "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/review"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
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
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewhttp "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/httpapi"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	searches "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/elasticsearch"
	searchgorm "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/gormdb"
	searchhttp "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/httpapi"
	searchproject "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/projectaccess"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphhttp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/httpapi"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowexecution "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/execution"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowhttp "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/httpapi"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowreview "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/review"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
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
	logger := telemetry.NewLogger(os.Stdout, "lanverse-api", os.Getenv("ENVIRONMENT"))
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
	episodeNodeCatalog, err := authoringdomain.SystemCatalog()
	if err != nil {
		cancelSync()
		logger.Error("system Episode node catalog is invalid", "error", err)
		os.Exit(1)
	}
	shotNodeCatalog, err := authoringdomain.SystemShotCatalog()
	if err != nil {
		cancelSync()
		logger.Error("system Shot node catalog is invalid", "error", err)
		os.Exit(1)
	}
	authoringStore := authoringgorm.New(database)
	for _, catalog := range []authoringdomain.Catalog{episodeNodeCatalog, shotNodeCatalog} {
		if _, err = authoringStore.EnsureCatalog(syncContext, catalog, time.Now().UTC(), uuid.NewString); err != nil {
			cancelSync()
			logger.Error("system node catalog synchronization failed", "catalog", catalog.Key, "error", err)
			os.Exit(1)
		}
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
	costStore := costgorm.New(database)
	costService := costapp.NewService(costStore, costapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
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
	agentRuntimeRevisions := []agentcontract.RuntimeRevision{{
		BundleHash: agentcontract.StoryGraphSkillBundleHash, BaseURL: configuration.AgentURL,
		ImageDigest: configuration.AgentRuntimeImageDigest,
	}}
	for _, revision := range configuration.AgentRuntimeAdditionalRevisions {
		agentRuntimeRevisions = append(agentRuntimeRevisions, agentcontract.RuntimeRevision{
			BundleHash: revision.BundleHash, BaseURL: revision.BaseURL, ImageDigest: revision.ImageDigest,
		})
	}
	agentRuntimeCatalog, err := agentcontract.NewRuntimeCatalog(agentRuntimeRevisions)
	if err != nil {
		logger.Error("agent runtime configuration failed", "error", err)
		os.Exit(1)
	}
	agentRuntime := agentclient.New(agentRuntimeCatalog, agentSigner, nil)
	bibleStore := biblegorm.New(database)
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	bibleHandler := biblehttp.New(bibleService, tokenVerifier)
	bibleWorker := bibleapp.NewWorker(bibleStore, agentRuntime, func() time.Time { return time.Now().UTC() }, configuration.AgentPollInterval, configuration.AgentClaimLease, logger)
	sourceEvidenceService := bibleapp.NewSourceEvidenceService(bibleStore, bibleapp.SourceEvidenceConfig{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	sourceEvidenceWorker := bibleapp.NewSourceEvidenceWorker(
		bibleStore, sourceEvidenceService, agentRuntime, func() time.Time { return time.Now().UTC() },
		configuration.AgentPollInterval, configuration.AgentClaimLease, logger,
	)
	storyAnalysisWorker := bibleapp.NewStoryAnalysisWorker(
		bibleStore, agentRuntime, func() time.Time { return time.Now().UTC() },
		configuration.AgentPollInterval, configuration.AgentClaimLease, logger,
	)
	planningStore := planninggorm.New(database)
	planningService := planningapp.NewService(planningStore, planningapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	planningHandler := planninghttp.New(planningService, tokenVerifier)
	storyboardStore := storyboardgorm.New(database)
	storyboardService := storyboardapp.NewService(storyboardStore, storyboardapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})
	storyboardHandler := storyboardhttp.New(storyboardService, tokenVerifier)
	storyboardWorker := storyboardapp.NewWorker(storyboardStore, agentRuntime, func() time.Time { return time.Now().UTC() }, configuration.AgentPollInterval, configuration.AgentClaimLease, logger)
	reviewStore := reviewgorm.New(database)
	reviewService := reviewapp.NewService(reviewStore, reviewapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		ClaimLease: configuration.ReviewClaimLease,
	})
	assetService := assetapp.NewService(assetgorm.New(database), objects, assetapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
		Bucket: configuration.ObjectStoreBucket, StorageProfile: "private-primary",
		Region: configuration.ObjectStoreRegion, MaxImageBytes: 20 << 20,
	})
	candidateService := generationapp.NewService(
		generationgorm.New(database), generationasset.NewReadiness(assetService), generationapp.Config{},
	)
	selectionService := generationapp.NewSelectionService(
		generationgorm.New(database), candidateService, generationreview.NewDecisionReader(reviewService),
		generationapp.SelectionConfig{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
	)
	storyGraphStore := storygraphgorm.New(database)
	storyGraphQueryService := storygraphapp.NewQueryService(storyGraphStore)
	storyGraphHandler := storygraphhttp.New(storyGraphQueryService, tokenVerifier)
	searchIndex, err := searches.New(searches.Config{
		Addresses: []string{configuration.ElasticsearchURL}, Username: configuration.ElasticsearchUsername,
		Password: configuration.ElasticsearchPassword, ScriptAlias: configuration.ElasticsearchScriptAlias,
		StoryGraphAlias: configuration.ElasticsearchStoryGraphAlias,
	})
	if err != nil {
		logger.Error("search Elasticsearch configuration failed", "error", err)
		os.Exit(1)
	}
	searchService := searchapp.NewService(searchproject.New(projectService), searchgorm.New(database), searchIndex)
	searchHandler := searchhttp.New(searchService, tokenVerifier)
	projectHandler := projecthttp.New(projectService, tokenVerifier)
	costHandler := costhttp.New(costService, tokenVerifier)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString,
	})
	workflowStore := workflowgorm.New(database)
	workflowCompiler := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflowapp.Config{Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString},
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
	humanGateOwners, err := workflowexecution.NewHumanGateOwnerRouter(
		workflowproduction.New(bibleService, planningService, storyboardService),
		workflowgeneration.NewHumanGateApplier(selectionService),
	)
	if err != nil {
		logger.Error("workflow Human Gate owner composition failed", "error", err)
		os.Exit(1)
	}
	workflowSignalService := workflowapp.NewSignalService(
		workflowStore, temporalRuntime,
		workflowapp.SignalConfig{
			Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString, Owner: humanGateOwners,
		},
	)
	humanGateCoordinator := workflowapp.NewHumanGateCoordinator(
		workflowreview.NewDecisionReader(reviewService), workflowSignalService, workflowStore,
	)
	reviewHandler := reviewhttp.New(reviewService, humanGateCoordinator, tokenVerifier)
	httpMetrics := telemetry.NewHTTPMetrics()
	server := &http.Server{
		Addr: configuration.ListenAddress,
		Handler: bootstrap.NewAPIHandler(bootstrap.RuntimeOptions{
			Build:   bootstrap.BuildInfo{Service: "lanverse-api", Version: buildVersion, Commit: buildCommit, BuiltAt: buildTime},
			Metrics: httpMetrics,
			Logger:  logger,
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
				costHandler.Register(mux)
				scriptHandler.Register(mux)
				bibleHandler.Register(mux)
				planningHandler.Register(mux)
				storyboardHandler.Register(mux)
				storyGraphHandler.Register(mux)
				searchHandler.Register(mux)
				workflowHandler.Register(mux)
				reviewHandler.Register(mux)
			},
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go bibleWorker.Run(shutdownSignal)
	go sourceEvidenceWorker.Run(shutdownSignal)
	go storyAnalysisWorker.Run(shutdownSignal)
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
