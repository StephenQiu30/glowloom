package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferenceGenerationQueryContract(t *testing.T) {
	var document struct {
		Paths map[string]struct {
			Get struct {
				OperationID string                     `json:"operationId"`
				RequestBody json.RawMessage            `json:"requestBody"`
				Responses   map[string]json.RawMessage `json:"responses"`
			} `json:"get"`
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
	get := document.Paths["/api/projects/{project_id}/reference-generation-targets/{generation_target_id}"].Get
	if get.OperationID != "getReferenceGenerationProgress" || len(get.RequestBody) != 0 {
		t.Fatal("missing closed read operation")
	}
	for _, status := range []string{"200", "401", "403", "404", "409", "422", "500"} {
		if len(get.Responses[status]) == 0 {
			t.Fatalf("missing response %s", status)
		}
	}
	schema := document.Components.Schemas["ReferenceGenerationProgressResponse"]
	fields := []string{"workspace_id", "project_id", "generation_target_ref", "plan_ref", "reference_target_ref", "generation_round", "execution", "content_hash"}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties || len(schema.Properties) != len(fields) || len(schema.Required) != len(fields) {
		t.Fatal("unbounded generation response")
	}
	for _, field := range fields {
		if len(schema.Properties[field]) == 0 {
			t.Fatalf("missing %s", field)
		}
	}
	var execution struct {
		OneOf []struct {
			Type string `json:"type"`
			Ref  string `json:"$ref"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(schema.Properties["execution"], &execution); err != nil || len(execution.OneOf) != 2 || execution.OneOf[0].Ref != "#/components/schemas/ReferenceExecutionProgressResponse" || execution.OneOf[1].Type != "null" {
		t.Fatal("unprepared execution is not explicit null")
	}
}
