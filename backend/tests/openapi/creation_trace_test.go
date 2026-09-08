package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestCreationReadContractsKeepVersionedResultsAndNoStore(t *testing.T) {
	var doc struct {
		Paths map[string]struct {
			Get struct {
				RequestBody json.RawMessage `json:"requestBody"`
				Responses   map[string]struct {
					Headers map[string]struct {
						Schema struct {
							Const string `json:"const"`
						} `json:"schema"`
					} `json:"headers"`
					Content map[string]struct {
						Schema struct {
							Properties map[string]struct {
								Ref string `json:"$ref"`
							} `json:"properties"`
						} `json:"schema"`
					} `json:"content"`
				} `json:"responses"`
			} `json:"get"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openapi.Document(), &doc); err != nil {
		t.Fatal(err)
	}
	for path, schema := range map[string]string{
		"/api/creation-runs/{run_id}/manifest":                         "CreationManifestSnapshot",
		"/api/creation-runs/{run_id}/steps/{step_id}/attempts":         "CreationAttemptHistory",
		"/api/projects/{project_id}/text-world-versions/{version_id}":  "BibleTextWorldVersion",
		"/api/projects/{project_id}/text-intent-versions/{version_id}": "StoryboardTextIntentVersion",
	} {
		item, ok := doc.Paths[path]
		operation := item.Get
		if !ok {
			t.Fatalf("missing GET %s", path)
		}
		if len(operation.RequestBody) != 0 {
			t.Errorf("GET declares body: %s", path)
		}
		response := operation.Responses["200"]
		if response.Headers["Cache-Control"].Schema.Const != "no-store" {
			t.Errorf("cache contract missing: %s", path)
		}
		if response.Content["application/json"].Schema.Properties["data"].Ref != "#/components/schemas/"+schema {
			t.Errorf("untyped result: %s", path)
		}
		for _, status := range []string{"401", "403", "404", "409", "422", "503"} {
			if _, ok := operation.Responses[status]; !ok {
				t.Errorf("missing %s for %s", status, path)
			}
		}
	}
}
