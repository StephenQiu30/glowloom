package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestNodeOutputContractCanonicalizesTypedPortBindings(t *testing.T) {
	output := workflow.NodeOutputSnapshot{
		SchemaVersion: workflow.NodeOutputSchemaVersion,
		Bindings: []workflow.NodeOutputBinding{
			{
				Port: "storyboards", ValueType: "storyboards",
				ReferenceID: "00000000-0000-0000-0000-000000000202", ReferenceVersion: "2",
				ContentHash: strings.Repeat("b", 64),
			},
			{
				Port: "bible", ValueType: "production_bible",
				ReferenceID: "00000000-0000-0000-0000-000000000101", ReferenceVersion: "1",
				ContentHash: strings.Repeat("a", 64),
			},
		},
	}
	normalized, canonical, outputHash, err := workflow.BuildNodeOutput(output)
	if err != nil {
		t.Fatalf("build node output: %v", err)
	}
	if len(outputHash) != 64 || normalized.Bindings[0].Port != "bible" || normalized.Bindings[1].Port != "storyboards" {
		t.Fatalf("normalized node output = %#v hash %q", normalized, outputHash)
	}

	reordered := output
	reordered.Bindings = []workflow.NodeOutputBinding{output.Bindings[1], output.Bindings[0]}
	_, reorderedCanonical, reorderedHash, err := workflow.BuildNodeOutput(reordered)
	if err != nil || string(reorderedCanonical) != string(canonical) || reorderedHash != outputHash {
		t.Fatalf("binding order changed output identity: output=%s hash=%s err=%v", reorderedCanonical, reorderedHash, err)
	}

	decoded, decodedCanonical, decodedHash, err := workflow.ParseNodeOutput(json.RawMessage(`{
		"bindings":[
			{"content_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","reference_version":"2","reference_id":"00000000-0000-0000-0000-000000000202","value_type":"storyboards","port":"storyboards"},
			{"content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reference_version":"1","reference_id":"00000000-0000-0000-0000-000000000101","value_type":"production_bible","port":"bible"}
		],
		"schema_version":"node-output"
	}`))
	if err != nil || decodedHash != outputHash || string(decodedCanonical) != string(canonical) || decoded.Bindings[0].Port != "bible" {
		t.Fatalf("parse canonical output: output=%#v canonical=%s hash=%s err=%v", decoded, decodedCanonical, decodedHash, err)
	}
}

func TestNodeOutputContractRejectsAmbiguousOrInvalidBindings(t *testing.T) {
	valid := workflow.NodeOutputSnapshot{
		SchemaVersion: workflow.NodeOutputSchemaVersion,
		Bindings: []workflow.NodeOutputBinding{{
			Port: "candidate", ValueType: "production_bible_candidate",
			ReferenceID: "00000000-0000-0000-0000-000000000101", ReferenceVersion: "1",
			ContentHash: strings.Repeat("a", 64),
		}},
	}
	mutations := []func(*workflow.NodeOutputSnapshot){
		func(value *workflow.NodeOutputSnapshot) { value.SchemaVersion = "unknown-node-output" },
		func(value *workflow.NodeOutputSnapshot) { value.Bindings = nil },
		func(value *workflow.NodeOutputSnapshot) { value.Bindings[0].Port = "Invalid Port" },
		func(value *workflow.NodeOutputSnapshot) { value.Bindings[0].ValueType = "Invalid Type" },
		func(value *workflow.NodeOutputSnapshot) { value.Bindings[0].ReferenceID = "not-a-uuid" },
		func(value *workflow.NodeOutputSnapshot) { value.Bindings[0].ReferenceVersion = "" },
		func(value *workflow.NodeOutputSnapshot) { value.Bindings[0].ContentHash = "not-a-hash" },
		func(value *workflow.NodeOutputSnapshot) {
			value.Bindings = append(value.Bindings, value.Bindings[0])
		},
	}
	for index, mutate := range mutations {
		changed := valid
		changed.Bindings = append([]workflow.NodeOutputBinding(nil), valid.Bindings...)
		mutate(&changed)
		if _, _, _, err := workflow.BuildNodeOutput(changed); err == nil {
			t.Fatalf("invalid node output mutation %d was accepted: %#v", index, changed)
		}
	}
	if _, _, _, err := workflow.ParseNodeOutput(json.RawMessage(`{"schema_version":"node-output","bindings":[],"extra":true}`)); err == nil {
		t.Fatal("node output parser accepted an unknown field")
	}
}
