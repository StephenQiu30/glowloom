package review_test

import (
	"context"
	"errors"
	"testing"
	"time"

	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	review "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
)

func TestHumanTaskClaimExpiryAndImmutableDecision(t *testing.T) {
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	repository := newHumanTaskRepository()
	identities := []string{"task-1", "claim-a", "claim-b", "decision-1"}
	service := reviewapp.NewService(repository, reviewapp.Config{
		Now: func() time.Time { return now },
		NewID: func() string {
			value := identities[0]
			identities = identities[1:]
			return value
		},
		ClaimLease: 5 * time.Minute,
	})
	ctx := context.Background()
	opened, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: "workspace-1", ProjectID: "project-1", WorkflowRunID: "run-1", NodeRunID: "node-run-1",
		SubjectType: "workflow_node_output", SubjectID: "node-run-1", SubjectRevision: 7,
		CandidateIDs: []string{}, RubricVersion: "production-bible-review-v1",
	})
	if err != nil || opened.Status != "OPEN" || opened.Revision != 1 {
		t.Fatalf("open human task: task=%#v err=%v", opened, err)
	}
	replayedOpen, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: "workspace-1", ProjectID: "project-1", WorkflowRunID: "run-1", NodeRunID: "node-run-1",
		SubjectType: "workflow_node_output", SubjectID: "node-run-1", SubjectRevision: 7,
		CandidateIDs: []string{}, RubricVersion: "production-bible-review-v1",
	})
	if err != nil || replayedOpen.ID != opened.ID {
		t.Fatalf("replay open human task: task=%#v err=%v", replayedOpen, err)
	}

	first, err := service.Claim(ctx, reviewapp.Actor{UserID: "reviewer-a"}, reviewapp.ClaimCommand{
		TaskID: opened.ID, ExpectedRevision: opened.Revision, IdempotencyKey: "claim-a",
	})
	if err != nil || first.Task.Status != "CLAIMED" || first.ClaimToken != "claim-a" {
		t.Fatalf("first claim: result=%#v err=%v", first, err)
	}
	now = now.Add(6 * time.Minute)
	second, err := service.Claim(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.ClaimCommand{
		TaskID: opened.ID, ExpectedRevision: first.Task.Revision, IdempotencyKey: "claim-b",
	})
	if err != nil || second.Task.ClaimedBy == nil || *second.Task.ClaimedBy != "reviewer-b" || second.ClaimToken != "claim-b" {
		t.Fatalf("expired claim takeover: result=%#v err=%v", second, err)
	}
	if _, err = service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-a"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: first.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, Decision: "approved", IdempotencyKey: "decision-old-token",
	}); err == nil {
		t.Fatal("expired claim token submitted a decision")
	}

	decided, err := service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: second.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, Decision: "approved", IdempotencyKey: "decision-approved",
	})
	if err != nil || decided.Task.Status != "COMPLETED" || decided.Decision.Decision != "approved" {
		t.Fatalf("record decision: result=%#v err=%v", decided, err)
	}
	replayedDecision, err := service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: second.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, Decision: "approved", IdempotencyKey: "decision-approved",
	})
	if err != nil || replayedDecision.Decision.ID != decided.Decision.ID {
		t.Fatalf("replay decision: result=%#v err=%v", replayedDecision, err)
	}
	if _, err = service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: second.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, Decision: "rejected", IdempotencyKey: "decision-approved",
	}); err == nil {
		t.Fatal("decision idempotency key accepted different terminal input")
	}
}

type humanTaskRepository struct {
	task             review.HumanTask
	decision         review.ReviewDecision
	claimReceipts    map[string]review.ClaimResult
	decisionReceipts map[string]review.DecisionResult
}

func newHumanTaskRepository() *humanTaskRepository {
	return &humanTaskRepository{
		claimReceipts: make(map[string]review.ClaimResult), decisionReceipts: make(map[string]review.DecisionResult),
	}
}

func (repo *humanTaskRepository) EnsureTask(_ context.Context, desired review.HumanTask) (review.HumanTask, error) {
	if repo.task.ID == "" {
		repo.task = desired
		return repo.task, nil
	}
	if !review.SameTaskBinding(repo.task, desired) {
		return review.HumanTask{}, errors.New("task binding drift")
	}
	return repo.task, nil
}

func (repo *humanTaskRepository) Claim(
	_ context.Context,
	actor reviewapp.Actor,
	command reviewapp.ClaimCommand,
	claimToken string,
	expiresAt time.Time,
	now time.Time,
) (review.ClaimResult, error) {
	if receipt, exists := repo.claimReceipts[actor.UserID+":"+command.IdempotencyKey]; exists {
		return receipt, nil
	}
	if repo.task.ID != command.TaskID || repo.task.Revision != command.ExpectedRevision || repo.task.Status == "COMPLETED" {
		return review.ClaimResult{}, errors.New("claim conflict")
	}
	if repo.task.Status == "CLAIMED" && repo.task.ClaimExpiresAt != nil && repo.task.ClaimExpiresAt.After(now) {
		return review.ClaimResult{}, errors.New("task is already claimed")
	}
	repo.task.Status, repo.task.ClaimedBy, repo.task.ClaimToken = "CLAIMED", &actor.UserID, &claimToken
	repo.task.ClaimExpiresAt, repo.task.UpdatedAt = &expiresAt, now
	repo.task.Revision++
	result := review.ClaimResult{Task: repo.task, ClaimToken: claimToken}
	repo.claimReceipts[actor.UserID+":"+command.IdempotencyKey] = result
	return result, nil
}

func (repo *humanTaskRepository) Decide(
	_ context.Context,
	actor reviewapp.Actor,
	command reviewapp.DecideCommand,
	desired review.ReviewDecision,
	now time.Time,
) (review.DecisionResult, error) {
	key := actor.UserID + ":" + command.IdempotencyKey
	if receipt, exists := repo.decisionReceipts[key]; exists {
		if receipt.Decision.Decision != command.Decision {
			return review.DecisionResult{}, errors.New("decision input mismatch")
		}
		return receipt, nil
	}
	if repo.task.ID != command.TaskID || repo.task.Revision != command.ExpectedTaskRevision ||
		repo.task.SubjectRevision != command.ExpectedSubjectRevision || repo.task.Status != "CLAIMED" ||
		repo.task.ClaimedBy == nil || *repo.task.ClaimedBy != actor.UserID || repo.task.ClaimToken == nil ||
		*repo.task.ClaimToken != command.ClaimToken || repo.task.ClaimExpiresAt == nil || !repo.task.ClaimExpiresAt.After(now) {
		return review.DecisionResult{}, errors.New("decision conflict")
	}
	repo.decision = desired
	repo.task.Status, repo.task.UpdatedAt = "COMPLETED", now
	repo.task.ClaimToken, repo.task.ClaimExpiresAt = nil, nil
	repo.task.Revision++
	result := review.DecisionResult{Task: repo.task, Decision: repo.decision}
	repo.decisionReceipts[key] = result
	return result, nil
}
