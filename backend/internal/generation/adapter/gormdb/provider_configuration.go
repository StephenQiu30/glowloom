package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type ProviderConfigurationStore struct{ database *gorm.DB }
type providerConfigurationRepository struct{ database *gorm.DB }

func NewProviderConfigurationStore(database *gorm.DB) *ProviderConfigurationStore {
	return &ProviderConfigurationStore{database: database}
}

func (store *ProviderConfigurationStore) WithinProviderConfigurationTransaction(
	ctx context.Context,
	operation func(application.ProviderConfigurationRepository) error,
) error {
	if store == nil || store.database == nil || store.database.Config == nil || operation == nil {
		return errors.New("Media Provider configuration transaction is not configured")
	}
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&providerConfigurationRepository{database: transaction})
	})
}

func (repo *providerConfigurationRepository) AuthorizeWorkspaceOwner(
	ctx context.Context,
	actor application.Actor,
	workspaceID string,
) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound("Workspace not found")
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return normalizeProviderWorkspaceNotFound(err)
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&workspace, "id = ?", workspaceUUID).Error; err != nil {
		return normalizeProviderWorkspaceNotFound(err)
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where(
		"workspace_id = ? AND user_id = ? AND status = ?", workspaceUUID, userID, "active",
	).First(&membership).Error; err != nil {
		return normalizeProviderWorkspaceNotFound(err)
	}
	if user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	if workspace.Status != "active" || membership.Role != "owner" {
		return forbidden()
	}
	return nil
}

func (repo *providerConfigurationRepository) AuthorizeProviderProject(
	ctx context.Context,
	actor application.Actor,
	projectID string,
) (application.ProviderProjectScope, error) {
	return (&repository{database: repo.database}).AuthorizeProviderProject(ctx, actor, projectID)
}

func (repo *providerConfigurationRepository) AuthorizeExecutableProviderProject(
	ctx context.Context,
	actor application.Actor,
	projectID string,
) (application.ProviderProjectScope, error) {
	return (&repository{database: repo.database}).authorizeProject(ctx, actor, "", projectID, "write")
}

func (repo *providerConfigurationRepository) LockProviderWorkspace(ctx context.Context, workspaceID string) error {
	return lockProviderWorkspace(ctx, repo.database, workspaceID)
}

func lockProviderWorkspace(ctx context.Context, database *gorm.DB, workspaceID string) error {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return application.ErrProjectProviderBindingNotFound
	}
	var record model.Workspace
	if err = database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").First(&record, "id = ?", workspace).Error; err != nil {
		return normalizeProviderConfigurationNotFound(err, application.ErrProjectProviderBindingNotFound)
	}
	return nil
}

func (repo *providerConfigurationRepository) FindReceipt(
	ctx context.Context,
	workspaceID, operation, key string,
) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repo.database, workspaceID, operation, key)
}

func (repo *providerConfigurationRepository) EnsureReceipt(
	ctx context.Context,
	receipt platformcommand.Receipt,
) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repo.database, receipt)
}

func (repo *providerConfigurationRepository) LatestProviderConnectionForUpdate(
	ctx context.Context,
	workspaceID, connectionKey string,
) (domain.ProviderConnectionVersion, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.ProviderConnectionVersion{}, application.ErrProviderConnectionNotFound
	}
	var record model.ProviderConnectionVersion
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND connection_key = ?", workspace, connectionKey).
		Order("revision DESC").First(&record).Error; err != nil {
		return domain.ProviderConnectionVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProviderConnectionNotFound)
	}
	return providerConnectionDomain(record)
}

func (repo *providerConfigurationRepository) FindProviderConnection(
	ctx context.Context,
	connectionID string,
) (domain.ProviderConnectionVersion, error) {
	id, err := uuid.Parse(connectionID)
	if err != nil {
		return domain.ProviderConnectionVersion{}, application.ErrProviderConnectionNotFound
	}
	var record model.ProviderConnectionVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProviderConnectionVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProviderConnectionNotFound)
	}
	return providerConnectionDomain(record)
}

func (repo *providerConfigurationRepository) CreateProviderCredential(
	ctx context.Context,
	value domain.ProviderCredentialVersion,
) (domain.ProviderCredentialVersion, error) {
	record, err := providerCredentialRecord(value)
	if err != nil {
		return domain.ProviderCredentialVersion{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.ProviderCredentialVersion{}, fmt.Errorf("create Media Provider credential: %w", err)
	}
	return providerCredentialDomain(record), nil
}

func (repo *providerConfigurationRepository) FindProviderCredential(
	ctx context.Context,
	credentialID string,
) (domain.ProviderCredentialVersion, error) {
	id, err := uuid.Parse(credentialID)
	if err != nil {
		return domain.ProviderCredentialVersion{}, application.ErrProviderCredentialNotFound
	}
	var record model.ProviderCredentialVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProviderCredentialVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProviderCredentialNotFound)
	}
	return providerCredentialDomain(record), nil
}

func (repo *providerConfigurationRepository) LatestProviderCredentialForUpdate(
	ctx context.Context,
	workspaceID, connectionKey string,
) (domain.ProviderCredentialVersion, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.ProviderCredentialVersion{}, application.ErrProviderCredentialNotFound
	}
	var record model.ProviderCredentialVersion
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND connection_key = ?", workspace, connectionKey).
		Order("revision DESC").First(&record).Error; err != nil {
		return domain.ProviderCredentialVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProviderCredentialNotFound)
	}
	return providerCredentialDomain(record), nil
}

func (repo *providerConfigurationRepository) CreateProviderConnection(
	ctx context.Context,
	value domain.ProviderConnectionVersion,
) (domain.ProviderConnectionVersion, error) {
	record, err := providerConnectionRecord(value)
	if err != nil {
		return domain.ProviderConnectionVersion{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.ProviderConnectionVersion{}, fmt.Errorf("create Media Provider connection: %w", err)
	}
	return providerConnectionDomain(record)
}

func (repo *providerConfigurationRepository) ListLatestProviderConnections(
	ctx context.Context,
	workspaceID string,
) ([]domain.ProviderConnectionVersion, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return nil, application.ErrProviderConnectionNotFound
	}
	var records []model.ProviderConnectionVersion
	if err = repo.database.WithContext(ctx).Where("workspace_id = ?", workspace).
		Order("connection_key ASC, revision DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProviderConnectionVersion, 0, len(records))
	lastKey := ""
	for _, record := range records {
		if record.ConnectionKey == lastKey {
			continue
		}
		value, decodeErr := providerConnectionDomain(record)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result, lastKey = append(result, value), record.ConnectionKey
	}
	return result, nil
}

func (repo *providerConfigurationRepository) LatestProviderModelProfileForUpdate(
	ctx context.Context,
	workspaceID, profileKey string,
) (domain.ProviderModelProfileVersion, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.ProviderModelProfileVersion{}, application.ErrProviderProfileNotFound
	}
	var record model.ProviderModelProfileVersion
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND profile_key = ?", workspace, profileKey).
		Order("revision DESC").First(&record).Error; err != nil {
		return domain.ProviderModelProfileVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProviderProfileNotFound)
	}
	return providerProfileDomain(record)
}

func (repo *providerConfigurationRepository) FindProviderModelProfile(
	ctx context.Context,
	profileID string,
) (domain.ProviderModelProfileVersion, error) {
	id, err := uuid.Parse(profileID)
	if err != nil {
		return domain.ProviderModelProfileVersion{}, application.ErrProviderProfileNotFound
	}
	var record model.ProviderModelProfileVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProviderModelProfileVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProviderProfileNotFound)
	}
	return providerProfileDomain(record)
}

func (repo *providerConfigurationRepository) CreateProviderModelProfile(
	ctx context.Context,
	value domain.ProviderModelProfileVersion,
) (domain.ProviderModelProfileVersion, error) {
	record, err := providerProfileRecord(value)
	if err != nil {
		return domain.ProviderModelProfileVersion{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.ProviderModelProfileVersion{}, fmt.Errorf("create Media Provider model profile: %w", err)
	}
	return providerProfileDomain(record)
}

func (repo *providerConfigurationRepository) ListLatestProviderModelProfiles(
	ctx context.Context,
	workspaceID string,
) ([]domain.ProviderModelProfileVersion, error) {
	workspace, err := uuid.Parse(workspaceID)
	if err != nil {
		return nil, application.ErrProviderProfileNotFound
	}
	var records []model.ProviderModelProfileVersion
	if err = repo.database.WithContext(ctx).Where("workspace_id = ?", workspace).
		Order("profile_key ASC, revision DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProviderModelProfileVersion, 0, len(records))
	lastKey := ""
	for _, record := range records {
		if record.ProfileKey == lastKey {
			continue
		}
		value, decodeErr := providerProfileDomain(record)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result, lastKey = append(result, value), record.ProfileKey
	}
	return result, nil
}

func (repo *providerConfigurationRepository) LatestProjectProviderBindingForUpdate(
	ctx context.Context,
	workspaceID, projectID, purpose string,
) (domain.ProjectProviderBindingVersion, error) {
	ids, err := parseProviderConfigurationUUIDs(workspaceID, projectID)
	if err != nil {
		return domain.ProjectProviderBindingVersion{}, application.ErrProjectProviderBindingNotFound
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND workspace_id = ?", ids[1], ids[0]).First(&project).Error; err != nil {
		return domain.ProjectProviderBindingVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProjectProviderBindingNotFound)
	}
	var record model.ProjectProviderBindingVersion
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND project_id = ? AND purpose = ?", ids[0], ids[1], purpose).
		Order("revision DESC").First(&record).Error; err != nil {
		return domain.ProjectProviderBindingVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProjectProviderBindingNotFound)
	}
	return projectProviderBindingDomain(record), nil
}

func (repo *providerConfigurationRepository) FindProjectProviderBinding(
	ctx context.Context,
	bindingID string,
) (domain.ProjectProviderBindingVersion, error) {
	id, err := uuid.Parse(bindingID)
	if err != nil {
		return domain.ProjectProviderBindingVersion{}, application.ErrProjectProviderBindingNotFound
	}
	var record model.ProjectProviderBindingVersion
	if err = repo.database.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return domain.ProjectProviderBindingVersion{}, normalizeProviderConfigurationNotFound(err, application.ErrProjectProviderBindingNotFound)
	}
	return projectProviderBindingDomain(record), nil
}

func (repo *providerConfigurationRepository) CreateProjectProviderBinding(
	ctx context.Context,
	value domain.ProjectProviderBindingVersion,
) (domain.ProjectProviderBindingVersion, error) {
	record, err := projectProviderBindingRecord(value)
	if err != nil {
		return domain.ProjectProviderBindingVersion{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		return domain.ProjectProviderBindingVersion{}, fmt.Errorf("create Project Media Provider binding: %w", err)
	}
	return projectProviderBindingDomain(record), nil
}

func (repo *providerConfigurationRepository) ListLatestProjectProviderBindings(
	ctx context.Context,
	workspaceID, projectID string,
) ([]domain.ProjectProviderBindingVersion, error) {
	ids, err := parseProviderConfigurationUUIDs(workspaceID, projectID)
	if err != nil {
		return nil, application.ErrProjectProviderBindingNotFound
	}
	var records []model.ProjectProviderBindingVersion
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND project_id = ?", ids[0], ids[1]).
		Order("purpose ASC, revision DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ProjectProviderBindingVersion, 0, len(records))
	lastPurpose := ""
	for _, record := range records {
		if record.Purpose == lastPurpose {
			continue
		}
		result, lastPurpose = append(result, projectProviderBindingDomain(record)), record.Purpose
	}
	return result, nil
}

func providerCredentialRecord(value domain.ProviderCredentialVersion) (model.ProviderCredentialVersion, error) {
	ids, err := parseProviderConfigurationUUIDs(value.ID, value.WorkspaceID, value.CreatedBy)
	if err != nil {
		return model.ProviderCredentialVersion{}, err
	}
	return model.ProviderCredentialVersion{
		ID: ids[0], WorkspaceID: ids[1], ConnectionKey: value.ConnectionKey, Revision: value.Revision,
		ProviderKey: value.ProviderKey, CipherSuite: value.CipherSuite, KeyID: value.KeyID,
		Nonce: append([]byte(nil), value.Nonce...), Ciphertext: append([]byte(nil), value.Ciphertext...),
		SecretFingerprint: value.SecretFingerprint, CreatedBy: ids[2], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func providerCredentialDomain(value model.ProviderCredentialVersion) domain.ProviderCredentialVersion {
	return domain.ProviderCredentialVersion{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ConnectionKey: value.ConnectionKey,
		Revision: value.Revision, ProviderKey: value.ProviderKey, CipherSuite: value.CipherSuite,
		KeyID: value.KeyID, Nonce: append([]byte(nil), value.Nonce...), Ciphertext: append([]byte(nil), value.Ciphertext...),
		SecretFingerprint: value.SecretFingerprint, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func providerConnectionRecord(value domain.ProviderConnectionVersion) (model.ProviderConnectionVersion, error) {
	ids, err := parseProviderConfigurationUUIDs(value.ID, value.WorkspaceID, value.CredentialVersionID, value.CreatedBy)
	if err != nil {
		return model.ProviderConnectionVersion{}, err
	}
	config, err := json.Marshal(value.ResolvedConfig)
	if err != nil {
		return model.ProviderConnectionVersion{}, err
	}
	return model.ProviderConnectionVersion{
		ID: ids[0], WorkspaceID: ids[1], ConnectionKey: value.ConnectionKey, Revision: value.Revision,
		SourcePresetKey: value.SourcePresetKey, SourcePresetVersion: value.SourcePresetVersion,
		PresetSnapshotHash: value.PresetSnapshotHash, ProviderKey: value.ProviderKey, DisplayName: value.DisplayName,
		CredentialVersionID: ids[2], ResolvedConfig: datatypes.JSON(config), State: value.State,
		AdapterContractVersion: value.AdapterContractVersion, ContentHash: value.ContentHash,
		CreatedBy: ids[3], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func providerConnectionDomain(value model.ProviderConnectionVersion) (domain.ProviderConnectionVersion, error) {
	config := map[string]any{}
	if err := json.Unmarshal(value.ResolvedConfig, &config); err != nil {
		return domain.ProviderConnectionVersion{}, fmt.Errorf("decode Media Provider connection config: %w", err)
	}
	return domain.ProviderConnectionVersion{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ConnectionKey: value.ConnectionKey,
		Revision: value.Revision, SourcePresetKey: value.SourcePresetKey, SourcePresetVersion: value.SourcePresetVersion,
		PresetSnapshotHash: value.PresetSnapshotHash, ProviderKey: value.ProviderKey, DisplayName: value.DisplayName,
		CredentialVersionID: value.CredentialVersionID.String(), ResolvedConfig: config, State: value.State,
		AdapterContractVersion: value.AdapterContractVersion, ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func providerProfileRecord(value domain.ProviderModelProfileVersion) (model.ProviderModelProfileVersion, error) {
	ids, err := parseProviderConfigurationUUIDs(value.ID, value.WorkspaceID, value.CreatedBy)
	if err != nil {
		return model.ProviderModelProfileVersion{}, err
	}
	creationSource, err := json.Marshal(value.CreationSource)
	if err != nil {
		return model.ProviderModelProfileVersion{}, err
	}
	defaults, err := json.Marshal(value.Defaults)
	if err != nil {
		return model.ProviderModelProfileVersion{}, err
	}
	return model.ProviderModelProfileVersion{
		ID: ids[0], WorkspaceID: ids[1], ProfileKey: value.ProfileKey, Revision: value.Revision,
		CreationSource: datatypes.JSON(creationSource), ConnectionKey: value.ConnectionKey,
		ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID, Modality: value.Modality,
		Family: value.Family, AdapterTransportContract: value.AdapterTransportContract,
		CapabilitySchemaVersion: value.CapabilitySchemaVersion, BillingMetric: value.BillingMetric,
		Defaults: datatypes.JSON(defaults), State: value.State, ContentHash: value.ContentHash,
		CreatedBy: ids[2], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func providerProfileDomain(value model.ProviderModelProfileVersion) (domain.ProviderModelProfileVersion, error) {
	creationSource, defaults := map[string]any{}, map[string]any{}
	if err := json.Unmarshal(value.CreationSource, &creationSource); err != nil {
		return domain.ProviderModelProfileVersion{}, fmt.Errorf("decode Media Provider model creation source: %w", err)
	}
	if err := json.Unmarshal(value.Defaults, &defaults); err != nil {
		return domain.ProviderModelProfileVersion{}, fmt.Errorf("decode Media Provider model defaults: %w", err)
	}
	return domain.ProviderModelProfileVersion{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProfileKey: value.ProfileKey,
		Revision: value.Revision, CreationSource: creationSource, ConnectionKey: value.ConnectionKey,
		ProviderKey: value.ProviderKey, ExternalModelID: value.ExternalModelID, Modality: value.Modality,
		Family: value.Family, AdapterTransportContract: value.AdapterTransportContract,
		CapabilitySchemaVersion: value.CapabilitySchemaVersion, BillingMetric: value.BillingMetric,
		Defaults: defaults, State: value.State, ContentHash: value.ContentHash,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func projectProviderBindingRecord(value domain.ProjectProviderBindingVersion) (model.ProjectProviderBindingVersion, error) {
	ids, err := parseProviderConfigurationUUIDs(
		value.ID, value.WorkspaceID, value.ProjectID, value.ConnectionVersionID,
		value.CredentialVersionID, value.ModelProfileVersionID, value.CreatedBy,
	)
	if err != nil {
		return model.ProjectProviderBindingVersion{}, err
	}
	return model.ProjectProviderBindingVersion{
		ID: ids[0], WorkspaceID: ids[1], ProjectID: ids[2], Purpose: value.Purpose, Revision: value.Revision,
		ConnectionVersionID: ids[3], CredentialVersionID: ids[4], ModelProfileVersionID: ids[5],
		ProviderKey: value.ProviderKey, Modality: value.Modality,
		AdapterContractVersion: value.AdapterContractVersion, ContentHash: value.ContentHash,
		CreatedBy: ids[6], CreatedAt: value.CreatedAt.UTC(),
	}, nil
}

func projectProviderBindingDomain(value model.ProjectProviderBindingVersion) domain.ProjectProviderBindingVersion {
	return domain.ProjectProviderBindingVersion{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		Purpose: value.Purpose, Revision: value.Revision, ConnectionVersionID: value.ConnectionVersionID.String(),
		CredentialVersionID: value.CredentialVersionID.String(), ModelProfileVersionID: value.ModelProfileVersionID.String(),
		ProviderKey: value.ProviderKey, Modality: value.Modality, AdapterContractVersion: value.AdapterContractVersion,
		ContentHash: value.ContentHash, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt.UTC(),
	}
}

func parseProviderConfigurationUUIDs(values ...string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		result[index] = parsed
	}
	return result, nil
}

func normalizeProviderConfigurationNotFound(err, target error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return target
	}
	return err
}

func normalizeProviderWorkspaceNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound("Workspace not found")
	}
	return err
}
