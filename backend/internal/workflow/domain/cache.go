package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"time"
)

const NodeCacheKeySchemaVersion = "node-cache-key-v1"

var nodeCacheHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type NodeCacheKeyMaterial struct {
	SchemaVersion             string   `json:"schema_version"`
	NodeDefinitionContentHash string   `json:"node_definition_content_hash"`
	ConfigHash                string   `json:"config_hash"`
	NormalizedInputHash       string   `json:"normalized_input_hash"`
	FrozenPolicyHash          string   `json:"frozen_policy_hash,omitempty"`
	FrozenModelHash           string   `json:"frozen_model_hash,omitempty"`
	FrozenPromptHash          string   `json:"frozen_prompt_hash,omitempty"`
	FrozenSkillHash           string   `json:"frozen_skill_hash,omitempty"`
	InputArtifactHashes       []string `json:"input_artifact_hashes,omitempty"`
	RuntimeContractVersion    string   `json:"runtime_contract_version"`
}

type NodeCacheEntry struct {
	ID, WorkspaceID, CacheKey                        string
	KeyMaterial                                      NodeCacheKeyMaterial
	Output                                           json.RawMessage
	OutputHash, SourceWorkflowRunID, SourceNodeRunID string
	CreatedAt                                        time.Time
}

func BuildNodeCacheKey(material NodeCacheKeyMaterial) (NodeCacheKeyMaterial, string, error) {
	if material.SchemaVersion != NodeCacheKeySchemaVersion ||
		!nodeCacheHashPattern.MatchString(material.NodeDefinitionContentHash) ||
		!nodeCacheHashPattern.MatchString(material.ConfigHash) ||
		!nodeCacheHashPattern.MatchString(material.NormalizedInputHash) ||
		!compilerVersionPattern.MatchString(material.RuntimeContractVersion) ||
		!validOptionalNodeCacheHash(material.FrozenPolicyHash) ||
		!validOptionalNodeCacheHash(material.FrozenModelHash) ||
		!validOptionalNodeCacheHash(material.FrozenPromptHash) ||
		!validOptionalNodeCacheHash(material.FrozenSkillHash) {
		return NodeCacheKeyMaterial{}, "", errors.New("invalid node cache key material")
	}

	artifactHashes := append([]string(nil), material.InputArtifactHashes...)
	for _, value := range artifactHashes {
		if !nodeCacheHashPattern.MatchString(value) {
			return NodeCacheKeyMaterial{}, "", errors.New("invalid node cache artifact hash")
		}
	}
	slices.Sort(artifactHashes)
	artifactHashes = slices.Compact(artifactHashes)
	if len(artifactHashes) == 0 {
		artifactHashes = nil
	}
	material.InputArtifactHashes = artifactHashes
	encoded, err := json.Marshal(material)
	if err != nil {
		return NodeCacheKeyMaterial{}, "", err
	}
	return material, sha256Hex(encoded), nil
}

func BuildNodeCacheMaterial(
	execution NodeExecution,
	input NodeInputSnapshot,
	runtimeContractVersion string,
) (NodeCacheKeyMaterial, string, error) {
	if !nodeCacheHashPattern.MatchString(execution.DefinitionContentHash) ||
		(execution.CachePolicy != "never" && execution.CachePolicy != "by_inputs") {
		return NodeCacheKeyMaterial{}, "", errors.New("invalid cacheable node execution")
	}
	normalizedInput, _, inputHash, err := BuildNodeInput(input)
	if err != nil {
		return NodeCacheKeyMaterial{}, "", err
	}
	material := NodeCacheKeyMaterial{
		SchemaVersion: NodeCacheKeySchemaVersion, NodeDefinitionContentHash: execution.DefinitionContentHash,
		ConfigHash: sha256Hex(normalizedInput.Config), NormalizedInputHash: inputHash,
		RuntimeContractVersion: runtimeContractVersion,
	}
	artifactHashes := make([]string, 0, len(normalizedInput.Bindings)+len(normalizedInput.FrozenInputs))
	for _, binding := range normalizedInput.Bindings {
		if binding.SourceKind == NodeInputSourceNodeOutput {
			artifactHashes = append(artifactHashes, binding.ContentHash)
		}
	}
	frozenByKind := make(map[string][]string)
	for _, reference := range normalizedInput.FrozenInputs {
		artifactHashes = append(artifactHashes, reference.Hash)
		switch reference.Kind {
		case "policy", "model", "prompt", "skill":
			frozenByKind[reference.Kind] = append(frozenByKind[reference.Kind], reference.Hash)
		}
	}
	material.InputArtifactHashes = artifactHashes
	material.FrozenPolicyHash = aggregateFrozenNodeCacheHash(frozenByKind["policy"])
	material.FrozenModelHash = aggregateFrozenNodeCacheHash(frozenByKind["model"])
	material.FrozenPromptHash = aggregateFrozenNodeCacheHash(frozenByKind["prompt"])
	material.FrozenSkillHash = aggregateFrozenNodeCacheHash(frozenByKind["skill"])
	return BuildNodeCacheKey(material)
}

func aggregateFrozenNodeCacheHash(values []string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}
	encoded, _ := json.Marshal(values)
	return sha256Hex(encoded)
}

func validOptionalNodeCacheHash(value string) bool {
	return value == "" || nodeCacheHashPattern.MatchString(value)
}
