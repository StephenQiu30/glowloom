package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestDocumentIsThePublicAPIContract(t *testing.T) {
	var document struct {
		OpenAPI    string                     `json:"openapi"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version = %q, want 3.1.0", document.OpenAPI)
	}
	for _, path := range []string{
		"/api/auth/register",
		"/api/projects",
		"/api/projects/{project_id}/cost-budget",
		"/api/projects/{project_id}/cost-prices/{metric}",
		"/api/projects/{project_id}/current-script-document",
		"/api/document-revisions/{revision_id}/production-bibles",
		"/api/episodes/{episode_id}/storyboard-drafts",
		"/api/storyboard-draft-batches/{batch_id}/apply",
		"/api/storyboard-exports/{export_id}/download",
		"/api/projects/{project_id}/storygraph/current",
		"/api/projects/{project_id}/storygraph/versions/{version_id}",
		"/api/projects/{project_id}/storygraph/versions/{version_ref}/lens",
		"/api/projects/{project_id}/storygraph/versions/{version_ref}/nodes/{story_node_key}/trace",
		"/api/projects/{project_id}/storygraph/diff",
		"/api/projects/{project_id}/human-tasks",
		"/api/human-tasks/{human_task_id}",
		"/api/human-tasks/{human_task_id}/claims",
		"/api/human-tasks/{human_task_id}/claim-renewals",
		"/api/human-tasks/{human_task_id}/claim-releases",
		"/api/human-tasks/{human_task_id}/decisions",
		"/api/review-decisions/{review_decision_id}/resume",
	} {
		if _, exists := document.Paths[path]; !exists {
			t.Errorf("public contract is missing %s", path)
		}
	}
	if _, exists := document.Paths["/api/episodes/{episode_id}/storyboard-draft-batches"]; exists {
		t.Error("public contract still exposes the removed legacy storyboard route")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/budget-limit"]; exists {
		t.Error("public contract still exposes the Production-owned legacy budget route")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/cost-estimates"]; exists {
		t.Error("public contract exposes internal estimates before GenerationIntent coordination")
	}
	if _, exists := document.Paths["/api/projects/{project_id}/generation/image-provider-bindings"]; exists {
		t.Error("public contract still exposes the removed fixed image Provider binding route")
	}
	var projectSchema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["ProjectResponse"], &projectSchema); err != nil {
		t.Fatalf("decode ProjectResponse: %v", err)
	}
	if _, exists := projectSchema.Properties["budget_limit"]; exists {
		t.Error("ProjectResponse still owns budget_limit")
	}
	if _, exists := projectSchema.Properties["currency"]; exists {
		t.Error("ProjectResponse still owns budget currency")
	}
	if _, exists := document.Components.Schemas["CostBudgetResponse"]; !exists {
		t.Error("public contract is missing CostBudgetResponse")
	}
	if _, exists := document.Components.Schemas["CostPriceQuoteResponse"]; !exists {
		t.Error("public contract is missing CostPriceQuoteResponse")
	}
	for _, schema := range []string{"ImageProviderBindingPublishRequest", "ImageProviderBindingResponse"} {
		if _, exists := document.Components.Schemas[schema]; exists {
			t.Errorf("public contract still exposes removed schema %s", schema)
		}
	}
	for _, schema := range []string{"StoryGraphVersionResponse", "StoryGraphSubgraphResponse", "StoryGraphDiffResponse"} {
		if _, exists := document.Components.Schemas[schema]; !exists {
			t.Errorf("public contract is missing %s", schema)
		}
	}
	for _, schema := range []string{
		"HumanTaskListEnvelope", "HumanTaskDetailEnvelope", "HumanTaskCommandEnvelope",
		"HumanGateDecisionEnvelope", "HumanGateResumeEnvelope", "HumanGateCoordinationResponse",
	} {
		if _, exists := document.Components.Schemas[schema]; !exists {
			t.Errorf("public contract is missing %s", schema)
		}
	}
	var listClaim struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["HumanTaskClaimSummary"], &listClaim); err != nil {
		t.Fatalf("decode HumanTaskClaimSummary: %v", err)
	}
	if _, exposesToken := listClaim.Properties["claim_token"]; exposesToken {
		t.Error("HumanTask list claim summary exposes claim_token")
	}
}
