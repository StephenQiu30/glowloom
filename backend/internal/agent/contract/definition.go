package contract

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const StoryGraphSkillBundleHash = "c1b3da8229d3ff184ea8360b21e87d0fff36008d200943833d36e93e56cb2b42"

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
		DefinitionKey: "storygraph_stage", DefinitionVersion: "storygraph-stage-harness",
		PromptVersion: "build-storygraph-prompt", SkillBundleVersion: "build-storygraph",
		SkillBundleHash: StoryGraphSkillBundleHash, OutputSchemaVersion: "storygraph-candidate-schema",
		ModelCapability: "structured_text", CodexRuntimeContract: "codex-cli-ephemeral-read-only",
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
		"NOTICE.md",
		"SKILL.md",
		"references/continuity-review.md",
		"references/entity-reconciliation.md",
		"references/episode-segmentation.md",
		"references/interaction-continuity.md",
		"references/production-entities.md",
		"references/reference-brief.md",
		"references/reference-planning.md",
		"references/scene-facts.md",
		"references/scene-occurrences.md",
		"references/scene-structure.md",
		"references/script-spans.md",
		"references/shot-detail.md",
		"references/source-evidence.md",
		"references/story-analysis.md",
		"references/storyboard-table.md",
		"references/structure-identity-review.md",
		"references/visual-identity.md",
	}
}

func ComputeStoryGraphBundleHash(root string) (string, error) {
	manifest, _, err := BuildStoryGraphBundleManifest(root)
	return manifest.ContentHash, err
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
		return RuntimeCatalog{}, errors.New("agent runtime catalog is empty")
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
