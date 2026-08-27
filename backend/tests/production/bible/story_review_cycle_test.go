package bible_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type storyReviewRepositoryStub struct {
	seed bibleapp.StoryReviewSeed
}

func (stub *storyReviewRepositoryStub) LoadStoryReview(
	context.Context,
	bibleapp.StoryReviewCommand,
) (bibleapp.StoryReviewSeed, error) {
	return stub.seed, nil
}

func (*storyReviewRepositoryStub) EnsureStoryReviewInvocation(
	context.Context,
	bibleapp.StoryReviewPreparation,
) error {
	return nil
}

func (*storyReviewRepositoryStub) EnsureStoryRepairInvocation(
	context.Context,
	bibleapp.StoryRepairPreparation,
) error {
	return nil
}

func TestStoryReviewBudgetExhaustionRemainsNeedsReview(t *testing.T) {
	now := time.Date(2026, time.August, 28, 6, 0, 0, 0, time.UTC)
	evidence := domain.Evidence{
		SourceStart: 0, SourceEnd: 2, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
	}
	candidate := storyReconciliationReviewCandidate(evidence)
	candidateJSON := mustJSON(t, candidate)
	candidateID := uuid.NewString()
	candidateHash := strings.Repeat("a", 64)
	manifest, err := domain.BuildStoryReviewManifest(domain.StoryReviewManifestInput{
		ManifestID: uuid.NewString(), Version: 1, WorkspaceID: uuid.NewString(),
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		TargetCandidateRevisionID: candidateID, TargetCandidateRevisionHash: candidateHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	subject := candidate.CanonicalEntities[0].EntityKey
	reviewCandidate := mustJSON(t, agentcontract.StoryGraphReviewCandidate{
		ReviewedStage:             domain.ReconcileStoryStage,
		TargetCandidateRevisionID: candidateID, TargetCandidateRevisionHash: candidateHash,
		ReviewIssues: []agentcontract.StoryGraphReviewIssue{{
			IssueKey: "review:canonical", Code: "canonical_name_ambiguous", Severity: "blocking",
			Scope: "entity", SubjectKey: &subject, Summary: "规范名需要修复",
			Evidence: []agentcontract.StoryGraphEvidence{{
				SourceStart: evidence.SourceStart, SourceEnd: evidence.SourceEnd,
				TextHash: evidence.TextHash, ExactAnchor: evidence.ExactAnchor,
			}},
		}},
	})
	stub := &storyReviewRepositoryStub{seed: bibleapp.StoryReviewSeed{
		CurrentCandidateRevisionID: candidateID, CurrentCandidateRevisionHash: candidateHash,
		CurrentCandidateRevisionNo: 2, CurrentStageInstanceKey: strings.Repeat("b", 64),
		CurrentCandidate: candidateJSON, LatestManifest: &manifest, RepairsUsed: 1,
		Review: &bibleapp.StoryReviewInvocationState{
			InvocationID: uuid.NewString(), Status: "succeeded", ResultHash: strings.Repeat("c", 64),
			CandidateRevisionID: uuid.NewString(), CandidateRevisionHash: strings.Repeat("d", 64),
			Candidate: reviewCandidate,
		},
	}}
	service := bibleapp.NewStoryReviewService(
		stub,
		bibleapp.NewStoryCandidateRepairService(nil, bibleapp.Config{}),
		bibleapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	state, err := service.EnsureStoryReview(context.Background(), bibleapp.StoryReviewCommand{
		Actor:       bibleapp.Actor{UserID: uuid.NewString(), TokenVersion: 1},
		WorkspaceID: manifest.WorkspaceID, ProjectID: uuid.NewString(),
		WorkflowRunID: manifest.WorkflowRunID, NodeRunID: manifest.NodeRunID,
		CandidateRevisionID: candidateID, CandidateRevisionHash: candidateHash, MaxRepairRounds: 1,
	})
	if err != nil || state.Status != "needs_review" || state.FailureCode != "repair_budget_exhausted" {
		t.Fatalf("repair budget exhaustion escaped the explicit review state: state=%#v err=%v", state, err)
	}

	stub.seed.Review = &bibleapp.StoryReviewInvocationState{
		InvocationID: uuid.NewString(), Status: "failed", FailureCode: "execution_budget_exceeded",
	}
	state, err = service.EnsureStoryReview(context.Background(), bibleapp.StoryReviewCommand{
		Actor:       bibleapp.Actor{UserID: uuid.NewString(), TokenVersion: 1},
		WorkspaceID: manifest.WorkspaceID, ProjectID: uuid.NewString(),
		WorkflowRunID: manifest.WorkflowRunID, NodeRunID: manifest.NodeRunID,
		CandidateRevisionID: candidateID, CandidateRevisionHash: candidateHash, MaxRepairRounds: 1,
	})
	if err != nil || state.Status != "needs_review" || state.FailureCode != "execution_budget_exceeded" {
		t.Fatalf("model-call budget exhaustion became a retry or success: state=%#v err=%v", state, err)
	}
}
