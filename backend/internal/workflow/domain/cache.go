package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

func CanonicalNodeOutput(raw json.RawMessage) (json.RawMessage, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, "", errors.New("invalid node output snapshot")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, "", errors.New("invalid node output snapshot")
	}
	if _, object := value.(map[string]any); !object {
		return nil, "", errors.New("node output snapshot must be an object")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	return encoded, sha256Hex(encoded), nil
}

func validOptionalNodeCacheHash(value string) bool {
	return value == "" || nodeCacheHashPattern.MatchString(value)
}
