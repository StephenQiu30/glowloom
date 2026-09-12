package application

import (
	"context"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

type referenceExecutionProviderRepository interface {
	FindProjectProviderBinding(context.Context, string) (domain.ProjectProviderBindingVersion, error)
	FindProviderConnection(context.Context, string) (domain.ProviderConnectionVersion, error)
	FindProviderCredential(context.Context, string) (domain.ProviderCredentialVersion, error)
	FindProviderModelProfile(context.Context, string) (domain.ProviderModelProfileVersion, error)
	LatestProjectProviderBindingForUpdate(context.Context, string, string, string) (domain.ProjectProviderBindingVersion, error)
	LatestProviderConnectionForUpdate(context.Context, string, string) (domain.ProviderConnectionVersion, error)
	LatestProviderCredentialForUpdate(context.Context, string, string) (domain.ProviderCredentialVersion, error)
	LatestProviderModelProfileForUpdate(context.Context, string, string) (domain.ProviderModelProfileVersion, error)
}

type referenceExecutionProviderFacts struct {
	connection domain.ProviderConnectionVersion
	credential domain.ProviderCredentialVersion
	profile    domain.ProviderModelProfileVersion
}

func readReferenceExecutionProvider(ctx context.Context, repo referenceExecutionProviderRepository, workspace, project string, selected domain.GenerationRevisionRef) (referenceExecutionProviderFacts, error) {
	facts, err := readSelectedReferenceExecutionProvider(ctx, repo, workspace, project, selected)
	if errors.Is(err, ErrProjectProviderBindingNotFound) || errors.Is(err, ErrProviderConnectionNotFound) || errors.Is(err, ErrProviderCredentialNotFound) || errors.Is(err, ErrProviderProfileNotFound) {
		return referenceExecutionProviderFacts{}, referenceProviderConfigurationRequired(err)
	}
	return facts, err
}

func readSelectedReferenceExecutionProvider(ctx context.Context, repo referenceExecutionProviderRepository, workspace, project string, selected domain.GenerationRevisionRef) (referenceExecutionProviderFacts, error) {
	var facts referenceExecutionProviderFacts
	binding, err := repo.FindProjectProviderBinding(ctx, selected.ID)
	if err != nil {
		return facts, err
	}
	if validateProjectProviderBinding(binding) != nil || binding.ID != selected.ID || binding.Revision != selected.Revision || binding.ContentHash != selected.ContentHash || binding.WorkspaceID != workspace || binding.ProjectID != project || binding.Purpose != domain.ProviderPurposeReferenceAsset || binding.Modality != domain.MediaModalityImage {
		return facts, conflict("Selected Reference Provider binding has drifted")
	}
	connection, err := repo.FindProviderConnection(ctx, binding.ConnectionVersionID)
	if err != nil {
		return facts, err
	}
	credential, err := repo.FindProviderCredential(ctx, binding.CredentialVersionID)
	if err != nil {
		return facts, err
	}
	profile, err := repo.FindProviderModelProfile(ctx, binding.ModelProfileVersionID)
	if err != nil {
		return facts, err
	}
	if err = validateResolvedProviderFacts(binding, connection, credential, profile); err != nil {
		return facts, err
	}
	currentBinding, err := repo.LatestProjectProviderBindingForUpdate(ctx, workspace, project, domain.ProviderPurposeReferenceAsset)
	if err != nil {
		return facts, err
	}
	if currentBinding.ID != binding.ID || currentBinding.Revision != binding.Revision || currentBinding.ContentHash != binding.ContentHash {
		return facts, conflict("Selected Reference Provider binding is no longer current")
	}
	currentConnection, err := repo.LatestProviderConnectionForUpdate(ctx, workspace, connection.ConnectionKey)
	if err != nil {
		return facts, err
	}
	if currentConnection.ID != connection.ID || currentConnection.Revision != connection.Revision || currentConnection.ContentHash != connection.ContentHash || currentConnection.State != domain.ProviderStateEnabled || currentConnection.CredentialVersionID != credential.ID {
		return facts, conflict("Selected Reference Provider connection is no longer current")
	}
	currentProfile, err := repo.LatestProviderModelProfileForUpdate(ctx, workspace, profile.ProfileKey)
	if err != nil {
		return facts, err
	}
	if currentProfile.ID != profile.ID || currentProfile.Revision != profile.Revision || currentProfile.ContentHash != profile.ContentHash || currentProfile.State != domain.ProviderStateEnabled {
		return facts, conflict("Selected Reference Provider model profile is no longer current")
	}
	currentCredential, err := repo.LatestProviderCredentialForUpdate(ctx, workspace, connection.ConnectionKey)
	if err != nil {
		return facts, err
	}
	if currentCredential.ID != credential.ID || currentCredential.Revision != credential.Revision || currentCredential.SecretFingerprint != credential.SecretFingerprint {
		return facts, conflict("Selected Reference Provider credential is no longer current")
	}
	return referenceExecutionProviderFacts{connection, credential, profile}, nil
}

func referenceProviderConfigurationRequired(cause error) error {
	result := &Error{Code: "provider_configuration_required", Status: 409, Message: "Select a configured image Provider binding before Reference execution", NextAction: "configure_project_provider"}
	if cause == nil {
		return result
	}
	return errors.Join(result, cause)
}
