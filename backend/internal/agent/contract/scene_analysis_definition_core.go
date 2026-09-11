package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const (
	SceneAnalysisDefinitionCoreContractID = "storygraph-agent-definition-core-production"
	sceneAnalysisDefinitionCoreID         = "storygraph-scene-analysis-definition-core-production"
	sceneAnalysisDefinitionCoreVersion    = "2026.09.12"
	sceneAnalysisWireSchemaContentHash    = "6e6f279469637292ba85fbb7eb929b2b359947b09d939fdc06163247291498fd"
)

type SceneAnalysisPatchApplication struct {
	Kind string `json:"kind"`
}

type SceneAnalysisDefinitionVariant struct {
	VariantKey                    SceneAnalysisStageVariant     `json:"variant_key"`
	CapabilityKey                 string                        `json:"capability_key"`
	Lane                          string                        `json:"lane"`
	RuntimeClass                  string                        `json:"runtime_class"`
	InputContractID               string                        `json:"input_contract_id"`
	InputSchemaHash               string                        `json:"input_schema_hash"`
	OutputContractID              string                        `json:"output_contract_id"`
	OutputSchemaHash              string                        `json:"output_schema_hash"`
	NormalizedCandidateContractID string                        `json:"normalized_candidate_contract_id"`
	NormalizedCandidateSchemaHash string                        `json:"normalized_candidate_schema_hash"`
	NormalizerContractRef         string                        `json:"normalizer_contract_ref"`
	NormalizerContractHash        string                        `json:"normalizer_contract_hash"`
	InvariantContractRef          string                        `json:"invariant_contract_ref"`
	InvariantContractHash         string                        `json:"invariant_contract_hash"`
	PatchApplication              SceneAnalysisPatchApplication `json:"patch_application"`
	ResourcePolicyRef             string                        `json:"resource_policy_ref"`
	ResourcePolicyHash            string                        `json:"resource_policy_hash"`
	ExecutionPolicyRef            string                        `json:"execution_policy_ref"`
	ExecutionPolicyConstraintHash string                        `json:"execution_policy_constraint_hash"`
	ModelPolicyRef                string                        `json:"model_policy_ref"`
	ModelPolicyConstraintHash     string                        `json:"model_policy_constraint_hash"`
	ToolPolicyRef                 string                        `json:"tool_policy_ref"`
	ToolPolicyConstraintHash      string                        `json:"tool_policy_constraint_hash"`
}

type SceneAnalysisDefinitionCore struct {
	ContractID         string                           `json:"contract_id"`
	DefinitionCoreID   string                           `json:"definition_core_id"`
	DefinitionVersion  string                           `json:"definition_core_version"`
	WireSchemaID       string                           `json:"wire_schema_id"`
	WireSchemaHash     string                           `json:"wire_schema_hash"`
	VariantContracts   []SceneAnalysisDefinitionVariant `json:"variant_contracts"`
	DefinitionCoreHash string                           `json:"definition_core_hash"`
}

type sceneAnalysisExecutionPolicy struct {
	ContractID          string `json:"contract_id"`
	MaxModelCalls       int    `json:"max_model_calls"`
	MaxExecutionSeconds int    `json:"max_execution_seconds"`
	MaxOutputBytes      int    `json:"max_output_bytes"`
}

type sceneAnalysisModelPolicy struct {
	ContractID    string `json:"contract_id"`
	CapabilityKey string `json:"capability_key"`
	RuntimeClass  string `json:"runtime_class"`
}

type sceneAnalysisToolPolicy struct {
	ContractID   string   `json:"contract_id"`
	AllowedTools []string `json:"allowed_tools"`
}

type sceneAnalysisResourcePolicy struct {
	ContractID       string   `json:"contract_id"`
	BundleEntrypoint string   `json:"bundle_entrypoint"`
	ReferenceRefs    []string `json:"reference_refs"`
}

type sceneAnalysisNormalizerContract struct {
	ContractID            string `json:"contract_id"`
	StageKey              string `json:"stage_key"`
	ProfileKey            string `json:"profile_key"`
	InputContractID       string `json:"input_contract_id"`
	OutputContractID      string `json:"output_contract_id"`
	OutputSchemaHash      string `json:"output_schema_hash"`
	InvariantContractHash string `json:"invariant_contract_hash"`
	ValidationMode        string `json:"validation_mode"`
}

type sceneAnalysisInvariantContract struct {
	ContractID       string `json:"contract_id"`
	StageKey         string `json:"stage_key"`
	ProfileKey       string `json:"profile_key"`
	InputSchemaHash  string `json:"input_schema_hash"`
	OutputSchemaHash string `json:"output_schema_hash"`
	Enforcement      string `json:"enforcement"`
}

func BuildSceneAnalysisDefinitionCore() (SceneAnalysisDefinitionCore, json.RawMessage, error) {
	variants := make([]SceneAnalysisDefinitionVariant, 0, len(sceneAnalysisCandidateSchemas))
	for index, output := range sceneAnalysisCandidateSchemas {
		input := sceneAnalysisInputSchemas[index]
		variant, err := buildSceneAnalysisDefinitionVariant(input, output)
		if err != nil {
			return SceneAnalysisDefinitionCore{}, nil, err
		}
		variants = append(variants, variant)
	}
	core := SceneAnalysisDefinitionCore{
		ContractID: SceneAnalysisDefinitionCoreContractID, DefinitionCoreID: sceneAnalysisDefinitionCoreID,
		DefinitionVersion: sceneAnalysisDefinitionCoreVersion, WireSchemaID: SceneAnalysisWireSchemaVersion,
		WireSchemaHash: sceneAnalysisWireSchemaContentHash, VariantContracts: variants,
	}
	hash, err := sceneAnalysisDefinitionCoreHash(core)
	if err != nil {
		return SceneAnalysisDefinitionCore{}, nil, err
	}
	core.DefinitionCoreHash = hash
	encoded, err := encodeSceneAnalysisDefinitionCore(core)
	if err != nil {
		return SceneAnalysisDefinitionCore{}, nil, err
	}
	return core, encoded, nil
}

func DecodeSceneAnalysisDefinitionCore(raw json.RawMessage) (SceneAnalysisDefinitionCore, json.RawMessage, error) {
	var core SceneAnalysisDefinitionCore
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&core); err != nil {
		return SceneAnalysisDefinitionCore{}, nil, errors.New("invalid Scene Analysis Definition Core")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SceneAnalysisDefinitionCore{}, nil, errors.New("invalid Scene Analysis Definition Core")
	}
	expected, _, err := BuildSceneAnalysisDefinitionCore()
	if err != nil || !reflect.DeepEqual(core, expected) {
		return SceneAnalysisDefinitionCore{}, nil, errors.New("invalid Scene Analysis Definition Core")
	}
	encoded, err := encodeSceneAnalysisDefinitionCore(core)
	if err != nil {
		return SceneAnalysisDefinitionCore{}, nil, err
	}
	return core, encoded, nil
}

func buildSceneAnalysisDefinitionVariant(
	input SceneAnalysisInputSchema,
	output SceneAnalysisCandidateSchema,
) (SceneAnalysisDefinitionVariant, error) {
	if input.StageKey != output.StageKey || input.ProfileKey != output.ProfileKey {
		return SceneAnalysisDefinitionVariant{}, errors.New("Scene Analysis schema registries do not align")
	}
	variantKey := SceneAnalysisStageVariant{
		StageKey: output.StageKey, ProfileKey: output.ProfileKey, LaneKey: "primary",
		OutputSchemaVersion: output.OutputSchemaVersion,
	}
	if variantKey.Validate() != nil {
		return SceneAnalysisDefinitionVariant{}, errors.New("invalid Scene Analysis Definition variant")
	}
	capabilityKey := sceneAnalysisCapabilityKey(output.StageKey)
	lane := "style_blind_text"
	runtimeClass := "text"
	modelCapability := "structured_text"
	invariantEnforcement := "backend-scene-analysis-attempt-result-validate-for"
	if output.StageKey == VisualFoundationStageKey {
		lane = "preset_visual"
		runtimeClass = "vision"
		modelCapability = "vision"
		invariantEnforcement = "backend-visual-foundation-attempt-result-validate-for"
	}
	resourcePolicy := sceneAnalysisResourcePolicy{
		ContractID: "scene-analysis-loaded-resource-policy-production", BundleEntrypoint: "SKILL.md",
		ReferenceRefs: []string{sceneAnalysisReferencePath(output.StageKey)},
	}
	executionPolicy := sceneAnalysisExecutionPolicy{
		ContractID: "scene-analysis-execution-policy-production", MaxModelCalls: 1,
		MaxExecutionSeconds: 120, MaxOutputBytes: 131072,
	}
	modelPolicy := sceneAnalysisModelPolicy{
		ContractID: "scene-analysis-model-policy-production", CapabilityKey: modelCapability, RuntimeClass: runtimeClass,
	}
	toolPolicy := sceneAnalysisToolPolicy{
		ContractID: "scene-analysis-tool-policy-production", AllowedTools: []string{},
	}
	invariant := sceneAnalysisInvariantContract{
		ContractID: "scene-analysis-stage-invariant-production", StageKey: output.StageKey,
		ProfileKey: output.ProfileKey, InputSchemaHash: input.SchemaHash, OutputSchemaHash: output.SchemaHash,
		Enforcement: invariantEnforcement,
	}
	invariantHash, err := sceneAnalysisDefinitionArtifactHash(invariant)
	if err != nil {
		return SceneAnalysisDefinitionVariant{}, err
	}
	normalizer := sceneAnalysisNormalizerContract{
		ContractID: "scene-analysis-strict-candidate-validation-production", StageKey: output.StageKey,
		ProfileKey: output.ProfileKey, InputContractID: input.InputContractID,
		OutputContractID: output.CandidateType, OutputSchemaHash: output.SchemaHash,
		InvariantContractHash: invariantHash,
		ValidationMode:        "strict_decode_validate_and_canonical_hash",
	}
	resourceHash, err := sceneAnalysisDefinitionArtifactHash(resourcePolicy)
	if err != nil {
		return SceneAnalysisDefinitionVariant{}, err
	}
	executionHash, err := sceneAnalysisDefinitionArtifactHash(executionPolicy)
	if err != nil {
		return SceneAnalysisDefinitionVariant{}, err
	}
	modelHash, err := sceneAnalysisDefinitionArtifactHash(modelPolicy)
	if err != nil {
		return SceneAnalysisDefinitionVariant{}, err
	}
	toolHash, err := sceneAnalysisDefinitionArtifactHash(toolPolicy)
	if err != nil {
		return SceneAnalysisDefinitionVariant{}, err
	}
	normalizerHash, err := sceneAnalysisDefinitionArtifactHash(normalizer)
	if err != nil {
		return SceneAnalysisDefinitionVariant{}, err
	}
	return SceneAnalysisDefinitionVariant{
		VariantKey: variantKey, CapabilityKey: capabilityKey, Lane: lane, RuntimeClass: runtimeClass,
		InputContractID: input.InputContractID, InputSchemaHash: input.SchemaHash,
		OutputContractID: output.CandidateType, OutputSchemaHash: output.SchemaHash,
		NormalizedCandidateContractID: output.CandidateType, NormalizedCandidateSchemaHash: output.SchemaHash,
		NormalizerContractRef: normalizer.ContractID, NormalizerContractHash: normalizerHash,
		InvariantContractRef: invariant.ContractID, InvariantContractHash: invariantHash,
		PatchApplication:  SceneAnalysisPatchApplication{Kind: "none"},
		ResourcePolicyRef: resourcePolicy.ContractID, ResourcePolicyHash: resourceHash,
		ExecutionPolicyRef: executionPolicy.ContractID, ExecutionPolicyConstraintHash: executionHash,
		ModelPolicyRef: modelPolicy.ContractID, ModelPolicyConstraintHash: modelHash,
		ToolPolicyRef: toolPolicy.ContractID, ToolPolicyConstraintHash: toolHash,
	}, nil
}

func sceneAnalysisCapabilityKey(stageKey string) string {
	switch stageKey {
	case "propose_script_spans", "extract_scene_facts":
		return "parse-script-structure"
	case "resolve_identities", "derive_production_entities", "bind_scene_occurrences":
		return "build-production-bible"
	case "reconcile_interaction_continuity":
		return "map-scene-continuity"
	case VisualFoundationStageKey:
		return "resolve-visual-foundation"
	case "review_candidate":
		return "review-production"
	default:
		return ""
	}
}

func sceneAnalysisReferencePath(stageKey string) string {
	return map[string]string{
		"propose_script_spans": "references/script-spans.md", "extract_scene_facts": "references/scene-facts.md",
		"resolve_identities": "references/entity-reconciliation.md", "review_candidate": "references/structure-identity-review.md",
		"derive_production_entities": "references/production-entities.md", "bind_scene_occurrences": "references/scene-occurrences.md",
		"reconcile_interaction_continuity": "references/interaction-continuity.md",
		VisualFoundationStageKey:           "references/visual-identity.md",
	}[stageKey]
}

func sceneAnalysisDefinitionArtifactHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func sceneAnalysisDefinitionCoreHash(core SceneAnalysisDefinitionCore) (string, error) {
	core.DefinitionCoreHash = ""
	raw, err := json.Marshal(core)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	delete(root, "definition_core_hash")
	raw, err = json.Marshal(root)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func encodeSceneAnalysisDefinitionCore(core SceneAnalysisDefinitionCore) (json.RawMessage, error) {
	raw, err := json.Marshal(core)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
