package openapi_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/api/openapi"
)

func TestStructureIdentityQueryIsTypedReadOnlyAndCurrent(t *testing.T) {
	var document struct {
		Paths map[string]struct {
			Get struct {
				RequestBody json.RawMessage `json:"requestBody"`
				Parameters  []struct {
					Name string `json:"name"`
					In   string `json:"in"`
					Ref  string `json:"$ref"`
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
	operation, ok := document.Paths["/api/projects/{project_id}/structure-identity"]
	if !ok || len(operation.Get.RequestBody) != 0 {
		t.Fatal("missing read-only Structure Identity query")
	}
	if len(operation.Get.Parameters) != 1 || operation.Get.Parameters[0].Ref != "#/components/parameters/project_id" {
		t.Fatalf("query accepts undeclared selector: %#v", operation.Get.Parameters)
	}
	response := operation.Get.Responses["200"]
	if response.Headers["Cache-Control"].Schema.Const != "no-store" ||
		response.Content["application/json"].Schema.Properties["data"].Ref != "#/components/schemas/StructureIdentitySnapshotResponse" {
		t.Fatal("Structure Identity response is not a typed no-store snapshot")
	}
	for _, status := range []string{"401", "403", "404", "409", "422", "500"} {
		if _, exists := operation.Get.Responses[status]; !exists {
			t.Errorf("missing %s response", status)
		}
	}
}
