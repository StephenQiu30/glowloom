package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialschema"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/paramvalidation"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestMusicCredentialRequiresSeparateSigningKeys(t *testing.T) {
	r := credentialschema.NewRegistry()
	for _, secret := range []string{`{"api_key":"cannot-use-bearer"}`, `{"access_key_id":"abcd"}`, `{"access_key_id":"abcd","secret_access_key":"efgh","api_key":"ijkl"}`, `{"access_key_id":"abcd","secret_access_key":"efgh","secret_access_key":"ijkl"}`} {
		if _, err := r.Validate("volcengine_music", json.RawMessage(secret)); err == nil {
			t.Fatal("invalid music secret accepted")
		}
	}
	last, err := r.Validate("volcengine_music", json.RawMessage(`{"access_key_id":"test-ak","secret_access_key":"test-secret"}`))
	if err != nil || last != "cret" {
		t.Fatalf("safe receipt: %q %v", last, err)
	}
}

func TestCreationCandidatesHavePublishableSchemas(t *testing.T) {
	v, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []string{"volcengine_music", "volcengine_avatar", "volcengine_motion"} {
		for _, candidate := range domain.BuiltinModelCatalog(adapter).Models {
			version, err := candidate.OnboardingVersion(uuid.New(), uuid.New())
			if err != nil {
				t.Fatal(err)
			}
			if err := v.Validate(version); err != nil {
				t.Fatalf("%s: %v", adapter, err)
			}
			if !version.SupportsQuery || version.SupportsCancel || version.ExpectedMaxMS < 1800000 {
				t.Fatalf("invalid async contract: %+v", version)
			}
		}
	}
}
