package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestStoryGraphDefinitionFreezesTheOnlyAgentManifest(t *testing.T) {
	manifest := contract.StoryGraphDefinition()
	if manifest.DefinitionKey != "storygraph_stage" || manifest.SkillBundleHash == "" || len(manifest.Stages) != 10 || len(manifest.AllowedTools) != 0 {
		t.Fatalf("unexpected StoryGraph definition: %#v", manifest)
	}
	if err := manifest.ExecutionPolicy().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"production_bible", "storyboard_draft"} {
		if contract.IsStoryGraphStage(removed) {
			t.Fatalf("removed Agent invocation kind remained a stage: %s", removed)
		}
	}
	_, currentFile, _, _ := runtime.Caller(0)
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../.."))
	encoded, err := os.ReadFile(filepath.Join(repositoryRoot, "backend/tests/fixtures/agent/storygraph-definition.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		contract.AgentDefinitionManifest
		BundlePaths []string `json:"bundle_paths"`
	}
	if err = json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fixture.AgentDefinitionManifest, manifest) || !reflect.DeepEqual(fixture.BundlePaths, contract.StoryGraphBundlePaths()) {
		t.Fatalf("cross-language definition fixture drifted: fixture=%#v manifest=%#v", fixture, manifest)
	}
	computed, err := contract.ComputeStoryGraphBundleHash(filepath.Join(repositoryRoot, "agent/skills/build-storygraph"))
	if err != nil || computed != manifest.SkillBundleHash {
		t.Fatalf("bundle hash = %s, want %s, err=%v", computed, manifest.SkillBundleHash, err)
	}
}

func TestBundleRuntimeCatalogRoutesOnlyAnExactHash(t *testing.T) {
	manifest := contract.StoryGraphDefinition()
	oldHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	catalog, err := contract.NewRuntimeCatalog([]contract.RuntimeRevision{
		{
			BundleHash:  manifest.SkillBundleHash,
			BaseURL:     "http://agent-current:8787",
			ImageDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		{
			BundleHash:  oldHash,
			BaseURL:     "http://agent-old:8787",
			ImageDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	route, err := catalog.Resolve(manifest.SkillBundleHash)
	if err != nil || route.BaseURL != "http://agent-current:8787" {
		t.Fatalf("exact bundle route = %#v, err = %v", route, err)
	}
	oldRoute, err := catalog.Resolve(oldHash)
	if err != nil || oldRoute.BaseURL != "http://agent-old:8787" || oldRoute.ImageDigest != "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("old bundle route = %#v, err = %v", oldRoute, err)
	}
	if _, err = catalog.Resolve("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"); err != contract.ErrSkillBundleUnavailable {
		t.Fatalf("missing bundle route error = %v", err)
	}
}
