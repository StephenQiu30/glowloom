// Package providerprobe runs read-only checks in the credential-owning worker.
package providerprobe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	catalogflow "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/workflow"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
)

// AcceptedTestAccess checks the exact accepted receipt and current authority.
type AcceptedTestAccess interface {
	VerifyCredentialTestRequest(context.Context, catalogapp.CredentialTestRequest, uuid.UUID) error
}

// Activities holds worker-only dependencies; no API handler receives a private key.
type Activities struct {
	access   AcceptedTestAccess
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
	if a.access == nil {
		return unsupported, catalogapp.ErrInvalidCredentialTest
	}
	request := catalogapp.CredentialTestRequest{TestID: input.TestID, ActorID: input.ActorID, OrgID: input.OrgID, ProviderID: input.ProviderID, CredentialID: input.Credential.ID, RequestID: input.RequestID}
	authorize := func() error {
		return a.access.VerifyCredentialTestRequest(ctx, request, uuid.NewSHA1(input.TestID, []byte("credential_test_requested")))
	}
	if err := authorize(); err != nil {
		return unsupported, err
	}
	var row struct{ Config []byte }
	// This is dispatch admission: current rights, exact check/config/credential and
	// emergency controls are checked again immediately before using the key.
	r := a.db.WithContext(ctx).Raw(`SELECT config.config FROM catalog.provider_check check_run
 JOIN identity."user" actor ON actor.id=? AND actor.id=check_run.actor_id AND actor.platform_role='platform_admin' AND actor.status='active' AND NOT actor.is_delete
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
	if err := authorize(); err != nil {
		return unsupported, err
	}
	var selected struct {
		ModelID                     *uuid.UUID
		ModelVersionID              *uuid.UUID
		ProviderModelID, Capability string
	}
	r = a.db.WithContext(ctx).Raw(`SELECT check_run.model_id,check_run.model_version_id,v.provider_model_id,m.capability
 FROM catalog.provider_check check_run JOIN catalog.model_profile m ON m.id=check_run.model_id
 JOIN catalog.model_profile_version v ON v.id=check_run.model_version_id AND v.id=m.current_version_id
 WHERE check_run.id=? AND check_run.kind='model_access' AND NOT m.is_delete AND NOT v.is_delete`, input.TestID).Scan(&selected)
	if r.Error != nil {
		return unsupported, r.Error
	}
	if selected.ModelID != nil {
		claimed := a.db.WithContext(ctx).Exec(`UPDATE catalog.provider_check check_run SET dispatch_started_at=now()
 FROM catalog.provider p, catalog.provider_credential key, catalog.model_profile m
 WHERE check_run.id=? AND check_run.status='running' AND check_run.dispatch_started_at IS NULL
 AND p.id=check_run.provider_id AND p.current_config_version_id=check_run.config_version_id AND NOT p.is_delete
 AND key.id=check_run.credential_id AND key.status='active' AND NOT key.is_delete
 AND m.id=check_run.model_id AND m.current_version_id=check_run.model_version_id AND NOT m.is_delete`, input.TestID)
		if claimed.Error != nil {
			return unsupported, claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return catalogflow.AgentTestOutput{Result: catalogdomain.TestUnreachable, AuthResult: "unknown", AdapterResult: "ok", DiscoveryResult: "unsupported", ModelResult: "unknown"}, nil
		}
		probe := a.client.ProbeModel(ctx, configuration, selected.ProviderModelID, selected.Capability, credential.APIKey)
		result := catalogflow.AgentTestOutput{Result: catalogdomain.TestUnreachable, AuthResult: "unknown", AdapterResult: "ok", DiscoveryResult: "unsupported", ModelResult: probe.Result, TaskID: probe.TaskID}
		switch probe.Result {
		case "completed", "accepted":
			result.Result, result.AuthResult = catalogdomain.TestOK, "ok"
		case "rejected":
			result.AuthResult = probe.ErrorCode
			if probe.ErrorCode == "auth_failed" {
				result.Result = catalogdomain.TestAuthFailed
			}
			if probe.ErrorCode == "invalid_provider_request" {
				result.AuthResult = "invalid_parameters"
			}
		}
		return result, nil
	}
	// A stale model selection must never fall back to a weaker metadata probe.
	var isModel bool
	if err := a.db.WithContext(ctx).Raw(`SELECT kind='model_access' FROM catalog.provider_check WHERE id=?`, input.TestID).Scan(&isModel).Error; err != nil {
		return unsupported, err
	}
	if isModel {
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

// NewScopedActivities injects receipt/current-epoch authority at the actual key owner.
func NewScopedActivities(db *gorm.DB, unsealer *credentialseal.Unsealer, client *providerhttp.Client, access AcceptedTestAccess) *Activities {
	return &Activities{db: db, unsealer: unsealer, client: client, access: access}
}
