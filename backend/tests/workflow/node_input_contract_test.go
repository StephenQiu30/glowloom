package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestNodeInputContractCanonicalizesGraphBindingsAndFrozenReferences(t *testing.T) {
	input := workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion,
		Config:        json.RawMessage(`{"episode_count":12}`),
		Bindings: []workflow.NodeInputBinding{
			{
				Port: "script", ValueType: "script_revision", SourceKind: workflow.NodeInputSourceNodeOutput,
				SourceNodeID: "script", SourcePort: "script",
				ReferenceID: "00000000-0000-0000-0000-000000000101", ReferenceVersion: "1",
				ContentHash: strings.Repeat("a", 64),
			},
			{
				Port: "bible", ValueType: "production_bible", SourceKind: workflow.NodeInputSourceVariable,
				Variable: "selected_bible", Value: json.RawMessage(`{"hash":"` + strings.Repeat("b", 64) + `","id":"00000000-0000-0000-0000-000000000202"}`),
			},
		},
		FrozenInputs: []authoring.FrozenReference{
			{Kind: "script_revision", ID: "00000000-0000-0000-0000-000000000303", Version: "1", Hash: strings.Repeat("c", 64)},
			{Kind: "policy", ID: "00000000-0000-0000-0000-000000000404", Version: "1.0.0", Hash: strings.Repeat("d", 64)},
		},
	}
	normalized, canonical, inputHash, err := workflow.BuildNodeInput(input)
	if err != nil {
		t.Fatalf("build node input: %v", err)
	}
	if len(inputHash) != 64 || normalized.Bindings[0].Port != "bible" || normalized.Bindings[1].Port != "script" ||
		normalized.FrozenInputs[0].Kind != "policy" {
		t.Fatalf("normalized node input = %#v hash=%q", normalized, inputHash)
	}

	reordered := input
	reordered.Bindings = []workflow.NodeInputBinding{input.Bindings[1], input.Bindings[0]}
	reordered.FrozenInputs = []authoring.FrozenReference{input.FrozenInputs[1], input.FrozenInputs[0]}
	_, reorderedCanonical, reorderedHash, err := workflow.BuildNodeInput(reordered)
	if err != nil || string(reorderedCanonical) != string(canonical) || reorderedHash != inputHash {
		t.Fatalf("input ordering changed identity: canonical=%s hash=%s err=%v", reorderedCanonical, reorderedHash, err)
	}
	decoded, decodedCanonical, decodedHash, err := workflow.ParseNodeInput(canonical)
	if err != nil || decodedHash != inputHash || string(decodedCanonical) != string(canonical) || decoded.Bindings[0].Port != "bible" {
		t.Fatalf("parse node input: input=%#v canonical=%s hash=%s err=%v", decoded, decodedCanonical, decodedHash, err)
	}

	changed := input
	changed.Bindings = append([]workflow.NodeInputBinding(nil), input.Bindings...)
	changed.Bindings[0].ContentHash = strings.Repeat("e", 64)
	if _, _, changedHash, changedErr := workflow.BuildNodeInput(changed); changedErr != nil || changedHash == inputHash {
		t.Fatalf("upstream content change did not change input identity: hash=%s err=%v", changedHash, changedErr)
	}
}

func TestNodeInputContractRejectsAmbiguousBindingsAndOutputPortDrift(t *testing.T) {
	valid := workflow.NodeInputSnapshot{
		SchemaVersion: workflow.NodeInputSchemaVersion, Config: json.RawMessage(`{}`),
		Bindings: []workflow.NodeInputBinding{{
			Port: "script", ValueType: "script_revision", SourceKind: workflow.NodeInputSourceNodeOutput,
			SourceNodeID: "script", SourcePort: "script",
			ReferenceID: "00000000-0000-0000-0000-000000000101", ReferenceVersion: "1",
			ContentHash: strings.Repeat("a", 64),
		}},
		FrozenInputs: []authoring.FrozenReference{{
			Kind: "script_revision", ID: "00000000-0000-0000-0000-000000000202", Version: "1",
			Hash: strings.Repeat("b", 64),
		}},
	}
	invalid := valid
	invalid.Bindings = append([]workflow.NodeInputBinding(nil), valid.Bindings...)
	invalid.Bindings = append(invalid.Bindings, invalid.Bindings[0])
	if _, _, _, err := workflow.BuildNodeInput(invalid); err == nil {
		t.Fatal("node input accepted duplicate target ports")
	}
	invalid = valid
	invalid.Bindings = append([]workflow.NodeInputBinding(nil), valid.Bindings...)
	invalid.Bindings[0].Value = json.RawMessage(`{"node_output":"cannot also have a variable value"}`)
	if _, _, _, err := workflow.BuildNodeInput(invalid); err == nil {
		t.Fatal("node output input binding accepted a variable value")
	}
	if _, _, _, err := workflow.ParseNodeInput(json.RawMessage(`{"schema_version":"node-input","config":{},"bindings":[],"frozen_inputs":[],"extra":true}`)); err == nil {
		t.Fatal("node input parser accepted an unknown field")
	}

	output := successfulExecutorOutput()
	expected := []authoring.PortDefinition{{Key: "candidate", ValueType: "production_bible_candidate", Required: true}}
	if err := workflow.ValidateNodeOutputPorts(output, expected); err != nil {
		t.Fatalf("validate matching node output: %v", err)
	}
	missing := output
	missing.Bindings = nil
	if err := workflow.ValidateNodeOutputPorts(missing, expected); err == nil {
		t.Fatal("output port validation accepted a missing required binding")
	}
	drifted := output
	drifted.Bindings = append([]workflow.NodeOutputBinding(nil), output.Bindings...)
	drifted.Bindings[0].ValueType = "storyboard_candidate"
	if err := workflow.ValidateNodeOutputPorts(drifted, expected); err == nil {
		t.Fatal("output port validation accepted a value type drift")
	}
}
