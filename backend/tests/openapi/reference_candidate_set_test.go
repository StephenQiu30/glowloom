package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferenceCandidateSetContractUsesExactInputs(t *testing.T) {
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
	for path, operation := range map[string]string{
		"/api/projects/{project_id}/reference-executions/{execution_id}/candidate-sets": "materializeReferenceCandidateSet",
		"/api/projects/{project_id}/reference-candidate-sets/{set_id}":                  "getReferenceCandidateSet",
	} {
		var methods map[string]struct {
			OperationID string `json:"operationId"`
		}
		if err := json.Unmarshal(document.Paths[path], &methods); err != nil {
			t.Fatal(err)
		}
		if len(methods) != 1 {
			t.Fatal("unexpected candidate Set mutation surface")
		}
		for _, method := range methods {
			if method.OperationID != operation {
				t.Fatal("candidate Set operation missing")
			}
		}
	}
	for name, count := range map[string]int{"ReferenceCandidateSetRequest": 3, "ReferenceCandidateSetResponse": 15} {
		schema := document.Components.Schemas[name]
		if schema.AdditionalProperties || len(schema.Required) != count || len(schema.Properties) != count {
			t.Fatalf("%s must be closed", name)
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
}
