// Command notificationbrowser serves an isolated browser protocol fixture using
// the actual notification, authentication, profile and workspace HTTP adapters.
// It requires an already initialized disposable loopback PostgreSQL database.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/adapter/credentialproof"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/identity/domain"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

const fixturePassword = "long password for tests"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := os.Getenv("GLOWLOOM_TEST_NOTIFICATION_DB_DSN")
	u, err := url.Parse(dsn)
	if err != nil || !loopback(u.Hostname()) || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "notifications") {
		return errors.New("GLOWLOOM_TEST_NOTIFICATION_DB_DSN must name a disposable notifications* database on loopback")
	}
	origin := os.Getenv("GLOWLOOM_NOTIFICATION_BROWSER_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}
	public, err := url.Parse(origin)
	if err != nil || !loopback(public.Hostname()) || public.Scheme != "http" {
		return errors.New("browser fixture origin must be loopback HTTP")
	}
	address := os.Getenv("GLOWLOOM_NOTIFICATION_BROWSER_ADDR")
	if address == "" {
		address = "127.0.0.1:55469"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || !loopback(host) {
		return errors.New("browser fixture listener must be loopback")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	conn, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		return fmt.Errorf("open notification browser database: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("close notification fixture: %v", err)
		}
	}()
	protector, err := credentialproof.New("notification-browser-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		return err
	}
	store := pg.NewStore(conn.DB)
	auth := identity.NewEmailAuth(store, protector, identity.EmailPolicy{TermsVersion: "test", SourceHourlyLimit: 100, GlobalHourlyLimit: 1000}, time.Now)
	for _, account := range []struct{ email, role string }{{"notification-admin@example.invalid", "platform_admin"}, {"notification-user@example.invalid", "creator"}} {
		if err = seedAccount(ctx, conn.DB, store, account.email, account.role); err != nil {
			return err
		}
	}
	login, err := auth.Login(ctx, "notification-admin@example.invalid", fixturePassword, false)
	if err != nil {
		return err
	}
	actor := identity.Principal{ID: login.User.ID, SessionID: login.Session.ID, CredentialEpoch: login.User.SessionEpoch, PlatformRole: login.User.PlatformRole, PlatformEpoch: login.User.PlatformEpoch}
	service := identity.NewNotifications(pg.NewNotificationStore(conn.DB), time.Now)
	if err = seedAnnouncements(ctx, conn.DB, service, actor); err != nil {
		return err
	}
	if err = seedTaskNotifications(ctx, conn.DB, service); err != nil {
		return err
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if err = router.SetTrustedProxies(nil); err != nil {
		return err
	}
	router.Use(httpapi.Middleware(origin))
	router.NoRoute(func(c *gin.Context) { httpapi.WriteProblem(c, 404, "not_found", nil) })
	router.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "fixture": "notifications"}) })
	api := router.Group("/api", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	authHTTP := identityhttp.NewEmailAuthHandler(auth, origin, false)
	authHTTP.Register(api)
	identityhttp.NewAuthPolicyHandler(identity.NewAuthPolicyQuery(store, "", identityhttp.LegalPackageVersion(), false)).Register(api)
	notifications := identityhttp.NewNotificationHandler(service)
	notifications.RegisterPublic(api)
	protected := api.Group("", authHTTP.RequireSession())
	notifications.Register(protected)
	identityhttp.NewSelfProfileHandler(identity.NewSelfProfile(pg.NewSelfProfileStore(conn.DB, nil), time.Now)).RegisterReadAndPreferences(protected)
	teams := pgworkspace.NewTeamStore(conn.DB, func(tx *gorm.DB) workspace.TeamActorAccess { return pg.NewTeamActorStore(tx, protector) })
	workspacehttp.NewTeamHandler(workspace.NewTeams(teams, time.Now), workspace.NewTeamDirectory(teams, protector, time.Now)).Register(protected)
	server := &http.Server{Addr: address, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	log.Printf("isolated notification browser fixture listening at http://%s for %s", address, origin)
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func loopback(host string) bool {
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func seedAccount(ctx context.Context, db *gorm.DB, store *pg.Store, email, role string) error {
	var count int64
	if err := db.WithContext(ctx).Table(`identity."user"`).Where("email_key=?", email).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		now := time.Now().UTC()
		hash, err := domain.HashPassword(fixturePassword, "")
		if err != nil {
			return err
		}
		if err = store.EmailTransaction(ctx, func(tx identity.EmailTx) error {
			return tx.CreatePersonalAccount(domain.User{ID: uuid.New(), OrgID: uuid.New(), LoginName: uuid.NewString(), Email: email, EmailVersion: 1, EmailVerifiedAt: now, DisplayName: "通知验收账号", PasswordHash: hash, PasswordChangedAt: now, TermsVersion: "test", TermsAcceptedAt: now})
		}); err != nil {
			return err
		}
	}
	return db.WithContext(ctx).Exec(`UPDATE identity."user" SET platform_role=? WHERE email_key=?`, role, email).Error
}

func seedAnnouncements(ctx context.Context, db *gorm.DB, service *identity.Notifications, actor identity.Principal) error {
	var count int64
	if err := db.WithContext(ctx).Table("identity.announcement").Where("created_by=?", actor.ID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, sample := range []struct{ title, body, placement, audience string }{{"服务维护通知", "服务将在今晚进行维护，可在通知中心查看详情。", "banner", "public"}, {"通知中心已更新", "现在可以查看历史通知，并一键标记已读。", "modal", "authenticated"}, {"新版本通知", "通知管理支持受众、时间和展示位置筛选。", "inbox", "authenticated"}} {
		a, err := service.Save(ctx, actor, uuid.Nil, identity.SaveAnnouncementInput{CommandID: uuid.New(), Content: identity.AnnouncementContent{Title: sample.title, Body: sample.body, Placement: sample.placement, Audience: sample.audience, StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour), DurationSeconds: 5, ActionPath: "/notifications", Inbox: true}})
		if err != nil {
			return err
		}
		if _, err = service.Command(ctx, actor, a.ID, "publish", identity.AnnouncementCommand{CommandID: uuid.New(), ExpectedVersion: a.Version}); err != nil {
			return err
		}
	}
	return nil
}

// Browser samples use the real trusted projection use case. Actual source
// Outbox/Kafka transport is exercised separately by the Operation integration test.
func seedTaskNotifications(ctx context.Context, db *gorm.DB, service *identity.Notifications) error {
	for _, sample := range []struct{ email, status string }{{"notification-admin@example.invalid", "completed"}, {"notification-user@example.invalid", "failed"}} {
		var user struct{ ID uuid.UUID }
		if err := db.WithContext(ctx).Raw(`SELECT id FROM identity."user" WHERE email_key=?`, sample.email).Scan(&user).Error; err != nil {
			return err
		}
		event := uuid.NewSHA1(user.ID, []byte("notification-browser-task-event"))
		operation := uuid.NewSHA1(user.ID, []byte("notification-browser-task-operation"))
		if err := service.ProjectTask(ctx, identity.TaskNotificationInput{EventID: event, RecipientID: user.ID, OperationID: operation, Status: sample.status, OccurredAt: time.Now().UTC()}); err != nil {
			return err
		}
	}
	return nil
}
