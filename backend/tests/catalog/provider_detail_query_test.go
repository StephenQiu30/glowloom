package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

type providerDetailStore struct {
	provider   domain.Provider
	credential *catalogapp.ProviderCredentialSummary
	called     bool
}

func (s *providerDetailStore) FindProviderDetailForAdmin(_ context.Context, _, _, _ uuid.UUID) (domain.Provider, *catalogapp.ProviderCredentialSummary, error) {
	s.called = true
	return s.provider, s.credential, nil
}

func TestProviderDetailQueryReturnsOnlySafeMetadataAndSchema(t *testing.T) {
	actor := adminPrincipal()
	provider := validProvider()
	provider.CreateTime = time.Now()
	provider.UpdateTime = provider.CreateTime
	testedAt := time.Now()
	result := domain.TestOK
	store := &providerDetailStore{provider: provider, credential: &catalogapp.ProviderCredentialSummary{
		ID: uuid.New(), ProviderID: provider.ID, Label: "primary", Last4: "A9F2",
		Status: domain.CredentialActive, LastTestedAt: &testedAt, LastTestResult: &result,
		CreateTime: time.Now(), UpdateTime: time.Now(),
	}}
	detail, err := catalogapp.NewProviderDetailQuery(store, credentialschema.NewRegistry()).Execute(t.Context(), actor, provider.ID)
	if err != nil || !store.called || detail.Provider.ID != provider.ID ||
		detail.Credential == nil || detail.Credential.Last4 != "A9F2" ||
		len(detail.CredentialSchema) != 2 || detail.CredentialSchema[0].Name != "api_key" ||
		detail.CredentialSchema[0].Type != "password" || detail.CredentialSchema[1].Name != "group_id" {
		t.Fatalf("provider detail %+v: %v", detail, err)
	}
	encoded, err := json.Marshal(detail)
	if err != nil || strings.Contains(string(encoded), "ciphertext") || strings.Contains(string(encoded), "key_id") ||
		strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe provider detail %s: %v", encoded, err)
	}
}

func TestProviderDetailQueryRejectsInvalidCallerAndUnsupportedAdapter(t *testing.T) {
	actor := adminPrincipal()
	provider := validProvider()
	provider.CreateTime = time.Now()
	provider.UpdateTime = provider.CreateTime
	for _, tc := range []struct {
		name     string
		actor    identityapp.Principal
		provider domain.Provider
		want     error
		read     bool
	}{
		{"producer", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleProducer}, provider, identityapp.ErrForbidden, false},
		{"first login", identityapp.Principal{ID: actor.ID, OrgID: actor.OrgID, Role: identitydomain.RoleAdmin, MustChangePassword: true}, provider, identityapp.ErrForbidden, false},
		{"unsupported adapter", actor, func() domain.Provider { p := provider; p.AdapterKey = "missing"; return p }(), catalogapp.ErrUnsupportedCredentialSchema, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &providerDetailStore{provider: tc.provider}
			_, err := catalogapp.NewProviderDetailQuery(store, credentialschema.NewRegistry()).Execute(t.Context(), tc.actor, provider.ID)
			if !errors.Is(err, tc.want) || store.called != tc.read {
				t.Fatalf("unexpected detail access error=%v read=%t", err, store.called)
			}
		})
	}
}

func TestCredentialFieldSchemaMatchesSupportedAdapters(t *testing.T) {
	registry := credentialschema.NewRegistry()
	for _, adapter := range []string{"volcengine_ark", "openrouter"} {
		fields, ok := registry.Fields(adapter)
		if !ok || len(fields) != 1 || fields[0].Name != "api_key" ||
			fields[0].Type != "password" || !fields[0].Required {
			t.Fatalf("%s fields %+v supported=%t", adapter, fields, ok)
		}
		fields[0].Name = "changed"
		again, _ := registry.Fields(adapter)
		if again[0].Name != "api_key" {
			t.Fatalf("%s schema was mutated", adapter)
		}
	}
	if fields, ok := registry.Fields("missing"); ok || len(fields) != 0 {
		t.Fatalf("unknown adapter fields %+v supported=%t", fields, ok)
	}
}
