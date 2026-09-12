package openapi_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferenceBundleAdmissionContract(t *testing.T) {
	var document struct {
		Components struct {
			Schemas map[string]struct {
				AdditionalProperties *bool    `json:"additionalProperties"`
				Required             []string `json:"required"`
				Properties           map[string]struct {
					Type  json.RawMessage `json:"type"`
					Const json.RawMessage `json:"const"`
					Ref   string          `json:"$ref"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatal(err)
	}
	schema := document.Components.Schemas["ReferenceBundleAdmissionResponse"]
	fields := []string{"policy_ref", "internal_review_ready", "selection_ready", "publication_ready", "internal_review_blockers", "formal_use_blockers"}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties || len(schema.Required) != len(fields) || len(schema.Properties) != len(fields) {
		t.Fatal("purpose admission must be a closed contract")
	}
	for _, field := range fields {
		if _, ok := schema.Properties[field]; !ok || !slices.Contains(schema.Required, field) {
			t.Fatalf("missing required field %s", field)
		}
	}
	if string(schema.Properties["internal_review_ready"].Type) != `"boolean"` {
		t.Fatal("missing internal review readiness")
	}
	for _, field := range []string{"selection_ready", "publication_ready"} {
		property := schema.Properties[field]
		if string(property.Type) != `"boolean"` || string(property.Const) != "false" {
			t.Fatalf("bundle input cannot authorize %s", field)
		}
	}
	evaluation := document.Components.Schemas["ReferenceBundleEvaluationResponse"]
	if !slices.Contains(evaluation.Required, "admission") || evaluation.Properties["admission"].Ref != "#/components/schemas/ReferenceBundleAdmissionResponse" {
		t.Fatal("bundle query must expose derived admission")
	}
}
