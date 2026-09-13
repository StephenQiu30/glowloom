package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferenceCandidateBundleContractOnlyExposesExactRead(t *testing.T) {
	var document struct {
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				AdditionalProperties bool                       `json:"additionalProperties"`
				Required             []string                   `json:"required"`
				Properties           map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatal(err)
	}
	var path map[string]struct {
		OperationID string          `json:"operationId"`
		RequestBody json.RawMessage `json:"requestBody"`
	}
	if err := json.Unmarshal(document.Paths["/api/projects/{project_id}/reference-candidate-bundles/{bundle_id}"], &path); err != nil {
		t.Fatal(err)
	}
	if len(path) != 1 || path["get"].OperationID != "getReferenceCandidateBundle" || len(path["get"].RequestBody) != 0 {
		t.Fatal("candidate Bundle is a read-only exact resource")
	}
	schema := document.Components.Schemas["ReferenceCandidateBundleResponse"]
	if schema.AdditionalProperties || len(schema.Required) != 13 || len(schema.Properties) != 13 {
		t.Fatal("candidate Bundle response must be closed")
	}
	for _, key := range schema.Required {
		if _, ok := schema.Properties[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
	for _, key := range []string{"selected", "rights_approved", "object_key", "url", "prompt"} {
		if _, ok := schema.Properties[key]; ok {
			t.Fatalf("unsafe field %s", key)
		}
	}
}
