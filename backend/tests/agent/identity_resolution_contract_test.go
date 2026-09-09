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
	ValidCandidate json.RawMessage `json:"valid_candidate"`
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
