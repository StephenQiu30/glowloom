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
	generationopenai "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationreview "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/review"
	providersecret "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/secretstore"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
	presetgorm "github.com/StephenQiu30/lanverse/backend/internal/preset/adapter/gormdb"
	presetcatalog "github.com/StephenQiu30/lanverse/backend/internal/preset/catalog"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	planninggorm "github.com/StephenQiu30/lanverse/backend/internal/production/planning/adapter/gormdb"
	planningapp "github.com/StephenQiu30/lanverse/backend/internal/production/planning/application"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	referencegorm "github.com/StephenQiu30/lanverse/backend/internal/production/reference/adapter/gormdb"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	storyboardgeneration "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/generation"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	worldgorm "github.com/StephenQiu30/lanverse/backend/internal/production/world/adapter/gormdb"
	worldapp "github.com/StephenQiu30/lanverse/backend/internal/production/world/application"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	workflowgeneration "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/generation"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowproduction "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/production"
	workflowtemporal "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
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
	agentHTTPClient := agentclient.New(agentRuntimeCatalog, agentSigner, nil)
	sceneAnalysisService, err := agentapp.NewSceneAnalysisService(
		agentgorm.NewSceneAnalysisStore(database), agentHTTPClient,
		agentSigner,
		agentapp.SceneAnalysisConfig{
			Now: now, NewID: uuid.NewString, AgentImageDigest: configuration.AgentRuntimeImageDigest,
		},
	)
	if err != nil {
		return fmt.Errorf("workflow Scene Analysis service initialization failed: %w", err)
	}
	visualBroker, err := agentapp.NewVisualFoundationMediaBroker(objects, agentHTTPClient)
	if err != nil {
		return fmt.Errorf("workflow Visual Foundation media Broker initialization failed: %w", err)
	}
	visualStore, err := agentgorm.NewVisualFoundationStore(
		database,
		workflowgorm.ValidateCurrentVisualFoundationInput,
	)
	if err != nil {
		return fmt.Errorf("workflow Visual Foundation store initialization failed: %w", err)
	}
	visualFoundationService, err := agentapp.NewVisualFoundationExecutionService(
		visualStore, visualBroker, agentSigner,
		agentapp.VisualFoundationExecutionConfig{
			Now: now, NewID: uuid.NewString, AgentImageDigest: configuration.AgentRuntimeImageDigest,
		},
	)
	if err != nil {
		return fmt.Errorf("workflow Visual Foundation service initialization failed: %w", err)
	}
	referencePlanStore, err := agentgorm.NewReferencePlanStore(
		database,
		workflowgorm.ValidateCurrentReferencePlanInput,
	)
	if err != nil {
		return fmt.Errorf("workflow Reference Plan store initialization failed: %w", err)
	}
	referencePlanService, err := agentapp.NewReferencePlanExecutionService(
		referencePlanStore, agentHTTPClient, agentSigner,
		agentapp.ReferencePlanExecutionConfig{
			Now: now, NewID: uuid.NewString, AgentImageDigest: configuration.AgentRuntimeImageDigest,
			ValidateCandidate: workflowapp.ValidateReferencePlanCandidateProjection,
		},
	)
	if err != nil {
		return fmt.Errorf("workflow Reference Plan service initialization failed: %w", err)
	}
	referenceBriefRelease, err := agentapp.BuildStageReleaseRecord(
		agentcontract.ReferenceBriefStageKey,
		configuration.AgentRuntimeImageDigest,
		now(),
	)
	if err != nil {
		return fmt.Errorf("workflow Reference Brief release initialization failed: %w", err)
	}
	referenceBriefStageRelease := agentcontract.ReferenceBriefStageRelease{
		StageKey:         agentcontract.ReferenceBriefStageKey,
		StageReleaseHash: referenceBriefRelease.Identity.StageReleaseHash,
	}
	referenceBriefStore, err := agentgorm.NewReferenceBriefStore(
		database,
		referencegorm.ValidateCurrentReferenceBriefInput,
	)
	if err != nil {
		return fmt.Errorf("workflow Reference Brief store initialization failed: %w", err)
	}
	referenceBriefService, err := agentapp.NewReferenceBriefExecutionService(
		referenceBriefStore, agentHTTPClient, agentSigner,
		agentapp.ReferenceBriefExecutionConfig{
			Now: now, NewID: uuid.NewString, AgentImageDigest: configuration.AgentRuntimeImageDigest,
		},
	)
	if err != nil {
		return fmt.Errorf("workflow Reference Brief service initialization failed: %w", err)
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
	providerRegistry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{generationopenai.NewFactory(nil, objects, time.Now)})
	if err != nil {
		return fmt.Errorf("workflow Media Provider registry is invalid: %w", err)
	}
	providerCatalog, err := generationapp.NewMediaPresetCatalog(generationapp.BuiltinMediaPresets(), providerRegistry)
	if err != nil {
		return fmt.Errorf("workflow Media Provider preset catalog is invalid: %w", err)
	}
	providerSecrets := providersecret.OpenFixed()
	referenceStore := generationgorm.New(database)
	referenceExecution, err := generationapp.NewReferenceCallExecutionService(referenceStore, providerRegistry, providerSecrets, now, uuid.NewString)
	if err != nil {
		return fmt.Errorf("workflow Reference execution composition failed: %w", err)
	}
	referenceRecovery, err := generationapp.NewReferenceCallDispatchService(referenceStore, providerRegistry, now, uuid.NewString)
	if err != nil {
		return fmt.Errorf("workflow Reference recovery composition failed: %w", err)
	}
	referenceMedia, err := generationapp.NewReferenceStagedMediaService(referenceStore, objects, generationdomain.ReferenceObjectStoreRef{Profile: "minio", Bucket: configuration.ObjectStoreBucket}, now)
	if err != nil {
		return fmt.Errorf("workflow Reference staged media composition failed: %w", err)
	}
	referenceCalls, err := workflowgeneration.NewReferenceObservationNodeExecutor(referenceExecution, referenceRecovery, referenceMedia, generationapp.NewReferenceExecutionQuery(referenceStore))
	if err != nil {
		return fmt.Errorf("workflow Reference call composition failed: %w", err)
	}
	visionReader, err := generationapp.NewVisionReviewMediaReader(referenceStore, objects, generationdomain.ReferenceObjectStoreRef{Profile: "minio", Bucket: configuration.ObjectStoreBucket})
	if err != nil {
		return fmt.Errorf("workflow Vision Review media composition failed: %w", err)
	}
	visionStore, err := agentgorm.NewVisionReviewStore(database, generationgorm.ValidateCurrentVisionReviewInput)
	if err != nil {
		return fmt.Errorf("workflow Vision Review store composition failed: %w", err)
	}
	visionService, err := agentapp.NewVisionReviewExecutionService(visionStore, visionReader, agentHTTPClient, agentSigner, agentapp.VisionReviewExecutionConfig{Now: now, NewID: uuid.NewString, AgentImageDigest: configuration.AgentRuntimeImageDigest})
	if err != nil {
		return fmt.Errorf("workflow Vision Review service composition failed: %w", err)
	}
	visionRelease, err := agentapp.BuildStageReleaseRecord(agentcontract.VisionReviewStageKey, configuration.AgentRuntimeImageDigest, now())
	if err != nil {
		return fmt.Errorf("workflow Vision Review release composition failed: %w", err)
	}
	providerConfigurationService := generationapp.NewProviderConfigurationService(
		generationgorm.NewProviderConfigurationStore(database), providerCatalog, providerSecrets,
		generationapp.ProviderConfigurationConfig{Now: now, NewID: uuid.NewString},
	)
	providerService := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), generationapp.NewRuntimeGateway(
			generationapp.NewFrozenProviderRuntime(generationgorm.NewProviderConfigurationStore(database), providerSecrets), providerRegistry),
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
	workflowStore := workflowgorm.New(database)
	productionWorldService, err := workflowapp.NewProductionWorldAssemblyService(
		workflowStore,
		workflowapp.ProductionWorldAssemblyConfig{Now: now, NewID: uuid.NewString},
	)
	if err != nil {
		return fmt.Errorf("Production World assembly composition failed: %w", err)
	}
	visualSourceService, err := worldapp.NewVisualFoundationSourceService(
		worldgorm.NewVisualFoundationSourceRepository(database),
	)
	if err != nil {
		return fmt.Errorf("workflow Visual Foundation source composition failed: %w", err)
	}
	referencePlanSources := workflowgorm.NewReferencePlanSourceStore(database)
	storyGraphQueries := storygraphapp.NewQueryService(storygraphgorm.New(database))
	activities, err := NewWorkflowRuntime(
		workflowStore, scriptService, evidenceService, storyAnalysisService, storyReviewService, bibleService, projectService, planningService, planningOwnerService, storyGraphService, storyboardService, reviewService,
		imageBindings, candidateSets, referenceTargetBuilder, imagePreparations, providerService,
		episodeSegmentationService, episodeAnalysisService, referenceCalls,
		workflowproduction.SceneAnalysisDependencies{
			Sources: scriptSourceService, Candidates: sceneAnalysisService,
			StructureIdentities: bibleapp.NewStructureIdentityQuery(bibleStore, projectService),
			ProductionWorld:     productionWorldService,
			VisualFoundation: &workflowproduction.VisualFoundationDependencies{
				Selections: presetgorm.NewProjectSelectionStore(database), FindRelease: presetcatalog.FindCuratedRelease,
				Worlds:  storyGraphQueries,
				Sources: visualSourceService, Candidates: visualFoundationService,
			},
			ReferencePlan: &workflowproduction.ReferencePlanDependencies{
				Selections: presetgorm.NewProjectSelectionStore(database), FindRelease: presetcatalog.FindCuratedRelease,
				Worlds: storyGraphQueries, Sources: referencePlanSources, Candidates: referencePlanService,
			},
			ReferenceBrief: &workflowproduction.ReferenceBriefDependencies{
				Inputs: referencegorm.NewStore(database), Candidates: referenceBriefService,
				StageRelease: referenceBriefStageRelease,
			},
			VisionReview: &workflowproduction.VisionReviewDependencies{Inputs: referenceStore, Execution: visionService, StageReleaseHash: visionRelease.Identity.StageReleaseHash},
		},
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
