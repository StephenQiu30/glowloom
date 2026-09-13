package agent_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestVisionReviewRuntimeDeclaresVisionStageAndExactInput(t *testing.T) {
	core, _, err := contract.BuildSceneAnalysisDefinitionCore()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, variant := range core.VariantContracts {
		if variant.VariantKey.StageKey != contract.VisionReviewStageKey {
			continue
		}
		found = true
		if variant.RuntimeClass != "vision" || variant.Lane != "preset_visual" ||
			variant.CapabilityKey != "review-reference-artifact" ||
			variant.InputContractID != contract.VisionReviewInputContractID ||
			variant.OutputContractID != "vision_review_candidate" ||
			variant.PatchApplication.Kind != "none" {
			t.Fatalf("incorrect vision capability: %+v", variant)
		}
	}
	if !found {
		t.Fatal("Vision Review must be a registered vision capability")
	}
	releases, err := contract.BuildSceneAnalysisStageReleases("sha256:" + strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(releases, func(value contract.SceneAnalysisStageRelease) bool {
		return value.VariantKey.StageKey == contract.VisionReviewStageKey
	})
	if index < 0 {
		t.Fatal("Vision Review Stage Release is missing")
	}
	release := releases[index]
	paths, err := contract.SceneAnalysisLoadedResourcePaths(release)
	if err != nil || !slices.Equal(paths, []string{"SKILL.md", "references/vision-review.md"}) ||
		release.RuntimeClass != "vision" || release.BundleContentHash != contract.StoryGraphSkillBundleHash ||
		release.InputContractID != contract.VisionReviewInputContractID ||
		release.OutputContractID != "vision_review_candidate" || release.PromptCompilerHash == releases[0].PromptCompilerHash {
		t.Fatalf("Vision Review Release lost its exact resource/runtime bindings: %+v paths=%v err=%v", release, paths, err)
	}
}
