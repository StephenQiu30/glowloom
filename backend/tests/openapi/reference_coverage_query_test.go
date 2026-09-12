package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestReferenceCoverageQueriesAreTypedReadOnlySnapshots(t *testing.T) {
	var document struct {
		Paths map[string]struct {
			Get struct {
				RequestBody json.RawMessage `json:"requestBody"`
				Parameters  []struct {
					Ref string `json:"$ref"`
				} `json:"parameters"`
				Responses map[string]struct {
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
	if err := json.Unmarshal(openapi.Document(), &document); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		path       string
		parameters []string
		schema     string
	}{
		{"/api/projects/{project_id}/reference-coverage", []string{"#/components/parameters/project_id"}, "#/components/schemas/ReferenceCoverageMatrixResponse"},
		{"/api/projects/{project_id}/reference-executions/{execution_id}", []string{"#/components/parameters/project_id", "#/components/parameters/execution_id"}, "#/components/schemas/ReferenceExecutionProgressResponse"},
		{"/api/projects/{project_id}/reference-targets/{target_version_id}", []string{"#/components/parameters/project_id", "#/components/parameters/target_version_id"}, "#/components/schemas/ReferenceTargetDetailResponse"},
	}
	for _, check := range checks {
		operation, ok := document.Paths[check.path]
		if !ok || len(operation.Get.RequestBody) != 0 || len(operation.Get.Parameters) != len(check.parameters) {
			t.Fatalf("missing read-only query %s", check.path)
		}
		for index, parameter := range check.parameters {
			if operation.Get.Parameters[index].Ref != parameter {
				t.Fatalf("%s parameters=%#v", check.path, operation.Get.Parameters)
			}
		}
		response := operation.Get.Responses["200"]
		if response.Headers["Cache-Control"].Schema.Const != "no-store" ||
			response.Content["application/json"].Schema.Properties["data"].Ref != check.schema {
			t.Fatalf("%s is not a typed no-store snapshot", check.path)
		}
		for _, status := range []string{"401", "403", "404", "409", "422", "500"} {
			if _, exists := operation.Get.Responses[status]; !exists {
				t.Errorf("%s missing %s response", check.path, status)
			}
		}
	}
}
