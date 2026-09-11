package agent_test

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestSceneAnalysisStageReleasesBindCoreBundleAndLoadedResources(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "../../.."))
	liveBundle, _, err := contract.BuildStoryGraphBundleManifest(filepath.Join(repositoryRoot, "agent/skills/build-storygraph"))
	if err != nil {
		t.Fatal(err)
	}
	pinnedBundle, err := contract.PinnedStoryGraphBundleManifest()
	if err != nil || pinnedBundle.ContentHash != liveBundle.ContentHash || !slices.Equal(pinnedBundle.Files, liveBundle.Files) {
		t.Fatalf("pinned Bundle artifact drifted from the installed Skill: %#v err=%v", pinnedBundle, err)
	}

	imageDigest := "sha256:" + strings.Repeat("7", 64)
	releases, err := contract.BuildSceneAnalysisStageReleases(imageDigest)
	if err != nil || len(releases) != 7 {
		t.Fatalf("build Scene Analysis Stage Releases: count=%d err=%v", len(releases), err)
	}
	core, _, err := contract.BuildSceneAnalysisDefinitionCore()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for _, release := range releases {
		if release.ContractID != contract.SceneAnalysisStageReleaseContractID ||
			release.AgentDefinitionCoreHash != core.DefinitionCoreHash ||
			release.BundleContentHash != pinnedBundle.ContentHash || release.RuntimeImageDigest != imageDigest {
			t.Fatalf("Stage Release did not bind its immutable roots: %#v", release)
		}
		if _, duplicate := seen[release.StageReleaseHash]; duplicate {
			t.Fatal("Stage Release hash was reused")
		}
		seen[release.StageReleaseHash] = struct{}{}
		encoded, err := contract.EncodeSceneAnalysisStageRelease(release)
		if err != nil {
			t.Fatal(err)
		}
		decoded, canonical, err := contract.DecodeSceneAnalysisStageRelease(encoded)
		if err != nil || decoded.StageReleaseHash != release.StageReleaseHash || !slices.Equal(canonical, encoded) {
			t.Fatalf("Stage Release did not round-trip: %#v err=%v", decoded, err)
		}
		paths, err := contract.SceneAnalysisLoadedResourcePaths(release)
		if err != nil || len(paths) != 2 || paths[0] != "SKILL.md" {
			t.Fatalf("Stage Release loaded-resource proof is invalid: %v err=%v", paths, err)
		}
	}

	changed, err := contract.BuildSceneAnalysisStageReleases("sha256:" + strings.Repeat("8", 64))
	if err != nil || changed[0].StageReleaseHash == releases[0].StageReleaseHash {
		t.Fatal("runtime image drift did not change the Stage Release identity")
	}
}

func TestSceneAnalysisStageReleaseRejectsMutableOrIncompleteManifest(t *testing.T) {
	releases, err := contract.BuildSceneAnalysisStageReleases("sha256:" + strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := contract.EncodeSceneAnalysisStageRelease(releases[0])
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"unknown current ref": func(value map[string]any) { value["model_policy_ref"] = "current" },
		"missing resource": func(value map[string]any) {
			value["reference_refs"] = []any{}
		},
		"hash drift": func(value map[string]any) { value["stage_release_hash"] = strings.Repeat("a", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(encoded, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			if _, _, err := contract.DecodeSceneAnalysisStageRelease(mustJSON(t, candidate)); err == nil {
				t.Fatal("invalid Stage Release was accepted")
			}
		})
	}
}
