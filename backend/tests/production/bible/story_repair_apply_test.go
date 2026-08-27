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

func TestApplyStoryCandidateRepairPatchChangesOnlyTheFrozenFragment(t *testing.T) {
	evidence := domain.Evidence{
		SourceStart: 0, SourceEnd: 2, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
	}
	candidate := storyReconciliationReviewCandidate(evidence)
	parent := mustJSON(t, candidate)
	fragment := mustJSON(t, candidate.CanonicalEntities[0])
	fragmentHash, err := agentcontract.StoryGraphCandidateFragmentHash(fragment)
	if err != nil {
		t.Fatal(err)
	}
	targetID := uuid.NewString()
	input := agentcontract.StoryGraphRepairStageInput{
		TargetCandidateRevisionID: targetID, TargetCandidateRevisionHash: strings.Repeat("a", 64),
		ReviewCandidateRevisionID: uuid.NewString(), ReviewCandidateRevisionHash: strings.Repeat("b", 64),
		TargetIssue: agentcontract.StoryGraphReviewIssue{
			IssueKey: "issue:canonical", Code: "canonical_name_ambiguous", Severity: "blocking",
			Scope: "entity", SubjectKey: stringPointer("character:lin-yi"), Summary: "规范名冲突",
			Evidence: []agentcontract.StoryGraphEvidence{{
				SourceStart: evidence.SourceStart, SourceEnd: evidence.SourceEnd,
				TextHash: evidence.TextHash, ExactAnchor: evidence.ExactAnchor,
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
		TargetCandidateRevisionID: targetID, TargetCandidateRevisionHash: input.TargetCandidateRevisionHash,
		Operations: []agentcontract.StoryGraphRepairOperation{{
			TargetCandidateKey: "character:lin-yi", BaseFragmentHash: fragmentHash,
			FieldName:   "canonical_name",
			Replacement: agentcontract.StoryGraphRepairReplacement{Text: stringPointer("林逸")},
		}},
		ReviewIssues: []agentcontract.StoryGraphReviewIssue{},
	}

	repaired, err := bibleapp.ApplyStoryCandidateRepairPatch(parent, input, patch)
	if err != nil {
		t.Fatalf("apply bounded Candidate repair Patch: %v", err)
	}
	var decoded domain.StoryReconciliationCandidate
	if err = json.Unmarshal(repaired, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.CanonicalEntities[0].CanonicalName != "林逸" ||
		decoded.CanonicalEntities[0].EntityKey != candidate.CanonicalEntities[0].EntityKey ||
		!reflect.DeepEqual(decoded.CanonicalEntities[0].Evidence, candidate.CanonicalEntities[0].Evidence) {
		t.Fatalf("bounded Candidate repair escaped its field: %#v", decoded.CanonicalEntities[0])
	}

	forgedInput := input
	forgedInput.AllowedTargets = append([]agentcontract.StoryGraphRepairAllowedTarget(nil), input.AllowedTargets...)
	forgedInput.AllowedTargets[0].Fragment = json.RawMessage(`{"entity_key":"character:lin-yi","canonical_name":"伪造"}`)
	forgedInput.AllowedTargets[0].BaseFragmentHash, err = agentcontract.StoryGraphCandidateFragmentHash(
		forgedInput.AllowedTargets[0].Fragment,
	)
	if err != nil {
		t.Fatal(err)
	}
	patch.Operations[0].BaseFragmentHash = forgedInput.AllowedTargets[0].BaseFragmentHash
	if _, err = bibleapp.ApplyStoryCandidateRepairPatch(parent, forgedInput, patch); err == nil {
		t.Fatal("Candidate repair accepted a frozen fragment that was not in the parent revision")
	}
}

func TestStoryCandidateStaleClosureIsExactAndExcludesTheAppliedRepair(t *testing.T) {
	root := bibleapp.CandidateRevisionRef{ID: uuid.NewString(), Hash: strings.Repeat("a", 64)}
	reviewRevision := bibleapp.CandidateRevisionRef{ID: uuid.NewString(), Hash: strings.Repeat("b", 64)}
	aggregateRevision := bibleapp.CandidateRevisionRef{ID: uuid.NewString(), Hash: strings.Repeat("c", 64)}
	segmentRevision := bibleapp.CandidateRevisionRef{ID: uuid.NewString(), Hash: strings.Repeat("d", 64)}
	appliedRepairID := uuid.NewString()
	reviewID, segmentID, detailID, unrelatedID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	dependencies := []bibleapp.CandidateStageDependency{
		{
			InvocationID: reviewID, StageInstanceKey: strings.Repeat("1", 64),
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{{
				CandidateRevisionID: root.ID, CandidateRevisionHash: root.Hash,
			}},
			CandidateRevisions: []bibleapp.CandidateRevisionRef{reviewRevision},
		},
		{
			InvocationID: appliedRepairID, StageInstanceKey: strings.Repeat("2", 64),
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{{
				CandidateRevisionID: root.ID, CandidateRevisionHash: root.Hash,
			}},
		},
		{
			StageInstanceKey: strings.Repeat("3", 64),
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{{
				CandidateRevisionID: reviewRevision.ID, CandidateRevisionHash: reviewRevision.Hash,
			}},
			CandidateRevisions: []bibleapp.CandidateRevisionRef{aggregateRevision},
		},
		{
			InvocationID: segmentID, StageInstanceKey: strings.Repeat("4", 64),
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{{
				CandidateRevisionID: aggregateRevision.ID, CandidateRevisionHash: aggregateRevision.Hash,
			}},
			CandidateRevisions: []bibleapp.CandidateRevisionRef{segmentRevision},
		},
		{
			InvocationID: detailID, StageInstanceKey: strings.Repeat("5", 64),
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{{
				CandidateRevisionID: segmentRevision.ID, CandidateRevisionHash: segmentRevision.Hash,
			}},
		},
		{
			InvocationID: unrelatedID, StageInstanceKey: strings.Repeat("6", 64),
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{{
				CandidateRevisionID: uuid.NewString(), CandidateRevisionHash: strings.Repeat("d", 64),
			}},
		},
	}

	closure, err := bibleapp.StoryCandidateStaleClosure(root, appliedRepairID, nil, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if len(closure) != 4 || closure[0].InvocationID != reviewID ||
		closure[0].CauseCandidateRevisionID != root.ID ||
		closure[1].InvocationID != "" || closure[1].StageInstanceKey != strings.Repeat("3", 64) ||
		closure[1].CauseCandidateRevisionID != reviewRevision.ID ||
		closure[2].InvocationID != segmentID ||
		closure[2].CauseCandidateRevisionID != aggregateRevision.ID ||
		closure[3].InvocationID != detailID ||
		closure[3].CauseCandidateRevisionID != segmentRevision.ID {
		t.Fatalf("unexpected Candidate stale closure: %#v", closure)
	}
}
