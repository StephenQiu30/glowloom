package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	agentclient "github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	agentgrant "github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
	assetgorm "github.com/StephenQiu30/lanverse/backend/internal/asset/adapter/gormdb"
	assetapp "github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	"github.com/StephenQiu30/lanverse/backend/internal/config"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	generationasset "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/asset"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationreview "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/review"
	providersecret "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/secretstore"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	storyboardgeneration "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/generation"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
)

func RunWorkflowWorker(ctx context.Context, logger *slog.Logger) error {
	configuration, err := config.Load()
	if err != nil {
		return fmt.Errorf("workflow worker configuration is invalid: %w", err)
	}
	connectContext, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
	database, err := platformdatabase.Open(connectContext, configuration.DatabaseURL, os.Stderr)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("workflow worker database connection failed: %w", err)
	}
	defer func() {
		if closeErr := platformdatabase.Close(database); closeErr != nil {
			logger.Error("workflow worker database close failed", "error", closeErr)
		}
	}()
	temporalRuntime, err := workflowtemporal.New(workflowtemporal.Config{
		Address: configuration.TemporalAddress, Namespace: configuration.TemporalNamespace,
		TaskQueue: configuration.TemporalTaskQueue,
	})
	if err != nil {
		return fmt.Errorf("workflow worker Temporal connection failed: %w", err)
	}
	defer temporalRuntime.Close()
	now := func() time.Time { return time.Now().UTC() }
	objects, err := objectstore.Open(objectstore.Config{
		Endpoint: configuration.ObjectStoreEndpoint, PublicEndpoint: configuration.ObjectStorePublicEndpoint,
		AccessKey: configuration.ObjectStoreAccessKey, SecretKey: configuration.ObjectStoreSecretKey,
		Bucket: configuration.ObjectStoreBucket, Region: configuration.ObjectStoreRegion,
		Secure: configuration.ObjectStoreSecure, PublicSecure: configuration.ObjectStorePublicSecure,
	})
	if err != nil {
		return fmt.Errorf("workflow worker object storage configuration failed: %w", err)
	}
	objectContext, cancelObjects := context.WithTimeout(ctx, 15*time.Second)
	if err = objects.EnsureBucket(objectContext); err != nil {
		cancelObjects()
		return fmt.Errorf("workflow worker object storage initialization failed: %w", err)
	}
	cancelObjects()
	scriptStore := scriptgorm.New(database)
	scriptService := scriptapp.NewService(scriptStore, nil, scriptapp.Config{Now: now, NewID: uuid.NewString})
	scriptSourceService := scriptapp.NewSourceService(
		scriptStore, scriptapp.SourceConfig{Now: now, NewID: uuid.NewString},
	)
	agentSigner, err := agentgrant.NewSigner(configuration.AgentExecutionSecret, now)
	if err != nil {
		return fmt.Errorf("workflow Agent execution grant configuration failed: %w", err)
	}
	agentRuntimeCatalog, err := agentcontract.NewRuntimeCatalog([]agentcontract.RuntimeRevision{{
		BundleHash: agentcontract.SceneAnalysisSkillBundleHash, BaseURL: configuration.AgentURL,
		ImageDigest: configuration.AgentRuntimeImageDigest,
	}})
	if err != nil {
		return fmt.Errorf("workflow Agent runtime configuration failed: %w", err)
	}
	sceneAnalysisService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), agentclient.New(agentRuntimeCatalog, agentSigner, nil),
		agentSigner,
		agentapp.SceneAnalysisConfig{
			Now: now, NewID: uuid.NewString, AgentImageDigest: configuration.AgentRuntimeImageDigest,
		},
	)
	if err != nil {
		return fmt.Errorf("workflow Scene Analysis service initialization failed: %w", err)
	}
	bibleStore := biblegorm.New(database)
	bibleService := bibleapp.NewService(bibleStore, bibleapp.Config{Now: now, NewID: uuid.NewString})
	evidenceService := bibleapp.NewSourceEvidenceService(bibleStore, bibleapp.SourceEvidenceConfig{
		Now: now, NewID: uuid.NewString,
	})
	storyAnalysisService := bibleapp.NewStoryAnalysisService(bibleStore, bibleapp.StoryAnalysisConfig{
		Now: now, NewID: uuid.NewString, FanIn: 2,
	})
	episodeSegmentationService := bibleapp.NewEpisodeSegmentationService(bibleStore, bibleapp.EpisodeSegmentationConfig{
		Now: now, NewID: uuid.NewString,
	})
	storyReviewService := bibleapp.NewStoryReviewService(
		bibleStore,
		bibleapp.NewStoryCandidateRepairService(bibleStore, bibleapp.Config{Now: now, NewID: uuid.NewString}),
		bibleapp.Config{Now: now, NewID: uuid.NewString},
	)
	projectService := projectapp.NewService(projectgorm.New(database), now, uuid.NewString)
	planningStore := planninggorm.New(database)
	planningService := planningapp.NewService(planningStore, planningapp.Config{Now: now, NewID: uuid.NewString})
	planningOwnerService := planningapp.NewEpisodePlanningService(planningStore, planningapp.Config{Now: now, NewID: uuid.NewString})
	storyGraphService := storygraphapp.NewService(storygraphgorm.New(database), storygraphapp.Config{Now: now, NewID: uuid.NewString})
	episodeAnalysisService := planningapp.NewEpisodeAnalysisService(
		planningStore, planningapp.EpisodeAnalysisConfig{Now: now, NewID: uuid.NewString},
	)
	storyboardService := storyboardapp.NewService(
		storyboardgorm.New(database), storyboardapp.Config{Now: now, NewID: uuid.NewString},
	)
	reviewService := reviewapp.NewService(
		reviewgorm.New(database), reviewapp.Config{Now: now, NewID: uuid.NewString},
	)
	costConfig := costapp.Config{Now: now, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: now, NewID: uuid.NewString}
	assetService := assetapp.NewService(assetgorm.New(database), objects, assetapp.Config{
		Now: now, NewID: uuid.NewString, Bucket: configuration.ObjectStoreBucket,
		StorageProfile: "private-primary", Region: configuration.ObjectStoreRegion, MaxImageBytes: 20 << 20,
	})
	candidateService := generationapp.NewService(
		generationgorm.New(database), generationasset.NewReadiness(assetService), generationapp.Config{},
	)
	providerRegistry, err := generationapp.NewMediaFactoryRegistry(nil)
	if err != nil {
		return fmt.Errorf("workflow Media Provider registry is invalid: %w", err)
	}
	providerCatalog, err := generationapp.NewMediaPresetCatalog(generationapp.BuiltinMediaPresets(), providerRegistry)
	if err != nil {
		return fmt.Errorf("workflow Media Provider preset catalog is invalid: %w", err)
	}
	providerSecrets := providersecret.OpenFixed()
	providerConfigurationService := generationapp.NewProviderConfigurationService(
		generationgorm.NewProviderConfigurationStore(database), providerCatalog, providerSecrets,
		generationapp.ProviderConfigurationConfig{Now: now, NewID: uuid.NewString},
	)
	providerService := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), nil,
		generationapp.ProviderConfig{Now: now, NewID: uuid.NewString},
	)
	logger.Info("workflow Media Provider configuration ready", "secret_store_available", providerSecrets.Available(),
		"connection_presets", len(providerConfigurationService.Catalog().Connections),
		"model_presets", len(providerConfigurationService.Catalog().Models))
	referenceTargetBuilder := generationapp.NewReferenceTargetBuilderService(
		generationgorm.New(database),
		generationapp.ReferenceTargetBuilderConfig{Now: now, NewID: uuid.NewString},
	)
	imagePreparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: now, NewID: uuid.NewString, ClaimTTL: 5 * time.Minute},
	)
	candidateSets := generationapp.NewOutputMaterializationService(
		providerService, generationasset.NewProviderOutputReadiness(assetService), candidateService,
	)
	selectionService := generationapp.NewSelectionService(
		generationgorm.New(database), candidateService, generationreview.NewDecisionReader(reviewService),
		generationapp.SelectionConfig{Now: now, NewID: uuid.NewString},
	)
	imageBindings := storyboardapp.NewShotImageBindingService(
		storyboardgorm.New(database), storyboardgeneration.NewSelectedImageSource(selectionService),
		storyboardapp.Config{Now: now, NewID: uuid.NewString},
	)
	activities, err := NewWorkflowRuntime(
		workflowgorm.New(database), scriptService, evidenceService, storyAnalysisService, storyReviewService, bibleService, projectService, planningService, planningOwnerService, storyGraphService, storyboardService, reviewService,
		imageBindings, candidateSets, referenceTargetBuilder, imagePreparations, providerService,
		episodeSegmentationService, episodeAnalysisService,
		workflowproduction.SceneAnalysisDependencies{Sources: scriptSourceService, Candidates: sceneAnalysisService},
	)
	if err != nil {
		return fmt.Errorf("workflow runtime composition failed: %w", err)
	}
	runtimeWorker, err := temporalRuntime.NewWorker(activities)
	if err != nil {
		return fmt.Errorf("workflow Temporal Worker composition failed: %w", err)
	}
	if err = runtimeWorker.Start(); err != nil {
		return fmt.Errorf("workflow Temporal Worker start failed: %w", err)
	}
	logger.Info("lanverse workflow worker started", "namespace", configuration.TemporalNamespace, "task_queue", configuration.TemporalTaskQueue)

	<-ctx.Done()
	logger.Info("lanverse workflow worker stopping")
	runtimeWorker.Stop()
	return nil
}
