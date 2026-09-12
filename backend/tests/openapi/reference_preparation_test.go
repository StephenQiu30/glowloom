package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferencePreparationContract(t *testing.T) {
	var document struct {
		Paths map[string]struct {
			Post struct {
				OperationID string                     `json:"operationId"`
				Responses   map[string]json.RawMessage `json:"responses"`
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
	for path, operation := range map[string]string{
		"/api/projects/{project_id}/reference-targets/{target_version_id}/generation-authorizations":              "authorizeInitialReferenceGeneration",
		"/api/projects/{project_id}/reference-generation-targets":                                                 "buildInitialReferenceGenerationTarget",
		"/api/projects/{project_id}/reference-generation-targets/{generation_target_id}/execution-authorizations": "authorizeInitialReferenceExecution",
		"/api/projects/{project_id}/reference-generation-targets/{generation_target_id}/executions":               "prepareInitialReferenceExecution",
	} {
		post := document.Paths[path].Post
		if post.OperationID != operation {
			t.Fatalf("missing operation %s", operation)
		}
		for _, status := range []string{"201", "401", "403", "404", "409", "422", "500"} {
			if len(post.Responses[status]) == 0 {
				t.Fatalf("missing %s response %s", operation, status)
			}
		}
	}
	for name, fields := range map[string][]string{
		"ReferenceGenerationAuthorizationRequest":  {"workspace_id", "plan_version_id", "plan_content_hash", "target_content_hash", "brief_revision_id", "brief_revision_hash", "candidate_bundle_count", "idempotency_key"},
		"ReferenceGenerationTargetBuildRequest":    {"workspace_id", "generation_authorization_ref", "brief_revision_id", "brief_revision_hash", "slot_policies", "idempotency_key"},
		"ReferenceExecutionAuthorizationRequest":   {"workspace_id", "target_hash", "selected_provider_binding_ref", "idempotency_key"},
		"ReferenceExecutionPreparationRequest":     {"workspace_id", "target_hash", "execution_authorization_ref", "idempotency_key"},
		"ReferenceGenerationAuthorizationResponse": {"generation_authorization_ref"},
		"ReferenceGenerationTargetBuildResponse":   {"generation_target_ref"},
		"ReferenceExecutionAuthorizationResponse":  {"execution_authorization_ref"},
		"ReferenceExecutionPreparationResponse":    {"execution_ref"},
		"ReferenceOutputSlotPolicyRequest":         {"view_role", "allowed_media_types", "aspect_ratio", "min_width", "min_height", "max_bytes"},
		"ReferenceCommandActionRef":                {"id", "content_hash"},
	} {
		schema := document.Components.Schemas[name]
		if schema.AdditionalProperties == nil || *schema.AdditionalProperties || len(schema.Properties) != len(fields) || len(schema.Required) != len(fields) {
			t.Fatalf("unbounded command %s", name)
		}
		for _, field := range fields {
			if len(schema.Properties[field]) == 0 {
				t.Fatalf("missing %s.%s", name, field)
			}
		}
	}
}
