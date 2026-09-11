package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestProjectPresetPublicContract(t *testing.T) {
	var document struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Paths["/api/presets"]["get"] == nil ||
		document.Paths["/api/projects/{project_id}/preset-selection"]["get"] == nil ||
		document.Paths["/api/projects/{project_id}/preset-selection"]["put"] == nil {
		t.Fatal("Project Preset catalog and selection operations must be public")
	}
	for _, schema := range []string{"PresetRelease", "ProjectPresetSelection", "ProjectPresetSelectionRequest"} {
		if document.Components.Schemas[schema] == nil {
			t.Fatalf("missing schema %s", schema)
		}
	}
}
