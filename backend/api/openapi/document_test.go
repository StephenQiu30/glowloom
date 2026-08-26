package openapi

import (
	"encoding/json"
	"testing"
)

func TestDocumentIsThePublicAPIContract(t *testing.T) {
	var document struct {
		OpenAPI    string                     `json:"openapi"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(Document(), &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version = %q, want 3.1.0", document.OpenAPI)
	}
	for _, path := range []string{
		"/api/v1/auth/register",
		"/api/v1/projects",
		"/api/v1/projects/{project_id}/cost-budget",
		"/api/v1/projects/{project_id}/current-script-document",
		"/api/v1/document-revisions/{revision_id}/production-bibles",
		"/api/v1/episodes/{episode_id}/storyboard-drafts",
		"/api/v1/storyboard-draft-batches/{batch_id}/apply",
		"/api/v1/storyboard-exports/{export_id}/download",
	} {
		if _, exists := document.Paths[path]; !exists {
			t.Errorf("public contract is missing %s", path)
		}
	}
	if _, exists := document.Paths["/api/v1/episodes/{episode_id}/storyboard-draft-batches"]; exists {
		t.Error("public contract still exposes the removed legacy storyboard route")
	}
	if _, exists := document.Paths["/api/v1/projects/{project_id}/budget-limit"]; exists {
		t.Error("public contract still exposes the Production-owned legacy budget route")
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
}
