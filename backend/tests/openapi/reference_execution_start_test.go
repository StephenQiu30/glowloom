package openapi_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferenceExecutionStartContract(t *testing.T) {
	var document struct {
		Paths map[string]struct {
			Post struct {
				OperationID string `json:"operationId"`
				RequestBody struct {
					Content map[string]struct {
						Schema struct {
							Ref string `json:"$ref"`
						} `json:"schema"`
					} `json:"content"`
				} `json:"requestBody"`
				Responses map[string]json.RawMessage `json:"responses"`
			} `json:"post"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				AdditionalProperties *bool                      `json:"additionalProperties"`
				Required             []string                   `json:"required"`
				Properties           map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatal(err)
	}
	op := document.Paths["/api/projects/{project_id}/reference-executions/{execution_id}/workflow-runs"].Post
	if op.OperationID != "startReferenceExecutionWorkflow" || op.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/ReferenceExecutionWorkflowStartRequest" {
		t.Fatal("missing typed Reference execution start")
	}
	for _, status := range []string{"202", "401", "403", "404", "409", "422", "500"} {
		if len(op.Responses[status]) == 0 {
			t.Fatalf("missing response %s", status)
		}
	}
	for name, fields := range map[string][]string{
		"ReferenceExecutionWorkflowStartRequest":  {"execution_hash", "idempotency_key"},
		"ReferenceExecutionWorkflowStartResponse": {"workflow_run_id", "status"},
	} {
		schema := document.Components.Schemas[name]
		if schema.AdditionalProperties == nil || *schema.AdditionalProperties || len(schema.Properties) != len(fields) || len(schema.Required) != len(fields) {
			t.Fatalf("unbounded start contract: %s", name)
		}
		for _, field := range fields {
			if len(schema.Properties[field]) == 0 || !slices.Contains(schema.Required, field) {
				t.Fatalf("missing %s.%s", name, field)
			}
		}
	}
}
