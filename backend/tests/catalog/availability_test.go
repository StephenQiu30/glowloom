package catalog_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
)

func TestAvailabilityRequiresCurrentEvidenceForEachMode(t *testing.T) {
	facts := application.ModelAvailabilityFacts{ReleaseID: uuid.New(), ModelVersionID: uuid.New(), PriceVersionID: uuid.New(), ConfigVersionID: uuid.New(), CredentialID: uuid.New(), Enabled: true, CredentialReady: true, ContractCompatible: true, PriceReady: true, VersionsCurrent: true, Modes: []string{"text2image", "image2image"}, VerifiedModes: []string{"text2image"}}
	result := application.ProjectModelAvailability(facts)
	if !result.Available || !result.ModeAvailability[0].Available || result.ModeAvailability[1].Available {
		t.Fatalf("mode evidence was inherited: %+v", result)
	}
	for _, test := range []struct {
		name, reason string
		change       func(*application.ModelAvailabilityFacts)
	}{
		{"disabled", "disabled", func(f *application.ModelAvailabilityFacts) { f.Enabled = false }},
		{"credential", "credential_required", func(f *application.ModelAvailabilityFacts) { f.CredentialReady = false }},
		{"contract", "unsupported_contract", func(f *application.ModelAvailabilityFacts) { f.ContractCompatible = false }},
		{"price", "price_unverified", func(f *application.ModelAvailabilityFacts) { f.PriceReady = false }},
		{"rotation", "version_changed", func(f *application.ModelAvailabilityFacts) { f.VersionsCurrent = false }},
		{"release", "release_required", func(f *application.ModelAvailabilityFacts) { f.ReleaseID = uuid.Nil }},
		{"missing_credential_reference", "credential_required", func(f *application.ModelAvailabilityFacts) { f.CredentialID = uuid.Nil }},
		{"missing_model_reference", "unsupported_contract", func(f *application.ModelAvailabilityFacts) { f.ModelVersionID = uuid.Nil }},
		{"missing_price_reference", "price_unverified", func(f *application.ModelAvailabilityFacts) { f.PriceVersionID = uuid.Nil }},
		{"probe_only", "mode_unverified", func(f *application.ModelAvailabilityFacts) { f.VerifiedModes = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := facts
			test.change(&changed)
			got := application.ProjectModelAvailability(changed)
			if got.Available || !slices.Contains(got.UnavailableReasons, test.reason) {
				t.Fatalf("unsafe projection: %+v", got)
			}
		})
	}
	if got := application.ProjectModelAvailability(application.ModelAvailabilityFacts{}); got.Available {
		t.Fatal("unknown facts must fail closed")
	}
}
