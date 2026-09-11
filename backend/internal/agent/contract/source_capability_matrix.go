package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"slices"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const StoryGraphSourceCapabilityMatrixContractID = "storygraph-external-source-capability-matrix-production"

var storyGraphAllowedSourceCapabilities = []string{
	"build-production-bible",
	"design-reference-assets",
	"direct-storyboard",
	"map-scene-continuity",
	"parse-script-structure",
	"resolve-visual-foundation",
	"review-production",
}

//go:embed manifests/storygraph-source-capability-matrix.json
var pinnedStoryGraphSourceCapabilityMatrix []byte

type ExternalSkillRewriteCapability struct {
	SourceID                 string   `json:"source_id"`
	Capability               string   `json:"capability"`
	RewrittenProductionPaths []string `json:"rewritten_production_paths"`
}

type ExternalSkillRuntimePolicy struct {
	ExternalSkillDownload    bool `json:"external_skill_download"`
	NetworkDiscovery         bool `json:"network_discovery"`
	SourceProjectPathLoading bool `json:"source_project_path_loading"`
}

type StoryGraphSourceCapabilityMatrix struct {
	ContractID           string                           `json:"contract_id"`
	SourceMappingRoot    string                           `json:"source_mapping_root"`
	AllowedCapabilities  []string                         `json:"allowed_capabilities"`
	RewriteMappings      []ExternalSkillRewriteCapability `json:"rewrite_mappings"`
	RuntimePolicy        ExternalSkillRuntimePolicy       `json:"runtime_policy"`
	CapabilityMatrixRoot string                           `json:"capability_matrix_root"`
}

func PinnedStoryGraphSourceCapabilityMatrix() (StoryGraphSourceCapabilityMatrix, json.RawMessage, error) {
	return DecodeStoryGraphSourceCapabilityMatrix(pinnedStoryGraphSourceCapabilityMatrix)
}

func DecodeStoryGraphSourceCapabilityMatrix(raw json.RawMessage) (StoryGraphSourceCapabilityMatrix, json.RawMessage, error) {
	var matrix StoryGraphSourceCapabilityMatrix
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&matrix); err != nil {
		return StoryGraphSourceCapabilityMatrix{}, nil, errors.New("invalid StoryGraph Source Capability Matrix")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return StoryGraphSourceCapabilityMatrix{}, nil, errors.New("invalid StoryGraph Source Capability Matrix")
	}
	if err := validateStoryGraphSourceCapabilityMatrix(matrix); err != nil {
		return StoryGraphSourceCapabilityMatrix{}, nil, err
	}
	encoded, err := encodeStoryGraphSourceCapabilityMatrix(matrix)
	if err != nil {
		return StoryGraphSourceCapabilityMatrix{}, nil, err
	}
	return matrix, encoded, nil
}

func validateStoryGraphSourceCapabilityMatrix(matrix StoryGraphSourceCapabilityMatrix) error {
	mapping, _, err := PinnedStoryGraphSourceMapping()
	if err != nil {
		return errors.New("invalid StoryGraph Source Capability Matrix mapping")
	}
	if matrix.ContractID != StoryGraphSourceCapabilityMatrixContractID ||
		matrix.SourceMappingRoot != mapping.SourceMappingRoot ||
		!slices.Equal(matrix.AllowedCapabilities, storyGraphAllowedSourceCapabilities) ||
		matrix.RewriteMappings == nil || !hashPattern.MatchString(matrix.CapabilityMatrixRoot) {
		return errors.New("invalid StoryGraph Source Capability Matrix")
	}
	if matrix.RuntimePolicy.ExternalSkillDownload || matrix.RuntimePolicy.NetworkDiscovery ||
		matrix.RuntimePolicy.SourceProjectPathLoading {
		return errors.New("StoryGraph external Skill runtime isolation was bypassed")
	}

	rewriteSources := make(map[string]ExternalSkillSourceMapping)
	for _, source := range mapping.Mappings {
		if source.Classification == "rewrite" {
			rewriteSources[source.SourceID] = source
		}
	}
	if len(matrix.RewriteMappings) != len(rewriteSources) {
		return errors.New("StoryGraph external Skill rewrite capability coverage has drifted")
	}
	for index, rewrite := range matrix.RewriteMappings {
		if index > 0 && matrix.RewriteMappings[index-1].SourceID >= rewrite.SourceID ||
			!slices.Contains(matrix.AllowedCapabilities, rewrite.Capability) ||
			len(rewrite.RewrittenProductionPaths) == 0 || !slices.IsSorted(rewrite.RewrittenProductionPaths) {
			return errors.New("invalid StoryGraph external Skill rewrite capability")
		}
		for pathIndex := 1; pathIndex < len(rewrite.RewrittenProductionPaths); pathIndex++ {
			if rewrite.RewrittenProductionPaths[pathIndex-1] == rewrite.RewrittenProductionPaths[pathIndex] {
				return errors.New("invalid StoryGraph external Skill rewrite capability")
			}
		}
		source, exists := rewriteSources[rewrite.SourceID]
		if !exists || !slices.Equal(rewrite.RewrittenProductionPaths, source.RewrittenProductionPaths) {
			return errors.New("StoryGraph external Skill rewrite escaped its reviewed mapping")
		}
	}

	hash, err := storyGraphSourceCapabilityMatrixHash(matrix)
	if err != nil || hash != matrix.CapabilityMatrixRoot {
		return errors.New("StoryGraph Source Capability Matrix root has drifted")
	}
	return nil
}

func storyGraphSourceCapabilityMatrixHash(matrix StoryGraphSourceCapabilityMatrix) (string, error) {
	matrix.CapabilityMatrixRoot = ""
	raw, err := json.Marshal(matrix)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	delete(root, "capability_matrix_root")
	raw, err = json.Marshal(root)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeStoryGraphSourceCapabilityMatrix(matrix StoryGraphSourceCapabilityMatrix) (json.RawMessage, error) {
	raw, err := json.Marshal(matrix)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
