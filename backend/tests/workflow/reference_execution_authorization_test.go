package workflow_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	providersecret "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/secretstore"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type referenceExecutionFixture struct {
	service       *generationapp.ReferenceExecutionAuthorizationService
	command       generationapp.AuthorizeInitialReferenceExecutionCommand
	authorization generationdomain.ReferenceExecutionAuthorization
	configuration *generationapp.ProviderConfigurationService
	connection    generationapp.ProviderConnectionResult
	profile       generationapp.ProviderModelProfileResult
	binding       generationapp.ProjectProviderBindingResult
}

// Only the descriptor is registered: this test cannot create a Provider runtime.
type referenceAuthorizationMediaFactory struct{}

func (referenceAuthorizationMediaFactory) Descriptor() generationapp.MediaFactoryDescriptor {
	return generationapp.MediaFactoryDescriptor{ProviderKey: "openai", Modality: "image", AdapterContractVersion: "openai-image-api"}
}

func assertInitialReferenceExecutionAuthorization(t *testing.T, ctx context.Context, transactions generationapp.ReferenceExecutionAuthorizationTransactions, configurationTransactions generationapp.ProviderConfigurationTransactionManager, actor generationapp.Actor, target generationapp.ReferenceGenerationTarget, now time.Time) referenceExecutionFixture {
	t.Helper()
	clock := func() time.Time { return now }
	service, err := generationapp.NewReferenceExecutionAuthorizationService(transactions, clock, uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	command := generationapp.AuthorizeInitialReferenceExecutionCommand{WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, TargetRef: generationdomain.GenerationRevisionRef{ID: target.ID, Revision: target.Revision, ContentHash: target.ContentHash}, IdempotencyKey: "reference-execution-initial"}
	if missing, err := service.AuthorizeInitial(ctx, actor, command); !generationapp.IsCode(err, "provider_configuration_required") || missing.ContentHash != "" {
		t.Fatalf("missing Provider configuration was not explicit: %v", err)
	}
	key := filepath.Join(t.TempDir(), "reference-execution-test-key")
	if err := os.WriteFile(key, bytes.Repeat([]byte{0x61}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := generationapp.NewMediaFactoryRegistry([]generationapp.MediaAdapterFactory{referenceAuthorizationMediaFactory{}})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := generationapp.NewMediaPresetCatalog(generationapp.BuiltinMediaPresets(), registry)
	if err != nil {
		t.Fatal(err)
	}
	configuration := generationapp.NewProviderConfigurationService(configurationTransactions, catalog, providersecret.Open(key), generationapp.ProviderConfigurationConfig{Now: clock, NewID: uuid.NewString})
	connection, err := configuration.CreateConnection(ctx, actor, generationapp.CreateProviderConnectionCommand{WorkspaceID: target.WorkspaceID, ConnectionKey: "reference-primary", PresetKey: "openai.official-api", PresetVersion: 1, DisplayName: "Reference authorization fixture", Credentials: map[string]string{"api_key": "sk-reference-execution-fixture"}, IdempotencyKey: "reference-connection"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := configuration.CreateModelProfile(ctx, actor, generationapp.CreateProviderModelProfileCommand{WorkspaceID: target.WorkspaceID, ProfileKey: "reference-image", ConnectionKey: "reference-primary", PresetKey: "openai.gpt-image-2", PresetVersion: 1, IdempotencyKey: "reference-profile"})
	if err != nil {
		t.Fatal(err)
	}
	currentBindings, err := configuration.ListProjectBindings(ctx, actor, target.ProjectID)
	if err != nil || len(currentBindings) != 1 || currentBindings[0].Purpose != "reference_asset" {
		t.Fatalf("read the existing Visual Foundation fixture binding: %v", err)
	}
	binding, err := configuration.PublishProjectBinding(ctx, actor, generationapp.PublishProjectProviderBindingCommand{WorkspaceID: target.WorkspaceID, ProjectID: target.ProjectID, Purpose: "reference_asset", ConnectionVersionID: connection.Connection.ID, ModelProfileVersionID: profile.Profile.ID, ExpectedRevision: currentBindings[0].Revision, ExpectedContentHash: currentBindings[0].ContentHash, IdempotencyKey: "reference-binding"})
	if err != nil {
		t.Fatal(err)
	}
	command.SelectedProviderBindingRef = generationdomain.GenerationRevisionRef{ID: binding.Binding.ID, Revision: binding.Binding.Revision, ContentHash: binding.Binding.ContentHash}
	for _, failedID := range []string{"invalid-receipt-id", target.GenerationAuthorizationRef.ID} {
		failedService, err := generationapp.NewReferenceExecutionAuthorizationService(transactions, clock, func() string { return failedID })
		if err != nil {
			t.Fatal(err)
		}
		if value, err := failedService.AuthorizeInitial(ctx, actor, command); err == nil || value.ContentHash != "" {
			t.Fatal("invalid or conflicting receipt identity authorized execution")
		}
	}
	authorized, err := service.AuthorizeInitial(ctx, actor, command)
	if err != nil || authorized.GenerationTargetRef != command.TargetRef || authorized.SelectedProjectProviderBindingVersionRef != command.SelectedProviderBindingRef {
		t.Fatalf("authorize initial Reference execution: %v", err)
	}
	replay, err := service.AuthorizeInitial(ctx, actor, command)
	if err != nil || !reflect.DeepEqual(replay, authorized) {
		t.Fatalf("initial execution authorization replay: %v", err)
	}
	raw, err := json.Marshal(authorized)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := generationdomain.DecodeReferenceExecutionAuthorization(raw); err != nil || !reflect.DeepEqual(decoded, authorized) || bytes.Contains(raw, []byte("sk-reference")) || bytes.Contains(raw, []byte("ciphertext")) || bytes.Contains(raw, []byte("base_url")) {
		t.Fatalf("execution authorization contract or secret boundary: %v", err)
	}
	for _, mutate := range []func(*generationapp.AuthorizeInitialReferenceExecutionCommand){
		func(v *generationapp.AuthorizeInitialReferenceExecutionCommand) {
			v.SelectedProviderBindingRef.ContentHash = strings.Repeat("f", 64)
		},
		func(v *generationapp.AuthorizeInitialReferenceExecutionCommand) {
			v.SelectedProviderBindingRef.Revision++
		},
		func(v *generationapp.AuthorizeInitialReferenceExecutionCommand) {
			v.TargetRef.ContentHash = strings.Repeat("f", 64)
		},
		func(v *generationapp.AuthorizeInitialReferenceExecutionCommand) { v.ProjectID = uuid.NewString() },
	} {
		changed := command
		mutate(&changed)
		if value, err := service.AuthorizeInitial(ctx, actor, changed); err == nil || value.ContentHash != "" {
			t.Fatal("execution authorization accepted changed scope or identity")
		}
	}
	missing := command
	missing.SelectedProviderBindingRef.ID = uuid.NewString()
	if value, err := service.AuthorizeInitial(ctx, actor, missing); !generationapp.IsCode(err, "provider_configuration_required") || value.ContentHash != "" {
		t.Fatalf("missing selected binding did not require configuration: %v", err)
	}
	if value, err := service.AuthorizeInitial(ctx, generationapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion + 1}, command); err == nil || value.ContentHash != "" {
		t.Fatal("execution authorization ignored Token version")
	}
	return referenceExecutionFixture{service, command, authorized, configuration, connection, profile, binding}
}

func assertReferenceExecutionAuthorizationRejected(t *testing.T, ctx context.Context, fixture *referenceExecutionFixture, actor generationapp.Actor, checkNewKey bool) {
	t.Helper()
	if value, err := fixture.service.AuthorizeInitial(ctx, actor, fixture.command); err == nil || value.ContentHash != "" {
		t.Fatal("execution authorization replay accepted drifted facts")
	}
	if checkNewKey {
		command := fixture.command
		command.IdempotencyKey += ":drifted"
		if value, err := fixture.service.AuthorizeInitial(ctx, actor, command); err == nil || value.ContentHash != "" {
			t.Fatal("new execution authorization accepted drifted facts")
		}
	}
}
