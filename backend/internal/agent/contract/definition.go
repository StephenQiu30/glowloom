package contract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const StoryGraphSkillBundleHash = "4cf64c94b7d181945da678721db36c4bc45921a9c833164bdea46cb7af149c42"

var ErrSkillBundleUnavailable = errors.New("skill_bundle_unavailable")

type StageDefinition struct {
	Stage         string   `json:"stage"`
	CandidateType string   `json:"candidate_type"`
	References    []string `json:"references"`
}

type AgentDefinitionManifest struct {
	DefinitionKey        string            `json:"definition_key"`
	DefinitionVersion    string            `json:"definition_version"`
	PromptVersion        string            `json:"prompt_version"`
	SkillBundleVersion   string            `json:"skill_bundle_version"`
	SkillBundleHash      string            `json:"skill_bundle_hash"`
	OutputSchemaVersion  string            `json:"output_schema_version"`
	ModelCapability      string            `json:"model_capability"`
	CodexRuntimeContract string            `json:"codex_runtime_contract"`
	AllowedTools         []string          `json:"allowed_tools"`
	MaxModelCalls        int               `json:"max_model_calls"`
	MaxExecutionSeconds  int               `json:"max_execution_seconds"`
	Stages               []StageDefinition `json:"stages"`
}

func StoryGraphDefinition() AgentDefinitionManifest {
	return AgentDefinitionManifest{
		DefinitionKey: "storygraph_stage", DefinitionVersion: "storygraph-stage-harness-v1",
		PromptVersion: "build-storygraph-prompt-v1", SkillBundleVersion: "build-storygraph-v1",
		SkillBundleHash: StoryGraphSkillBundleHash, OutputSchemaVersion: "storygraph-candidate-schema-v1",
		ModelCapability: "structured_text", CodexRuntimeContract: "codex-cli-ephemeral-read-only-v1",
		AllowedTools: []string{}, MaxModelCalls: 2, MaxExecutionSeconds: 600,
		Stages: []StageDefinition{
			{"extract_source_evidence", "source_evidence_candidate", []string{"source-evidence.md"}},
			{"analyze_story", "story_analysis_candidate", []string{"story-analysis.md", "entity-reconciliation.md"}},
			{"reconcile_story", "story_reconciliation_candidate", []string{"entity-reconciliation.md", "story-analysis.md"}},
			{"segment_episodes", "episode_segmentation_candidate", []string{"episode-segmentation.md"}},
			{"analyze_episode", "episode_analysis_candidate", []string{"scene-structure.md", "visual-identity.md"}},
			{"reconcile_episode", "episode_reconciliation_candidate", []string{"scene-structure.md", "continuity-review.md"}},
			{"draft_storyboard", "storyboard_row_candidate", []string{"storyboard-table.md", "visual-identity.md"}},
			{"detail_shots", "shot_detail_candidate", []string{"shot-detail.md", "visual-identity.md"}},
			{"review_storygraph", "storygraph_review_candidate", []string{"continuity-review.md"}},
			{"repair_candidate", "candidate_repair_patch", []string{"continuity-review.md"}},
		},
	}
}

func (value AgentDefinitionManifest) ExecutionPolicy() StageExecutionPolicy {
	allowedTools := make([]string, len(value.AllowedTools))
	copy(allowedTools, value.AllowedTools)
	return StageExecutionPolicy{
		DefinitionKey: value.DefinitionKey, DefinitionVersion: value.DefinitionVersion,
		PromptVersion: value.PromptVersion, SkillBundleVersion: value.SkillBundleVersion,
		SkillBundleHash: value.SkillBundleHash, OutputSchemaVersion: value.OutputSchemaVersion,
		ModelCapability: value.ModelCapability, CodexRuntimeContract: value.CodexRuntimeContract,
		AllowedTools: allowedTools, MaxModelCalls: value.MaxModelCalls,
		MaxExecutionSeconds: value.MaxExecutionSeconds,
	}
}

func (value AgentDefinitionManifest) ValidatePolicy(policy StageExecutionPolicy) error {
	expected := value.ExecutionPolicy()
	if policy.DefinitionKey != expected.DefinitionKey || policy.DefinitionVersion != expected.DefinitionVersion || policy.PromptVersion != expected.PromptVersion || policy.SkillBundleVersion != expected.SkillBundleVersion || policy.SkillBundleHash != expected.SkillBundleHash || policy.OutputSchemaVersion != expected.OutputSchemaVersion || policy.ModelCapability != expected.ModelCapability || policy.CodexRuntimeContract != expected.CodexRuntimeContract || policy.AllowedTools == nil || len(policy.AllowedTools) != 0 || policy.MaxModelCalls < 1 || policy.MaxModelCalls > expected.MaxModelCalls || policy.MaxExecutionSeconds < 1 || policy.MaxExecutionSeconds > expected.MaxExecutionSeconds {
		return errors.New("StoryGraph execution policy is outside the definition manifest")
	}
	return nil
}

func IsStoryGraphStage(stage string) bool {
	_, ok := storyGraphStages[stage]
	return ok
}

func CandidateTypeForStage(stage string) (string, bool) {
	for _, definition := range StoryGraphDefinition().Stages {
		if definition.Stage == stage {
			return definition.CandidateType, true
		}
	}
	return "", false
}

func StoryGraphBundlePaths() []string {
	return []string{
		"SKILL.md",
		"references/continuity-review.md",
		"references/entity-reconciliation.md",
		"references/episode-segmentation.md",
		"references/scene-structure.md",
		"references/shot-detail.md",
		"references/source-evidence.md",
		"references/story-analysis.md",
		"references/storyboard-table.md",
		"references/visual-identity.md",
	}
}

func ComputeStoryGraphBundleHash(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("invalid StoryGraph bundle root")
	}
	allowed := StoryGraphBundlePaths()
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, path := range allowed {
		allowedSet[path] = struct{}{}
	}
	actual := map[string]struct{}{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("StoryGraph bundle contains a symlink")
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil || strings.HasPrefix(relative, "..") {
			return errors.New("StoryGraph bundle path escapes root")
		}
		actual[filepath.ToSlash(relative)] = struct{}{}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(actual) != len(allowedSet) {
		return "", errors.New("StoryGraph bundle file set is invalid")
	}
	for path := range actual {
		if _, ok := allowedSet[path]; !ok {
			return "", errors.New("StoryGraph bundle file set is invalid")
		}
	}

	sort.Strings(allowed)
	digest := sha256.New()
	var length [8]byte
	for _, relative := range allowed {
		content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if readErr != nil || !utf8.Valid(content) {
			return "", errors.New("StoryGraph bundle contains invalid UTF-8")
		}
		_, _ = digest.Write([]byte(relative))
		_, _ = digest.Write([]byte{0})
		binary.BigEndian.PutUint64(length[:], uint64(len(content)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(content)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

type RuntimeRevision struct {
	BundleHash  string
	BaseURL     string
	ImageDigest string
}

type RuntimeCatalog struct {
	routes map[string]RuntimeRevision
}

func NewRuntimeCatalog(revisions []RuntimeRevision) (RuntimeCatalog, error) {
	routes := make(map[string]RuntimeRevision, len(revisions))
	for _, revision := range revisions {
		parsed, err := url.ParseRequestURI(revision.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || !hashPattern.MatchString(revision.BundleHash) || !strings.HasPrefix(revision.ImageDigest, "sha256:") || !hashPattern.MatchString(strings.TrimPrefix(revision.ImageDigest, "sha256:")) {
			return RuntimeCatalog{}, fmt.Errorf("invalid Agent runtime revision")
		}
		if _, exists := routes[revision.BundleHash]; exists {
			return RuntimeCatalog{}, errors.New("duplicate Agent runtime bundle hash")
		}
		routes[revision.BundleHash] = revision
	}
	if len(routes) == 0 {
		return RuntimeCatalog{}, errors.New("Agent runtime catalog is empty")
	}
	return RuntimeCatalog{routes: routes}, nil
}

func (catalog RuntimeCatalog) Resolve(bundleHash string) (RuntimeRevision, error) {
	revision, ok := catalog.routes[bundleHash]
	if !ok {
		return RuntimeRevision{}, ErrSkillBundleUnavailable
	}
	return revision, nil
}
