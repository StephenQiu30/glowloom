package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

func TestEventEnvelopeV2SchemaIsTheContractSource(t *testing.T) {
	content, err := os.ReadFile("../../../api/event/event-envelope-v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		AdditionalProperties bool           `json:"additionalProperties"`
		Required             []string       `json:"required"`
		Properties           map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.AdditionalProperties {
		t.Fatal("event envelope schema must reject unknown fields")
	}
	want := []string{
		"aggregate_id",
		"aggregate_type",
		"aggregate_version",
		"correlation_id",
		"envelope_version",
		"event_id",
		"event_type",
		"occurred_at",
		"partition_key",
		"payload",
		"schema_version",
		"traceparent",
		"workspace_id",
	}
	sort.Strings(schema.Required)
	if !reflect.DeepEqual(schema.Required, want) {
		t.Fatalf("required fields = %#v, want %#v", schema.Required, want)
	}
	for _, field := range append(want, "causation_id") {
		if _, exists := schema.Properties[field]; !exists {
			t.Fatalf("schema property %q is missing", field)
		}
	}
}
