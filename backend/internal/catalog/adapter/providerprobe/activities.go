// Package providerprobe runs read-only checks in the credential-owning worker.
package providerprobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/workflow"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
)

// Activities holds worker-only dependencies; no API handler receives a private key.
type Activities struct {
	db       *gorm.DB
	unsealer *credentialseal.Unsealer
	client   *providerhttp.Client
}

// NewActivities injects storage, private-key access and bounded HTTP transport.
func NewActivities(db *gorm.DB, unsealer *credentialseal.Unsealer, client *providerhttp.Client) *Activities {
	return &Activities{db: db, unsealer: unsealer, client: client}
}

// TestCredential rechecks current authority and the exact saved configuration before probing.
func (a *Activities) TestCredential(ctx context.Context, input catalogflow.AgentTestInput) (catalogflow.AgentTestOutput, error) {
	unsupported := catalogflow.AgentTestOutput{Result: catalogdomain.TestUnsupported, AuthResult: "unknown", AdapterResult: "unsupported", DiscoveryResult: "unsupported"}
	var row struct{ Config []byte }
	// This is dispatch admission: current rights, exact check/config/credential and
	// emergency controls are checked again immediately before using the key.
	r := a.db.WithContext(ctx).Raw(`SELECT config.config FROM catalog.provider_check check_run
 JOIN identity."user" actor ON actor.id=? AND actor.id=check_run.actor_id AND actor.platform_role='platform_admin' AND actor.status='active' AND NOT actor.is_delete AND NOT actor.must_change_password
 JOIN catalog.provider p ON p.id=check_run.provider_id AND NOT p.is_delete
 JOIN catalog.provider_config_version config ON config.id=check_run.config_version_id
 JOIN catalog.provider_preset_version pv ON pv.id=config.preset_version_id
 JOIN catalog.provider_preset preset ON preset.id=pv.preset_id AND NOT preset.emergency_disabled
 JOIN catalog.provider_credential credential ON credential.id=check_run.credential_id AND credential.status='active' AND NOT credential.is_delete
 WHERE check_run.id=? AND check_run.provider_id=? AND check_run.credential_id=? AND check_run.status='running'
 AND p.current_config_version_id=check_run.config_version_id AND p.adapter_key=?`, input.ActorID, input.TestID, input.ProviderID, input.Credential.ID, input.AdapterKey).Scan(&row)
	if r.Error != nil {
		return unsupported, fmt.Errorf("authorize provider probe: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return unsupported, nil
	}
	var configuration catalogdomain.ProviderConfig
	if json.Unmarshal(row.Config, &configuration) != nil || configuration.Validate() != nil {
		return unsupported, nil
	}
	envelope, err := base64.StdEncoding.DecodeString(input.Credential.Ciphertext)
	if err != nil {
		return unsupported, nil
	}
	secret, err := a.unsealer.Open(input.Credential.KeyID, input.ProviderID, input.Credential.ID, envelope)
	if err != nil {
		return unsupported, nil
	}
	defer clear(secret)
	var credential struct {
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(secret, &credential) != nil {
		return unsupported, nil
	}
	result := a.client.Probe(ctx, configuration, credential.APIKey)
	category := catalogdomain.TestUnreachable
	switch result.Auth {
	case "ok":
		category = catalogdomain.TestOK
	case "auth_failed":
		category = catalogdomain.TestAuthFailed
	}
	return catalogflow.AgentTestOutput{Result: category, AuthResult: result.Auth, AdapterResult: result.Adapter, DiscoveryResult: result.Discovery, Candidates: result.Candidates}, nil
}
