package generation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	providersecret "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/secretstore"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	testgorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
	"github.com/google/uuid"
)

type controlledMediaFactory struct {
	descriptor generationapp.MediaFactoryDescriptor
}

type gatedProviderConfigurationTransactions struct {
	delegate            generationapp.ProviderConfigurationTransactionManager
	afterOwnerLock      chan struct{}
	releaseOwnerLock    chan struct{}
	beforeWorkspaceLock chan struct{}
	ownerOnce           sync.Once
	workspaceOnce       sync.Once
}

type gatedProviderConfigurationRepository struct {
	generationapp.ProviderConfigurationRepository
	gate *gatedProviderConfigurationTransactions
}

func (transactions *gatedProviderConfigurationTransactions) WithinProviderConfigurationTransaction(
	ctx context.Context,
	operation func(generationapp.ProviderConfigurationRepository) error,
) error {
	return transactions.delegate.WithinProviderConfigurationTransaction(ctx, func(
		repository generationapp.ProviderConfigurationRepository,
	) error {
		return operation(&gatedProviderConfigurationRepository{
			ProviderConfigurationRepository: repository,
			gate:                            transactions,
		})
	})
}

func (repository *gatedProviderConfigurationRepository) AuthorizeWorkspaceOwner(
	ctx context.Context,
	actor generationapp.Actor,
	workspaceID string,
) error {
	if err := repository.ProviderConfigurationRepository.AuthorizeWorkspaceOwner(ctx, actor, workspaceID); err != nil {
		return err
	}
	if repository.gate.afterOwnerLock != nil {
		repository.gate.ownerOnce.Do(func() { close(repository.gate.afterOwnerLock) })
	}
	if repository.gate.releaseOwnerLock != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-repository.gate.releaseOwnerLock:
		}
	}
	return nil
}

func (repository *gatedProviderConfigurationRepository) LockProviderWorkspace(
	ctx context.Context,
	workspaceID string,
) error {
	if repository.gate.beforeWorkspaceLock != nil {
		repository.gate.workspaceOnce.Do(func() { close(repository.gate.beforeWorkspaceLock) })
	}
	return repository.ProviderConfigurationRepository.LockProviderWorkspace(ctx, workspaceID)
}

func (factory controlledMediaFactory) Descriptor() generationapp.MediaFactoryDescriptor {
	return factory.descriptor
}

func TestMediaPresetCatalogOnlyExposesRegisteredFactories(t *testing.T) {
	registry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{controlledMediaFactory{
		descriptor: generationapp.MediaFactoryDescriptor{
			ProviderKey: "openai", Modality: generationdomain.MediaModalityImage,
			AdapterContractVersion: "openai-image-api",
		},
	}})
	if err != nil {
		t.Fatalf("create media factory registry: %v", err)
	}
	catalog, err := generationapp.NewMediaPresetCatalog(generationapp.BuiltinMediaPresets(), registry)
	if err != nil {
		t.Fatalf("create media preset catalog: %v", err)
	}

	view := catalog.List()
	if len(view.Connections) != 1 || view.Connections[0].PresetKey != "openai.official-api" {
		t.Fatalf("available connection presets = %#v", view.Connections)
	}
	if len(view.Models) != 1 || view.Models[0].PresetKey != "openai.gpt-image-2" {
		t.Fatalf("available model presets = %#v", view.Models)
	}
	resolvedFactory, err := registry.Resolve("openai", generationdomain.MediaModalityImage, "openai-image-api")
	if err != nil || resolvedFactory == nil {
		t.Fatalf("resolve registered media factory: factory=%T err=%v", resolvedFactory, err)
	}
	for _, preset := range append(view.Connections, view.Models...) {
		if preset.ProviderKey == "volcengine_ark" || preset.ProviderKey == "google_gemini" {
			t.Fatalf("catalog exposed a preset without a registered factory: %#v", preset)
		}
	}
	emptyRegistry, err := generationapp.NewMediaFactoryRegistry(nil)
	if err != nil {
		t.Fatalf("create empty media factory registry: %v", err)
	}
	emptyCatalog, err := generationapp.NewMediaPresetCatalog(generationapp.BuiltinMediaPresets(), emptyRegistry)
	if err != nil {
		t.Fatalf("create zero-configuration media preset catalog: %v", err)
	}
	if emptyView := emptyCatalog.List(); len(emptyView.Connections) != 0 || len(emptyView.Models) != 0 {
		t.Fatalf("zero-factory catalog exposed unavailable presets: %#v", emptyView)
	}
	if _, err = emptyCatalog.ResolveConnection("openai.official-api", 1); !errors.Is(err, generationapp.ErrProviderPresetNotFound) {
		t.Fatalf("zero-factory catalog resolved an unavailable connection: %v", err)
	}
	if _, err = generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{
		controlledMediaFactory{descriptor: registryDescriptor("openai", generationdomain.MediaModalityImage, "openai-image-api")},
		controlledMediaFactory{descriptor: registryDescriptor("openai", generationdomain.MediaModalityImage, "openai-image-api")},
	}); err == nil {
		t.Fatal("media factory registry accepted a duplicated execution identity")
	}

	versionedPresets := generationapp.BuiltinMediaPresets()
	for _, preset := range slices.Clone(versionedPresets.Connections) {
		if preset.PresetKey == "openai.official-api" {
			preset.PresetVersion = 2
			preset.DisplayName = "OpenAI 官方图像 API"
			versionedPresets.Connections = append(versionedPresets.Connections, preset)
		}
	}
	for _, preset := range slices.Clone(versionedPresets.Models) {
		if preset.PresetKey == "openai.gpt-image-2" {
			preset.PresetVersion = 2
			versionedPresets.Models = append(versionedPresets.Models, preset)
		}
	}
	versionedCatalog, err := generationapp.NewMediaPresetCatalog(versionedPresets, registry)
	if err != nil {
		t.Fatalf("create versioned media preset catalog: %v", err)
	}
	if _, err = versionedCatalog.ResolveConnection("openai.official-api", 1); err != nil {
		t.Fatalf("resolve retained connection preset version: %v", err)
	}
	if _, err = versionedCatalog.ResolveConnection("openai.official-api", 2); err != nil {
		t.Fatalf("resolve latest connection preset version: %v", err)
	}
	versionedView := versionedCatalog.List()
	if len(versionedView.Connections) != 1 || versionedView.Connections[0].PresetVersion != 2 ||
		len(versionedView.Models) != 1 || versionedView.Models[0].PresetVersion != 2 {
		t.Fatalf("versioned catalog did not retain exact versions and expose latest summaries: %#v", versionedView)
	}

	mutablePresets := generationapp.BuiltinMediaPresets()
	for index := range mutablePresets.Connections {
		if mutablePresets.Connections[index].PresetKey == "openai.official-api" {
			mutablePresets.Connections[index].FixedConfig["nested"] = map[string]any{"marker": "original"}
		}
	}
	immutableCatalog, err := generationapp.NewMediaPresetCatalog(mutablePresets, registry)
	if err != nil {
		t.Fatalf("create immutable media preset catalog: %v", err)
	}
	for index := range mutablePresets.Connections {
		if mutablePresets.Connections[index].PresetKey == "openai.official-api" {
			mutablePresets.Connections[index].FixedConfig["nested"].(map[string]any)["marker"] = "source-mutated"
		}
	}
	firstResolved, err := immutableCatalog.ResolveConnection("openai.official-api", 1)
	if err != nil {
		t.Fatalf("resolve immutable connection preset: %v", err)
	}
	firstResolved.FixedConfig["nested"].(map[string]any)["marker"] = "result-mutated"
	secondResolved, err := immutableCatalog.ResolveConnection("openai.official-api", 1)
	if err != nil || secondResolved.FixedConfig["nested"].(map[string]any)["marker"] != "original" {
		t.Fatalf("media preset catalog leaked mutable state: preset=%#v err=%v", secondResolved, err)
	}
}

func registryDescriptor(providerKey, modality, contractVersion string) generationapp.MediaFactoryDescriptor {
	return generationapp.MediaFactoryDescriptor{
		ProviderKey: providerKey, Modality: modality, AdapterContractVersion: contractVersion,
	}
}

func TestProviderSecretStoreEncryptsWithBoundContextAndStableFingerprint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "provider-root-key")
	if err := os.WriteFile(root, bytes.Repeat([]byte{0x5a}, 32), 0o600); err != nil {
		t.Fatalf("write test root key: %v", err)
	}
	store := providersecret.Open(root)
	if !store.Available() {
		t.Fatal("valid root key did not make the secret store available")
	}
	secretContext := generationdomain.ProviderSecretContext{
		WorkspaceID: "019fb2e0-a000-7000-8000-000000000001",
		ProviderKey: "openai", CredentialID: "019fb2e0-a000-7000-8000-000000000002", Revision: 1,
	}
	plaintext := []byte(`{"api_key":"sk-test-only"}`)
	encrypted, err := store.Encrypt(context.Background(), secretContext, plaintext)
	if err != nil {
		t.Fatalf("encrypt Provider secret: %v", err)
	}
	if encrypted.CipherSuite != generationdomain.ProviderCipherAES256GCM || encrypted.KeyID == "" ||
		len(encrypted.Nonce) == 0 || len(encrypted.Ciphertext) == 0 || encrypted.Fingerprint == "" ||
		bytes.Contains(encrypted.Ciphertext, plaintext) {
		t.Fatalf("invalid encrypted Provider secret metadata: %#v", encrypted)
	}
	decrypted, err := store.Decrypt(context.Background(), secretContext, encrypted)
	if err != nil || !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypt Provider secret: plaintext=%q err=%v", decrypted, err)
	}
	repeated, err := store.Encrypt(context.Background(), secretContext, plaintext)
	if err != nil || repeated.Fingerprint != encrypted.Fingerprint || bytes.Equal(repeated.Ciphertext, encrypted.Ciphertext) {
		t.Fatalf("secret fingerprint/ciphertext behavior drifted: repeated=%#v err=%v", repeated, err)
	}
	tampered := secretContext
	tampered.WorkspaceID = "019fb2e0-a000-7000-8000-000000000003"
	if _, err = store.Decrypt(context.Background(), tampered, encrypted); !errors.Is(err, providersecret.ErrDecrypt) {
		t.Fatalf("cross-workspace ciphertext was accepted: %v", err)
	}
	invalidNonce := encrypted
	invalidNonce.Nonce = append(append([]byte(nil), encrypted.Nonce...), 0x01)
	if _, err = store.Decrypt(context.Background(), secretContext, invalidNonce); !errors.Is(err, providersecret.ErrDecrypt) {
		t.Fatalf("invalid AES-GCM nonce did not fail closed: %v", err)
	}
	tamperedCiphertext := encrypted
	tamperedCiphertext.Ciphertext = append([]byte(nil), encrypted.Ciphertext...)
	tamperedCiphertext.Ciphertext[0] ^= 0x01
	if _, err = store.Decrypt(context.Background(), secretContext, tamperedCiphertext); !errors.Is(err, providersecret.ErrDecrypt) {
		t.Fatalf("tampered AES-GCM ciphertext did not fail closed: %v", err)
	}
}

func TestProviderSecretStoreMissingOrWrongRootKeyFailsClosedWithoutBlockingOpen(t *testing.T) {
	missing := providersecret.Open(filepath.Join(t.TempDir(), "missing"))
	if missing.Available() {
		t.Fatal("missing Provider root key reported available")
	}
	_, err := missing.Encrypt(context.Background(), generationdomain.ProviderSecretContext{}, []byte("secret"))
	if !errors.Is(err, providersecret.ErrUnavailable) {
		t.Fatalf("missing root key error = %v", err)
	}
	for name, rootBytes := range map[string][]byte{
		"short":            bytes.Repeat([]byte{0x33}, 31),
		"long":             bytes.Repeat([]byte{0x34}, 33),
		"trailing newline": append(bytes.Repeat([]byte{0x44}, 32), '\n'),
	} {
		t.Run(name, func(t *testing.T) {
			invalidPath := filepath.Join(t.TempDir(), "invalid-root-key")
			if writeErr := os.WriteFile(invalidPath, rootBytes, 0o600); writeErr != nil {
				t.Fatalf("write invalid Provider root key: %v", writeErr)
			}
			if providersecret.Open(invalidPath).Available() {
				t.Fatal("invalid Provider root key reported available")
			}
		})
	}

	rootDirectory := t.TempDir()
	firstPath, secondPath := filepath.Join(rootDirectory, "first"), filepath.Join(rootDirectory, "second")
	if err = os.WriteFile(firstPath, bytes.Repeat([]byte{0x11}, 32), 0o600); err != nil {
		t.Fatalf("write first root key: %v", err)
	}
	if err = os.WriteFile(secondPath, bytes.Repeat([]byte{0x22}, 32), 0o600); err != nil {
		t.Fatalf("write second root key: %v", err)
	}
	secretContext := generationdomain.ProviderSecretContext{
		WorkspaceID: "019fb2e0-a000-7000-8000-000000000001", ProviderKey: "openai",
		CredentialID: "019fb2e0-a000-7000-8000-000000000002", Revision: 1,
	}
	encrypted, err := providersecret.Open(firstPath).Encrypt(context.Background(), secretContext, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt with first root key: %v", err)
	}
	if _, err = providersecret.Open(secondPath).Decrypt(context.Background(), secretContext, encrypted); !errors.Is(err, providersecret.ErrDecrypt) {
		t.Fatalf("wrong root key error = %v", err)
	}
}

func TestProviderConfigurationVersionsAreOwnerOnlyImmutableRestartSafeAndSecretFree(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Provider configuration journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Provider configuration database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Provider configuration catalog: %v", err)
	}
	for _, fact := range []any{
		&model.ProviderCredentialVersion{}, &model.ProviderConnectionVersion{},
		&model.ProviderModelProfileVersion{}, &model.ProjectProviderBindingVersion{},
	} {
		if !database.Migrator().HasTable(fact) {
			t.Fatalf("GORM catalog did not synchronize Provider fact %T", fact)
		}
	}
	if database.Migrator().HasTable("gen_provider_binding_versions") {
		t.Fatal("empty GORM catalog synchronized the removed fixed Provider binding table")
	}

	now := time.Date(2026, time.August, 29, 15, 0, 0, 0, time.UTC)
	workspaceID, projectID, ownerID, editorID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	records := []any{
		&[]model.UserAccount{
			{ID: ownerID, EmailNormalized: "provider-owner-" + ownerID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Owner", Status: "active", CreatedAt: now, UpdatedAt: now},
			{ID: editorID, EmailNormalized: "provider-editor-" + editorID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Editor", Status: "active", CreatedAt: now, UpdatedAt: now},
		},
		&model.Workspace{ID: workspaceID, Name: "Provider config", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&[]model.Membership{
			{ID: uuid.New(), WorkspaceID: workspaceID, UserID: ownerID, Role: "owner", Status: "active", JoinedAt: now},
			{ID: uuid.New(), WorkspaceID: workspaceID, UserID: editorID, Role: "editor", Status: "active", JoinedAt: now},
		},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Provider config", AspectRatio: "16:9", Language: "zh-CN", TargetDurationMS: 60000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	for _, record := range records {
		if err = database.Create(record).Error; err != nil {
			t.Fatalf("seed Provider configuration fixture %T: %v", record, err)
		}
	}
	testgorm.RegisterOwnedWorkspaceFixtureCleanup(t, database, testgorm.OwnedWorkspaceFixture{
		UserIDs:     []string{ownerID.String(), editorID.String()},
		WorkspaceID: workspaceID.String(),
	})
	root := filepath.Join(t.TempDir(), "provider-root-key")
	if err = os.WriteFile(root, bytes.Repeat([]byte{0x6b}, 32), 0o600); err != nil {
		t.Fatalf("write Provider root key: %v", err)
	}
	registry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{controlledMediaFactory{
		descriptor: generationapp.MediaFactoryDescriptor{
			ProviderKey: "openai", Modality: generationdomain.MediaModalityImage,
			AdapterContractVersion: "openai-image-api",
		},
	}})
	if err != nil {
		t.Fatalf("create Provider registry: %v", err)
	}
	catalog, err := generationapp.NewMediaPresetCatalog(generationapp.BuiltinMediaPresets(), registry)
	if err != nil {
		t.Fatalf("create Provider catalog: %v", err)
	}
	configurationStore := generationgorm.NewProviderConfigurationStore(database)
	configuration := generationapp.NewProviderConfigurationService(
		configurationStore, catalog, providersecret.Open(root),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	owner := generationapp.Actor{UserID: ownerID.String(), TokenVersion: 1}
	editor := generationapp.Actor{UserID: editorID.String(), TokenVersion: 1}
	connectionCommand := generationapp.CreateProviderConnectionCommand{
		WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", PresetKey: "openai.official-api",
		PresetVersion: 1, ExpectedRevision: 0, DisplayName: "OpenAI primary",
		Credentials: map[string]string{"api_key": "sk-provider-config-test"}, IdempotencyKey: "provider-connection-create",
	}
	tamperedConnection := connectionCommand
	tamperedConnection.ConnectionKey = "openai-tampered"
	tamperedConnection.ConfigOverrides = map[string]any{"base_url": "https://attacker.example.test"}
	tamperedConnection.IdempotencyKey = "provider-connection-tampered"
	if _, err = configuration.CreateConnection(ctx, owner, tamperedConnection); generationErrorCode(err) != "invalid_request" {
		t.Fatalf("Provider connection accepted an arbitrary URL override: %T %v", err, err)
	}
	missingFactoryConnection := connectionCommand
	missingFactoryConnection.ConnectionKey = "google-unavailable"
	missingFactoryConnection.PresetKey = "google.gemini-api"
	missingFactoryConnection.IdempotencyKey = "provider-connection-missing-factory"
	if _, err = configuration.CreateConnection(ctx, owner, missingFactoryConnection); generationErrorCode(err) != "invalid_request" {
		t.Fatalf("Provider connection accepted a preset without a Factory: %T %v", err, err)
	}
	unknownPresetConnection := connectionCommand
	unknownPresetConnection.ConnectionKey = "openai-unknown"
	unknownPresetConnection.PresetKey = "openai.unknown-api"
	unknownPresetConnection.IdempotencyKey = "provider-connection-unknown-preset"
	if _, err = configuration.CreateConnection(ctx, owner, unknownPresetConnection); generationErrorCode(err) != "invalid_request" {
		t.Fatalf("Provider connection accepted an unknown preset: %T %v", err, err)
	}
	connection, err := configuration.CreateConnection(ctx, owner, connectionCommand)
	if err != nil || connection.Connection.Revision != 1 || connection.Credential.Revision != 1 ||
		connection.Connection.CredentialVersionID != connection.Credential.ID ||
		len(connection.Credential.Fingerprint) != 12 {
		t.Fatalf("create Provider connection: result=%#v err=%v", connection, err)
	}
	if strings.Contains(connection.Credential.Fingerprint, "sk-") {
		t.Fatalf("Provider credential view leaked plaintext: %#v", connection.Credential)
	}
	legacyInputHash, hashErr := platformcommand.InputHash(struct {
		ActorID string
		Command generationapp.CreateProviderConnectionCommand
	}{ActorID: owner.UserID, Command: connectionCommand})
	if hashErr != nil {
		t.Fatalf("compute legacy Provider command hash: %v", hashErr)
	}
	if connection.Receipt.InputHash == legacyInputHash {
		t.Fatal("Provider command receipt persisted a bare SHA-256 derived from plaintext credentials")
	}
	replay, err := configuration.CreateConnection(ctx, owner, connectionCommand)
	if err != nil || replay.Connection.ID != connection.Connection.ID || replay.Receipt.ID != connection.Receipt.ID {
		t.Fatalf("replay Provider connection: result=%#v err=%v", replay, err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("id = ?", connection.Receipt.ID).
		UpdateColumn("resource_id", uuid.New()).Error; err != nil {
		t.Fatalf("inject Provider command receipt drift: %v", err)
	}
	if _, err = configuration.CreateConnection(ctx, owner, connectionCommand); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider command replay accepted a drifted receipt resource: %T %v", err, err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where("id = ?", connection.Receipt.ID).
		UpdateColumn("resource_id", connection.Connection.ID).Error; err != nil {
		t.Fatalf("restore Provider command receipt after drift test: %v", err)
	}
	retiredPresetCatalog, err := generationapp.NewMediaPresetCatalog(generationdomain.MediaPresets{}, registry)
	if err != nil {
		t.Fatalf("create retired Provider preset catalog: %v", err)
	}
	retiredPresetService := generationapp.NewProviderConfigurationService(
		configurationStore,
		retiredPresetCatalog,
		providersecret.Open(root),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	retiredReplay, err := retiredPresetService.CreateConnection(ctx, owner, connectionCommand)
	if err != nil || retiredReplay.Connection.ID != connection.Connection.ID ||
		retiredReplay.Receipt.ID != connection.Receipt.ID {
		t.Fatalf("replay Provider connection after preset retirement: result=%#v err=%v", retiredReplay, err)
	}
	forbiddenCommand := connectionCommand
	forbiddenCommand.ConnectionKey, forbiddenCommand.IdempotencyKey = "editor-connection", "provider-editor-create"
	if _, err = configuration.CreateConnection(ctx, editor, forbiddenCommand); generationErrorCode(err) != "forbidden" {
		t.Fatalf("editor created Provider connection: %T %v", err, err)
	}
	missingSecretService := generationapp.NewProviderConfigurationService(
		generationgorm.NewProviderConfigurationStore(database), catalog,
		providersecret.Open(filepath.Join(t.TempDir(), "missing-root")),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	missingCommand := connectionCommand
	missingCommand.ConnectionKey, missingCommand.IdempotencyKey = "openai-missing-root", "provider-missing-root"
	if _, err = missingSecretService.CreateConnection(ctx, owner, missingCommand); generationErrorCode(err) != "secret_store_unavailable" {
		t.Fatalf("missing root key Provider command error: %T %v", err, err)
	}

	profileCommand := generationapp.CreateProviderModelProfileCommand{
		WorkspaceID: workspaceID.String(), ProfileKey: "gpt-image-main", ConnectionKey: "openai-primary",
		PresetKey: "openai.gpt-image-2", PresetVersion: 1, ExpectedRevision: 0,
		IdempotencyKey: "provider-profile-create",
	}
	profile, err := configuration.CreateModelProfile(ctx, owner, profileCommand)
	if err != nil || profile.Profile.ExternalModelID != "gpt-image-2" || profile.Profile.Modality != "image" {
		t.Fatalf("create Provider model profile: result=%#v err=%v", profile, err)
	}
	tamperedProfileCommand := profileCommand
	tamperedProfileCommand.ProfileKey = "gpt-image-tampered"
	tamperedProfileCommand.ExternalModelID = "attacker-model"
	tamperedProfileCommand.IdempotencyKey = "provider-profile-tampered-model"
	if _, err = configuration.CreateModelProfile(ctx, owner, tamperedProfileCommand); generationErrorCode(err) != "invalid_request" {
		t.Fatalf("fixed Provider model preset accepted a different external model: %T %v", err, err)
	}
	retiredProfileReplay, err := retiredPresetService.CreateModelProfile(ctx, owner, profileCommand)
	if err != nil || retiredProfileReplay.Profile.ID != profile.Profile.ID ||
		retiredProfileReplay.Receipt.ID != profile.Receipt.ID {
		t.Fatalf("replay Provider profile after preset retirement: result=%#v err=%v", retiredProfileReplay, err)
	}
	binding, err := configuration.PublishProjectBinding(ctx, owner, generationapp.PublishProjectProviderBindingCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
		ConnectionVersionID: connection.Connection.ID, ModelProfileVersionID: profile.Profile.ID,
		ExpectedRevision: 0, IdempotencyKey: "provider-binding-publish",
	})
	if err != nil || binding.Binding.Revision != 1 ||
		binding.Binding.CredentialVersionID != connection.Credential.ID || binding.Binding.Modality != "image" {
		t.Fatalf("publish Project Provider binding: result=%#v err=%v", binding, err)
	}
	if _, err = configuration.ResolveProjectBinding(ctx, editor, projectID.String(), "reference_asset"); err != nil {
		t.Fatalf("authorized editor could not resolve executable Provider binding: %v", err)
	}
	if _, err = configuration.ListProjectBindings(ctx, editor, projectID.String()); generationErrorCode(err) != "forbidden" {
		t.Fatalf("editor listed Owner Provider configuration: %T %v", err, err)
	}
	if _, err = configuration.PublishProjectBinding(ctx, editor, generationapp.PublishProjectProviderBindingCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
		ConnectionVersionID: connection.Connection.ID, ModelProfileVersionID: profile.Profile.ID,
		ExpectedRevision: 1, ExpectedContentHash: binding.Binding.ContentHash,
		IdempotencyKey: "provider-binding-editor",
	}); generationErrorCode(err) != "forbidden" {
		t.Fatalf("editor published a Project Provider binding: %T %v", err, err)
	}

	restarted := generationapp.NewProviderConfigurationService(
		generationgorm.NewProviderConfigurationStore(database), catalog, providersecret.Open(root),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	resolved, err := restarted.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset")
	if err != nil || resolved.Binding.ID != binding.Binding.ID || resolved.Credential.ID != connection.Credential.ID {
		t.Fatalf("resolve Provider binding after restart: result=%#v err=%v", resolved, err)
	}
	persistedCredential := mustProviderCredential(t, configurationStore, connection.Credential.ID)
	credentialValues, err := decryptProviderCredential(providersecret.Open(root), persistedCredential)
	if err != nil || credentialValues["api_key"] != "sk-provider-config-test" {
		t.Fatalf("decrypt Provider credential after restart: values=%#v err=%v", credentialValues, err)
	}
	wrongRoot := filepath.Join(t.TempDir(), "wrong-root")
	if err = os.WriteFile(wrongRoot, bytes.Repeat([]byte{0x7c}, 32), 0o600); err != nil {
		t.Fatalf("write wrong Provider root key: %v", err)
	}
	wrongRootStore := providersecret.Open(wrongRoot)
	wrongKeyService := generationapp.NewProviderConfigurationService(
		generationgorm.NewProviderConfigurationStore(database), catalog, wrongRootStore,
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	if _, err = missingSecretService.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset"); generationErrorCode(err) != "secret_store_unavailable" {
		t.Fatalf("missing Provider root key did not close the execution resolver: %T %v", err, err)
	}
	if _, err = wrongKeyService.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset"); generationErrorCode(err) != "secret_store_unavailable" {
		t.Fatalf("wrong Provider root key did not close the execution resolver: %T %v", err, err)
	}
	if _, err = decryptProviderCredential(wrongRootStore, persistedCredential); !errors.Is(err, providersecret.ErrDecrypt) {
		t.Fatalf("wrong Provider root key error: %T %v", err, err)
	}
	if _, err = configuration.RotateCredential(ctx, owner, generationapp.RotateProviderCredentialCommand{
		WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", ExpectedRevision: 9,
		ExpectedContentHash: connection.Connection.ContentHash,
		Credentials:         map[string]string{"api_key": "sk-provider-config-rotated"},
		IdempotencyKey:      "provider-credential-rotate-conflict",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider credential rotation accepted stale revision: %T %v", err, err)
	}
	if _, err = configuration.RotateCredential(ctx, owner, generationapp.RotateProviderCredentialCommand{
		WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", ExpectedRevision: 1,
		ExpectedContentHash: strings.Repeat("0", 64),
		Credentials:         map[string]string{"api_key": "sk-provider-config-rotated"},
		IdempotencyKey:      "provider-credential-rotate-hash-conflict",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider credential rotation accepted stale content hash: %T %v", err, err)
	}
	if _, err = configuration.RotateCredential(ctx, editor, generationapp.RotateProviderCredentialCommand{
		WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", ExpectedRevision: 1,
		ExpectedContentHash: connection.Connection.ContentHash,
		Credentials:         map[string]string{"api_key": "sk-provider-config-editor"},
		IdempotencyKey:      "provider-credential-rotate-editor",
	}); generationErrorCode(err) != "forbidden" {
		t.Fatalf("editor rotated a Provider credential: %T %v", err, err)
	}
	tamperedPresets := generationapp.BuiltinMediaPresets()
	for index := range tamperedPresets.Connections {
		if tamperedPresets.Connections[index].PresetKey == "openai.official-api" {
			tamperedPresets.Connections[index].DisplayName = "Tampered OpenAI preset"
		}
	}
	tamperedCatalog, catalogErr := generationapp.NewMediaPresetCatalog(tamperedPresets, registry)
	if catalogErr != nil {
		t.Fatalf("create tampered Provider catalog: %v", catalogErr)
	}
	tamperedPresetService := generationapp.NewProviderConfigurationService(
		configurationStore, tamperedCatalog, providersecret.Open(root),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	if _, err = tamperedPresetService.RotateCredential(ctx, owner, generationapp.RotateProviderCredentialCommand{
		WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", ExpectedRevision: 1,
		ExpectedContentHash: connection.Connection.ContentHash,
		Credentials:         map[string]string{"api_key": "sk-provider-config-rotated"},
		IdempotencyKey:      "provider-credential-rotate-preset-drift",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider credential rotation accepted a drifted preset snapshot: %T %v", err, err)
	}
	rotated, err := wrongKeyService.RotateCredential(ctx, owner, generationapp.RotateProviderCredentialCommand{
		WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", ExpectedRevision: 1,
		ExpectedContentHash: connection.Connection.ContentHash,
		Credentials:         map[string]string{"api_key": "sk-provider-config-rotated"},
		IdempotencyKey:      "provider-credential-rotate",
	})
	if err != nil || rotated.Connection.Revision != 2 || rotated.Credential.Revision != 2 ||
		rotated.Connection.CredentialVersionID != rotated.Credential.ID {
		t.Fatalf("rotate Provider credential: result=%#v err=%v", rotated, err)
	}
	if _, err = configuration.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset"); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("stale Project binding accepted rotated connection: %T %v", err, err)
	}
	if _, err = configuration.PublishProjectBinding(ctx, owner, generationapp.PublishProjectProviderBindingCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
		ConnectionVersionID: connection.Connection.ID, ModelProfileVersionID: profile.Profile.ID,
		ExpectedRevision: 1, ExpectedContentHash: binding.Binding.ContentHash,
		IdempotencyKey: "provider-binding-stale-connection",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Project binding accepted a superseded Provider connection: %T %v", err, err)
	}
	rotatedBinding, err := configuration.PublishProjectBinding(ctx, owner, generationapp.PublishProjectProviderBindingCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
		ConnectionVersionID: rotated.Connection.ID, ModelProfileVersionID: profile.Profile.ID,
		ExpectedRevision: 1, ExpectedContentHash: binding.Binding.ContentHash,
		IdempotencyKey: "provider-binding-credential-rotation",
	})
	if err != nil || rotatedBinding.Binding.Revision != 2 {
		t.Fatalf("republish Project binding after credential rotation: result=%#v err=%v", rotatedBinding, err)
	}
	if _, err = configuration.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset"); generationErrorCode(err) != "secret_store_unavailable" {
		t.Fatalf("old Provider root key accepted the re-entered credential: %T %v", err, err)
	}
	resolvedRotatedBinding, err := wrongKeyService.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset")
	if err != nil || resolvedRotatedBinding.Binding.ID != rotatedBinding.Binding.ID || resolvedRotatedBinding.Credential.ID != rotated.Credential.ID {
		t.Fatalf("resolve rotated Provider credential: result=%#v err=%v", resolvedRotatedBinding, err)
	}
	rotatedValues, err := decryptProviderCredential(
		wrongRootStore,
		mustProviderCredential(t, configurationStore, rotated.Credential.ID),
	)
	if err != nil || rotatedValues["api_key"] != "sk-provider-config-rotated" {
		t.Fatalf("decrypt re-entered Provider credential: values=%#v err=%v", rotatedValues, err)
	}
	if _, err = configuration.SetModelProfileState(ctx, editor, generationapp.SetProviderModelProfileStateCommand{
		WorkspaceID: workspaceID.String(), ProfileKey: "gpt-image-main", State: "disabled",
		ExpectedRevision: 1, ExpectedContentHash: profile.Profile.ContentHash,
		IdempotencyKey: "provider-profile-disable-editor",
	}); generationErrorCode(err) != "forbidden" {
		t.Fatalf("editor disabled a Provider model profile: %T %v", err, err)
	}
	disabledProfile, err := configuration.SetModelProfileState(ctx, owner, generationapp.SetProviderModelProfileStateCommand{
		WorkspaceID: workspaceID.String(), ProfileKey: "gpt-image-main", State: "disabled",
		ExpectedRevision: 1, ExpectedContentHash: profile.Profile.ContentHash,
		IdempotencyKey: "provider-profile-disable",
	})
	if err != nil || disabledProfile.Profile.Revision != 2 || disabledProfile.Profile.State != "disabled" {
		t.Fatalf("disable Provider model profile: result=%#v err=%v", disabledProfile, err)
	}
	disabledReplay, err := configuration.SetModelProfileState(ctx, owner, generationapp.SetProviderModelProfileStateCommand{
		WorkspaceID: workspaceID.String(), ProfileKey: "gpt-image-main", State: "disabled",
		ExpectedRevision: 1, ExpectedContentHash: profile.Profile.ContentHash,
		IdempotencyKey: "provider-profile-disable",
	})
	if err != nil || disabledReplay.Profile.ID != disabledProfile.Profile.ID || disabledReplay.Receipt.ID != disabledProfile.Receipt.ID {
		t.Fatalf("replay Provider model profile disable: result=%#v err=%v", disabledReplay, err)
	}
	if _, err = wrongKeyService.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset"); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("disabled Provider model profile remained executable: %T %v", err, err)
	}
	if _, err = configuration.PublishProjectBinding(ctx, owner, generationapp.PublishProjectProviderBindingCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
		ConnectionVersionID: rotated.Connection.ID, ModelProfileVersionID: profile.Profile.ID,
		ExpectedRevision: 2, ExpectedContentHash: rotatedBinding.Binding.ContentHash,
		IdempotencyKey: "provider-binding-stale-profile",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Project binding accepted a superseded Provider model profile: %T %v", err, err)
	}
	enabledProfile, err := configuration.SetModelProfileState(ctx, owner, generationapp.SetProviderModelProfileStateCommand{
		WorkspaceID: workspaceID.String(), ProfileKey: "gpt-image-main", State: "enabled",
		ExpectedRevision: 2, ExpectedContentHash: disabledProfile.Profile.ContentHash,
		IdempotencyKey: "provider-profile-enable",
	})
	if err != nil || enabledProfile.Profile.Revision != 3 || enabledProfile.Profile.State != "enabled" {
		t.Fatalf("enable Provider model profile: result=%#v err=%v", enabledProfile, err)
	}
	bindingAfterProfileEnable, err := configuration.PublishProjectBinding(ctx, owner, generationapp.PublishProjectProviderBindingCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
		ConnectionVersionID: rotated.Connection.ID, ModelProfileVersionID: enabledProfile.Profile.ID,
		ExpectedRevision: 2, ExpectedContentHash: rotatedBinding.Binding.ContentHash,
		IdempotencyKey: "provider-binding-publish-after-profile-enable",
	})
	if err != nil || bindingAfterProfileEnable.Binding.Revision != 3 || bindingAfterProfileEnable.Binding.ModelProfileVersionID != enabledProfile.Profile.ID {
		t.Fatalf("publish Project binding after Profile enable: result=%#v err=%v", bindingAfterProfileEnable, err)
	}
	listed, err := configuration.ListWorkspaceConfiguration(ctx, owner, workspaceID.String())
	if err != nil || len(listed.Connections) != 1 || listed.Connections[0].Connection.ID != rotated.Connection.ID ||
		len(listed.Profiles) != 1 || listed.Profiles[0].ID != enabledProfile.Profile.ID ||
		listed.Connections[0].Credential.Fingerprint == "" {
		t.Fatalf("list latest Provider configuration: result=%#v err=%v", listed, err)
	}
	disableGate := &gatedProviderConfigurationTransactions{
		delegate: configurationStore, afterOwnerLock: make(chan struct{}), releaseOwnerLock: make(chan struct{}),
	}
	publishGate := &gatedProviderConfigurationTransactions{
		delegate: configurationStore, beforeWorkspaceLock: make(chan struct{}),
	}
	disableConfiguration := generationapp.NewProviderConfigurationService(
		disableGate, catalog, wrongRootStore,
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	publishConfiguration := generationapp.NewProviderConfigurationService(
		publishGate, catalog, wrongRootStore,
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	type connectionStateOutcome struct {
		result generationapp.ProviderConnectionResult
		err    error
	}
	raceContext, cancelRace := context.WithTimeout(ctx, 10*time.Second)
	defer cancelRace()
	disableDone := make(chan connectionStateOutcome, 1)
	go func() {
		result, disableErr := disableConfiguration.SetConnectionState(raceContext, owner, generationapp.SetProviderConnectionStateCommand{
			WorkspaceID: workspaceID.String(), ConnectionKey: "openai-primary", State: "disabled",
			ExpectedRevision: 2, ExpectedContentHash: rotated.Connection.ContentHash,
			IdempotencyKey: "provider-connection-disable",
		})
		disableDone <- connectionStateOutcome{result: result, err: disableErr}
	}()
	waitProviderConfigurationSignal(t, disableGate.afterOwnerLock, "connection disable Workspace lock")
	publishDone := make(chan error, 1)
	go func() {
		_, publishErr := publishConfiguration.PublishProjectBinding(raceContext, owner, generationapp.PublishProjectProviderBindingCommand{
			WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Purpose: "reference_asset",
			ConnectionVersionID: rotated.Connection.ID, ModelProfileVersionID: enabledProfile.Profile.ID,
			ExpectedRevision: 3, ExpectedContentHash: bindingAfterProfileEnable.Binding.ContentHash,
			IdempotencyKey: "provider-binding-racing-disable",
		})
		publishDone <- publishErr
	}()
	waitProviderConfigurationSignal(t, publishGate.beforeWorkspaceLock, "Project binding Workspace lock attempt")
	select {
	case publishErr := <-publishDone:
		t.Fatalf("Project binding bypassed the held Workspace lock: %T %v", publishErr, publishErr)
	default:
	}
	close(disableGate.releaseOwnerLock)
	var disabled connectionStateOutcome
	select {
	case disabled = <-disableDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Provider connection disable did not finish after releasing its Workspace lock")
	}
	if disabled.err != nil || disabled.result.Connection.Revision != 3 ||
		disabled.result.Connection.State != "disabled" {
		t.Fatalf("disable Provider connection: result=%#v err=%v", disabled.result, disabled.err)
	}
	var publishErr error
	select {
	case publishErr = <-publishDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Project binding publish did not finish after the racing disable")
	}
	if generationErrorCode(publishErr) != "state_conflict" {
		t.Fatalf("Project binding published a connection disabled ahead of its Workspace lock: %T %v", publishErr, publishErr)
	}
	var bindingRevisionCount int64
	if err = database.Model(&model.ProjectProviderBindingVersion{}).
		Where("workspace_id = ? AND project_id = ? AND purpose = ?", workspaceID, projectID, "reference_asset").
		Count(&bindingRevisionCount).Error; err != nil || bindingRevisionCount != 3 {
		t.Fatalf("racing Project binding appended facts: count=%d err=%v", bindingRevisionCount, err)
	}
	if _, err = wrongKeyService.ResolveProjectBinding(ctx, owner, projectID.String(), "reference_asset"); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("disabled Provider connection remained executable: %T %v", err, err)
	}

	connectionCommand.Credentials["api_key"] = "sk-input-drift"
	if _, err = configuration.CreateConnection(ctx, owner, connectionCommand); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider connection idempotency input drift was accepted: %T %v", err, err)
	}
	if bytes.Contains(mustCredentialCiphertext(t, configurationStore, connection.Credential.ID), []byte("sk-provider-config-test")) {
		t.Fatal("Provider credential plaintext was persisted")
	}
	immutableFacts := []struct {
		name        string
		value       any
		id          string
		field       string
		replacement any
		expected    error
	}{
		{"ProviderCredentialVersion", &model.ProviderCredentialVersion{}, connection.Credential.ID, "provider_key", "tampered", model.ErrImmutableProviderCredentialVersion},
		{"ProviderConnectionVersion", &model.ProviderConnectionVersion{}, connection.Connection.ID, "display_name", "Tampered", model.ErrImmutableProviderConnectionVersion},
		{"ProviderModelProfileVersion", &model.ProviderModelProfileVersion{}, profile.Profile.ID, "external_model_id", "tampered", model.ErrImmutableProviderModelProfileVersion},
		{"ProjectProviderBindingVersion", &model.ProjectProviderBindingVersion{}, binding.Binding.ID, "provider_key", "tampered", model.ErrImmutableProjectProviderBindingVersion},
	}
	for _, fact := range immutableFacts {
		if updateErr := database.Model(fact.value).Where("id = ?", fact.id).Update(fact.field, fact.replacement).Error; !errors.Is(updateErr, fact.expected) {
			t.Fatalf("%s update error = %v, want %v", fact.name, updateErr, fact.expected)
		}
		if deleteErr := database.Where("id = ?", fact.id).Delete(fact.value).Error; !errors.Is(deleteErr, fact.expected) {
			t.Fatalf("%s delete error = %v, want %v", fact.name, deleteErr, fact.expected)
		}
	}
}

func waitProviderConfigurationSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func mustCredentialCiphertext(
	t *testing.T,
	transactions generationapp.ProviderConfigurationTransactionManager,
	credentialID string,
) []byte {
	t.Helper()
	credential := mustProviderCredential(t, transactions, credentialID)
	return append([]byte(nil), credential.Ciphertext...)
}

func mustProviderCredential(
	t *testing.T,
	transactions generationapp.ProviderConfigurationTransactionManager,
	credentialID string,
) generationdomain.ProviderCredentialVersion {
	t.Helper()
	var credential generationdomain.ProviderCredentialVersion
	err := transactions.WithinProviderConfigurationTransaction(context.Background(), func(
		repository generationapp.ProviderConfigurationRepository,
	) error {
		persisted, findErr := repository.FindProviderCredential(context.Background(), credentialID)
		if findErr != nil {
			return findErr
		}
		credential = persisted
		return nil
	})
	if err != nil {
		t.Fatalf("load Provider credential: %v", err)
	}
	return credential
}

func decryptProviderCredential(
	store *providersecret.Store,
	credential generationdomain.ProviderCredentialVersion,
) (map[string]string, error) {
	plaintext, err := store.Decrypt(context.Background(), generationdomain.ProviderSecretContext{
		WorkspaceID:  credential.WorkspaceID,
		ProviderKey:  credential.ProviderKey,
		CredentialID: credential.ID,
		Revision:     credential.Revision,
		KeyID:        credential.KeyID,
	}, generationdomain.EncryptedProviderSecret{
		CipherSuite: credential.CipherSuite,
		KeyID:       credential.KeyID,
		Nonce:       credential.Nonce,
		Ciphertext:  credential.Ciphertext,
		Fingerprint: credential.SecretFingerprint,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		for index := range plaintext {
			plaintext[index] = 0
		}
	}()
	values := map[string]string{}
	if err = json.Unmarshal(plaintext, &values); err != nil {
		return nil, err
	}
	return values, nil
}
