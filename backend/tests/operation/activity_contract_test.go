package operation_test

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	catalogworkflow "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/workflow"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
)

type activityExample struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

func TestProviderActivityJSONContracts(t *testing.T) {
	// Synthetic wire examples protect historical Temporal payload compatibility.
	// Public HTTP contracts are generated from handler annotations and DTOs.
	tests := []struct {
		name        string
		example     string
		checkModels func(*testing.T, activityExample)
	}{
		{
			name: "provider_submit",
			example: `{
				"input": {
					"operation_id": "11111111-1111-4111-8111-111111111111",
					"provider_request_key": "operation/11111111-1111-4111-8111-111111111111/submit",
					"adapter_key": "mock",
					"provider_model_id": "mock-image-v1",
					"capability": "image.generate",
					"mode": "text_to_image",
					"params": {"seed": 42},
					"inputs": [{"role": "prompt", "text": "A blue lantern on a wooden table"}],
					"output_count": 1
				},
				"output": {
					"outcome": "accepted",
					"provider_task_id": "mock-task-11111111",
					"error": null
				}
			}`,
			checkModels: func(t *testing.T, example activityExample) {
				checkActivityModel[workflow.ProviderSubmitInput](t, example.Input)
				checkActivityModel[workflow.ProviderSubmitOutput](t, example.Output)
				assertStringField(t, example.Input, "adapter_key", "mock")
				assertStringField(t, example.Output, "outcome", "accepted")
			},
		},
		{
			name: "provider_query",
			example: `{
				"input": {"provider_task_id": "mock-task-11111111"},
				"output": {
					"state": "pending",
					"provider_task_id": "mock-task-11111111",
					"result_urls": [],
					"usage": null,
					"error": null
				}
			}`,
			checkModels: func(t *testing.T, example activityExample) {
				checkActivityModel[workflow.ProviderQueryInput](t, example.Input)
				checkActivityModel[workflow.ProviderQueryOutput](t, example.Output)
				assertStringField(t, example.Input, "provider_task_id", "mock-task-11111111")
				assertStringField(t, example.Output, "state", "pending")
				assertStringField(t, example.Output, "provider_task_id", "mock-task-11111111")
			},
		},
		{
			name: "provider_cancel",
			example: `{
				"input": {"provider_task_id": "mock-task-11111111"},
				"output": {"outcome": "cancelled"}
			}`,
			checkModels: func(t *testing.T, example activityExample) {
				checkActivityModel[workflow.ProviderCancelInput](t, example.Input)
				checkActivityModel[workflow.ProviderCancelOutput](t, example.Output)
				assertStringField(t, example.Output, "outcome", "cancelled")
			},
		},
		{
			name: "provider_test_credential",
			example: `{
				"input": {
					"provider_id": "49f48d62-ff66-44c5-a25d-ea131d4a607d",
					"provider_key": "configured-openrouter",
					"adapter_key": "openrouter",
					"credential": {
						"id": "8ff0a79a-0e57-4754-a0e9-93fab15409fa",
						"key_id": "agent-test",
						"ciphertext": "YQ=="
					}
				},
				"output": {"result": "ok"}
			}`,
			checkModels: func(t *testing.T, example activityExample) {
				checkActivityModel[catalogworkflow.AgentTestInput](t, example.Input)
				checkActivityModel[catalogworkflow.AgentTestOutput](t, example.Output)
				assertStringField(t, example.Input, "adapter_key", "openrouter")
				assertStringField(t, example.Output, "result", "ok")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var example activityExample
			decodeActivityJSON(t, []byte(test.example), &example)
			if len(example.Input) == 0 || len(example.Output) == 0 {
				t.Fatal("activity example requires input and output")
			}
			test.checkModels(t, example)
		})
	}
}

func checkActivityModel[T any](t *testing.T, raw json.RawMessage) {
	t.Helper()
	var value T
	decodeActivityJSON(t, raw, &value)
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode activity model: %v", err)
	}
	var expected, actual map[string]json.RawMessage
	decodeActivityJSON(t, raw, &expected)
	decodeActivityJSON(t, encoded, &actual)
	for key, want := range expected {
		got, ok := actual[key]
		if !ok {
			t.Errorf("round trip dropped field %q", key)
			continue
		}
		var wantValue, gotValue any
		decodeActivityJSON(t, want, &wantValue)
		decodeActivityJSON(t, got, &gotValue)
		if !reflect.DeepEqual(gotValue, wantValue) {
			t.Errorf("round trip field %q = %v, want %v", key, gotValue, wantValue)
		}
	}

	expected["unknown_contract_field"] = json.RawMessage(`true`)
	unknown, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("encode unknown field example: %v", err)
	}
	var invalid T
	decoder := json.NewDecoder(bytes.NewReader(unknown))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&invalid); err == nil {
		t.Error("activity model accepted an unknown JSON field")
	}
}

func assertStringField(t *testing.T, raw json.RawMessage, key, want string) {
	t.Helper()
	var object map[string]json.RawMessage
	decodeActivityJSON(t, raw, &object)
	var got string
	decodeActivityJSON(t, object[key], &got)
	if got != want {
		t.Errorf("example field %q = %q, want %q", key, got, want)
	}
}

func decodeActivityJSON(t *testing.T, data []byte, into any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		t.Fatalf("decode activity JSON: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("activity JSON has trailing data: %v", err)
	}
}
