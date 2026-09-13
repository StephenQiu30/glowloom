package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

const SceneAnalysisStageReleaseContractID = "storygraph-stage-release-production"

type SceneAnalysisStageRelease struct {
	ContractID                    string                        `json:"contract_id"`
	WireSchemaID                  string                        `json:"wire_schema_id"`
	WireSchemaHash                string                        `json:"wire_schema_hash"`
	AgentDefinitionCoreHash       string                        `json:"agent_definition_core_hash"`
	VariantKey                    SceneAnalysisStageVariant     `json:"variant_key"`
	CapabilityKey                 string                        `json:"capability_key"`
	Lane                          string                        `json:"lane"`
	RuntimeClass                  string                        `json:"runtime_class"`
	BundleContentHash             string                        `json:"bundle_content_hash"`
	InputContractID               string                        `json:"input_contract_id"`
	InputSchemaHash               string                        `json:"input_schema_hash"`
	OutputContractID              string                        `json:"output_contract_id"`
	OutputSchemaHash              string                        `json:"output_schema_hash"`
	NormalizedCandidateContractID string                        `json:"normalized_candidate_contract_id"`
	NormalizedCandidateSchemaHash string                        `json:"normalized_candidate_schema_hash"`
	NormalizerContractRef         string                        `json:"normalizer_contract_ref"`
	NormalizerContractHash        string                        `json:"normalizer_contract_hash"`
	PatchApplication              SceneAnalysisPatchApplication `json:"patch_application"`
	PromptCompilerRef             string                        `json:"prompt_compiler_ref"`
	PromptCompilerHash            string                        `json:"prompt_compiler_hash"`
	ReferenceRefs                 []BundleFile                  `json:"reference_refs"`
	RecipeRefs                    []BundleFile                  `json:"recipe_refs"`
	RubricRefs                    []BundleFile                  `json:"rubric_refs"`
	LoadedResourceSetHash         string                        `json:"loaded_resource_set_hash"`
	ExecutionPolicyRef            string                        `json:"execution_policy_ref"`
	ExecutionPolicyHash           string                        `json:"execution_policy_hash"`
	ModelPolicyRef                string                        `json:"model_policy_ref"`
	ModelPolicyHash               string                        `json:"model_policy_hash"`
	ToolPolicyRef                 string                        `json:"tool_policy_ref"`
	ToolPolicyHash                string                        `json:"tool_policy_hash"`
	RuntimeImageDigest            string                        `json:"runtime_image_digest"`
	StageReleaseHash              string                        `json:"stage_release_hash"`
}

type sceneAnalysisPromptCompiler struct {
	ContractID       string   `json:"contract_id"`
	Harness          string   `json:"harness"`
	GuidanceOrder    []string `json:"guidance_order"`
	WireSchemaHash   string   `json:"wire_schema_hash"`
	OutputSchemaHash string   `json:"output_schema_hash"`
}

type sceneAnalysisLoadedResourceSet struct {
	ContractID string       `json:"contract_id"`
	Resources  []BundleFile `json:"resources"`
}

func BuildSceneAnalysisStageReleases(runtimeImageDigest string) ([]SceneAnalysisStageRelease, error) {
	if !strings.HasPrefix(runtimeImageDigest, "sha256:") ||
		!hashPattern.MatchString(strings.TrimPrefix(runtimeImageDigest, "sha256:")) {
		return nil, errors.New("invalid Scene Analysis runtime image digest")
	}
	core, _, err := BuildSceneAnalysisDefinitionCore()
	if err != nil {
		return nil, err
	}
	bundle, err := PinnedStoryGraphBundleManifest()
	if err != nil {
		return nil, err
	}
	entrypoint, ok := bundleFileByPath(bundle, bundle.BundleEntrypoint)
	if !ok {
		return nil, errors.New("StoryGraph bundle entrypoint is missing")
	}
	releases := make([]SceneAnalysisStageRelease, 0, len(core.VariantContracts))
	for _, variant := range core.VariantContracts {
		resource, exists := bundleFileByPath(bundle, sceneAnalysisReferencePath(variant.VariantKey.StageKey))
		if !exists {
			return nil, errors.New("Scene Analysis resource is missing from the pinned bundle")
		}
		referenceRefs := []BundleFile{resource}
		rubricRefs := []BundleFile{}
		if variant.VariantKey.StageKey == "review_candidate" {
			referenceRefs = []BundleFile{}
			rubricRefs = []BundleFile{resource}
		}
		harness := "scene-analysis-harness"
		if variant.VariantKey.StageKey == VisualFoundationStageKey {
			harness = "visual-foundation-harness"
		} else if variant.VariantKey.StageKey == "plan_reference_assets" {
			harness = "reference-plan-harness"
		} else if variant.VariantKey.StageKey == ReferenceBriefStageKey {
			harness = "reference-brief-harness"
		} else if variant.VariantKey.StageKey == VisionReviewStageKey {
			harness = "vision-review-harness"
		}
		promptCompiler := sceneAnalysisPromptCompiler{
			ContractID: "scene-analysis-prompt-compiler-production", Harness: harness,
			GuidanceOrder: []string{entrypoint.Path, resource.Path}, WireSchemaHash: core.WireSchemaHash,
			OutputSchemaHash: variant.OutputSchemaHash,
		}
		promptCompilerHash, hashErr := sceneAnalysisStageArtifactHash(promptCompiler)
		if hashErr != nil {
			return nil, hashErr
		}
		loadedResources := []BundleFile{entrypoint, resource}
		slices.SortFunc(loadedResources, func(left, right BundleFile) int { return strings.Compare(left.Path, right.Path) })
		loadedResourceSetHash, hashErr := sceneAnalysisStageArtifactHash(sceneAnalysisLoadedResourceSet{
			ContractID: "scene-analysis-loaded-resource-set-production", Resources: loadedResources,
		})
		if hashErr != nil {
			return nil, hashErr
		}
		release := SceneAnalysisStageRelease{
			ContractID: SceneAnalysisStageReleaseContractID, WireSchemaID: core.WireSchemaID,
			WireSchemaHash: core.WireSchemaHash, AgentDefinitionCoreHash: core.DefinitionCoreHash,
			VariantKey: variant.VariantKey, CapabilityKey: variant.CapabilityKey, Lane: variant.Lane,
			RuntimeClass: variant.RuntimeClass, BundleContentHash: bundle.ContentHash,
			InputContractID: variant.InputContractID, InputSchemaHash: variant.InputSchemaHash,
			OutputContractID: variant.OutputContractID, OutputSchemaHash: variant.OutputSchemaHash,
			NormalizedCandidateContractID: variant.NormalizedCandidateContractID,
			NormalizedCandidateSchemaHash: variant.NormalizedCandidateSchemaHash,
			NormalizerContractRef:         variant.NormalizerContractRef, NormalizerContractHash: variant.NormalizerContractHash,
			PatchApplication: variant.PatchApplication, PromptCompilerRef: promptCompiler.ContractID,
			PromptCompilerHash: promptCompilerHash,
			ReferenceRefs:      referenceRefs, RecipeRefs: []BundleFile{}, RubricRefs: rubricRefs,
			LoadedResourceSetHash: loadedResourceSetHash, ExecutionPolicyRef: variant.ExecutionPolicyRef,
			ExecutionPolicyHash: variant.ExecutionPolicyConstraintHash,
			ModelPolicyRef:      variant.ModelPolicyRef, ModelPolicyHash: variant.ModelPolicyConstraintHash,
			ToolPolicyRef: variant.ToolPolicyRef, ToolPolicyHash: variant.ToolPolicyConstraintHash,
			RuntimeImageDigest: runtimeImageDigest,
		}
		release.StageReleaseHash, hashErr = sceneAnalysisStageReleaseHash(release)
		if hashErr != nil {
			return nil, hashErr
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func EncodeSceneAnalysisStageRelease(release SceneAnalysisStageRelease) (json.RawMessage, error) {
	if err := validateSceneAnalysisStageRelease(release); err != nil {
		return nil, err
	}
	return encodeSceneAnalysisStageRelease(release)
}

func DecodeSceneAnalysisStageRelease(raw json.RawMessage) (SceneAnalysisStageRelease, json.RawMessage, error) {
	var release SceneAnalysisStageRelease
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&release); err != nil {
		return SceneAnalysisStageRelease{}, nil, errors.New("invalid Scene Analysis Stage Release")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SceneAnalysisStageRelease{}, nil, errors.New("invalid Scene Analysis Stage Release")
	}
	if err := validateSceneAnalysisStageRelease(release); err != nil {
		return SceneAnalysisStageRelease{}, nil, err
	}
	encoded, err := encodeSceneAnalysisStageRelease(release)
	if err != nil {
		return SceneAnalysisStageRelease{}, nil, err
	}
	return release, encoded, nil
}

func SceneAnalysisLoadedResourcePaths(release SceneAnalysisStageRelease) ([]string, error) {
	if err := validateSceneAnalysisStageRelease(release); err != nil {
		return nil, err
	}
	bundle, err := PinnedStoryGraphBundleManifest()
	if err != nil {
		return nil, err
	}
	entrypoint, ok := bundleFileByPath(bundle, bundle.BundleEntrypoint)
	if !ok {
		return nil, errors.New("StoryGraph bundle entrypoint is missing")
	}
	resources := []BundleFile{entrypoint}
	resources = append(resources, release.ReferenceRefs...)
	resources = append(resources, release.RecipeRefs...)
	resources = append(resources, release.RubricRefs...)
	slices.SortFunc(resources, func(left, right BundleFile) int { return strings.Compare(left.Path, right.Path) })
	paths := make([]string, len(resources))
	for index, resource := range resources {
		paths[index] = resource.Path
	}
	return paths, nil
}

func validateSceneAnalysisStageRelease(release SceneAnalysisStageRelease) error {
	if !strings.HasPrefix(release.RuntimeImageDigest, "sha256:") ||
		!hashPattern.MatchString(strings.TrimPrefix(release.RuntimeImageDigest, "sha256:")) {
		return errors.New("invalid Scene Analysis Stage Release")
	}
	expected, err := BuildSceneAnalysisStageReleases(release.RuntimeImageDigest)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(expected, func(candidate SceneAnalysisStageRelease) bool {
		return candidate.VariantKey == release.VariantKey
	})
	if index < 0 || !reflect.DeepEqual(expected[index], release) {
		return errors.New("invalid Scene Analysis Stage Release")
	}
	return nil
}

func bundleFileByPath(bundle BundleContentManifest, path string) (BundleFile, bool) {
	index := slices.IndexFunc(bundle.Files, func(file BundleFile) bool { return file.Path == path })
	if index < 0 {
		return BundleFile{}, false
	}
	return bundle.Files[index], true
}

func sceneAnalysisStageArtifactHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platformcanonical.Hash(raw)
}

func sceneAnalysisStageReleaseHash(release SceneAnalysisStageRelease) (string, error) {
	release.StageReleaseHash = ""
	raw, err := json.Marshal(release)
	if err != nil {
		return "", err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	delete(root, "stage_release_hash")
	return sceneAnalysisStageArtifactHash(root)
}

func encodeSceneAnalysisStageRelease(release SceneAnalysisStageRelease) (json.RawMessage, error) {
	raw, err := json.Marshal(release)
	if err != nil {
		return nil, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
