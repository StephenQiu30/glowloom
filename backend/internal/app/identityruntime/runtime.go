package identityruntime

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	identitymail "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/mail"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// NewIdentityAuth composes email authentication against an existing connection.
// Loading this runtime neither changes the schema nor enables registration.
func NewAuth(database *gorm.DB, cfg config.Config, now func() time.Time) (*identityapp.Auth, error) {
	protector, err := credentialproof.LoadFile(cfg.IdentityKeyFile)
	if err != nil {
		return nil, err
	}
	return identityapp.NewEmailAuth(pgidentity.NewStore(database), protector, identityapp.EmailPolicy{
		TermsVersion: cfg.IdentityTermsVersion, SourceHourlyLimit: cfg.IdentityMailSourceHourlyLimit, GlobalHourlyLimit: cfg.IdentityMailGlobalHourlyLimit,
	}, now), nil
}

// NewIdentityMailDelivery uses the same private key and existing SMTP channel.
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
