package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestStoryGraphBundleManifestCoversEveryInstalledResource(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../.."))
	bundleRoot := filepath.Join(repositoryRoot, "agent/skills/build-storygraph")

	manifest, encoded, err := contract.BuildStoryGraphBundleManifest(bundleRoot)
	if err != nil {
		t.Fatalf("build StoryGraph bundle manifest: %v", err)
	}
	if manifest.ContractID != contract.StoryGraphBundleContractID || manifest.BundleEntrypoint != "SKILL.md" ||
		manifest.ContentHash == "" || len(manifest.Files) == 0 {
		t.Fatalf("unexpected StoryGraph bundle manifest: %#v", manifest)
	}
	paths := make([]string, len(manifest.Files))
	for index, file := range manifest.Files {
		paths[index] = file.Path
	}
	if !slices.IsSorted(paths) || !slices.Contains(paths, "references/scene-facts.md") {
		t.Fatalf("bundle file manifest is incomplete or unsorted: %v", paths)
	}
	decoded, canonical, err := contract.DecodeStoryGraphBundleManifest(encoded)
	if err != nil || decoded.ContentHash != manifest.ContentHash || !slices.Equal(canonical, encoded) {
		t.Fatalf("bundle manifest did not round-trip: decoded=%#v err=%v", decoded, err)
	}

	copyRoot := filepath.Join(t.TempDir(), "build-storygraph")
	if err = copyBundleDirectory(bundleRoot, copyRoot); err != nil {
		t.Fatal(err)
	}
	baseline, _, err := contract.BuildStoryGraphBundleManifest(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	changedPath := filepath.Join(copyRoot, "references/scene-facts.md")
	content, err := os.ReadFile(changedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(changedPath, append(content, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, _, err := contract.BuildStoryGraphBundleManifest(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ContentHash == baseline.ContentHash {
		t.Fatal("production Scene Fact reference drift did not change the bundle identity")
	}

	var drifted map[string]any
	if err = json.Unmarshal(encoded, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted["provider"] = "latest"
	if _, _, err = contract.DecodeStoryGraphBundleManifest(mustJSON(t, drifted)); err == nil {
		t.Fatal("unknown Bundle manifest field was accepted")
	}
	delete(drifted, "provider")
	drifted["notice_hash"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err = contract.DecodeStoryGraphBundleManifest(mustJSON(t, drifted)); err == nil {
		t.Fatal("Bundle manifest hash drift was accepted")
	}

	symlinkRoot := t.TempDir()
	outsideAgent := filepath.Join(symlinkRoot, "outside", "agent")
	if err = copyBundleDirectory(bundleRoot, filepath.Join(outsideAgent, "skills", "build-storygraph")); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outsideAgent, filepath.Join(symlinkRoot, "agent")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = contract.BuildStoryGraphBundleManifest(filepath.Join(symlinkRoot, "agent", "skills", "build-storygraph")); err == nil {
		t.Fatal("symlinked Bundle parent was accepted")
	}
}

func copyBundleDirectory(sourceRoot, targetRoot string) error {
	return filepath.WalkDir(sourceRoot, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o700)
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, content, 0o600)
	})
}

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
