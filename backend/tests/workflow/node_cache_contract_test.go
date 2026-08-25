package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

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

	first, firstHash, err := workflow.CanonicalNodeOutput(json.RawMessage(`{"z":2,"artifact_ids":["a"],"a":1}`))
	if err != nil {
		t.Fatalf("canonicalize node output: %v", err)
	}
	second, secondHash, err := workflow.CanonicalNodeOutput(json.RawMessage(`{"a":1,"z":2,"artifact_ids":["a"]}`))
	if err != nil || string(first) != string(second) || firstHash != secondHash || len(firstHash) != 64 {
		t.Fatalf("node output is not canonical: first=%s/%s second=%s/%s err=%v", first, firstHash, second, secondHash, err)
	}
	if _, _, err = workflow.CanonicalNodeOutput(json.RawMessage(`["not","an","object"]`)); err == nil {
		t.Fatal("node cache accepted a non-object output snapshot")
	}
}
