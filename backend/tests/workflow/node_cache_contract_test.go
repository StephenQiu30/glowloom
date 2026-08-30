package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestNodeCacheKeyCoversEveryFrozenExecutionInput(t *testing.T) {
	material := workflow.NodeCacheKeyMaterial{
		SchemaVersion:             workflow.NodeCacheKeySchemaVersion,
		NodeDefinitionContentHash: strings.Repeat("1", 64),
		ConfigHash:                strings.Repeat("2", 64),
		NormalizedInputHash:       strings.Repeat("3", 64),
		FrozenPolicyHash:          strings.Repeat("4", 64),
		FrozenModelHash:           strings.Repeat("5", 64),
		FrozenPromptHash:          strings.Repeat("6", 64),
		FrozenSkillHash:           strings.Repeat("7", 64),
		InputArtifactHashes:       []string{strings.Repeat("b", 64), strings.Repeat("a", 64)},
		RuntimeContractVersion:    "1.0.0",
	}
	normalized, key, err := workflow.BuildNodeCacheKey(material)
	if err != nil {
		t.Fatalf("build node cache key: %v", err)
	}
	if len(key) != 64 || len(normalized.InputArtifactHashes) != 2 ||
		normalized.InputArtifactHashes[0] != strings.Repeat("a", 64) {
		t.Fatalf("normalized node cache key = material %#v key %q", normalized, key)
	}

	reordered := material
	reordered.InputArtifactHashes = []string{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("a", 64)}
	_, reorderedKey, err := workflow.BuildNodeCacheKey(reordered)
	if err != nil || reorderedKey != key {
		t.Fatalf("artifact ordering changed cache identity: key=%q err=%v", reorderedKey, err)
	}

	mutations := []func(*workflow.NodeCacheKeyMaterial){
		func(value *workflow.NodeCacheKeyMaterial) { value.NodeDefinitionContentHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) { value.ConfigHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) { value.NormalizedInputHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) { value.FrozenPolicyHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) { value.FrozenModelHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) { value.FrozenPromptHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) { value.FrozenSkillHash = strings.Repeat("c", 64) },
		func(value *workflow.NodeCacheKeyMaterial) {
			value.InputArtifactHashes = []string{strings.Repeat("c", 64)}
		},
		func(value *workflow.NodeCacheKeyMaterial) { value.RuntimeContractVersion = "1.0.1" },
	}
	for index, mutate := range mutations {
		changed := material
		changed.InputArtifactHashes = append([]string(nil), material.InputArtifactHashes...)
		mutate(&changed)
		_, changedKey, changedErr := workflow.BuildNodeCacheKey(changed)
		if changedErr != nil || changedKey == key {
			t.Fatalf("mutation %d did not change cache identity: key=%q err=%v", index, changedKey, changedErr)
		}
	}
}

func TestNodeCacheContractRejectsIncompleteKeysAndCanonicalizesOutput(t *testing.T) {
	material := workflow.NodeCacheKeyMaterial{
		SchemaVersion: workflow.NodeCacheKeySchemaVersion, NodeDefinitionContentHash: strings.Repeat("1", 64),
		ConfigHash: strings.Repeat("2", 64), NormalizedInputHash: strings.Repeat("3", 64),
		RuntimeContractVersion: "1.0.0",
	}
	invalid := material
	invalid.ConfigHash = "not-a-hash"
	if _, _, err := workflow.BuildNodeCacheKey(invalid); err == nil {
		t.Fatal("node cache key accepted an invalid config hash")
	}
	invalid = material
	invalid.FrozenModelHash = "not-a-hash"
	if _, _, err := workflow.BuildNodeCacheKey(invalid); err == nil {
		t.Fatal("node cache key accepted an invalid optional frozen hash")
	}

	first, firstHash, err := workflow.CanonicalNodeOutput(json.RawMessage(`{
		"bindings":[{"value_type":"production_bible","port":"bible","reference_version":"1","content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reference_id":"00000000-0000-0000-0000-000000000101"}],
		"schema_version":"node-output"
	}`))
	if err != nil {
		t.Fatalf("canonicalize node output: %v", err)
	}
	second, secondHash, err := workflow.CanonicalNodeOutput(json.RawMessage(`{
		"schema_version":"node-output",
		"bindings":[{"port":"bible","value_type":"production_bible","reference_id":"00000000-0000-0000-0000-000000000101","reference_version":"1","content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]
	}`))
	if err != nil || string(first) != string(second) || firstHash != secondHash || len(firstHash) != 64 {
		t.Fatalf("node output is not canonical: first=%s/%s second=%s/%s err=%v", first, firstHash, second, secondHash, err)
	}
	if _, _, err = workflow.CanonicalNodeOutput(json.RawMessage(`["not","an","object"]`)); err == nil {
		t.Fatal("node cache accepted a non-object output snapshot")
	}
}

func TestNodeCacheMaterialDerivesFromFrozenNodeExecutionFacts(t *testing.T) {
	input := successfulNodeInput()
	input.Config = json.RawMessage(`{"temperature":0}`)
	input.Bindings = []workflow.NodeInputBinding{{
		Port: "script", ValueType: "script_revision", SourceKind: workflow.NodeInputSourceNodeOutput,
		SourceNodeID: "script", SourcePort: "script",
		ReferenceID: "00000000-0000-0000-0000-000000000101", ReferenceVersion: "1",
		ContentHash: strings.Repeat("c", 64),
	}}
	input.FrozenInputs = append(input.FrozenInputs, authoring.FrozenReference{
		Kind: "policy", ID: "00000000-0000-0000-0000-000000000202", Version: "1.0.0",
		Hash: strings.Repeat("e", 64),
	})
	material, cacheKey, err := workflow.BuildNodeCacheMaterial(workflow.NodeExecution{
		DefinitionContentHash: strings.Repeat("f", 64), CachePolicy: "by_inputs",
	}, input, "1.0.0")
	if err != nil || len(cacheKey) != 64 || material.ConfigHash == "" || material.NormalizedInputHash == "" ||
		material.FrozenPolicyHash != strings.Repeat("e", 64) || len(material.InputArtifactHashes) != 3 {
		t.Fatalf("derived node cache material = %#v key=%s err=%v", material, cacheKey, err)
	}
	changed := input
	changed.Config = json.RawMessage(`{"temperature":1}`)
	_, changedKey, changedErr := workflow.BuildNodeCacheMaterial(workflow.NodeExecution{
		DefinitionContentHash: strings.Repeat("f", 64), CachePolicy: "by_inputs",
	}, changed, "1.0.0")
	if changedErr != nil || changedKey == cacheKey {
		t.Fatalf("node config change did not invalidate cache: key=%s err=%v", changedKey, changedErr)
	}
}
