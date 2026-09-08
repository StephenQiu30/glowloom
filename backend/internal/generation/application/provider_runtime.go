package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

// ProviderRuntimeConfig is scoped to one synchronous adapter invocation.
// Implementations must not retain Credentials or start background work.
type ProviderRuntimeConfig struct {
	Connection  domain.ProviderConnectionVersion
	Profile     domain.ProviderModelProfileVersion
	Credentials []byte
}
type ExecutableMediaFactory interface {
	MediaAdapterFactory
	NewRuntime(ProviderRuntimeConfig) (ProviderGateway, error)
}
type ProviderRuntimeResolver interface {
	WithRuntime(context.Context, ProviderSubmission, func(ProviderRuntimeConfig) error) error
}
type ProviderRuntimeSecrets interface {
	Decrypt(context.Context, domain.ProviderSecretContext, domain.EncryptedProviderSecret) ([]byte, error)
}
type FrozenProviderRuntime struct {
	transactions ProviderConfigurationTransactionManager
	secrets      ProviderRuntimeSecrets
}

func NewFrozenProviderRuntime(transactions ProviderConfigurationTransactionManager, secrets ProviderRuntimeSecrets) *FrozenProviderRuntime {
	return &FrozenProviderRuntime{transactions: transactions, secrets: secrets}
}
func (r *FrozenProviderRuntime) WithRuntime(ctx context.Context, s ProviderSubmission, use func(ProviderRuntimeConfig) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.transactions == nil || r.secrets == nil || use == nil {
		return errors.New("provider runtime is unavailable")
	}
	for _, id := range []string{s.WorkspaceID, s.ProjectID, s.BindingID, s.ConnectionVersionID, s.CredentialVersionID, s.ModelProfileVersionID} {
		if !validUUID(id) {
			return errors.New("invalid frozen provider reference")
		}
	}
	var connection domain.ProviderConnectionVersion
	var credential domain.ProviderCredentialVersion
	var profile domain.ProviderModelProfileVersion
	err := r.transactions.WithinProviderConfigurationTransaction(ctx, func(repo ProviderConfigurationRepository) error {
		binding, err := repo.FindProjectProviderBinding(ctx, s.BindingID)
		if err != nil {
			return err
		}
		connection, err = repo.FindProviderConnection(ctx, s.ConnectionVersionID)
		if err != nil {
			return err
		}
		credential, err = repo.FindProviderCredential(ctx, s.CredentialVersionID)
		if err != nil {
			return err
		}
		profile, err = repo.FindProviderModelProfile(ctx, s.ModelProfileVersionID)
		if err != nil {
			return err
		}
		if err = validateResolvedProviderFacts(binding, connection, credential, profile); err != nil {
			return err
		}
		if binding.ID != s.BindingID || binding.WorkspaceID != s.WorkspaceID || binding.ProjectID != s.ProjectID || binding.Revision != s.BindingRevision || binding.ContentHash != s.BindingContentHash || connection.ID != s.ConnectionVersionID || credential.ID != s.CredentialVersionID || profile.ID != s.ModelProfileVersionID || profile.Revision != s.ModelProfileRevision || profile.ContentHash != s.ModelProfileContentHash || profile.ProviderKey != s.ProviderKey || profile.ExternalModelID != s.ExternalModelID || profile.BillingMetric != s.BillingMetric {
			return errors.New("frozen provider submission has drifted")
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("resolve frozen provider: %w", err)
	}
	// External work begins only after the database transaction has ended.
	plaintext, err := r.secrets.Decrypt(ctx, domain.ProviderSecretContext{WorkspaceID: credential.WorkspaceID, ProviderKey: credential.ProviderKey, CredentialID: credential.ID, Revision: credential.Revision, KeyID: credential.KeyID}, domain.EncryptedProviderSecret{CipherSuite: credential.CipherSuite, KeyID: credential.KeyID, Nonce: credential.Nonce, Ciphertext: credential.Ciphertext, Fingerprint: credential.SecretFingerprint})
	if err != nil {
		return fmt.Errorf("decrypt provider credential: %w", err)
	}
	defer wipeProviderSecret(plaintext)
	if err = ctx.Err(); err != nil {
		return err
	}
	return use(ProviderRuntimeConfig{Connection: connection, Profile: profile, Credentials: plaintext})
}

type RuntimeGateway struct {
	resolver ProviderRuntimeResolver
	registry *MediaFactoryRegistry
}

func NewRuntimeGateway(resolver ProviderRuntimeResolver, registry *MediaFactoryRegistry) *RuntimeGateway {
	return &RuntimeGateway{resolver: resolver, registry: registry}
}
func (g *RuntimeGateway) call(ctx context.Context, s ProviderSubmission, operation string) (ProviderOutcome, error) {
	if err := ctx.Err(); err != nil {
		return ProviderOutcome{}, err
	}
	if g == nil || g.resolver == nil || g.registry == nil {
		return ProviderOutcome{}, errors.New("provider runtime is unavailable")
	}
	var outcome ProviderOutcome
	err := g.resolver.WithRuntime(ctx, s, func(config ProviderRuntimeConfig) error {
		factory, err := g.registry.Resolve(config.Connection.ProviderKey, config.Profile.Modality, config.Connection.AdapterContractVersion)
		if err != nil {
			return err
		}
		executable, ok := factory.(ExecutableMediaFactory)
		if !ok {
			return errors.New("provider adapter is not executable")
		}
		adapter, err := executable.NewRuntime(config)
		if err != nil {
			return err
		}
		if adapter == nil {
			return errors.New("provider adapter is unavailable")
		}
		switch operation {
		case "preflight":
			return adapter.Preflight(ctx, s)
		case "submit":
			outcome, err = adapter.Submit(ctx, s)
		case "query":
			outcome, err = adapter.Query(ctx, s)
		default:
			return errors.New("invalid provider operation")
		}
		return err
	})
	return outcome, err
}
func (g *RuntimeGateway) Preflight(ctx context.Context, s ProviderSubmission) error {
	_, err := g.call(ctx, s, "preflight")
	return err
}
func (g *RuntimeGateway) Submit(ctx context.Context, s ProviderSubmission) (ProviderOutcome, error) {
	return g.call(ctx, s, "submit")
}
func (g *RuntimeGateway) Query(ctx context.Context, s ProviderSubmission) (ProviderOutcome, error) {
	return g.call(ctx, s, "query")
}
