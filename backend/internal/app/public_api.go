package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/yjsruntime"

	trialwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/aitrial"
	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	copywire "github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopy"
	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	biblehttp "github.com/StephenQiu30/glowloom-server/backend/internal/bible/adapter/http"
	bibleapp "github.com/StephenQiu30/glowloom-server/backend/internal/bible/application"
	canvashttp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/http"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvaslive "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/websocket"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialschema"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	cataloghttp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/gltf"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	toolhttp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/http"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
	pgoperation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	prompthttp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/http"
	pgprompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	scripthttp "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/http"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// NewBusinessRouter assembles the public workspace API.
// Its stores are the same injected dependencies used by the API role.
// @title Glowloom API
// @version 0.1
// @description Glowloom local workspace API for projects, canvases, media, scripts, bible, model catalog and operations.
// @BasePath /
func NewBusinessRouter(logger *zap.Logger, ready ReadyCheck, tp trace.TracerProvider, cfg config.Config, database *gorm.DB, storage *objectstorage.Client, sessions identityruntime.SessionDependencies, lifetime ...context.Context) (*gin.Engine, error) {
	if database == nil {
		return nil, fmt.Errorf("public API requires database")
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
	router := NewRouter(logger, ready, tp)
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
	emailAuth, err := identityruntime.NewAuth(database, cfg, time.Now)
	if err != nil {
		return nil, fmt.Errorf("configure email authentication: %w", err)
	}
	if err := identityruntime.ConfigureSessionSecurity(emailAuth, cfg, sessions); err != nil {
		return nil, fmt.Errorf("configure session security: %w", err)
	}
	auth, registerSelf, err := identityruntime.RegisterSelf(database, emailAuth, cfg, storage, api, time.Now)
	if err != nil {
		return nil, fmt.Errorf("configure self account services: %w", err)
	}
	auth.Register(api)
	identityhttp.NewAuthPolicyHandler(identityapp.NewAuthPolicyQuery(pgidentity.NewStore(database), identityhttp.AcceptedLegalVersion(cfg.IdentityTermsVersion), identityhttp.LegalPackageVersion(), cfg.SMTPEnabled && cfg.IdentityMailSourceHourlyLimit > 0 && cfg.IdentityMailGlobalHourlyLimit > 0)).Register(api)
	protected.Use(auth.RequireSession())
	registerSelf(protected)
	identityProtected := protected.Group("")
	workspaceStore := workspacewire.ProvideWorkspaceLifecycleStore(database)
	protected.Use(workspacehttp.RequireCreationScope(scopewire.ProvideRequestScopeAccess(database)))
	protected.Use(workspacehttp.RequireProjectAccess(workspaceStore))
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(workspaceStore), workspaceapp.NewCreateProjectCommand(workspaceStore, time.Now), workspaceapp.NewListStylePresetsQuery(workspaceStore)).Register(protected)
	workspacehttp.NewProjectLifecycleHandler(workspaceapp.NewProjectLifecycle(workspaceStore, time.Now)).Register(protected)
	workspacehttp.NewProjectCreationRecoveryHandler(workspaceapp.NewProjectCreationRecoveryQuery(workspaceStore)).Register(protected)
	workspacehttp.NewProjectCopyHandler(workspaceapp.NewProjectCopyService(copywire.ProvideProjectCopyStore(database), time.Now)).Register(protected)
	mediahttp.NewTransferHandler(mediawire.ProvideMediaTransferStore(database)).Register(protected)
	mediahttp.NewPurgeHandler(mediawire.ProvideMediaPurgeStore(database)).Register(protected)
	mediahttp.NewStorageUsageHandler(mediawire.ProvideMediaStorageUsage(database, storage)).Register(protected)
	workspacehttp.NewProjectFolderHandler(workspaceapp.NewProjectFolders(workspacewire.ProvideProjectFolderStore(database), time.Now)).Register(protected)
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader {
		return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
	}
	canvasStore := pgcanvas.NewScopedStore(database, mediaFactory, mediawire.NewCanvasProjectAccess)
	canvasService := canvasapp.NewService(canvasStore)
	canvashttp.NewHandler(canvasService).RegisterLocation(identityProtected)
	live := canvaslive.NewHandler(canvaswire.SocketAuthority(database, auth), cfg.PublicOrigin)
	if cfg.CanvasRuntimeURL != "" && len(cfg.CanvasRuntimeToken) >= 32 {
		live.WithYjs(canvasapp.NewYjsService(canvasStore, yjsruntime.New(cfg.CanvasRuntimeURL, cfg.CanvasRuntimeToken)))
	}
	socketLifetime := context.Background()
	if len(lifetime) > 0 {
		socketLifetime = lifetime[0]
	}
	socketRoutes := api.Group("")
	socketRoutes.Use(auth.RequireSession())
	live.Register(socketLifetime, socketRoutes)
	canvashttp.NewHandler(canvasService).WithNotification(live.Notify).Register(protected)
	mediahttp.NewHandler(mediaapp.NewAssetQuery(pgmedia.NewScopedStore(database, mediawire.NewMediaScopeAccess), storage)).Register(protected)
	mediahttp.NewDocumentHandler(mediaapp.NewDocumentSources(pgmedia.NewScopedDocumentSourceStore(database, mediawire.NewMediaScopeAccess), storage)).Register(protected)
	library := pgmedia.NewScopedLibraryStore(database, mediawire.NewMediaLibraryProjectAccess, time.Now, mediawire.NewMediaScopeAccess)
	mediahttp.NewLibraryHandler(library).Register(protected)
	mediahttp.NewLibraryMediaHandler(mediaapp.NewLibraryMediaQuery(library, storage, storage)).Register(protected)
	scripts := scriptwire.ProvideServices(database, storage)
	scripthttp.NewSourceHandler(scripts.Sources).Register(protected)
	scripthttp.NewSourceRecoveryHandler(scripts.Recovery).Register(protected)
	scripthttp.NewReviewHandler(scripts.Episodes, scripts.History).Register(protected)
	scripthttp.NewVersionHandler(scripts.History, scripts.Adopt).Register(protected)
	scripthttp.NewImportHandler(scripts.Imports).Register(protected)
	modelValidator, err := gltf.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("configure model upload validation: %w", err)
	}
	uploadStore := pgmedia.NewScopedStore(database, mediawire.NewMediaScopeAccess)
	scopedUpload := mediaapp.NewScopedUploadService(uploadStore, uploadStore, mediaflow.NewUploadProber(modelValidator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(storage), time.Now)
	mediahttp.NewUploadHandler(scopedUpload).Register(protected)
	mediahttp.NewPackageHandler(mediawire.ProvideMediaPackages(database, storage, scopedUpload)).Register(protected)
	exports := mediawire.ProvideMediaExportStore(database)
	toolhttp.NewHandler(exports, toolapp.NewExportQuery(exports, storage, storage)).Register(protected)
	transcriber, err := mediawire.ProvideTranscriber(cfg)
	if err != nil {
		return nil, err
	}
	toolhttp.NewTranscriptionHandler(mediawire.ProvideMediaTranscriptionStore(database, transcriber != nil)).Register(protected)
	depths := mediawire.ProvideMediaDepthStore(database, cfg.VideoDepthPythonPath != "")
	toolhttp.NewDepthHandler(depths, toolapp.NewDepthQuery(depths, storage, storage)).Register(protected)
	if err := trialwire.RegisterAPI(socketLifetime, identityProtected, database, storage, cfg); err != nil {
		return nil, err
	}
	catalogStore := operationwire.NewCatalogStore(database)
	cataloghttp.NewPresetHandler(catalogapp.NewProviderPresets(catalogStore)).Register(identityProtected)
	cataloghttp.NewConfigHandler(catalogapp.NewProviderConfigs(catalogStore)).Register(identityProtected)
	cataloghttp.NewReleaseHandler(catalogapp.NewModelReleases(catalogStore)).Register(identityProtected)
	cataloghttp.NewCheckHandler(catalogapp.NewProviderChecks(catalogStore)).Register(identityProtected)
	cataloghttp.NewCredentialHistoryHandler(catalogapp.NewCredentialHistory(catalogStore)).Register(identityProtected)
	cataloghttp.NewReceiptHandler(catalogapp.NewConfigReceipts(catalogStore)).Register(identityProtected)
	credentialSchema := credentialschema.NewRegistry()
	parameterValidator, err := paramvalidation.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("configure model parameter validation: %w", err)
	}
	biblehttp.NewHandler(bibleapp.NewService(biblewire.ProvideBibleStore(database, storage, parameterValidator), time.Now)).Register(protected)
	priceValidator, err := pricevalidation.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("configure model price validation: %w", err)
	}
	var setCredential *catalogapp.SetCredentialCommand
	if cfg.CredentialPublicKeyFile != "" || cfg.CredentialKeyID != "" {
		file, err := os.Open(cfg.CredentialPublicKeyFile)
		if err != nil {
			return nil, fmt.Errorf("open credential public key: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, (16<<10)+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) > 16<<10 {
			return nil, fmt.Errorf("credential public key unreadable or oversized")
		}
		sealer, err := credentialseal.NewSealer(cfg.CredentialKeyID, data)
		if err != nil {
			return nil, fmt.Errorf("configure credential public key: %w", err)
		}
		macKey, err := hex.DecodeString(cfg.CredentialRequestMACKey)
		if err != nil || len(macKey) != 32 {
			return nil, fmt.Errorf("credential writes require a 32-byte hexadecimal request MAC key")
		}
		setCredential = catalogapp.NewSetCredentialCommand(catalogStore, credentialSchema, sealer, time.Now, macKey)
		clear(macKey)
	}
	cataloghttp.NewHandler(catalogapp.NewListModelsQuery(catalogStore)).Register(protected)
	workspacehttp.NewModelDefaultsHandler(workspaceapp.NewModelDefaultsService(workspaceStore, time.Now)).Register(protected)
	prompthttp.NewHandler(promptapp.NewPreferences(operationwire.ProvidePromptPreferenceStore(database), time.Now)).Register(identityProtected)
	prompthttp.NewDraftHandler(pgprompt.NewDraftStore(database, promptwire.DraftAccess)).Register(protected)
	cataloghttp.NewAdminHandler(cataloghttp.AdminDependencies{
		Providers:         catalogapp.NewListProvidersQuery(catalogStore),
		Provider:          catalogapp.NewProviderDetailQuery(catalogStore, credentialSchema),
		CreateProvider:    catalogapp.NewCreateProviderCommand(catalogStore, credentialSchema, time.Now),
		UpdateProvider:    catalogapp.NewUpdateProviderCommand(catalogStore, time.Now),
		SetCredential:     setCredential,
		DisableCredential: catalogapp.NewDisableCredentialCommand(catalogStore, time.Now),
		CredentialTest:    catalogapp.NewRequestCredentialTestCommand(catalogStore),
		Models:            catalogapp.NewAdminModelsQuery(catalogStore),
		CreateModel:       catalogapp.NewCreateModelCommand(catalogStore, time.Now),
		PublishVersion:    catalogapp.NewPublishModelVersionCommand(catalogStore, parameterValidator, time.Now),
		PublishPrice:      catalogapp.NewPublishPriceRuleCommand(catalogStore, priceValidator, time.Now),
		SetModelStatus:    catalogapp.NewSetModelStatusCommand(catalogStore, time.Now),
	}).Register(identityProtected)
	operations := pgoperation.NewScopedStoreWithEditor(database, func(tx *gorm.DB) operationapp.QuotePromptCompiler {
		return promptapp.NewCompiler(operationwire.ProvidePromptPreferenceStore(tx))
	}, func(tx *gorm.DB) operationapp.EditorSnapshots {
		return pgprompt.NewDraftStore(tx, promptwire.DraftAccess)
	}, operationwire.ProvideOperationProjectAccess, operationwire.ProvideOperationWorkerAccess, operationwire.ProvideOperationPlatformAccess)
	operationhttp.NewSubmissionHandler(operations).Register(protected)
	operationhttp.NewProviderTrialHandler(operations).Register(protected)
	operationhttp.NewHandler(operationhttp.Dependencies{
		FreeQuote:    operationapp.NewCreateFreeQuoteCommand(operations),
		BatchQuote:   operationapp.NewCreateBatchFreeQuoteCommand(operations),
		Confirm:      operationapp.NewConfirmSingleQuoteCommand(operations),
		ConfirmBatch: operationapp.NewConfirmBatchQuoteCommand(operations),
		Query:        operationapp.NewPublicQuery(operations),
		Control:      operationapp.NewWorkflowControlCommand(operations),
	}).Register(protected)
	return router, nil
}
