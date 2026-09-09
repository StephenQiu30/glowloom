package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

type identityResolutionFixture struct {
	ValidCandidate           json.RawMessage `json:"valid_candidate"`
	ExpectedInputHash        string          `json:"expected_input_hash"`
	ExpectedStageInstanceKey string          `json:"expected_stage_instance_key"`
}

func TestIdentityResolutionCandidatePartitionsEveryRawMentionExactlyOnce(t *testing.T) {
	identity := loadIdentityResolutionFixture(t)
	sceneAnalysis := loadStoryGraphSceneAnalysisWireFixture(t)

	if err := contract.ValidateIdentityResolutionCandidate(
		identity.ValidCandidate,
		sceneAnalysis.ValidSceneFactCandidate,
		map[string]struct{}{},
	); err != nil {
		t.Fatalf("valid IdentityResolution candidate rejected: %v", err)
	}
	var candidate contract.IdentityResolutionCandidate
	if err := json.Unmarshal(identity.ValidCandidate, &candidate); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, cluster := range candidate.ResolvedClusters {
		kinds[cluster.Kind] = true
	}
	if !kinds["character"] || !kinds["location"] || !kinds["prop"] {
		t.Fatalf("IdentityResolution kinds = %v", kinds)
	}

	var duplicate map[string]any
	if err := json.Unmarshal(identity.ValidCandidate, &duplicate); err != nil {
		t.Fatal(err)
	}
	clusters := duplicate["resolved_clusters"].([]any)
	firstMentions := clusters[0].(map[string]any)["mention_refs"].([]any)
	secondMentions := clusters[1].(map[string]any)["mention_refs"].([]any)
	clusters[1].(map[string]any)["mention_refs"] = append(secondMentions, firstMentions[0])
	if err := contract.ValidateIdentityResolutionCandidate(
		mustJSON(t, duplicate),
		sceneAnalysis.ValidSceneFactCandidate,
		map[string]struct{}{},
	); err == nil {
		t.Fatal("duplicate raw mention partition was accepted")
	}

	var missing map[string]any
	if err := json.Unmarshal(identity.ValidCandidate, &missing); err != nil {
		t.Fatal(err)
	}
	missingClusters := missing["resolved_clusters"].([]any)
	missingMentions := missingClusters[0].(map[string]any)["mention_refs"].([]any)
	missingClusters[0].(map[string]any)["mention_refs"] = missingMentions[:1]
	if err := contract.ValidateIdentityResolutionCandidate(
		mustJSON(t, missing),
		sceneAnalysis.ValidSceneFactCandidate,
		map[string]struct{}{},
	); err == nil {
		t.Fatal("missing raw mention partition was accepted")
	}

	var unknown map[string]any
	if err := json.Unmarshal(identity.ValidCandidate, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["visual_style"] = "赛博朋克"
	if err := contract.ValidateIdentityResolutionCandidate(
		mustJSON(t, unknown),
		sceneAnalysis.ValidSceneFactCandidate,
		map[string]struct{}{},
	); err == nil {
		t.Fatal("unknown visual field was accepted")
	}

	var reuse map[string]any
	if err := json.Unmarshal(identity.ValidCandidate, &reuse); err != nil {
		t.Fatal(err)
	}
	reuseCluster := reuse["resolved_clusters"].([]any)[0].(map[string]any)
	reuseCluster["resolution"] = "reuse"
	reuseCluster["reuse_identity_key"] = "character:existing:linzhou"
	if err := contract.ValidateIdentityResolutionCandidate(
		mustJSON(t, reuse),
		sceneAnalysis.ValidSceneFactCandidate,
		map[string]struct{}{},
	); err == nil {
		t.Fatal("identity reuse outside the input allowlist was accepted")
	}
	if err := contract.ValidateIdentityResolutionCandidate(
		mustJSON(t, reuse),
		sceneAnalysis.ValidSceneFactCandidate,
		map[string]struct{}{"character:existing:linzhou": {}},
	); err != nil {
		t.Fatalf("allowlisted identity reuse was rejected: %v", err)
	}
}

func TestIdentityResolutionStageUsesTheProductionVariant(t *testing.T) {
	variant := contract.SceneAnalysisStageVariant{
		StageKey:            "resolve_identities",
		ProfileKey:          "default",
		LaneKey:             "primary",
		OutputSchemaVersion: contract.IdentityResolutionCandidateSchemaVersion,
	}
	if err := variant.Validate(); err != nil {
		t.Fatalf("IdentityResolution stage variant rejected: %v", err)
	}

	identity := loadIdentityResolutionFixture(t)
	sceneAnalysis := loadStoryGraphSceneAnalysisWireFixture(t)
	base, err := contract.DecodeSceneAnalysisInvocation(sceneAnalysis.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(contract.IdentityResolutionInput{
		SourceVersionID:                base.Payload.SourceRefs[0].VersionID,
		SourceHash:                     base.Payload.SourceRefs[0].ContentHash,
		NormalizedText:                 "第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。",
		SceneFactCandidateRevisionID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		SceneFactCandidateRevisionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SceneFactCandidate:             sceneAnalysis.ValidSceneFactCandidate,
		AllowedReuseIdentityKeys:       []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := contract.NewSceneAnalysisInvocation(
		base.InvocationID,
		base.AttemptID,
		base.StageRelease,
		base.Control,
		base.Budget,
		contract.SceneAnalysisPayload{
			Variant:    variant,
			Scope:      base.Payload.Scope,
			SourceRefs: base.Payload.SourceRefs,
			UpstreamCandidates: []contract.SceneAnalysisCandidateRevisionIdentity{{
				StageKey: "extract_scene_facts", ShardKey: "script:full",
				CandidateRevisionID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				CandidateRevisionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SourceInvocationID:    "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
				SourceResultHash:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			}},
			Shard:      base.Payload.Shard,
			StageInput: input,
		},
	)
	if err != nil {
		t.Fatalf("build IdentityResolution invocation: %v", err)
	}
	if invocation.InputHash != identity.ExpectedInputHash ||
		invocation.StageInstanceKey() != identity.ExpectedStageInstanceKey {
		t.Fatalf(
			"IdentityResolution invocation identity drifted: input=%s stage=%s",
			invocation.InputHash,
			invocation.StageInstanceKey(),
		)
	}
}

func loadIdentityResolutionFixture(t *testing.T) identityResolutionFixture {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve IdentityResolution fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(
		filepath.Dir(current), "..", "fixtures", "agent", "storygraph-identity-resolution.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var fixture identityResolutionFixture
	if err = json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
