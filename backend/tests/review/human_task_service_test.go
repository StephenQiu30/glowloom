package review_test

import (
	"context"
	"errors"
	"testing"
	"time"

	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
	review "github.com/StephenQiu30/lanverse/backend/internal/review/domain"
)

const humanTaskSubjectHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestHumanTaskClaimExpiryAndImmutableDecision(t *testing.T) {
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	repository := newHumanTaskRepository()
	identities := []string{
		"task-1", "claim-a", "claim-b", "decision-old-token", "decision-1", "decision-replay", "decision-mismatch",
	}
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
		SubjectHash: humanTaskSubjectHash, CandidateIDs: []string{},
		AllowedDecisions: []string{"approved", "changes_requested", "rejected"}, RubricVersion: "production-bible-review",
	})
	if err != nil || opened.Status != "OPEN" || opened.Revision != 1 {
		t.Fatalf("open human task: task=%#v err=%v", opened, err)
	}
	replayedOpen, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: "workspace-1", ProjectID: "project-1", WorkflowRunID: "run-1", NodeRunID: "node-run-1",
		SubjectType: "workflow_node_output", SubjectID: "node-run-1", SubjectRevision: 7,
		SubjectHash: humanTaskSubjectHash, CandidateIDs: []string{},
		AllowedDecisions: []string{"approved", "changes_requested", "rejected"}, RubricVersion: "production-bible-review",
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
		ExpectedSubjectRevision: 7, ExpectedSubjectHash: humanTaskSubjectHash,
		Decision: "approved", IdempotencyKey: "decision-old-token",
	}); err == nil {
		t.Fatal("expired claim token submitted a decision")
	}

	decided, err := service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: second.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, ExpectedSubjectHash: humanTaskSubjectHash,
		Decision: "approved", IdempotencyKey: "decision-approved",
	})
	if err != nil || decided.Task.Status != "COMPLETED" || decided.Decision.Decision != "approved" {
		t.Fatalf("record decision: result=%#v err=%v", decided, err)
	}
	replayedDecision, err := service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: second.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, ExpectedSubjectHash: humanTaskSubjectHash,
		Decision: "approved", IdempotencyKey: "decision-approved",
	})
	if err != nil || replayedDecision.Decision.ID != decided.Decision.ID {
		t.Fatalf("replay decision: result=%#v err=%v", replayedDecision, err)
	}
	if _, err = service.Decide(ctx, reviewapp.Actor{UserID: "reviewer-b"}, reviewapp.DecideCommand{
		TaskID: opened.ID, ClaimToken: second.ClaimToken, ExpectedTaskRevision: second.Task.Revision,
		ExpectedSubjectRevision: 7, ExpectedSubjectHash: humanTaskSubjectHash,
		Decision: "rejected", IdempotencyKey: "decision-approved",
	}); err == nil {
		t.Fatal("decision idempotency key accepted different terminal input")
	}
}

func TestHumanTaskOwnerRenewsAndReleasesClaimIdempotently(t *testing.T) {
	now := time.Date(2026, time.August, 26, 3, 0, 0, 0, time.UTC)
	repository := newHumanTaskRepository()
	identities := []string{"task-lease", "claim-a", "claim-b"}
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
	task, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: "workspace-lease", ProjectID: "project-lease", WorkflowRunID: "run-lease", NodeRunID: "node-lease",
		SubjectType: "workflow_node_output", SubjectID: "subject-lease", SubjectRevision: 3,
		SubjectHash: humanTaskSubjectHash, CandidateIDs: []string{},
		AllowedDecisions: []string{"approved", "changes_requested", "rejected"}, RubricVersion: "lease-review",
	})
	if err != nil {
		t.Fatalf("open lease task: %v", err)
	}
	actorA := reviewapp.Actor{UserID: "reviewer-a"}
	actorB := reviewapp.Actor{UserID: "reviewer-b"}
	claimed, err := service.Claim(ctx, actorA, reviewapp.ClaimCommand{
		TaskID: task.ID, ExpectedRevision: task.Revision, IdempotencyKey: "lease-claim-a",
	})
	if err != nil {
		t.Fatalf("claim lease task: %v", err)
	}
	now = now.Add(4 * time.Minute)
	renewCommand := reviewapp.RenewCommand{
		TaskID: task.ID, ClaimToken: claimed.ClaimToken,
		ExpectedRevision: claimed.Task.Revision, IdempotencyKey: "lease-renew-a",
	}
	renewed, err := service.Renew(ctx, actorA, renewCommand)
	if err != nil || renewed.ClaimToken != claimed.ClaimToken || renewed.Task.ClaimExpiresAt == nil ||
		!renewed.Task.ClaimExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("renew claim lease: result=%#v err=%v", renewed, err)
	}
	replayedRenew, err := service.Renew(ctx, actorA, renewCommand)
	if err != nil || replayedRenew.Task.Revision != renewed.Task.Revision {
		t.Fatalf("replay claim renewal: result=%#v err=%v", replayedRenew, err)
	}
	if _, err = service.Release(ctx, actorB, reviewapp.ReleaseCommand{
		TaskID: task.ID, ClaimToken: claimed.ClaimToken,
		ExpectedRevision: renewed.Task.Revision, IdempotencyKey: "lease-release-b",
	}); err == nil {
		t.Fatal("another reviewer released an active claim")
	}
	releaseCommand := reviewapp.ReleaseCommand{
		TaskID: task.ID, ClaimToken: claimed.ClaimToken,
		ExpectedRevision: renewed.Task.Revision, IdempotencyKey: "lease-release-a",
	}
	released, err := service.Release(ctx, actorA, releaseCommand)
	if err != nil || released.Status != "OPEN" || released.ClaimedBy != nil ||
		released.ClaimToken != nil || released.ClaimExpiresAt != nil {
		t.Fatalf("release claim lease: task=%#v err=%v", released, err)
	}
	replayedRelease, err := service.Release(ctx, actorA, releaseCommand)
	if err != nil || replayedRelease.Revision != released.Revision {
		t.Fatalf("replay claim release: task=%#v err=%v", replayedRelease, err)
	}
	claimedByB, err := service.Claim(ctx, actorB, reviewapp.ClaimCommand{
		TaskID: task.ID, ExpectedRevision: released.Revision, IdempotencyKey: "lease-claim-b",
	})
	if err != nil || claimedByB.Task.ClaimedBy == nil || *claimedByB.Task.ClaimedBy != actorB.UserID {
		t.Fatalf("claim released task: result=%#v err=%v", claimedByB, err)
	}
	if _, err = service.Renew(ctx, actorA, reviewapp.RenewCommand{
		TaskID: task.ID, ClaimToken: claimed.ClaimToken,
		ExpectedRevision: claimedByB.Task.Revision, IdempotencyKey: "lease-renew-old-token",
	}); err == nil {
		t.Fatal("released claim token renewed a later reviewer's lease")
	}
}

func TestHumanTaskExpireSweepReopensExpiredClaimOnce(t *testing.T) {
	now := time.Date(2026, time.August, 26, 4, 0, 0, 0, time.UTC)
	repository := newHumanTaskRepository()
	identities := []string{"task-expire", "claim-expire"}
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
	task, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: "workspace-expire", ProjectID: "project-expire", WorkflowRunID: "run-expire", NodeRunID: "node-expire",
		SubjectType: "workflow_node_output", SubjectID: "subject-expire", SubjectRevision: 2,
		SubjectHash: humanTaskSubjectHash, CandidateIDs: []string{},
		AllowedDecisions: []string{"approved", "changes_requested", "rejected"}, RubricVersion: "expire-review",
	})
	if err != nil {
		t.Fatalf("open expiring task: %v", err)
	}
	claimed, err := service.Claim(ctx, reviewapp.Actor{UserID: "reviewer-expire"}, reviewapp.ClaimCommand{
		TaskID: task.ID, ExpectedRevision: task.Revision, IdempotencyKey: "expire-claim",
	})
	if err != nil {
		t.Fatalf("claim expiring task: %v", err)
	}
	if count, expireErr := service.ExpireClaims(ctx, 10); expireErr != nil || count != 0 {
		t.Fatalf("expire active claim: count=%d err=%v", count, expireErr)
	}
	now = now.Add(6 * time.Minute)
	count, err := service.ExpireClaims(ctx, 10)
	if err != nil || count != 1 || repository.task.Status != "OPEN" || repository.task.ClaimedBy != nil ||
		repository.task.ClaimToken != nil || repository.task.ClaimExpiresAt != nil ||
		repository.task.Revision != claimed.Task.Revision+1 {
		t.Fatalf("expire claim once: count=%d task=%#v err=%v", count, repository.task, err)
	}
	if count, err = service.ExpireClaims(ctx, 10); err != nil || count != 0 {
		t.Fatalf("replay claim expiry sweep: count=%d err=%v", count, err)
	}
}

type humanTaskRepository struct {
	task             review.HumanTask
	decision         review.ReviewDecision
	claimReceipts    map[string]review.ClaimResult
	renewReceipts    map[string]renewReceipt
	releaseReceipts  map[string]releaseReceipt
	decisionReceipts map[string]review.DecisionResult
}

type renewReceipt struct {
	command reviewapp.RenewCommand
	result  review.ClaimResult
}

type releaseReceipt struct {
	command reviewapp.ReleaseCommand
	result  review.HumanTask
}

func newHumanTaskRepository() *humanTaskRepository {
	return &humanTaskRepository{
		claimReceipts: make(map[string]review.ClaimResult), renewReceipts: make(map[string]renewReceipt),
		releaseReceipts: make(map[string]releaseReceipt), decisionReceipts: make(map[string]review.DecisionResult),
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

func (repo *humanTaskRepository) FindTaskByNode(_ context.Context, workspaceID, nodeRunID string) (review.HumanTask, error) {
	if repo.task.ID == "" || repo.task.WorkspaceID != workspaceID || repo.task.NodeRunID != nodeRunID {
		return review.HumanTask{}, reviewapp.ErrNotFound
	}
	return repo.task, nil
}

func (repo *humanTaskRepository) GetDecision(_ context.Context, _ reviewapp.Actor, decisionID string) (review.DecisionResult, error) {
	if repo.decision.ID != decisionID || repo.task.ID == "" {
		return review.DecisionResult{}, reviewapp.ErrNotFound
	}
	return review.DecisionResult{Task: repo.task, Decision: repo.decision}, nil
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

func (repo *humanTaskRepository) Renew(
	_ context.Context,
	actor reviewapp.Actor,
	command reviewapp.RenewCommand,
	expiresAt time.Time,
	now time.Time,
) (review.ClaimResult, error) {
	key := actor.UserID + ":" + command.IdempotencyKey
	if receipt, exists := repo.renewReceipts[key]; exists {
		if receipt.command != command {
			return review.ClaimResult{}, errors.New("renew input mismatch")
		}
		return receipt.result, nil
	}
	if repo.task.ID != command.TaskID || repo.task.Revision != command.ExpectedRevision || repo.task.Status != "CLAIMED" ||
		repo.task.ClaimedBy == nil || *repo.task.ClaimedBy != actor.UserID || repo.task.ClaimToken == nil ||
		*repo.task.ClaimToken != command.ClaimToken || repo.task.ClaimExpiresAt == nil || !repo.task.ClaimExpiresAt.After(now) {
		return review.ClaimResult{}, errors.New("renew conflict")
	}
	repo.task.ClaimExpiresAt, repo.task.UpdatedAt = &expiresAt, now
	repo.task.Revision++
	result := review.ClaimResult{Task: repo.task, ClaimToken: command.ClaimToken}
	repo.renewReceipts[key] = renewReceipt{command: command, result: result}
	return result, nil
}

func (repo *humanTaskRepository) Release(
	_ context.Context,
	actor reviewapp.Actor,
	command reviewapp.ReleaseCommand,
	now time.Time,
) (review.HumanTask, error) {
	key := actor.UserID + ":" + command.IdempotencyKey
	if receipt, exists := repo.releaseReceipts[key]; exists {
		if receipt.command != command {
			return review.HumanTask{}, errors.New("release input mismatch")
		}
		return receipt.result, nil
	}
	if repo.task.ID != command.TaskID || repo.task.Revision != command.ExpectedRevision || repo.task.Status != "CLAIMED" ||
		repo.task.ClaimedBy == nil || *repo.task.ClaimedBy != actor.UserID || repo.task.ClaimToken == nil ||
		*repo.task.ClaimToken != command.ClaimToken || repo.task.ClaimExpiresAt == nil || !repo.task.ClaimExpiresAt.After(now) {
		return review.HumanTask{}, errors.New("release conflict")
	}
	repo.task.Status, repo.task.ClaimedBy, repo.task.ClaimToken, repo.task.ClaimExpiresAt = "OPEN", nil, nil, nil
	repo.task.UpdatedAt = now
	repo.task.Revision++
	repo.releaseReceipts[key] = releaseReceipt{command: command, result: repo.task}
	return repo.task, nil
}

func (repo *humanTaskRepository) ExpireClaims(_ context.Context, limit int, now time.Time) (int, error) {
	if limit < 1 || repo.task.Status != "CLAIMED" || repo.task.ClaimExpiresAt == nil || repo.task.ClaimExpiresAt.After(now) {
		return 0, nil
	}
	repo.task.Status, repo.task.ClaimedBy, repo.task.ClaimToken, repo.task.ClaimExpiresAt = "OPEN", nil, nil, nil
	repo.task.UpdatedAt = now
	repo.task.Revision++
	return 1, nil
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
