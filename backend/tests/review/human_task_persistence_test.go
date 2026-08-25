package review_test

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	reviewgorm "github.com/StephenQiu30/lanverse/backend/internal/review/adapter/gormdb"
	reviewapp "github.com/StephenQiu30/lanverse/backend/internal/review/application"
)

func TestHumanTaskPersistsClaimTakeoverAndOneDecision(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL human task journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	now := time.Date(2026, time.August, 26, 1, 0, 0, 0, time.UTC)
	workspaceID, projectID := uuid.New(), uuid.New()
	reviewerA, reviewerB := uuid.New(), uuid.New()
	users := []model.UserAccount{
		{ID: reviewerA, EmailNormalized: "reviewer-a@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "A", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: reviewerB, EmailNormalized: "reviewer-b@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "B", Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&users).Error; err != nil {
		t.Fatalf("seed reviewers: %v", err)
	}
	if err = database.Create(&model.Workspace{ID: workspaceID, Name: "Review", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	memberships := []model.Membership{
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: reviewerA, Role: "editor", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: reviewerB, Role: "editor", Status: "active", JoinedAt: now},
	}
	if err = database.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}
	if err = database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "Review Project", AspectRatio: "9:16", Language: "zh-CN",
		TargetDurationMS: 60000, BudgetLimit: decimal.Zero, Currency: "CNY", Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed project: %v", err)
	}

	service := reviewapp.NewService(reviewgorm.New(database), reviewapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimLease: 5 * time.Minute,
	})
	task, err := service.Open(ctx, reviewapp.OpenCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		SubjectType: "workflow_node_output", SubjectID: uuid.NewString(), SubjectRevision: 3,
		CandidateIDs: []string{uuid.NewString(), uuid.NewString()}, RubricVersion: "storyboard-review-v1",
	})
	if err != nil {
		t.Fatalf("open persisted human task: %v", err)
	}
	first, err := service.Claim(ctx, reviewapp.Actor{UserID: reviewerA.String(), TokenVersion: 1}, reviewapp.ClaimCommand{
		TaskID: task.ID, ExpectedRevision: task.Revision, IdempotencyKey: "persisted-claim-a",
	})
	if err != nil {
		t.Fatalf("claim persisted human task: %v", err)
	}
	now = now.Add(time.Minute)
	renewCommand := reviewapp.RenewCommand{
		TaskID: task.ID, ClaimToken: first.ClaimToken,
		ExpectedRevision: first.Task.Revision, IdempotencyKey: "persisted-renew-a",
	}
	renewed, err := service.Renew(ctx, reviewapp.Actor{UserID: reviewerA.String(), TokenVersion: 1}, renewCommand)
	if err != nil || renewed.ClaimToken != first.ClaimToken || renewed.Task.ClaimExpiresAt == nil ||
		!renewed.Task.ClaimExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("renew persisted human task claim: result=%#v err=%v", renewed, err)
	}
	replayedRenew, err := service.Renew(ctx, reviewapp.Actor{UserID: reviewerA.String(), TokenVersion: 1}, renewCommand)
	if err != nil || replayedRenew.Task.Revision != renewed.Task.Revision {
		t.Fatalf("replay persisted claim renewal: result=%#v err=%v", replayedRenew, err)
	}
	now = now.Add(6 * time.Minute)
	second, err := service.Claim(ctx, reviewapp.Actor{UserID: reviewerB.String(), TokenVersion: 1}, reviewapp.ClaimCommand{
		TaskID: task.ID, ExpectedRevision: renewed.Task.Revision, IdempotencyKey: "persisted-claim-b",
	})
	if err != nil {
		t.Fatalf("take over expired persisted claim: %v", err)
	}
	releaseCommand := reviewapp.ReleaseCommand{
		TaskID: task.ID, ClaimToken: second.ClaimToken,
		ExpectedRevision: second.Task.Revision, IdempotencyKey: "persisted-release-b",
	}
	released, err := service.Release(ctx, reviewapp.Actor{UserID: reviewerB.String(), TokenVersion: 1}, releaseCommand)
	if err != nil || released.Status != "OPEN" || released.ClaimedBy != nil || released.ClaimToken != nil ||
		released.ClaimExpiresAt != nil {
		t.Fatalf("release persisted claim: task=%#v err=%v", released, err)
	}
	replayedRelease, err := service.Release(ctx, reviewapp.Actor{UserID: reviewerB.String(), TokenVersion: 1}, releaseCommand)
	if err != nil || replayedRelease.Revision != released.Revision {
		t.Fatalf("replay persisted claim release: task=%#v err=%v", replayedRelease, err)
	}
	third, err := service.Claim(ctx, reviewapp.Actor{UserID: reviewerB.String(), TokenVersion: 1}, reviewapp.ClaimCommand{
		TaskID: task.ID, ExpectedRevision: released.Revision, IdempotencyKey: "persisted-claim-b-after-release",
	})
	if err != nil {
		t.Fatalf("claim released persisted human task: %v", err)
	}
	if _, err = service.Decide(ctx, reviewapp.Actor{UserID: reviewerA.String(), TokenVersion: 1}, reviewapp.DecideCommand{
		TaskID: task.ID, ClaimToken: first.ClaimToken, ExpectedTaskRevision: third.Task.Revision,
		ExpectedSubjectRevision: task.SubjectRevision, Decision: "approved", IdempotencyKey: "persisted-old-token",
	}); err == nil {
		t.Fatal("persisted expired claim token submitted a decision")
	}
	decided, err := service.Decide(ctx, reviewapp.Actor{UserID: reviewerB.String(), TokenVersion: 1}, reviewapp.DecideCommand{
		TaskID: task.ID, ClaimToken: third.ClaimToken, ExpectedTaskRevision: third.Task.Revision,
		ExpectedSubjectRevision: task.SubjectRevision, Decision: "approved", IdempotencyKey: "persisted-decision",
	})
	if err != nil {
		t.Fatalf("persist review decision: %v", err)
	}
	replayed, err := service.Decide(ctx, reviewapp.Actor{UserID: reviewerB.String(), TokenVersion: 1}, reviewapp.DecideCommand{
		TaskID: task.ID, ClaimToken: third.ClaimToken, ExpectedTaskRevision: third.Task.Revision,
		ExpectedSubjectRevision: task.SubjectRevision, Decision: "approved", IdempotencyKey: "persisted-decision",
	})
	if err != nil || replayed.Decision.ID != decided.Decision.ID {
		t.Fatalf("replay persisted decision: result=%#v err=%v", replayed, err)
	}

	var taskCount, decisionCount int64
	if err = database.Model(&model.HumanTask{}).Count(&taskCount).Error; err != nil {
		t.Fatalf("count human tasks: %v", err)
	}
	if err = database.Model(&model.ReviewDecision{}).Count(&decisionCount).Error; err != nil {
		t.Fatalf("count review decisions: %v", err)
	}
	if taskCount != 1 || decisionCount != 1 {
		t.Fatalf("persisted review counts = tasks %d decisions %d", taskCount, decisionCount)
	}
	var renewReceiptCount, releaseReceiptCount int64
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ?", workspaceID, "review.human_task.renew").
		Count(&renewReceiptCount).Error; err != nil {
		t.Fatalf("count claim renewal receipts: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ?", workspaceID, "review.human_task.release").
		Count(&releaseReceiptCount).Error; err != nil {
		t.Fatalf("count claim release receipts: %v", err)
	}
	if renewReceiptCount != 1 || releaseReceiptCount != 1 {
		t.Fatalf("lease receipt counts = renew %d release %d", renewReceiptCount, releaseReceiptCount)
	}
}
