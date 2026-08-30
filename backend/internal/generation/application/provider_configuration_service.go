package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const (
	createProviderConnectionOperation   = "generation.provider_connection.create"
	rotateProviderCredentialOperation   = "generation.provider_credential.rotate"
	setProviderConnectionStateOperation = "generation.provider_connection.set_state"
	createProviderProfileOperation      = "generation.provider_profile.create"
	setProviderProfileStateOperation    = "generation.provider_profile.set_state"
	publishProjectBindingOperation      = "generation.project_provider_binding.publish"
)

var (
	ErrProviderConnectionNotFound     = errors.New("Media Provider connection not found")
	ErrProviderCredentialNotFound     = errors.New("Media Provider credential not found")
	ErrProviderProfileNotFound        = errors.New("Media Provider model profile not found")
	ErrProjectProviderBindingNotFound = errors.New("Project Media Provider binding not found")
	providerConfigurationKeyPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,79}$`)
)

type ProviderConfigurationSecretStore interface {
	Available() bool
	MatchesKeyID(string) bool
	Fingerprint(context.Context, []byte) (string, error)
	Encrypt(context.Context, domain.ProviderSecretContext, []byte) (domain.EncryptedProviderSecret, error)
}

type ProviderConfigurationRepository interface {
	AuthorizeWorkspaceOwner(context.Context, Actor, string) error
	AuthorizeProviderProject(context.Context, Actor, string) (ProviderProjectScope, error)
	AuthorizeExecutableProviderProject(context.Context, Actor, string) (ProviderProjectScope, error)
	LockProviderWorkspace(context.Context, string) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	LatestProviderConnectionForUpdate(context.Context, string, string) (domain.ProviderConnectionVersion, error)
	FindProviderConnection(context.Context, string) (domain.ProviderConnectionVersion, error)
	CreateProviderCredential(context.Context, domain.ProviderCredentialVersion) (domain.ProviderCredentialVersion, error)
	FindProviderCredential(context.Context, string) (domain.ProviderCredentialVersion, error)
	LatestProviderCredentialForUpdate(context.Context, string, string) (domain.ProviderCredentialVersion, error)
	CreateProviderConnection(context.Context, domain.ProviderConnectionVersion) (domain.ProviderConnectionVersion, error)
	ListLatestProviderConnections(context.Context, string) ([]domain.ProviderConnectionVersion, error)
	LatestProviderModelProfileForUpdate(context.Context, string, string) (domain.ProviderModelProfileVersion, error)
	FindProviderModelProfile(context.Context, string) (domain.ProviderModelProfileVersion, error)
	CreateProviderModelProfile(context.Context, domain.ProviderModelProfileVersion) (domain.ProviderModelProfileVersion, error)
	ListLatestProviderModelProfiles(context.Context, string) ([]domain.ProviderModelProfileVersion, error)
	LatestProjectProviderBindingForUpdate(context.Context, string, string, string) (domain.ProjectProviderBindingVersion, error)
	FindProjectProviderBinding(context.Context, string) (domain.ProjectProviderBindingVersion, error)
	CreateProjectProviderBinding(context.Context, domain.ProjectProviderBindingVersion) (domain.ProjectProviderBindingVersion, error)
	ListLatestProjectProviderBindings(context.Context, string, string) ([]domain.ProjectProviderBindingVersion, error)
}

type ProviderConfigurationTransactionManager interface {
	WithinProviderConfigurationTransaction(context.Context, func(ProviderConfigurationRepository) error) error
}

type ProviderConfigurationConfig struct {
	Now   func() time.Time
	NewID func() string
}

type ProviderConfigurationService struct {
	transactions ProviderConfigurationTransactionManager
	catalog      *MediaPresetCatalog
	secrets      ProviderConfigurationSecretStore
	config       ProviderConfigurationConfig
}

type CreateProviderConnectionCommand struct {
	WorkspaceID, ConnectionKey, PresetKey, DisplayName string
	PresetVersion, ExpectedRevision                    int64
	ConfigOverrides                                    map[string]any
	Credentials                                        map[string]string
	IdempotencyKey                                     string
}

type CreateProviderModelProfileCommand struct {
	WorkspaceID, ProfileKey, ConnectionKey, PresetKey string
	PresetVersion, ExpectedRevision                   int64
	DefaultOverrides                                  map[string]any
	ExternalModelID                                   string
	IdempotencyKey                                    string
}

type RotateProviderCredentialCommand struct {
	WorkspaceID, ConnectionKey string
	ExpectedRevision           int64
	ExpectedContentHash        string
	Credentials                map[string]string
	IdempotencyKey             string
}

type SetProviderConnectionStateCommand struct {
	WorkspaceID, ConnectionKey, State string
	ExpectedRevision                  int64
	ExpectedContentHash               string
	IdempotencyKey                    string
}

type SetProviderModelProfileStateCommand struct {
	WorkspaceID, ProfileKey, State string
	ExpectedRevision               int64
	ExpectedContentHash            string
	IdempotencyKey                 string
}

type PublishProjectProviderBindingCommand struct {
	WorkspaceID, ProjectID, Purpose string
	ConnectionVersionID             string
	ModelProfileVersionID           string
	ExpectedRevision                int64
	ExpectedContentHash             string
	IdempotencyKey                  string
}

type ProviderCredentialView struct {
	ID, WorkspaceID, ConnectionKey, ProviderKey string
	Revision                                    int64
	Fingerprint                                 string
	CreatedBy                                   string
	CreatedAt                                   time.Time
}

type ProviderConnectionResult struct {
	Connection domain.ProviderConnectionVersion
	Credential ProviderCredentialView
	Receipt    platformcommand.Receipt
}

type ProviderModelProfileResult struct {
	Profile domain.ProviderModelProfileVersion
	Receipt platformcommand.Receipt
}

type ProjectProviderBindingResult struct {
	Binding domain.ProjectProviderBindingVersion
	Receipt platformcommand.Receipt
}

type ResolvedProjectProviderBinding struct {
	Binding    domain.ProjectProviderBindingVersion
	Connection domain.ProviderConnectionVersion
	Credential ProviderCredentialView
	Profile    domain.ProviderModelProfileVersion
}

type ProviderConfigurationList struct {
	Connections []ProviderConnectionResult
	Profiles    []domain.ProviderModelProfileVersion
}

type providerConfigurationReceipt struct {
	ResourceID string `json:"resource_id"`
}

func NewProviderConfigurationService(
	transactions ProviderConfigurationTransactionManager,
	catalog *MediaPresetCatalog,
	secrets ProviderConfigurationSecretStore,
	config ProviderConfigurationConfig,
) *ProviderConfigurationService {
	return &ProviderConfigurationService{transactions: transactions, catalog: catalog, secrets: secrets, config: config}
}

func (service *ProviderConfigurationService) Catalog() MediaPresetCatalogView {
	if service == nil || service.catalog == nil {
		return MediaPresetCatalogView{Connections: []MediaPresetSummary{}, Models: []MediaPresetSummary{}}
	}
	return service.catalog.List()
}

func (service *ProviderConfigurationService) CreateConnection(
	ctx context.Context,
	actor Actor,
	command CreateProviderConnectionCommand,
) (ProviderConnectionResult, error) {
	normalizeConnectionCommand(&command)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(command.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(command.ConnectionKey) ||
		!mediaPresetKeyPattern.MatchString(command.PresetKey) || command.PresetVersion < 1 ||
		command.ExpectedRevision != 0 || command.DisplayName == "" || len(command.DisplayName) > 120 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProviderConnectionResult{}, invalid("Invalid Media Provider connection command")
	}
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	var result ProviderConnectionResult
	err := service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if authorizeErr := repo.AuthorizeWorkspaceOwner(ctx, actor, command.WorkspaceID); authorizeErr != nil {
			return authorizeErr
		}
		hashCommand := command
		hashCommand.Credentials = nil
		inputHash, credentialFingerprint, hashErr := service.providerCredentialCommandInputHash(
			ctx,
			actor.UserID,
			hashCommand,
			command.Credentials,
		)
		if hashErr != nil {
			return hashErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, createProviderConnectionOperation, command.IdempotencyKey); findErr == nil {
			return service.replayConnection(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		preset, findErr := service.catalog.ResolveConnection(command.PresetKey, command.PresetVersion)
		if findErr != nil {
			return invalid("Media Provider connection preset is unavailable")
		}
		resolvedConfig, resolveErr := resolvePresetValues(
			preset.FixedConfig,
			preset.EditableFields,
			command.ConfigOverrides,
		)
		if resolveErr != nil {
			return invalid("Media Provider connection fields are invalid")
		}
		credentialPayload, resolveErr := resolveCredentialPayload(preset.CredentialFields, command.Credentials)
		if resolveErr != nil {
			return invalid("Media Provider credential fields are invalid")
		}
		defer wipeProviderSecret(credentialPayload)
		if _, findErr := repo.LatestProviderConnectionForUpdate(ctx, command.WorkspaceID, command.ConnectionKey); findErr == nil {
			return conflict("Media Provider connection already exists")
		} else if !errors.Is(findErr, ErrProviderConnectionNotFound) {
			return findErr
		}
		if !service.secrets.Available() {
			return secretStoreUnavailable()
		}
		credentialID, connectionID, idErr := service.newIDs()
		if idErr != nil {
			return idErr
		}
		encrypted, encryptErr := service.secrets.Encrypt(ctx, domain.ProviderSecretContext{
			WorkspaceID: command.WorkspaceID, ProviderKey: preset.ProviderKey,
			CredentialID: credentialID, Revision: 1,
		}, credentialPayload)
		if encryptErr != nil {
			return secretStoreUnavailable()
		}
		if encrypted.Fingerprint != credentialFingerprint {
			return errors.New("Media Provider credential fingerprint is inconsistent")
		}
		credential := domain.ProviderCredentialVersion{
			ID: credentialID, WorkspaceID: command.WorkspaceID, ConnectionKey: command.ConnectionKey,
			Revision: 1, ProviderKey: preset.ProviderKey, CipherSuite: encrypted.CipherSuite,
			KeyID: encrypted.KeyID, Nonce: encrypted.Nonce, Ciphertext: encrypted.Ciphertext,
			SecretFingerprint: encrypted.Fingerprint, CreatedBy: actor.UserID, CreatedAt: now,
		}
		credential, createErr := repo.CreateProviderCredential(ctx, credential)
		if createErr != nil {
			return createErr
		}
		presetHash, hashErr := platformcommand.InputHash(preset)
		if hashErr != nil {
			return hashErr
		}
		connection := domain.ProviderConnectionVersion{
			ID: connectionID, WorkspaceID: command.WorkspaceID, ConnectionKey: command.ConnectionKey,
			Revision: 1, SourcePresetKey: preset.PresetKey, SourcePresetVersion: preset.PresetVersion,
			PresetSnapshotHash: presetHash, ProviderKey: preset.ProviderKey, DisplayName: command.DisplayName,
			CredentialVersionID: credential.ID, ResolvedConfig: resolvedConfig, State: domain.ProviderStateEnabled,
			AdapterContractVersion: preset.AdapterContractVersion, CreatedBy: actor.UserID, CreatedAt: now,
		}
		connection.ContentHash, hashErr = providerConnectionContentHash(connection)
		if hashErr != nil {
			return hashErr
		}
		connection, createErr = repo.CreateProviderConnection(ctx, connection)
		if createErr != nil {
			return createErr
		}
		receipt, receiptErr := service.ensureConfigurationReceipt(
			ctx, repo, actor, command.WorkspaceID, createProviderConnectionOperation,
			command.IdempotencyKey, inputHash, connection.ID, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result = ProviderConnectionResult{Connection: connection, Credential: credentialView(credential), Receipt: receipt}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) RotateCredential(
	ctx context.Context,
	actor Actor,
	command RotateProviderCredentialCommand,
) (ProviderConnectionResult, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ConnectionKey = strings.TrimSpace(command.ConnectionKey)
	command.ExpectedContentHash = strings.TrimSpace(command.ExpectedContentHash)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(command.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(command.ConnectionKey) || command.ExpectedRevision < 1 ||
		!intentHashPattern.MatchString(command.ExpectedContentHash) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProviderConnectionResult{}, invalid("Invalid Media Provider credential rotation command")
	}
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	var result ProviderConnectionResult
	err := service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if authorizeErr := repo.AuthorizeWorkspaceOwner(ctx, actor, command.WorkspaceID); authorizeErr != nil {
			return authorizeErr
		}
		hashCommand := command
		hashCommand.Credentials = nil
		inputHash, credentialFingerprint, hashErr := service.providerCredentialCommandInputHash(
			ctx,
			actor.UserID,
			hashCommand,
			command.Credentials,
		)
		if hashErr != nil {
			return hashErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, rotateProviderCredentialOperation, command.IdempotencyKey); findErr == nil {
			return service.replayConnection(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		latest, findErr := repo.LatestProviderConnectionForUpdate(ctx, command.WorkspaceID, command.ConnectionKey)
		if findErr != nil {
			return findErr
		}
		if latest.Revision != command.ExpectedRevision || latest.ContentHash != command.ExpectedContentHash {
			return conflict("Media Provider connection revision has changed")
		}
		preset, findErr := service.catalog.ResolveConnection(latest.SourcePresetKey, latest.SourcePresetVersion)
		if findErr != nil || preset.ProviderKey != latest.ProviderKey ||
			preset.AdapterContractVersion != latest.AdapterContractVersion {
			return conflict("Media Provider connection preset has drifted")
		}
		presetHash, hashErr := platformcommand.InputHash(preset)
		if hashErr != nil {
			return hashErr
		}
		if presetHash != latest.PresetSnapshotHash {
			return conflict("Media Provider connection preset snapshot has drifted")
		}
		credentialPayload, resolveErr := resolveCredentialPayload(preset.CredentialFields, command.Credentials)
		if resolveErr != nil {
			return invalid("Media Provider credential fields are invalid")
		}
		defer wipeProviderSecret(credentialPayload)
		if !service.secrets.Available() {
			return secretStoreUnavailable()
		}
		latestCredential, findErr := repo.LatestProviderCredentialForUpdate(ctx, command.WorkspaceID, command.ConnectionKey)
		if findErr != nil || latestCredential.ID != latest.CredentialVersionID ||
			validateProviderCredentialVersion(latestCredential) != nil {
			return conflict("Media Provider credential lineage has drifted")
		}
		credentialID, connectionID, idErr := service.newIDs()
		if idErr != nil {
			return idErr
		}
		credentialRevision := latestCredential.Revision + 1
		encrypted, encryptErr := service.secrets.Encrypt(ctx, domain.ProviderSecretContext{
			WorkspaceID: command.WorkspaceID, ProviderKey: latest.ProviderKey,
			CredentialID: credentialID, Revision: credentialRevision,
		}, credentialPayload)
		if encryptErr != nil {
			return secretStoreUnavailable()
		}
		if encrypted.Fingerprint != credentialFingerprint {
			return errors.New("Media Provider credential fingerprint is inconsistent")
		}
		credential := domain.ProviderCredentialVersion{
			ID: credentialID, WorkspaceID: command.WorkspaceID, ConnectionKey: command.ConnectionKey,
			Revision: credentialRevision, ProviderKey: latest.ProviderKey, CipherSuite: encrypted.CipherSuite,
			KeyID: encrypted.KeyID, Nonce: encrypted.Nonce, Ciphertext: encrypted.Ciphertext,
			SecretFingerprint: encrypted.Fingerprint, CreatedBy: actor.UserID, CreatedAt: now,
		}
		credential, createErr := repo.CreateProviderCredential(ctx, credential)
		if createErr != nil {
			return createErr
		}
		connection := latest
		connection.ID, connection.Revision, connection.CredentialVersionID = connectionID, latest.Revision+1, credential.ID
		connection.CreatedBy, connection.CreatedAt, connection.ContentHash = actor.UserID, now, ""
		connection.ContentHash, createErr = providerConnectionContentHash(connection)
		if createErr != nil {
			return createErr
		}
		connection, createErr = repo.CreateProviderConnection(ctx, connection)
		if createErr != nil {
			return createErr
		}
		receipt, receiptErr := service.ensureConfigurationReceipt(
			ctx, repo, actor, command.WorkspaceID, rotateProviderCredentialOperation,
			command.IdempotencyKey, inputHash, connection.ID, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result = ProviderConnectionResult{Connection: connection, Credential: credentialView(credential), Receipt: receipt}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) SetConnectionState(
	ctx context.Context,
	actor Actor,
	command SetProviderConnectionStateCommand,
) (ProviderConnectionResult, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ConnectionKey = strings.TrimSpace(command.ConnectionKey)
	command.State = strings.TrimSpace(command.State)
	command.ExpectedContentHash = strings.TrimSpace(command.ExpectedContentHash)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(command.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(command.ConnectionKey) || command.ExpectedRevision < 1 ||
		!intentHashPattern.MatchString(command.ExpectedContentHash) ||
		(command.State != domain.ProviderStateEnabled && command.State != domain.ProviderStateDisabled) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProviderConnectionResult{}, invalid("Invalid Media Provider connection state command")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command SetProviderConnectionStateCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ProviderConnectionResult{}, err
	}
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	var result ProviderConnectionResult
	err = service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if authorizeErr := repo.AuthorizeWorkspaceOwner(ctx, actor, command.WorkspaceID); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, setProviderConnectionStateOperation, command.IdempotencyKey); findErr == nil {
			return service.replayConnection(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		latest, findErr := repo.LatestProviderConnectionForUpdate(ctx, command.WorkspaceID, command.ConnectionKey)
		if findErr != nil {
			return findErr
		}
		if latest.Revision != command.ExpectedRevision || latest.ContentHash != command.ExpectedContentHash ||
			latest.State == command.State {
			return conflict("Media Provider connection revision or state has changed")
		}
		connectionID := strings.TrimSpace(service.config.NewID())
		if !validUUID(connectionID) {
			return errors.New("Media Provider connection identifier is invalid")
		}
		connection := latest
		connection.ID, connection.Revision, connection.State = connectionID, latest.Revision+1, command.State
		connection.CreatedBy, connection.CreatedAt, connection.ContentHash = actor.UserID, now, ""
		connection.ContentHash, findErr = providerConnectionContentHash(connection)
		if findErr != nil {
			return findErr
		}
		connection, findErr = repo.CreateProviderConnection(ctx, connection)
		if findErr != nil {
			return findErr
		}
		credential, findErr := repo.FindProviderCredential(ctx, connection.CredentialVersionID)
		if findErr != nil {
			return findErr
		}
		receipt, receiptErr := service.ensureConfigurationReceipt(
			ctx, repo, actor, command.WorkspaceID, setProviderConnectionStateOperation,
			command.IdempotencyKey, inputHash, connection.ID, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result = ProviderConnectionResult{Connection: connection, Credential: credentialView(credential), Receipt: receipt}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) CreateModelProfile(
	ctx context.Context,
	actor Actor,
	command CreateProviderModelProfileCommand,
) (ProviderModelProfileResult, error) {
	normalizeProfileCommand(&command)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(command.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(command.ProfileKey) ||
		!providerConfigurationKeyPattern.MatchString(command.ConnectionKey) ||
		!mediaPresetKeyPattern.MatchString(command.PresetKey) || command.PresetVersion < 1 ||
		command.ExpectedRevision != 0 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProviderModelProfileResult{}, invalid("Invalid Media Provider model profile command")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command CreateProviderModelProfileCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ProviderModelProfileResult{}, err
	}
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	var result ProviderModelProfileResult
	err = service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if authorizeErr := repo.AuthorizeWorkspaceOwner(ctx, actor, command.WorkspaceID); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, createProviderProfileOperation, command.IdempotencyKey); findErr == nil {
			return replayProfile(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		preset, findErr := service.catalog.ResolveModel(command.PresetKey, command.PresetVersion)
		if findErr != nil {
			return invalid("Media Provider model preset is unavailable")
		}
		defaults, resolveErr := resolvePresetValues(
			preset.FixedDefaults,
			preset.EditableOverrides,
			command.DefaultOverrides,
		)
		if resolveErr != nil {
			return invalid("Media Provider model profile fields are invalid")
		}
		externalModelID := preset.ExternalModelID
		if externalModelID != "" && command.ExternalModelID != "" && command.ExternalModelID != externalModelID {
			return invalid("Media Provider external model identity does not match the preset")
		}
		if externalModelID == "" {
			externalModelID = command.ExternalModelID
		}
		if !providerIdentifierPattern.MatchString(externalModelID) {
			return invalid("Media Provider external model identity is invalid")
		}
		if _, findErr := repo.LatestProviderModelProfileForUpdate(ctx, command.WorkspaceID, command.ProfileKey); findErr == nil {
			return conflict("Media Provider model profile already exists")
		} else if !errors.Is(findErr, ErrProviderProfileNotFound) {
			return findErr
		}
		connection, findErr := repo.LatestProviderConnectionForUpdate(ctx, command.WorkspaceID, command.ConnectionKey)
		if findErr != nil {
			return findErr
		}
		if connection.State != domain.ProviderStateEnabled || connection.ProviderKey != preset.ProviderKey ||
			connection.AdapterContractVersion != preset.AdapterContractVersion {
			return conflict("Media Provider model preset does not match the connection")
		}
		profileID := strings.TrimSpace(service.config.NewID())
		if !validUUID(profileID) {
			return errors.New("Media Provider model profile identifier is invalid")
		}
		presetHash, hashErr := platformcommand.InputHash(preset)
		if hashErr != nil {
			return hashErr
		}
		profile := domain.ProviderModelProfileVersion{
			ID: profileID, WorkspaceID: command.WorkspaceID, ProfileKey: command.ProfileKey, Revision: 1,
			CreationSource: map[string]any{"kind": "preset", "key": preset.PresetKey, "version": preset.PresetVersion, "snapshot_hash": presetHash},
			ConnectionKey:  command.ConnectionKey, ProviderKey: preset.ProviderKey, ExternalModelID: externalModelID,
			Modality: preset.Modality, Family: preset.Family, AdapterTransportContract: preset.AdapterTransportContract,
			CapabilitySchemaVersion: preset.CapabilitySchemaVersion, BillingMetric: preset.BillingMetric,
			Defaults: defaults, State: domain.ProviderStateEnabled, CreatedBy: actor.UserID, CreatedAt: now,
		}
		profile.ContentHash, hashErr = providerProfileContentHash(profile)
		if hashErr != nil {
			return hashErr
		}
		profile, createErr := repo.CreateProviderModelProfile(ctx, profile)
		if createErr != nil {
			return createErr
		}
		receipt, receiptErr := service.ensureConfigurationReceipt(
			ctx, repo, actor, command.WorkspaceID, createProviderProfileOperation,
			command.IdempotencyKey, inputHash, profile.ID, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result = ProviderModelProfileResult{Profile: profile, Receipt: receipt}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) SetModelProfileState(
	ctx context.Context,
	actor Actor,
	command SetProviderModelProfileStateCommand,
) (ProviderModelProfileResult, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProfileKey = strings.TrimSpace(command.ProfileKey)
	command.State = strings.TrimSpace(command.State)
	command.ExpectedContentHash = strings.TrimSpace(command.ExpectedContentHash)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(command.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(command.ProfileKey) || command.ExpectedRevision < 1 ||
		!intentHashPattern.MatchString(command.ExpectedContentHash) ||
		(command.State != domain.ProviderStateEnabled && command.State != domain.ProviderStateDisabled) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProviderModelProfileResult{}, invalid("Invalid Media Provider model profile state command")
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID string
		Command SetProviderModelProfileStateCommand
	}{ActorID: actor.UserID, Command: command})
	if err != nil {
		return ProviderModelProfileResult{}, err
	}
	now := service.config.Now().UTC().Truncate(time.Microsecond)
	var result ProviderModelProfileResult
	err = service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if authorizeErr := repo.AuthorizeWorkspaceOwner(ctx, actor, command.WorkspaceID); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, setProviderProfileStateOperation, command.IdempotencyKey); findErr == nil {
			return replayProfile(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		latest, findErr := repo.LatestProviderModelProfileForUpdate(ctx, command.WorkspaceID, command.ProfileKey)
		if findErr != nil {
			return findErr
		}
		if latest.Revision != command.ExpectedRevision || latest.ContentHash != command.ExpectedContentHash ||
			latest.State == command.State {
			return conflict("Media Provider model profile revision or state has changed")
		}
		profileID := strings.TrimSpace(service.config.NewID())
		if !validUUID(profileID) {
			return errors.New("Media Provider model profile identifier is invalid")
		}
		profile := latest
		profile.ID, profile.Revision, profile.State = profileID, latest.Revision+1, command.State
		profile.CreatedBy, profile.CreatedAt, profile.ContentHash = actor.UserID, now, ""
		profile.ContentHash, findErr = providerProfileContentHash(profile)
		if findErr != nil {
			return findErr
		}
		profile, findErr = repo.CreateProviderModelProfile(ctx, profile)
		if findErr != nil {
			return findErr
		}
		receipt, receiptErr := service.ensureConfigurationReceipt(
			ctx, repo, actor, command.WorkspaceID, setProviderProfileStateOperation,
			command.IdempotencyKey, inputHash, profile.ID, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result = ProviderModelProfileResult{Profile: profile, Receipt: receipt}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) ListWorkspaceConfiguration(
	ctx context.Context,
	actor Actor,
	workspaceID string,
) (ProviderConfigurationList, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(workspaceID) {
		return ProviderConfigurationList{}, invalid("Invalid Media Provider configuration query")
	}
	result := ProviderConfigurationList{
		Connections: []ProviderConnectionResult{}, Profiles: []domain.ProviderModelProfileVersion{},
	}
	err := service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if authorizeErr := repo.AuthorizeWorkspaceOwner(ctx, actor, workspaceID); authorizeErr != nil {
			return authorizeErr
		}
		connections, findErr := repo.ListLatestProviderConnections(ctx, workspaceID)
		if findErr != nil {
			return findErr
		}
		for _, connection := range connections {
			credential, credentialErr := repo.FindProviderCredential(ctx, connection.CredentialVersionID)
			if credentialErr != nil {
				return credentialErr
			}
			result.Connections = append(result.Connections, ProviderConnectionResult{
				Connection: connection, Credential: credentialView(credential),
			})
		}
		result.Profiles, findErr = repo.ListLatestProviderModelProfiles(ctx, workspaceID)
		return findErr
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) ListProjectBindings(
	ctx context.Context,
	actor Actor,
	projectID string,
) ([]domain.ProjectProviderBindingVersion, error) {
	projectID = strings.TrimSpace(projectID)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(projectID) {
		return nil, invalid("Invalid Project Media Provider binding query")
	}
	result := []domain.ProjectProviderBindingVersion{}
	err := service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		scope, authorizeErr := repo.AuthorizeProviderProject(ctx, actor, projectID)
		if authorizeErr != nil {
			return authorizeErr
		}
		var findErr error
		result, findErr = repo.ListLatestProjectProviderBindings(ctx, scope.WorkspaceID, scope.ProjectID)
		return findErr
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) PublishProjectBinding(
	ctx context.Context,
	actor Actor,
	command PublishProjectProviderBindingCommand,
) (ProjectProviderBindingResult, error) {
	normalizeBindingCommand(&command)
	if !service.valid() || !validPreparationActor(actor) ||
		(command.WorkspaceID != "" && !validUUID(command.WorkspaceID)) || !validUUID(command.ProjectID) ||
		!validProviderPurpose(command.Purpose) || !validUUID(command.ConnectionVersionID) ||
		!validUUID(command.ModelProfileVersionID) || command.ExpectedRevision < 0 ||
		(command.ExpectedRevision == 0 && command.ExpectedContentHash != "") ||
		(command.ExpectedRevision > 0 && !intentHashPattern.MatchString(command.ExpectedContentHash)) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ProjectProviderBindingResult{}, invalid("Invalid Project Media Provider binding command")
	}
	var result ProjectProviderBindingResult
	err := service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		if command.WorkspaceID == "" {
			scope, authorizeErr := repo.AuthorizeProviderProject(ctx, actor, command.ProjectID)
			if authorizeErr != nil {
				return authorizeErr
			}
			command.WorkspaceID, command.ProjectID = scope.WorkspaceID, scope.ProjectID
		} else {
			scope, authorizeErr := repo.AuthorizeProviderProject(ctx, actor, command.ProjectID)
			if authorizeErr != nil {
				return authorizeErr
			}
			if scope.WorkspaceID != command.WorkspaceID {
				return conflict("Project Media Provider binding workspace has drifted")
			}
		}
		if lockErr := repo.LockProviderWorkspace(ctx, command.WorkspaceID); lockErr != nil {
			return lockErr
		}
		inputHash, hashErr := platformcommand.InputHash(struct {
			ActorID string
			Command PublishProjectProviderBindingCommand
		}{ActorID: actor.UserID, Command: command})
		if hashErr != nil {
			return hashErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, publishProjectBindingOperation, command.IdempotencyKey); findErr == nil {
			return replayProjectBinding(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		connection, findErr := repo.FindProviderConnection(ctx, command.ConnectionVersionID)
		if findErr != nil {
			return findErr
		}
		profile, findErr := repo.FindProviderModelProfile(ctx, command.ModelProfileVersionID)
		if findErr != nil {
			return findErr
		}
		if connection.WorkspaceID != command.WorkspaceID || profile.WorkspaceID != command.WorkspaceID ||
			connection.State != domain.ProviderStateEnabled || profile.State != domain.ProviderStateEnabled ||
			connection.ConnectionKey != profile.ConnectionKey || connection.ProviderKey != profile.ProviderKey ||
			!purposeAllows(command.Purpose, profile) {
			return conflict("Project Media Provider binding facts do not match")
		}
		latest, findErr := repo.LatestProjectProviderBindingForUpdate(
			ctx,
			command.WorkspaceID,
			command.ProjectID,
			command.Purpose,
		)
		revision := int64(1)
		if findErr == nil {
			if latest.Revision != command.ExpectedRevision || latest.ContentHash != command.ExpectedContentHash {
				return conflict("Project Media Provider binding revision or hash has changed")
			}
			revision = latest.Revision + 1
		} else if !errors.Is(findErr, ErrProjectProviderBindingNotFound) {
			return findErr
		} else if command.ExpectedRevision != 0 || command.ExpectedContentHash != "" {
			return conflict("Project Media Provider binding revision or hash has changed")
		}
		currentConnection, findErr := repo.LatestProviderConnectionForUpdate(
			ctx, command.WorkspaceID, connection.ConnectionKey,
		)
		if findErr != nil || currentConnection.ID != connection.ID {
			return conflict("Project Media Provider binding connection is no longer current")
		}
		currentProfile, findErr := repo.LatestProviderModelProfileForUpdate(
			ctx, command.WorkspaceID, profile.ProfileKey,
		)
		if findErr != nil || currentProfile.ID != profile.ID {
			return conflict("Project Media Provider binding model profile is no longer current")
		}
		bindingID := strings.TrimSpace(service.config.NewID())
		if !validUUID(bindingID) {
			return errors.New("Project Media Provider binding identifier is invalid")
		}
		now := service.config.Now().UTC().Truncate(time.Microsecond)
		binding := domain.ProjectProviderBindingVersion{
			ID: bindingID, WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
			Purpose: command.Purpose, Revision: revision, ConnectionVersionID: connection.ID,
			CredentialVersionID: connection.CredentialVersionID, ModelProfileVersionID: profile.ID,
			ProviderKey: connection.ProviderKey, Modality: profile.Modality,
			AdapterContractVersion: connection.AdapterContractVersion, CreatedBy: actor.UserID, CreatedAt: now,
		}
		binding.ContentHash, hashErr = projectProviderBindingContentHash(binding)
		if hashErr != nil {
			return hashErr
		}
		binding, createErr := repo.CreateProjectProviderBinding(ctx, binding)
		if createErr != nil {
			return createErr
		}
		receipt, receiptErr := service.ensureConfigurationReceipt(
			ctx, repo, actor, command.WorkspaceID, publishProjectBindingOperation,
			command.IdempotencyKey, inputHash, binding.ID, now,
		)
		if receiptErr != nil {
			return receiptErr
		}
		result = ProjectProviderBindingResult{Binding: binding, Receipt: receipt}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) ResolveProjectBinding(
	ctx context.Context,
	actor Actor,
	projectID, purpose string,
) (ResolvedProjectProviderBinding, error) {
	projectID, purpose = strings.TrimSpace(projectID), strings.TrimSpace(purpose)
	if !service.valid() || !validPreparationActor(actor) || !validUUID(projectID) || !validProviderPurpose(purpose) {
		return ResolvedProjectProviderBinding{}, invalid("Invalid Project Media Provider binding query")
	}
	var result ResolvedProjectProviderBinding
	err := service.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		scope, authorizeErr := repo.AuthorizeExecutableProviderProject(ctx, actor, projectID)
		if authorizeErr != nil {
			return authorizeErr
		}
		if lockErr := repo.LockProviderWorkspace(ctx, scope.WorkspaceID); lockErr != nil {
			return lockErr
		}
		binding, findErr := repo.LatestProjectProviderBindingForUpdate(ctx, scope.WorkspaceID, scope.ProjectID, purpose)
		if findErr != nil {
			return findErr
		}
		connection, findErr := repo.FindProviderConnection(ctx, binding.ConnectionVersionID)
		if findErr != nil {
			return findErr
		}
		credential, findErr := repo.FindProviderCredential(ctx, binding.CredentialVersionID)
		if findErr != nil {
			return findErr
		}
		profile, findErr := repo.FindProviderModelProfile(ctx, binding.ModelProfileVersionID)
		if findErr != nil {
			return findErr
		}
		if err := validateResolvedProviderFacts(binding, connection, credential, profile); err != nil {
			return err
		}
		if !service.secrets.Available() || !service.secrets.MatchesKeyID(credential.KeyID) {
			return secretStoreUnavailable()
		}
		latestConnection, findErr := repo.LatestProviderConnectionForUpdate(ctx, binding.WorkspaceID, connection.ConnectionKey)
		if findErr != nil || latestConnection.ID != connection.ID || latestConnection.State != domain.ProviderStateEnabled {
			return conflict("Project Media Provider binding connection is no longer current")
		}
		latestProfile, findErr := repo.LatestProviderModelProfileForUpdate(ctx, binding.WorkspaceID, profile.ProfileKey)
		if findErr != nil || latestProfile.ID != profile.ID || latestProfile.State != domain.ProviderStateEnabled {
			return conflict("Project Media Provider binding model profile is no longer current")
		}
		result = ResolvedProjectProviderBinding{
			Binding: binding, Connection: connection, Credential: credentialView(credential),
			Profile: profile,
		}
		return nil
	})
	return result, normalizeProviderConfigurationError(err)
}

func (service *ProviderConfigurationService) valid() bool {
	return service != nil && service.transactions != nil && service.catalog != nil && service.secrets != nil &&
		service.config.Now != nil && service.config.NewID != nil
}

func (service *ProviderConfigurationService) newIDs() (string, string, error) {
	first, second := strings.TrimSpace(service.config.NewID()), strings.TrimSpace(service.config.NewID())
	if !validUUID(first) || !validUUID(second) {
		return "", "", errors.New("Media Provider configuration identifiers are invalid")
	}
	return first, second, nil
}

func (service *ProviderConfigurationService) ensureConfigurationReceipt(
	ctx context.Context,
	repo ProviderConfigurationRepository,
	actor Actor,
	workspaceID, operation, key, inputHash, resourceID string,
	now time.Time,
) (platformcommand.Receipt, error) {
	result, err := platformcommand.Result(providerConfigurationReceipt{ResourceID: resourceID})
	if err != nil {
		return platformcommand.Receipt{}, err
	}
	receiptID := strings.TrimSpace(service.config.NewID())
	if !validUUID(receiptID) {
		return platformcommand.Receipt{}, errors.New("Media Provider command receipt identifier is invalid")
	}
	return repo.EnsureReceipt(ctx, platformcommand.Receipt{
		ID: receiptID, WorkspaceID: workspaceID, Operation: operation, IdempotencyKey: key,
		InputHash: inputHash, ResourceID: resourceID, Result: result, CreatedBy: actor.UserID, CreatedAt: now,
	})
}

func (service *ProviderConfigurationService) replayConnection(
	ctx context.Context,
	repo ProviderConfigurationRepository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *ProviderConnectionResult,
) error {
	replayed, err := platformcommand.Replay[providerConfigurationReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if replayed.ResourceID == "" || replayed.ResourceID != receipt.ResourceID {
		return conflict("Media Provider command receipt has drifted")
	}
	connection, err := repo.FindProviderConnection(ctx, replayed.ResourceID)
	if err != nil {
		return err
	}
	if connection.WorkspaceID != receipt.WorkspaceID || validateProviderConnectionVersion(connection) != nil {
		return conflict("Media Provider command receipt resource has drifted")
	}
	credential, err := repo.FindProviderCredential(ctx, connection.CredentialVersionID)
	if err != nil {
		return err
	}
	if credential.WorkspaceID != receipt.WorkspaceID || credential.ID != connection.CredentialVersionID ||
		credential.ConnectionKey != connection.ConnectionKey || credential.ProviderKey != connection.ProviderKey ||
		validateProviderCredentialVersion(credential) != nil {
		return conflict("Media Provider command receipt resource has drifted")
	}
	*result = ProviderConnectionResult{Connection: connection, Credential: credentialView(credential), Receipt: receipt}
	return nil
}

func replayProfile(
	ctx context.Context,
	repo ProviderConfigurationRepository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *ProviderModelProfileResult,
) error {
	replayed, err := platformcommand.Replay[providerConfigurationReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if replayed.ResourceID == "" || replayed.ResourceID != receipt.ResourceID {
		return conflict("Media Provider command receipt has drifted")
	}
	profile, err := repo.FindProviderModelProfile(ctx, replayed.ResourceID)
	if err != nil {
		return err
	}
	if profile.WorkspaceID != receipt.WorkspaceID || validateProviderModelProfileVersion(profile) != nil {
		return conflict("Media Provider command receipt resource has drifted")
	}
	*result = ProviderModelProfileResult{Profile: profile, Receipt: receipt}
	return nil
}

func replayProjectBinding(
	ctx context.Context,
	repo ProviderConfigurationRepository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *ProjectProviderBindingResult,
) error {
	replayed, err := platformcommand.Replay[providerConfigurationReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	if replayed.ResourceID == "" || replayed.ResourceID != receipt.ResourceID {
		return conflict("Media Provider command receipt has drifted")
	}
	binding, err := repo.FindProjectProviderBinding(ctx, replayed.ResourceID)
	if err != nil {
		return err
	}
	if binding.WorkspaceID != receipt.WorkspaceID || validateProjectProviderBinding(binding) != nil {
		return conflict("Media Provider command receipt resource has drifted")
	}
	*result = ProjectProviderBindingResult{Binding: binding, Receipt: receipt}
	return nil
}

func resolvePresetValues(fixed map[string]any, fields []domain.MediaPresetField, overrides map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(fixed)+len(overrides))
	for key, value := range fixed {
		result[key] = value
	}
	allowed := make(map[string]domain.MediaPresetField, len(fields))
	for _, field := range fields {
		allowed[field.Key] = field
	}
	for key, value := range overrides {
		field, exists := allowed[key]
		if !exists || !validPresetFieldValue(field, value) {
			return nil, errors.New("unsupported Media Provider preset override")
		}
		result[key] = value
	}
	for _, field := range fields {
		if field.Required {
			if _, exists := result[field.Key]; !exists {
				return nil, errors.New("required Media Provider preset field is missing")
			}
		}
	}
	return result, nil
}

func resolveCredentialPayload(fields []domain.MediaPresetField, values map[string]string) ([]byte, error) {
	allowed := make(map[string]domain.MediaPresetField, len(fields))
	for _, field := range fields {
		allowed[field.Key] = field
	}
	for key, value := range values {
		field, exists := allowed[key]
		if !exists || !validPresetFieldValue(field, value) {
			return nil, errors.New("unsupported Media Provider credential field")
		}
	}
	for _, field := range fields {
		if field.Required && strings.TrimSpace(values[field.Key]) == "" {
			return nil, errors.New("required Media Provider credential field is missing")
		}
	}
	return json.Marshal(values)
}

func (service *ProviderConfigurationService) providerCredentialCommandInputHash(
	ctx context.Context,
	actorID string,
	command any,
	credentials map[string]string,
) (string, string, error) {
	payload, err := json.Marshal(credentials)
	if err != nil {
		return "", "", err
	}
	defer wipeProviderSecret(payload)
	fingerprint, err := service.secrets.Fingerprint(ctx, payload)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", "", err
		}
		return "", "", secretStoreUnavailable()
	}
	inputHash, err := platformcommand.InputHash(struct {
		ActorID               string
		Command               any
		CredentialFingerprint string
	}{ActorID: actorID, Command: command, CredentialFingerprint: fingerprint})
	if err != nil {
		return "", "", err
	}
	return inputHash, fingerprint, nil
}

func wipeProviderSecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func validPresetFieldValue(field domain.MediaPresetField, value any) bool {
	if field.Type != "string" {
		return false
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	length := int64(len(strings.TrimSpace(text)))
	if field.Required && length == 0 || field.Minimum > 0 && length < field.Minimum || field.Maximum > 0 && length > field.Maximum {
		return false
	}
	if field.Pattern != "" {
		pattern, err := regexp.Compile(field.Pattern)
		return err == nil && pattern.MatchString(text)
	}
	return true
}

func normalizeConnectionCommand(command *CreateProviderConnectionCommand) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ConnectionKey = strings.TrimSpace(command.ConnectionKey)
	command.PresetKey = strings.TrimSpace(command.PresetKey)
	command.DisplayName = strings.TrimSpace(command.DisplayName)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
}

func normalizeProfileCommand(command *CreateProviderModelProfileCommand) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProfileKey = strings.TrimSpace(command.ProfileKey)
	command.ConnectionKey = strings.TrimSpace(command.ConnectionKey)
	command.PresetKey = strings.TrimSpace(command.PresetKey)
	command.ExternalModelID = strings.TrimSpace(command.ExternalModelID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
}

func normalizeBindingCommand(command *PublishProjectProviderBindingCommand) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.Purpose = strings.TrimSpace(command.Purpose)
	command.ConnectionVersionID = strings.TrimSpace(command.ConnectionVersionID)
	command.ModelProfileVersionID = strings.TrimSpace(command.ModelProfileVersionID)
	command.ExpectedContentHash = strings.TrimSpace(command.ExpectedContentHash)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
}

func credentialView(value domain.ProviderCredentialVersion) ProviderCredentialView {
	fingerprint := value.SecretFingerprint
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[:12]
	}
	return ProviderCredentialView{
		ID: value.ID, WorkspaceID: value.WorkspaceID, ConnectionKey: value.ConnectionKey,
		ProviderKey: value.ProviderKey, Revision: value.Revision, Fingerprint: fingerprint,
		CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt,
	}
}

func validateResolvedProviderFacts(
	binding domain.ProjectProviderBindingVersion,
	connection domain.ProviderConnectionVersion,
	credential domain.ProviderCredentialVersion,
	profile domain.ProviderModelProfileVersion,
) error {
	if validateProjectProviderBinding(binding) != nil || validateProviderConnectionVersion(connection) != nil ||
		validateProviderCredentialVersion(credential) != nil || validateProviderModelProfileVersion(profile) != nil ||
		binding.WorkspaceID != connection.WorkspaceID || binding.WorkspaceID != credential.WorkspaceID ||
		binding.WorkspaceID != profile.WorkspaceID || binding.ConnectionVersionID != connection.ID ||
		binding.CredentialVersionID != credential.ID || binding.ModelProfileVersionID != profile.ID ||
		connection.CredentialVersionID != credential.ID || connection.ConnectionKey != credential.ConnectionKey ||
		connection.ConnectionKey != profile.ConnectionKey || binding.ProviderKey != connection.ProviderKey ||
		binding.ProviderKey != credential.ProviderKey || binding.ProviderKey != profile.ProviderKey ||
		binding.Modality != profile.Modality || binding.AdapterContractVersion != connection.AdapterContractVersion ||
		connection.State != domain.ProviderStateEnabled || profile.State != domain.ProviderStateEnabled {
		return conflict("Project Media Provider binding facts have drifted")
	}
	return nil
}

func validateProviderConnectionVersion(value domain.ProviderConnectionVersion) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(value.ConnectionKey) || value.Revision < 1 ||
		!mediaPresetKeyPattern.MatchString(value.SourcePresetKey) || value.SourcePresetVersion < 1 ||
		!intentHashPattern.MatchString(value.PresetSnapshotHash) || !providerIdentifierPattern.MatchString(value.ProviderKey) ||
		strings.TrimSpace(value.DisplayName) == "" || !validUUID(value.CredentialVersionID) ||
		(value.State != domain.ProviderStateEnabled && value.State != domain.ProviderStateDisabled) ||
		!providerIdentifierPattern.MatchString(value.AdapterContractVersion) || !validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Media Provider connection facts have drifted")
	}
	hash, err := providerConnectionContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Media Provider connection facts have drifted")
	}
	return nil
}

func validateProviderCredentialVersion(value domain.ProviderCredentialVersion) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(value.ConnectionKey) || value.Revision < 1 ||
		!providerIdentifierPattern.MatchString(value.ProviderKey) || value.CipherSuite != domain.ProviderCipherAES256GCM ||
		strings.TrimSpace(value.KeyID) == "" || len(value.Nonce) != 12 || len(value.Ciphertext) < 16 ||
		!intentHashPattern.MatchString(value.SecretFingerprint) || !validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Media Provider credential facts have drifted")
	}
	return nil
}

func validateProviderModelProfileVersion(value domain.ProviderModelProfileVersion) error {
	if !validUUID(value.ID) || !validUUID(value.WorkspaceID) ||
		!providerConfigurationKeyPattern.MatchString(value.ProfileKey) || value.Revision < 1 || len(value.CreationSource) == 0 ||
		!providerConfigurationKeyPattern.MatchString(value.ConnectionKey) ||
		!providerIdentifierPattern.MatchString(value.ProviderKey) || !providerIdentifierPattern.MatchString(value.ExternalModelID) ||
		(value.Modality != domain.MediaModalityImage && value.Modality != domain.MediaModalityVideo) ||
		!providerIdentifierPattern.MatchString(value.Family) || !providerIdentifierPattern.MatchString(value.AdapterTransportContract) ||
		!providerIdentifierPattern.MatchString(value.CapabilitySchemaVersion) ||
		(value.BillingMetric != "generation.image.call" && value.BillingMetric != "generation.video.call") ||
		(value.State != domain.ProviderStateEnabled && value.State != domain.ProviderStateDisabled) ||
		!validUUID(value.CreatedBy) || value.CreatedAt.IsZero() {
		return conflict("Media Provider model profile facts have drifted")
	}
	hash, err := providerProfileContentHash(value)
	if err != nil || hash != value.ContentHash {
		return conflict("Media Provider model profile facts have drifted")
	}
	return nil
}

func providerConnectionContentHash(value domain.ProviderConnectionVersion) (string, error) {
	value.ID, value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", "", time.Time{}
	return platformcommand.InputHash(value)
}

func providerProfileContentHash(value domain.ProviderModelProfileVersion) (string, error) {
	value.ID, value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", "", time.Time{}
	return platformcommand.InputHash(value)
}

func projectProviderBindingContentHash(value domain.ProjectProviderBindingVersion) (string, error) {
	value.ID, value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", "", time.Time{}
	return platformcommand.InputHash(value)
}

func validProviderPurpose(value string) bool {
	return value == domain.ProviderPurposeReferenceAsset || value == domain.ProviderPurposeShotFrame ||
		value == domain.ProviderPurposeShotVideo
}

func purposeAllows(purpose string, profile domain.ProviderModelProfileVersion) bool {
	if purpose == domain.ProviderPurposeShotVideo {
		return profile.Modality == domain.MediaModalityVideo && profile.ProviderKey == domain.MediaProviderVolcengine && profile.Family == "seedance"
	}
	return (purpose == domain.ProviderPurposeReferenceAsset || purpose == domain.ProviderPurposeShotFrame) &&
		profile.Modality == domain.MediaModalityImage
}

func secretStoreUnavailable() error {
	return &Error{Code: "secret_store_unavailable", Message: "Media Provider secret store is unavailable", Status: 503}
}

func normalizeProviderConfigurationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Media Provider command input has drifted")
	}
	if errors.Is(err, ErrProviderPresetNotFound) {
		return invalid("Media Provider preset is unavailable")
	}
	if errors.Is(err, ErrProviderConnectionNotFound) || errors.Is(err, ErrProviderCredentialNotFound) ||
		errors.Is(err, ErrProviderProfileNotFound) || errors.Is(err, ErrProjectProviderBindingNotFound) {
		return notFound("Media Provider configuration was not found")
	}
	return err
}
