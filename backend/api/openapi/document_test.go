package openapi

import (
	"encoding/json"
	"testing"
)

func TestDocumentIsThePublicAPIContract(t *testing.T) {
	var document struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
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
}
