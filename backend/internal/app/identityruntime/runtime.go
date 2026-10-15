// Package identityruntime composes email identity and owned mail delivery in the existing process.
package identityruntime

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identitymail "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/mail"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// NewAuth composes email authentication against an existing connection.
// Loading this runtime neither changes the schema nor enables registration.
func NewAuth(database *gorm.DB, cfg config.Config, now func() time.Time) (*identityapp.Auth, error) {
	protector, err := credentialproof.LoadFile(cfg.IdentityKeyFile)
	if err != nil {
		return nil, err
	}
	return identityapp.NewEmailAuth(pgidentity.NewInvitationEmailStore(database, func(tx *gorm.DB) identityapp.TeamGovernance { return pgworkspace.NewTeamGovernance(tx) }), protector, identityapp.EmailPolicy{
		TermsVersion: http.AcceptedLegalVersion(cfg.IdentityTermsVersion), SourceHourlyLimit: cfg.IdentityMailSourceHourlyLimit, GlobalHourlyLimit: cfg.IdentityMailGlobalHourlyLimit,
	}, now), nil
}

// NewMailDelivery uses the same private key and existing SMTP channel.
func NewMailDelivery(store identityapp.MailDeliveryStore, cfg config.Config, now func() time.Time) (*identityapp.MailDelivery, error) {
	if !cfg.SMTPEnabled {
		return nil, errors.New("identity SMTP is disabled")
	}
	protector, err := credentialproof.LoadFile(cfg.IdentityKeyFile)
	if err != nil {
		return nil, err
	}
	sender, err := identitymail.NewSender(identitymail.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, TLSMode: cfg.SMTPTLSMode, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, FromEmail: cfg.SMTPFromEmail, FromName: cfg.SMTPFromName, PublicOrigin: cfg.PublicOrigin})
	if err != nil {
		return nil, err
	}
	return identityapp.NewMailDelivery(store, protector, sender, now), nil
}

// RegisterSelf installs public invitation preview and composes authenticated account services.
func RegisterSelf(database *gorm.DB, auth *identityapp.Auth, cfg config.Config, storage *objectstorage.Client, public *gin.RouterGroup, now func() time.Time) (*http.EmailAuthHandler, func(*gin.RouterGroup), error) {
	protector, err := credentialproof.LoadFile(cfg.IdentityKeyFile)
	if err != nil {
		return nil, nil, err
	}
	sessions := http.NewSessionDirectoryHandler(identityapp.NewSessionDirectory(pgidentity.NewSessionDirectoryStore(database), now), strings.HasPrefix(cfg.PublicOrigin, "https://"))
	handler := http.NewEmailAuthHandler(auth, cfg.PublicOrigin, strings.HasPrefix(cfg.PublicOrigin, "https://")).WithSessionRevocation(sessions.Revoke)
	governanceStore := pgidentity.NewGovernanceStore(database, func(tx *gorm.DB) identityapp.TeamGovernance { return pgworkspace.NewTeamGovernance(tx) })
	governance := identityapp.NewGovernance(governanceStore, protector, now)
	invitations := http.NewInvitationHandler(governance)
	invitations.RegisterPublic(public, cfg.PublicOrigin)
	var previewSigner mediaapp.PreviewSigner
	var avatarObjects mediaapp.UploadObjects
	if storage != nil {
		previewSigner = storage
		avatarObjects = mediaobjects.NewAvatarObjects(storage)
	}
	return handler, func(group *gin.RouterGroup) {
		http.NewSelfProfileHandler(identityapp.NewSelfProfile(pgidentity.NewSelfProfileStore(database, func(tx *gorm.DB) identityapp.AvatarAccess { return pgmedia.NewAvatarAccess(tx) }), now)).Register(group)
		http.NewEmailChangeHandler(identityapp.NewEmailChange(auth, pgidentity.NewEmailChangeStore(database), now), strings.HasPrefix(cfg.PublicOrigin, "https://")).Register(group)
		http.NewPlatformPasswordResetHandler(identityapp.NewPlatformPasswordReset(governanceStore, auth, now)).Register(group)
		avatars := pgmedia.NewAvatarStore(database, func(tx *gorm.DB) mediaapp.AvatarActorAccess { return pgidentity.NewStore(tx) })
		mediahttp.NewAvatarHandler(mediaapp.NewAvatarService(avatars, avatarObjects, now), mediaapp.NewAvatarQuery(avatars, previewSigner, now)).Register(group)
		sessions.Register(group)
		http.NewPlatformDirectoryHandler(identityapp.NewPlatformDirectory(pgidentity.NewPlatformDirectoryStore(database), protector, now)).Register(group)
		var exportObjects identityapp.ExportObjects
		if storage != nil {
			exportObjects = storage
		}
		http.NewDirectoryExportHandler(identityapp.NewDirectoryExport(pgidentity.NewDirectoryExportStore(database), protector, exportObjects, now), identityapp.NewDirectoryStatistics(pgidentity.NewPlatformDirectoryStore(database), now)).Register(group)
		http.NewGovernanceHandler(governance).Register(group)
		invitations.Register(group)
		teams := pgworkspace.NewTeamProfileStore(database, func(tx *gorm.DB) workspaceapp.TeamActorAccess { return pgidentity.NewTeamActorStore(tx, protector) }, func(tx *gorm.DB) workspaceapp.TeamAvatarAccess { return pgmedia.NewTeamAvatarAccess(tx) })
		teamService := workspaceapp.NewTeams(teams, now)
		workspacehttp.NewTeamHandler(teamService, workspaceapp.NewTeamDirectory(teams, protector, now)).Register(group)
		workspacehttp.NewTeamLifecycleHandler(teamService).Register(group)
		workspacehttp.NewPlatformTeamHandler(workspaceapp.NewPlatformTeams(teams, protector, now)).Register(group)
		workspacehttp.NewTeamAvatarHandler(workspaceapp.NewTeamAvatars(teams, previewSigner, now)).Register(group)
		http.NewEventHandler(identityapp.NewEventQuery(pgidentity.NewEventStore(database), protector, now)).Register(group)
		http.NewAccountDeletionHandler(identityapp.NewAccountDeletion(auth, pgidentity.NewAccountDeletionStore(database), now), strings.HasPrefix(cfg.PublicOrigin, "https://")).Register(group)
	}, nil
}

// StartMailLoop owns one cancellable mail runner in the existing API process.
// The caller must stop and join it before closing the injected database.
func StartMailLoop(ctx context.Context, database *gorm.DB, cfg config.Config, logger *zap.Logger) (func(), error) {
	if !cfg.SMTPEnabled {
		return func() {}, nil
	}
	delivery, err := NewMailDelivery(pgidentity.NewMailStore(database), cfg, time.Now)
	if err != nil {
		return nil, err
	}
	return StartMailRunner(ctx, delivery, logger, time.Second), nil
}

// BatchRunner executes one bounded identity work batch.
type BatchRunner interface {
	RunOnce(context.Context, int) (int, error)
}

// StartMailRunner owns delivery cancellation and joins the active batch on cleanup.
func StartMailRunner(ctx context.Context, delivery BatchRunner, logger *zap.Logger, interval time.Duration) func() {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if runCtx.Err() != nil {
					return
				}
				if _, err := delivery.RunOnce(runCtx, 20); err != nil && runCtx.Err() == nil {
					logger.Error("identity mail delivery attempt failed")
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}

// StartDirectoryExportLoop owns bounded export work in the existing API process.
// It is independent of SMTP; the caller cancels and joins before closing the DB.
func StartDirectoryExportLoop(ctx context.Context, database *gorm.DB, cfg config.Config, storage *objectstorage.Client, logger *zap.Logger) (func(), error) {
	if storage == nil {
		return func() {}, nil
	}
	protector, err := credentialproof.LoadFile(cfg.IdentityKeyFile)
	if err != nil {
		return nil, err
	}
	service := identityapp.NewDirectoryExport(pgidentity.NewDirectoryExportStore(database), protector, storage, time.Now)
	return StartExportRunner(ctx, service, logger, 5*time.Second), nil
}

// StartExportRunner owns export cancellation and joins the active batch on cleanup.
func StartExportRunner(ctx context.Context, runner BatchRunner, logger *zap.Logger, interval time.Duration) func() {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if runCtx.Err() != nil {
					return
				}
				if _, err := runner.RunOnce(runCtx, 2); err != nil && runCtx.Err() == nil {
					logger.Error("identity directory export batch failed")
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}
