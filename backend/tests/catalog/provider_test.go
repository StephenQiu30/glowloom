package catalog_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

func validProvider() domain.Provider {
	return domain.Provider{
		ID: uuid.New(), Key: "minimax", Name: "MiniMax", AdapterKey: "minimax",
		Region: domain.RegionDomestic, Status: domain.ProviderActive,
		ConcurrencyLimit: 10, RateLimitPerMin: 60, Revision: 1,
	}
}

func validCredential(providerID uuid.UUID) domain.Credential {
	return domain.Credential{
		ID: uuid.New(), ProviderID: providerID, Label: "primary",
		Ciphertext: []byte("sealed-data-only"), KeyID: "agent-2026", Last4: "A9F2",
		Status: domain.CredentialActive,
	}
}

func TestProviderRejectsInvalidSettingsAndDisablesOnce(t *testing.T) {
	provider := validProvider()
	if err := provider.Validate(); err != nil {
		t.Fatalf("valid provider: %v", err)
	}
	for _, mutate := range []func(*domain.Provider){
		func(p *domain.Provider) { p.Key = " " },
		func(p *domain.Provider) { p.AdapterKey = "" },
		func(p *domain.Provider) { p.Region = "elsewhere" },
		func(p *domain.Provider) { p.ConcurrencyLimit = 0 },
		func(p *domain.Provider) { p.RateLimitPerMin = -1 },
		func(p *domain.Provider) { p.Revision = 0 },
	} {
		candidate := provider
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, domain.ErrInvalidProvider) {
			t.Fatalf("invalid provider accepted: %v", err)
		}
	}
	if err := provider.Disable(); err != nil || provider.Status != domain.ProviderDisabled || provider.Revision != 2 {
		t.Fatalf("disable provider: status %q revision %d error %v", provider.Status, provider.Revision, err)
	}
	if err := provider.Disable(); !errors.Is(err, domain.ErrProviderDisabled) || provider.Revision != 2 {
		t.Fatalf("repeated disable: revision %d error %v", provider.Revision, err)
	}
}

func TestCredentialKeepsCiphertextOutOfJSONAndValidatesTestOutcome(t *testing.T) {
	credential := validCredential(uuid.New())
	if err := credential.Validate(); err != nil {
		t.Fatalf("valid credential: %v", err)
	}
	encoded, err := json.Marshal(credential)
	if err != nil || strings.Contains(string(encoded), "sealed-data-only") || strings.Contains(string(encoded), "Ciphertext") {
		t.Fatalf("credential JSON exposed ciphertext or failed: %v", err)
	}
	for _, mutate := range []func(*domain.Credential){
		func(c *domain.Credential) { c.ProviderID = uuid.Nil },
		func(c *domain.Credential) { c.Ciphertext = nil },
		func(c *domain.Credential) { c.KeyID = "" },
		func(c *domain.Credential) { c.Last4 = "123" },
		func(c *domain.Credential) { c.Status = "unknown" },
	} {
		candidate := credential
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, domain.ErrInvalidCredential) {
			t.Fatalf("invalid credential accepted: %v", err)
		}
	}
	when := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if err := credential.RecordTest(domain.TestUnsupported, when); err != nil ||
		credential.LastTestResult != domain.TestUnsupported || !credential.LastTestedAt.Equal(when) {
		t.Fatalf("unsupported test result: %v", err)
	}
	if err := credential.RecordTest("not_a_result", when); !errors.Is(err, domain.ErrInvalidTestResult) {
		t.Fatalf("invalid test result accepted: %v", err)
	}
	if err := credential.Disable(); err != nil || credential.Status != domain.CredentialDisabled {
		t.Fatalf("disable credential: %v", err)
	}
	if err := credential.RecordTest(domain.TestOK, when); !errors.Is(err, domain.ErrCredentialDisabled) {
		t.Fatalf("disabled credential tested: %v", err)
	}
}
