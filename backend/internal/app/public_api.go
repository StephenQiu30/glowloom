package app

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	// Generated docs register the public Swagger schema with swag at initialization.
	_ "github.com/StephenQiu30/lanverse/backend/docs"
	canvashttp "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/http"
	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// NewBusinessRouter assembles the workspace project query and canvas slice.
// Its stores are the same injected dependencies used by the API role.
// @title Lanverse API
// @version 0.1
// @description Single workspace project query, resource canvas commands and authorized media previews.
// @BasePath /
func NewBusinessRouter(logger *zap.Logger, ready ReadyCheck, tp trace.TracerProvider, cfg config.Config, database *gorm.DB, storage *objectstorage.Client) (*gin.Engine, error) {
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
	api := router.Group("/api")
	protected := api.Group("")
	protected.Use(identityhttp.Workspace(pgidentity.NewStore(database)))
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(pgworkspace.NewStore(database)), workspaceapp.NewCreateProjectCommand(pgworkspace.NewStore(database), time.Now), workspaceapp.NewListStylePresetsQuery(pgworkspace.NewStore(database))).Register(protected)
	mediaFactory := func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(pgmedia.NewStore(tx), nil) }
	canvashttp.NewHandler(canvasapp.NewService(pgcanvas.NewStore(database, mediaFactory))).Register(protected)
	mediahttp.NewHandler(mediaapp.NewAssetQuery(pgmedia.NewStore(database), storage)).Register(protected)
	router.GET("/swagger/doc.json", func(c *gin.Context) {
		body, err := swag.ReadDoc()
		if err != nil {
			httpapi.WriteProblem(c, 503, "dependency_unavailable", nil)
			return
		}
		c.Data(200, "application/json", []byte(body))
	})
	return router, nil
}
