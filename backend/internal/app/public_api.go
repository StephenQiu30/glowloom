package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/google/uuid"

	operationhttp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/http"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"

	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/yjsruntime"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	insightswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/insights"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	copywire "github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopy"
	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/adapter/memory"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	biblehttp "github.com/StephenQiu30/glowloom-server/backend/creation/bible/adapter/http"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/creation/bible/application"
	canvashttp "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/http"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/postgres"
	canvaslive "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/adapter/websocket"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	prompthttp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/http"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	scripthttp "github.com/StephenQiu30/glowloom-server/backend/creation/script/adapter/http"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/creation/script/adapter/objects"
	scriptapp "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	subjecthttp "github.com/StephenQiu30/glowloom-server/backend/creation/subject/adapter/http"
	subjectapp "github.com/StephenQiu30/glowloom-server/backend/creation/subject/application"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/http"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	billinghttp "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/http"
	billingredis "github.com/StephenQiu30/glowloom-server/backend/execution/billing/adapter/redis"
	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/execution/catalog/adapter/paramvalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/media/adapter/gltf"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/media/adapter/http"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	toolhttp "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/adapter/http"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/application"
)

// NewBusinessRouter assembles the public workspace API.
// Its stores are the same injected dependencies used by the API role.
// @title Glowloom API
// @version 0.1
// @description Glowloom local workspace API for projects, canvases, media, scripts, bible, model catalog and operations.
// @BasePath /
func NewBusinessRouter(logger *zap.Logger, ready ReadyCheck, tp trace.TracerProvider, cfg config.Config, database *gorm.DB, storage *objectstorage.Registry, sessions identityruntime.SessionDependencies, lifetime ...context.Context) (*gin.Engine, error) {
	if database == nil {
		return nil, fmt.Errorf("public API requires database")
	}
	if cfg.ObjectStorage == nil || storage == nil {
		return nil, fmt.Errorf("public API requires unified object storage configuration")
	}
	if cfg.Env != "local" {
		return nil, fmt.Errorf("current workspace API requires local environment")
	}
	if cfg.PublicOrigin == "" && cfg.Env == "local" {
		cfg.PublicOrigin = "http://localhost:3000"
	}
	if err := config.ValidatePublicOrigin(cfg.PublicOrigin, cfg.Env); err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(cfg.HTTPAddr)
	address := net.ParseIP(host)
	origin, originErr := url.Parse(cfg.PublicOrigin)
	browser := net.ParseIP(origin.Hostname())
	if err != nil || address == nil || !address.IsLoopback() || originErr != nil ||
		(origin.Hostname() != "localhost" && (browser == nil || !browser.IsLoopback())) {
		return nil, fmt.Errorf("%w: current workspace API requires loopback listener and browser origin", config.ErrInvalid)
	}
	diagnostics := memory.NewBuffer(4096, 24*time.Hour, time.Now)
	router := NewRouter(logger, ready, tp, diagnostics)
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	router.Use(httpapi.Middleware(cfg.PublicOrigin))
	router.HandleMethodNotAllowed = true
	router.NoRoute(func(c *gin.Context) { httpapi.WriteProblem(c, 404, "not_found", nil) })
	router.NoMethod(func(c *gin.Context) { httpapi.WriteProblem(c, 405, "method_not_allowed", nil) })
	api := router.Group("/api", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	protected := api.Group("")
	socketLifetime := context.Background()
	if len(lifetime) > 0 {
		socketLifetime = lifetime[0]
	}
	subjectObjects := subjectwire.UseRegistry(storage)
	emailAuth, err := identityruntime.NewAuth(database, cfg, time.Now)
	if err != nil {
		return nil, fmt.Errorf("configure email authentication: %w", err)
	}
	if err := identityruntime.ConfigureSessionSecurity(emailAuth, cfg, sessions); err != nil {
		return nil, fmt.Errorf("configure session security: %w", err)
	}
	auth, registerSelf, err := identityruntime.RegisterSelf(database, emailAuth, cfg, subjectObjects.Registry(), api, time.Now)
	if err != nil {
		return nil, fmt.Errorf("configure self account services: %w", err)
	}
	auth.Register(api)
	notifications := identityhttp.NewNotificationHandler(identityapp.NewNotifications(pgidentity.NewNotificationStore(database), time.Now))
	notifications.RegisterPublic(api)
	identityhttp.NewAuthPolicyHandler(identityapp.NewAuthPolicyQuery(pgidentity.NewStore(database), identityhttp.AcceptedLegalVersion(cfg.IdentityTermsVersion), identityhttp.LegalPackageVersion(), cfg.SMTPEnabled && cfg.IdentityMailSourceHourlyLimit > 0 && cfg.IdentityMailGlobalHourlyLimit > 0)).Register(api)
	protected.Use(auth.RequireSession())
	registerSelf(protected)
	notifications.Register(protected)
	insightswire.Register(protected, database, diagnostics, time.Now)
	creditLimiter := billingredis.NewCreditCommandLimiter(sessions.RedisClient, cfg.CreditCommandRequestsPerMinute, cfg.CreditCommandMaxAmountMicros, cfg.CreditCommandMaxDailyMicros)
	identityProtected := protected.Group("")
	gatewayRuntime, err := registerAIGateway(api, identityProtected, database, cfg)
	if err != nil {
		return nil, err
	}
	gatewayRuntime.Objects = subjectObjects.Registry()
	workspaceStore := workspacewire.ProvideWorkspaceLifecycleStore(database)
	protected.Use(workspacehttp.RequireCreationScope(scopewire.ProvideRequestScopeAccess(database)))
	protected.Use(workspacehttp.RequireProjectAccess(workspaceStore))
	billinghttp.NewWalletHandler(gatewayRuntime.Wallet, creditLimiter).Register(protected)
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(workspaceStore), workspaceapp.NewCreateProjectCommand(workspaceStore, time.Now), workspaceapp.NewListStylePresetsQuery(workspaceStore)).Register(protected)
	workspacehttp.NewStyleLibraryHandler(workspaceapp.NewStyleLibrary(workspacewire.ProvideStyleLibraryStore(database))).Register(protected)
	workspacehttp.RegisterIndependentCreation(protected, workspaceStore.EnsureIndependentCreation)
	workspacehttp.NewProjectBudgetHandler(workspaceapp.NewProjectBudgetQuery(aigateway.Defaults{Runtime: gatewayRuntime})).Register(protected)
	workspacehttp.NewProjectLifecycleHandler(workspaceapp.NewProjectLifecycle(workspaceStore, time.Now)).Register(protected)
	workspacehttp.RegisterProjectMemory(protected, workspaceapp.NewProjectMemories(workspaceStore, time.Now))
	workspacehttp.NewProjectCreationRecoveryHandler(workspaceapp.NewProjectCreationRecoveryQuery(workspaceStore)).Register(protected)
	copies := copywire.ProvideProjectCopyStoreWithObjects(database, mediawire.NewMediaScopeAccess)
	transfers := mediawire.ProvideRoutedMediaTransferStore(database)
	workspacehttp.NewProjectCopyHandler(workspaceapp.NewProjectCopyService(copies, time.Now)).Register(protected)
	mediahttp.NewTransferHandler(transfers, resolveLegacyMediaActor).Register(protected)
	mediahttp.NewPurgeHandler(mediawire.ProvideMediaPurgeStore(database), resolveLegacyMediaActor).Register(protected)
	usage := mediawire.ProvideRoutedMediaStorageUsage(database, subjectObjects.Registry(), cfg.ObjectStorage)
	mediahttp.NewStorageUsageHandler(usage, resolveLegacyMediaActor).Register(protected)
	workspacehttp.NewProjectFolderHandler(workspaceapp.NewProjectFolders(workspacewire.ProvideProjectFolderStore(database), time.Now)).Register(protected)
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader {
		return mediawire.LegacyAssetReader(mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess)))
	}
	canvasStore := pgcanvas.NewScopedStore(database, mediaFactory, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewPromptSources).WithCanvasUsage(canvaswire.NewInformationUsage)
	canvasService := canvasapp.NewService(canvasStore)
	canvashttp.NewHandler(canvasService).RegisterLocation(identityProtected)
	live := canvaslive.NewHandler(canvaswire.SocketAuthority(database, auth), cfg.PublicOrigin)
	if cfg.CanvasRuntimeURL != "" && len(cfg.CanvasRuntimeToken) >= 32 {
		live.WithYjs(canvasapp.NewYjsService(canvasStore, yjsruntime.New(cfg.CanvasRuntimeURL, cfg.CanvasRuntimeToken)))
	}
	socketRoutes := api.Group("")
	socketRoutes.Use(auth.RequireSession())
	live.Register(socketLifetime, socketRoutes)
	canvashttp.NewHandler(canvasService).WithNotification(live.Notify).Register(protected)
	storyboardCanvas := canvasapp.NewYjsService(canvasStore, yjsruntime.New(cfg.CanvasRuntimeURL, cfg.CanvasRuntimeToken))
	canvashttp.NewStoryboardHandler(storyboardCanvas, live.Notify).Register(protected)
	canvaswire.RegisterStoryboardDelivery(protected, storyboardCanvas, live.Notify, aigateway.Creation{Runtime: gatewayRuntime}, mediawire.LegacyAssetReader(mediaapp.NewAssetQuery(pgmedia.NewScopedStore(database, mediawire.NewMediaScopeAccess))), aigateway.WriteCreationProblem)
	canvashttp.NewAssistantHandler(canvasapp.NewAssistant(pgcanvas.NewAssistantStore(database, mediawire.NewCanvasProjectAccess))).Register(protected)
	canvashttp.NewAssetHandler(canvasapp.NewAssetQuery(pgcanvas.NewAssetStore(database, canvaswire.AssetAuthority))).Register(protected)
	canvashttp.NewAssetCreationHandler(canvasapp.NewAssetCreator(pgcanvas.NewAssetCreationStore(database, canvaswire.AssetCreationAuthority))).Register(protected)
	canvashttp.NewAssetCommandHandler(canvasapp.NewAssetCommands(pgcanvas.NewAssetCommandStore(database, canvaswire.NewAssetMutationAuthority(yjsruntime.New(cfg.CanvasRuntimeURL, cfg.CanvasRuntimeToken))))).WithNotification(live.Notify).Register(protected)
	assetReader := pgmedia.NewScopedStore(database, mediawire.NewMediaScopeAccess)
	documentReader := pgmedia.NewScopedDocumentSourceStore(database, mediawire.NewMediaScopeAccess)
	assetQuery := mediaapp.NewRoutedAssetQuery(assetReader, subjectObjects.Routed, cfg.ObjectStorage.Access.PreviewTTL)
	documents := mediaapp.NewRoutedDocumentSources(documentReader, subjectObjects.Routed)
	mediahttp.NewHandler(assetQuery, resolveLegacyMediaActor).Register(protected)
	mediahttp.NewVoiceLibraryHandler(assetReader, resolveLegacyMediaActor).Register(protected)
	mediahttp.NewDocumentHandler(documents, resolveLegacyMediaActor).Register(protected)
	library := pgmedia.NewScopedLibraryStore(database, mediawire.NewMediaLibraryProjectAccess, time.Now, mediawire.NewMediaScopeAccess)
	mediahttp.NewLibraryHandler(library, resolveLegacyMediaActor).Register(protected)
	libraryQuery := mediaapp.NewRoutedLibraryMediaQuery(library, subjectObjects.Routed, subjectObjects.Routed, cfg.ObjectStorage.Access.PreviewTTL)
	mediahttp.NewLibraryMediaHandler(libraryQuery, resolveLegacyMediaActor).Register(protected)
	scripts := scriptwire.ProvideServicesWithObjects(database, subjectObjects.Registry())
	scripthttp.NewSourceHandler(scripts.Sources, resolveLegacyScriptActor).Register(protected)
	scripthttp.NewSourceRecoveryHandler(scripts.Recovery, resolveLegacyScriptActor).Register(protected)
	scripthttp.NewReviewHandler(scripts.Episodes, scripts.History, resolveLegacyScriptActor).Register(protected)
	scripthttp.NewVersionHandler(scripts.History, scripts.Adopt, resolveLegacyScriptActor).Register(protected)
	scripthttp.NewImportHandler(scripts.Imports, resolveLegacyScriptActor).Register(protected)
	modelValidator, err := gltf.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("configure model upload validation: %w", err)
	}
	uploadStore := pgmedia.NewScopedStore(database, mediawire.NewMediaScopeAccess)
	registry := subjectObjects.Registry()
	normalizer, err := mediawire.ProvideUploadNormalizer(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure media normalization: %w", err)
	}
	scopedUpload := mediaapp.NewRoutedUploadService(uploadStore, uploadStore, pgmedia.NewUploadAdmission(uploadStore), mediaflow.NewUploadProber(modelValidator), normalizer, mediaflow.FFUploadRenderer{}, mediaobjects.NewRoutedUploadObjects(registry, pgmedia.NewStorageRoutes(database)), time.Now)
	uploadHandler := mediahttp.NewUploadHandler(scopedUpload, resolveLegacyMediaActor)
	uploadHandler.Register(protected)
	videoLinks, err := mediaflow.NewPublicVideoLinkResolver(mediaflow.VideoLinkResolverOptions{})
	if err != nil {
		return nil, fmt.Errorf("configure public video links: %w", err)
	}
	mediahttp.NewVideoLinkHandler(mediaapp.NewVideoLinkService(scopedUpload, videoLinks), resolveLegacyMediaActor).Register(protected)
	packages := mediawire.ProvideRoutedMediaPackages(database, registry, scopedUpload)
	mediahttp.NewPackageHandler(packages, resolveLegacyMediaActor).Register(protected)
	exports := mediawire.ProvideRoutedMediaExportStore(database, mediawire.NewMediaScopeAccess)
	exportQuery := toolapp.NewRoutedExportQuery(exports, subjectObjects.Routed, subjectObjects.Routed, cfg.ObjectStorage.Access.PreviewTTL)
	exportQuery.WithLaunchOrigin(cfg.PublicOrigin)
	exportHandler := toolhttp.NewHandler(exports, exportQuery)
	exportHandler.Register(protected)
	exportHandler.RegisterLaunch(api)
	depths := mediawire.ProvideRoutedMediaDepthStore(database, cfg.VideoDepthPythonPath != "", mediawire.NewMediaScopeAccess)
	depthQuery := toolapp.NewRoutedDepthQuery(depths, subjectObjects.Routed, subjectObjects.Routed, cfg.ObjectStorage.Access.PreviewTTL)
	toolhttp.NewDepthHandler(depths, depthQuery).Register(protected)
	parameterValidator, err := paramvalidation.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("configure model parameter validation: %w", err)
	}
	biblehttp.NewHandler(bibleapp.NewService(biblewire.ProvideBibleStoreWithObjects(database, subjectObjects.Registry(), parameterValidator), time.Now), resolveLegacyBibleActor).Register(protected)
	mediahttp.NewAssetDownloadHandler(mediawire.ProvideAssetDownloads(database, subjectObjects.Routed), resolveLegacyMediaActor).Register(protected)
	subjectStore := subjectwire.NewStore(database, subjectObjects)
	subjecthttp.NewHandler(subjectapp.NewService(subjectStore, time.Now), subjectStore).Register(protected)
	subjecthttp.NewVoiceHandler(aigateway.NewVoiceJobs(gatewayRuntime)).Register(protected)
	subjecthttp.NewFileHandler(subjectwire.NewFiles(database, subjectObjects)).Register(protected)
	subjecthttp.NewSourceHandler(subjectwire.NewSourceLibrary(database, subjectObjects, cfg.ObjectStorage.Access.PreviewTTL), uploadHandler).Register(protected)
	cataloghttp.NewHandler(catalogapp.NewListModelsQuery(aigateway.Catalog{DB: database})).Register(protected)
	workspacehttp.NewModelDefaultsHandler(workspaceapp.NewModelDefaultsService(aigateway.Defaults{Runtime: gatewayRuntime}, time.Now)).Register(protected)
	prompthttp.NewHandler(promptapp.NewPreferences(operationwire.ProvidePromptPreferenceStore(database), time.Now), resolveLegacyPromptActor).Register(identityProtected)
	prompthttp.NewDraftHandler(pgprompt.NewDraftStore(database, promptwire.DraftAccess).WithSubjectSources(subjectwire.NewPromptSources), resolveLegacyPromptActor).Register(protected)
	agentModel, agentRecover, err := ConfigureAgentBoundaryModel(database, cfg)
	if err != nil {
		return nil, fmt.Errorf("configure agent model gateway: %w", err)
	}
	documentExtractor, err := scriptwire.ProvideSourceExtractor(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure Creation extraction: %w", err)
	}
	if err := RegisterAgentBoundary(socketLifetime, router, protected, database, AgentBoundaryOptions{Gateway: gatewayRuntime, DocumentExtractor: documentExtractor, Origin: cfg.AgentOrigin, ServiceToken: cfg.AgentServiceToken, BusinessToken: cfg.AgentBusinessToken, Model: agentModel, ModelRecover: agentRecover, MediaObjects: subjectObjects.Routed, SourceObjects: scriptobjects.NewRoutedStorage(registry, pgmedia.NewStorageRoutes(database)), CanvasRuntime: yjsruntime.New(cfg.CanvasRuntimeURL, cfg.CanvasRuntimeToken), CanvasNotify: live.Notify, CreditOperationSagaEnabled: cfg.CreditOperationSagaEnabled}, logger); err != nil {
		return nil, fmt.Errorf("configure agent boundary: %w", err)
	}
	(aigateway.Creation{Runtime: gatewayRuntime}).RegisterCreation(protected)
	rebuiltHistory := aigateway.History{Creation: aigateway.Creation{Runtime: gatewayRuntime}}
	operationhttp.NewPersonalHistoryHandler(operationapp.NewPersonalHistoryQuery(rebuiltHistory)).Register(protected)
	operationhttp.NewPersonalHistoryCommandHandler(operationapp.NewPersonalHistoryCommander(rebuiltHistory)).Register(protected)
	operationhttp.NewHistoryPublicationHandler(operationapp.NewHistoryPublisher(rebuiltHistory), operationapp.NewPublishedWorkQuery(rebuiltHistory, operationwire.PublicationMedia(subjectObjects.Routed))).Register(protected, api)
	if len(lifetime) > 0 {
		stopLaunchCleanup := mediawire.StartExportLaunchCleanup(socketLifetime, exports, logger)
		context.AfterFunc(socketLifetime, stopLaunchCleanup)
	}
	return router, nil
}

func resolveLegacyScriptActor(c *gin.Context) (scriptapp.Actor, error) {
	if c == nil {
		return scriptapp.Actor{}, scriptapp.ErrSessionInvalid
	}
	actor := scriptwire.Actor(identityhttp.Principal(c))
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return scriptapp.Actor{}, scriptapp.ErrSessionInvalid
	}
	return actor, nil
}

func resolveLegacyMediaActor(c *gin.Context) (mediaapp.Actor, error) {
	if c == nil {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	principal := identityhttp.Principal(c)
	actor := mediaapp.Actor{ID: principal.ID, OrgID: principal.OrgID, ScopeID: principal.ScopeID, SessionID: principal.SessionID, CredentialEpoch: principal.CredentialEpoch, PlatformEpoch: principal.PlatformEpoch}
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
	}
	return actor, nil
}

func resolveLegacyPromptActor(c *gin.Context) (promptapp.Actor, error) {
	if c == nil {
		return promptapp.Actor{}, promptapp.ErrSessionInvalid
	}
	actor := identityhttp.Principal(c)
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return promptapp.Actor{}, promptapp.ErrSessionInvalid
	}
	return promptapp.Actor{
		ID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID,
		SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch,
	}, nil
}

func resolveLegacyBibleActor(c *gin.Context) (bibleapp.Actor, error) {
	if c == nil {
		return bibleapp.Actor{}, bibleapp.ErrSessionInvalid
	}
	actor := identityhttp.Principal(c)
	if actor.ID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return bibleapp.Actor{}, bibleapp.ErrSessionInvalid
	}
	return biblewire.Actor(actor), nil
}
