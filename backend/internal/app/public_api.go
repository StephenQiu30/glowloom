package app

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/swaggo/swag"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	_ "github.com/StephenQiu30/lanverse/backend/docs"
	canvashttp "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/http"
	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	redisidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/redis"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// NewBusinessRouter assembles the public identity, project query and canvas slice.
// Its stores are the same injected dependencies used by the API role.
// @title Lanverse API
// @version 0.1
// @description Public identity, project query and note canvas commands.
// @BasePath /
func NewBusinessRouter(logger *zap.Logger, ready ReadyCheck, tp trace.TracerProvider, cfg config.Config, database *gorm.DB, redisConn *redisclient.Client) (*gin.Engine, error) {
	if database == nil || redisConn == nil {
		return nil, fmt.Errorf("public API requires database and Redis")
	}
	if cfg.PublicOrigin == "" && cfg.Env == "local" {
		cfg.PublicOrigin = "http://localhost:3000"
	}
	if err := config.ValidatePublicOrigin(cfg.PublicOrigin, cfg.Env); err != nil {
		return nil, err
	}
	if cfg.SessionIdleTTL == 0 {
		cfg.SessionIdleTTL = 12 * time.Hour
	}
	if cfg.SessionAbsoluteTTL == 0 {
		cfg.SessionAbsoluteTTL = 7 * 24 * time.Hour
	}
	sessions, err := redisidentity.NewSessionStore(redisConn, cfg.SessionIdleTTL, cfg.SessionAbsoluteTTL, time.Now)
	if err != nil {
		return nil, err
	}
	limiter, err := redisidentity.NewLoginLimiter(redisConn)
	if err != nil {
		return nil, err
	}
	accounts := pgidentity.NewStore(database)
	auth := identityapp.NewAuthenticator(accounts, sessions)
	identity := identityhttp.NewHandler(identityapp.NewLoginCommand(accounts, sessions, limiter, time.Now), identityapp.NewLogoutCommand(auth, sessions, accounts, time.Now), identityapp.NewChangePasswordCommand(auth, accounts, sessions, accounts, time.Now), auth, accounts, cfg.Env != "local", cfg.SessionAbsoluteTTL)
	router := NewRouter(logger, ready, tp)
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	router.Use(httpapi.Middleware(cfg.PublicOrigin))
	router.HandleMethodNotAllowed = true
	router.NoRoute(func(c *gin.Context) { httpapi.WriteProblem(c, 404, "not_found", nil) })
	router.NoMethod(func(c *gin.Context) { httpapi.WriteProblem(c, 405, "method_not_allowed", nil) })
	api := router.Group("/api")
	identity.Register(api)
	protected := api.Group("")
	protected.Use(identity.RequireSession())
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(pgworkspace.NewStore(database))).Register(protected)
	canvashttp.NewHandler(canvasapp.NewService(pgcanvas.NewStore(database))).Register(protected)
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
