package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

type structureIdentityReviewFixture struct {
	InputIdentity       structureIdentityInputIdentity                    `json:"input_identity"`
	UpstreamCandidates  []contract.SceneAnalysisCandidateRevisionIdentity `json:"upstream_candidates"`
	DeterministicIssues []contract.CandidateReviewIssue                   `json:"deterministic_issues"`
	ValidCandidate      json.RawMessage                                   `json:"valid_candidate"`
}

type structureIdentityInputIdentity struct {
	SourceVersionID                string `json:"source_version_id"`
	SourceHash                     string `json:"source_hash"`
	SpanCandidateRevisionID        string `json:"span_candidate_revision_id"`
	SpanCandidateRevisionHash      string `json:"span_candidate_revision_hash"`
	SceneFactCandidateRevisionID   string `json:"scene_fact_candidate_revision_id"`
	SceneFactCandidateRevisionHash string `json:"scene_fact_candidate_revision_hash"`
	IdentityCandidateRevisionID    string `json:"identity_candidate_revision_id"`
	IdentityCandidateRevisionHash  string `json:"identity_candidate_revision_hash"`
}

func TestStructureIdentityReviewCandidatePreservesFrozenIdentityAndHasNoGateDecision(t *testing.T) {
	review := loadStructureIdentityReviewFixture(t)
	scene := loadStoryGraphSceneAnalysisWireFixture(t)
	identity := loadIdentityResolutionFixture(t)
	input := contract.StructureIdentityReviewInput{
		SourceVersionID:                review.InputIdentity.SourceVersionID,
		SourceHash:                     review.InputIdentity.SourceHash,
		NormalizedText:                 "第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。",
		SpanCandidateRevisionID:        review.InputIdentity.SpanCandidateRevisionID,
		SpanCandidateRevisionHash:      review.InputIdentity.SpanCandidateRevisionHash,
		SpanCandidate:                  scene.ValidScriptSpanCandidate,
		SceneFactCandidateRevisionID:   review.InputIdentity.SceneFactCandidateRevisionID,
		SceneFactCandidateRevisionHash: review.InputIdentity.SceneFactCandidateRevisionHash,
		SceneFactCandidate:             scene.ValidSceneFactCandidate,
		IdentityCandidateRevisionID:    review.InputIdentity.IdentityCandidateRevisionID,
		IdentityCandidateRevisionHash:  review.InputIdentity.IdentityCandidateRevisionHash,
		IdentityCandidate:              identity.ValidCandidate,
		DeterministicIssues:            review.DeterministicIssues,
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("valid structure/identity review input rejected: %v", err)
	}
	if err := contract.ValidateStructureIdentityReviewCandidate(review.ValidCandidate, input); err != nil {
		t.Fatalf("valid structure/identity review candidate rejected: %v", err)
	}
	blocker := contract.CandidateReviewIssue{
		IssueKey: "issue_identity_partition_0001", Code: "identity_partition_incomplete",
		Severity: "blocking", Scope: "project:demo", Summary: "身份提及分区不完整",
		Evidence: []contract.SourceEvidenceSpan{},
	}
	input.DeterministicIssues = []contract.CandidateReviewIssue{blocker}
	var withBlocker map[string]any
	if err := json.Unmarshal(review.ValidCandidate, &withBlocker); err != nil {
		t.Fatal(err)
	}
	issues := withBlocker["review_issues"].([]any)
	withBlocker["review_issues"] = append(issues,
		map[string]any{
			"issue_key": blocker.IssueKey, "code": blocker.Code, "severity": blocker.Severity,
			"scope": blocker.Scope, "summary": blocker.Summary, "evidence": []any{},
		},
	)
	if err := contract.ValidateStructureIdentityReviewCandidate(mustJSON(t, withBlocker), input); err != nil {
		t.Fatalf("review candidate preserving deterministic blocker rejected: %v", err)
	}
	omitted := withBlocker
	omitted["review_issues"] = issues
	if err := contract.ValidateStructureIdentityReviewCandidate(mustJSON(t, omitted), input); err == nil {
		t.Fatal("review candidate omitted a deterministic blocker")
	}

	var forbidden map[string]any
	if err := json.Unmarshal(review.ValidCandidate, &forbidden); err != nil {
		t.Fatal(err)
	}
	forbidden["gate_status"] = "passed"
	if err := contract.ValidateStructureIdentityReviewCandidate(mustJSON(t, forbidden), input); err == nil {
		t.Fatal("review candidate with a Gate decision was accepted")
	}

	variant := contract.SceneAnalysisStageVariant{
		StageKey: "review_candidate", ProfileKey: "structure_identity", LaneKey: "primary",
		OutputSchemaVersion: contract.StructureIdentityReviewCandidateSchemaVersion,
	}
	if err := variant.Validate(); err != nil {
		t.Fatalf("structure/identity review variant rejected: %v", err)
	}
	variant.ProfileKey = "default"
	if err := variant.Validate(); err == nil {
		t.Fatal("review variant with an unbound profile was accepted")
	}
}

func loadStructureIdentityReviewFixture(t *testing.T) structureIdentityReviewFixture {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve structure/identity review fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(
		filepath.Dir(current), "..", "fixtures", "agent", "storygraph-structure-identity-review.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var fixture structureIdentityReviewFixture
	if err = json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
