package agent

import (
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// ConfigureModelGateway binds current catalog credentials, worker authority and
// billing without making any provider request. Missing configured keys leave
// only the Agent model capability unavailable; other business APIs keep working.
func ConfigureModelGateway(database *gorm.DB, cfg config.Config) (app.ModelCapability, app.RecoverCapability, error) {
	if cfg.CredentialPrivateKeyPEM == "" && cfg.CredentialPrivateKeyFile == "" {
		return nil, nil, nil
	}
	private, err := cfg.CredentialPrivateBytes()
	if err != nil {
		return nil, nil, err
	}
	defer clear(private)
	keys, err := credentialseal.NewUnsealer(cfg.CredentialKeyID, private)
	if err != nil {
		return nil, nil, err
	}
	budget := billingpg.NewScopedStore(database, func(tx *gorm.DB) billing.ProjectAccess { return billingAccess{tx} })
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = 16
	transport.ResponseHeaderTimeout = 45 * time.Second
	gateway := modelgateway.New(database, operationStore(database), budget, keys, &http.Client{Timeout: 150 * time.Second, Transport: transport})
	return gateway.Model, gateway.Recover, nil
}
