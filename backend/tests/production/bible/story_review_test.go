package bible_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/google/uuid"
)

func TestBibleDeterministicGateKeepsModelOpinionSeparate(t *testing.T) {
	evidence := domain.Evidence{
		SourceStart: 0, SourceEnd: 2, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
	}
	unknown := "character:missing"
	candidate := storyReconciliationReviewCandidate(evidence)
	candidate.CanonicalWorldEntries = append(candidate.CanonicalWorldEntries, domain.StoryWorldEntryCandidate{
		EntryKey: "world:home", Category: "location", Title: "住处", Facts: []string{"林一住在这里"},
		Rules: []string{}, EntityKeys: []string{unknown}, EpisodeNumbers: []int{1},
		Evidence: []domain.Evidence{evidence}, Ambiguities: []string{},
	})
	candidate.Conflicts = []domain.ReviewIssue{{
		IssueKey: "issue:model", Code: "model_opinion", Severity: "blocking", Scope: "story",
		SubjectKey: &unknown, Summary: "模型认为存在冲突", Evidence: []domain.Evidence{evidence},
	}}

	gate, err := bibleapp.EvaluateStoryReconciliationGate(
		uuid.NewString(), strings.Repeat("a", 64), candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gate.GateVersion != "bible-deterministic-gate-v1" || len(gate.Blockers) != 1 ||
		gate.Blockers[0].Code != "world_unknown_entity" || gate.Blockers[0].SubjectKey != "world:home" {
		t.Fatalf("deterministic gate mixed in model opinion or lost its structural blocker: %#v", gate)
	}

	withoutModelOpinion := candidate
	withoutModelOpinion.Conflicts = []domain.ReviewIssue{}
	repeated, err := bibleapp.EvaluateStoryReconciliationGate(
		gate.TargetCandidateRevisionID, gate.TargetCandidateRevisionHash, withoutModelOpinion,
	)
	if err != nil || !reflect.DeepEqual(repeated, gate) {
		t.Fatalf("model issue changed deterministic gate: repeated=%#v error=%v", repeated, err)
	}
}

func TestBibleReviewOnlyAcceptsEvidenceScopedIssuesAndCannotRewriteGate(t *testing.T) {
	evidence := domain.Evidence{
		SourceStart: 0, SourceEnd: 2, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
	}
	candidate := storyReconciliationReviewCandidate(evidence)
	targetID := uuid.NewString()
	targetHash := strings.Repeat("b", 64)
	input := agentcontract.StoryGraphReviewStageInput{
		ReviewedStage: "reconcile_story", TargetCandidateRevisionID: targetID,
		TargetCandidateRevisionHash: targetHash, CandidateItemStart: 0, CandidateItemEnd: 1,
		TargetCandidate: mustJSON(t, candidate),
		DeterministicGate: agentcontract.StoryGraphDeterministicGateResult{
			GateVersion: "bible-deterministic-gate-v1", TargetCandidateRevisionID: targetID,
			TargetCandidateRevisionHash: targetHash, Blockers: []agentcontract.StoryGraphGateBlocker{},
		},
	}
	issue := agentcontract.StoryGraphReviewIssue{
		IssueKey: "issue:review", Code: "identity_ambiguous", Severity: "warning", Scope: "entity",
		SubjectKey: stringPointer("character:lin-yi"), Summary: "身份仍需确认", RepairHint: nil,
		Evidence: []agentcontract.StoryGraphEvidence{{
			SourceStart: evidence.SourceStart, SourceEnd: evidence.SourceEnd, TextHash: evidence.TextHash,
			ExactAnchor: evidence.ExactAnchor, EpisodeNumber: evidence.EpisodeNumber,
		}},
	}
	result := agentcontract.StoryGraphReviewCandidate{
		ReviewedStage: input.ReviewedStage, TargetCandidateRevisionID: targetID,
		TargetCandidateRevisionHash: targetHash, ReviewIssues: []agentcontract.StoryGraphReviewIssue{issue},
	}
	if err := agentcontract.ValidateStoryGraphReviewCandidate(input, result); err != nil {
		t.Fatalf("valid evidence-scoped review was rejected: %v", err)
	}

	forged := result
	forged.ReviewIssues = append([]agentcontract.StoryGraphReviewIssue(nil), result.ReviewIssues...)
	forged.ReviewIssues[0].Evidence = []agentcontract.StoryGraphEvidence{{
		SourceStart: 3, SourceEnd: 5, TextHash: strings.Repeat("f", 64), ExactAnchor: "伪造",
	}}
	if err := agentcontract.ValidateStoryGraphReviewCandidate(input, forged); err == nil {
		t.Fatal("review accepted Evidence outside the frozen Candidate Revision")
	}
	impersonating := result
	impersonating.ReviewIssues = append([]agentcontract.StoryGraphReviewIssue(nil), result.ReviewIssues...)
	impersonating.ReviewIssues[0].Code = "world_unknown_entity"
	if err := agentcontract.ValidateStoryGraphReviewCandidate(input, impersonating); err == nil {
		t.Fatal("model Review Issue impersonated a deterministic Gate blocker code")
	}

	encodedResult := mustJSON(t, result)
	withGateRewrite := append(encodedResult[:len(encodedResult)-1], []byte(`,"deterministic_blockers":[]}`)...)
	if _, err := agentcontract.DecodeStoryGraphReviewCandidate(withGateRewrite); err == nil {
		t.Fatal("review candidate accepted a model-owned deterministic blocker field")
	}
}

func TestBibleRepairPatchCannotEscapeFrozenFieldBoundary(t *testing.T) {
	fragment := json.RawMessage(`{"entity_key":"character:lin-yi","canonical_name":"林一"}`)
	fragmentHash, err := agentcontract.StoryGraphCandidateFragmentHash(fragment)
	if err != nil {
		t.Fatal(err)
	}
	if fragmentHash != "d4d2e657ebe16dd6ecab5d3aa2c8d5e536ffc385fba3ee9e0627e3ee24d8c17b" {
		t.Fatalf("candidate fragment canonical hash = %s", fragmentHash)
	}
	targetID := uuid.NewString()
	targetHash := strings.Repeat("c", 64)
	input := agentcontract.StoryGraphRepairStageInput{
		TargetCandidateRevisionID: targetID, TargetCandidateRevisionHash: targetHash,
		ReviewCandidateRevisionID: uuid.NewString(), ReviewCandidateRevisionHash: strings.Repeat("d", 64),
		TargetIssue: agentcontract.StoryGraphReviewIssue{
			IssueKey: "issue:repair", Code: "canonical_name", Severity: "blocking", Scope: "entity",
			SubjectKey: stringPointer("character:lin-yi"), Summary: "规范名冲突", RepairHint: nil,
			Evidence: []agentcontract.StoryGraphEvidence{{
				SourceStart: 0, SourceEnd: 2, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
			}},
		},
		AllowedTargets: []agentcontract.StoryGraphRepairAllowedTarget{{
			CandidateKey: "character:lin-yi", AllowedFields: []string{"canonical_name"},
			BaseFragmentHash: fragmentHash, Fragment: fragment,
		}},
		ReadOnlyAdjacency: []agentcontract.StoryGraphRepairReadOnlyFragment{},
		RepairRound:       1, MaxRepairRounds: 2,
	}
	patch := agentcontract.CandidateRepairPatch{
		TargetCandidateRevisionID: targetID, TargetCandidateRevisionHash: targetHash,
		Operations: []agentcontract.StoryGraphRepairOperation{{
			TargetCandidateKey: "character:lin-yi", BaseFragmentHash: fragmentHash,
			FieldName: "canonical_name", Replacement: agentcontract.StoryGraphRepairReplacement{Text: stringPointer("林一")},
		}},
		ReviewIssues: []agentcontract.StoryGraphReviewIssue{},
	}
	if err = agentcontract.ValidateCandidateRepairPatch(input, patch); err != nil {
		t.Fatalf("valid bounded repair patch was rejected: %v", err)
	}
	unsafeBoundary := input
	unsafeBoundary.AllowedTargets = append([]agentcontract.StoryGraphRepairAllowedTarget(nil), input.AllowedTargets...)
	unsafeBoundary.AllowedTargets[0].AllowedFields = []string{"graph_json"}
	if err = unsafeBoundary.Validate(); err == nil {
		t.Fatal("repair boundary allowed a published StoryGraph write field")
	}
	impersonatingBoundary := input
	impersonatingBoundary.TargetIssue.Code = "world_unknown_entity"
	if err = impersonatingBoundary.Validate(); err == nil {
		t.Fatal("repair boundary accepted a model Issue impersonating a deterministic Gate blocker")
	}

	escaped := patch
	escaped.Operations = append([]agentcontract.StoryGraphRepairOperation(nil), patch.Operations...)
	escaped.Operations[0].FieldName = "stable_spec"
	if err = agentcontract.ValidateCandidateRepairPatch(input, escaped); err == nil {
		t.Fatal("repair patch changed a field outside the frozen allowlist")
	}
	escaped.Operations[0].FieldName = "canonical_name"
	escaped.Operations[0].BaseFragmentHash = strings.Repeat("e", 64)
	if err = agentcontract.ValidateCandidateRepairPatch(input, escaped); err == nil {
		t.Fatal("repair patch ignored the frozen base fragment hash")
	}
	wrongType := patch
	wrongType.Operations = append([]agentcontract.StoryGraphRepairOperation(nil), patch.Operations...)
	wrongType.Operations[0].Replacement = agentcontract.StoryGraphRepairReplacement{Integer: intPointer(1)}
	if err = agentcontract.ValidateCandidateRepairPatch(input, wrongType); err == nil {
		t.Fatal("repair patch used a replacement type that does not match the frozen field")
	}
}

func storyReconciliationReviewCandidate(evidence domain.Evidence) domain.StoryReconciliationCandidate {
	return domain.StoryReconciliationCandidate{
		CanonicalEntities: []domain.StoryEntityCandidate{{
			EntityKey: "character:lin-yi", Kind: "character", CanonicalName: "林一", NormalizedName: "林一",
			Aliases: []string{}, StableSpec: domain.AssetSpecCandidate{
				Temperament: []string{}, Goals: []string{}, Relationships: []string{}, VisualElements: []string{},
				NegativeConstraints: []string{}, PerformanceTraits: []string{}, AllowedUsage: []string{},
			}, EpisodeNumbers: []int{1}, Evidence: []domain.Evidence{evidence},
			States: []domain.StoryEntityStateCandidate{}, Ambiguities: []string{},
		}},
		CanonicalWorldEntries: []domain.StoryWorldEntryCandidate{}, MergedClaims: []domain.StoryClaimCandidate{},
		MergedArcs: []domain.StoryArcCandidate{}, Conflicts: []domain.ReviewIssue{}, ReviewIssues: []domain.ReviewIssue{},
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func stringPointer(value string) *string { return &value }

func intPointer(value int) *int { return &value }
