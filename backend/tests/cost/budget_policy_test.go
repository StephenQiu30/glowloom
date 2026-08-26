package cost_test

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
)

func TestProjectBudgetHasOneCostOwner(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the cost budget journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open cost test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	if database.Migrator().HasColumn(&model.Project{}, "budget_limit") ||
		database.Migrator().HasColumn(&model.Project{}, "currency") {
		t.Fatal("Production Project still owns budget columns")
	}

	now := time.Date(2026, time.August, 26, 18, 0, 0, 0, time.UTC)
	workspaceID, otherWorkspaceID := uuid.New(), uuid.New()
	projectID, otherProjectID := uuid.New(), uuid.New()
	ownerID, editorID, viewerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	users := []model.UserAccount{
		costUser(ownerID, "owner", now), costUser(editorID, "editor", now),
		costUser(viewerID, "viewer", now), costUser(otherOwnerID, "other-owner", now),
	}
	if err = database.Create(&users).Error; err != nil {
		t.Fatalf("seed cost users: %v", err)
	}
	workspaces := []model.Workspace{
		{ID: workspaceID, Name: "Cost", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherWorkspaceID, Name: "Other Cost", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&workspaces).Error; err != nil {
		t.Fatalf("seed cost workspaces: %v", err)
	}
	memberships := []model.Membership{
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: ownerID, Role: "owner", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: editorID, Role: "editor", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: viewerID, Role: "viewer", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: otherWorkspaceID, UserID: otherOwnerID, Role: "owner", Status: "active", JoinedAt: now},
	}
	if err = database.Create(&memberships).Error; err != nil {
		t.Fatalf("seed cost memberships: %v", err)
	}
	projects := []model.Project{
		{ID: projectID, WorkspaceID: workspaceID, Name: "Cost Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherProjectID, WorkspaceID: otherWorkspaceID, Name: "Other Cost Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&projects).Error; err != nil {
		t.Fatalf("seed cost projects: %v", err)
	}

	service := costapp.NewService(costgorm.New(database), costapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	owner := costapp.Actor{UserID: ownerID.String(), TokenVersion: 1}
	editor := costapp.Actor{UserID: editorID.String(), TokenVersion: 1}
	viewer := costapp.Actor{UserID: viewerID.String(), TokenVersion: 1}
	otherOwner := costapp.Actor{UserID: otherOwnerID.String(), TokenVersion: 1}
	if _, err = service.GetBudget(ctx, viewer, projectID.String()); !costapp.IsCode(err, "not_found") {
		t.Fatalf("unset budget error = %v", err)
	}
	createCommand := costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "100.123456", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "cost-budget-create",
	}
	if _, err = service.SetBudget(ctx, editor, createCommand); !costapp.IsCode(err, "forbidden") {
		t.Fatalf("editor set a project budget: %v", err)
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "1.0000001", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "cost-budget-invalid-scale",
	}); !costapp.IsCode(err, "invalid_request") {
		t.Fatalf("budget accepted more than six decimal places: %v", err)
	}
	created, err := service.SetBudget(ctx, owner, createCommand)
	if err != nil || created.Policy.Revision != 1 || created.Policy.LimitAmount.StringFixed(6) != "100.123456" ||
		created.Policy.Currency != "USD" || created.Receipt.Operation != "cost.budget.set" {
		t.Fatalf("create cost budget: result=%#v err=%v", created, err)
	}
	replayed, err := service.SetBudget(ctx, owner, createCommand)
	if err != nil || replayed.Policy.ID != created.Policy.ID || replayed.Receipt.ID != created.Receipt.ID {
		t.Fatalf("replay cost budget create: result=%#v err=%v", replayed, err)
	}
	driftedCreate := createCommand
	driftedCreate.LimitAmount = "101"
	if _, err = service.SetBudget(ctx, owner, driftedCreate); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("budget idempotency key accepted different input: %v", err)
	}

	updateCommand := costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "250.000001", Currency: "USD",
		ExpectedRevision: 1, IdempotencyKey: "cost-budget-update",
	}
	const callers = 8
	results := make(chan costapp.BudgetResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, updateErr := service.SetBudget(ctx, owner, updateCommand)
			if updateErr != nil {
				errorsFound <- updateErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for updateErr := range errorsFound {
		t.Fatalf("update the same budget concurrently: %v", updateErr)
	}
	var updatedPolicyID, updatedReceiptID string
	for result := range results {
		if updatedPolicyID == "" {
			updatedPolicyID, updatedReceiptID = result.Policy.ID, result.Receipt.ID
		}
		if result.Policy.ID != updatedPolicyID || result.Policy.Revision != 2 ||
			result.Policy.LimitAmount.StringFixed(6) != "250.000001" || result.Receipt.ID != updatedReceiptID {
			t.Fatalf("concurrent cost budget update drifted: %#v", result)
		}
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "300", Currency: "USD",
		ExpectedRevision: 1, IdempotencyKey: "cost-budget-stale",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("stale budget revision update error = %v", err)
	}
	read, err := service.GetBudget(ctx, viewer, projectID.String())
	if err != nil || read.Revision != 2 || read.LimitAmount.StringFixed(6) != "250.000001" {
		t.Fatalf("read current cost budget: policy=%#v err=%v", read, err)
	}
	projectService := projectapp.NewService(projectgorm.New(database), func() time.Time { return now }, uuid.NewString)
	preflight, err := projectService.DeletePreflight(ctx, projectapp.Actor{UserID: ownerID.String(), TokenVersion: 1}, projectID.String())
	if err != nil || preflight.Allowed || len(preflight.Blockers) != 1 || preflight.Blockers[0].Code != "HAS_COST_BUDGET" {
		t.Fatalf("cost budget delete preflight: result=%#v err=%v", preflight, err)
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", viewerID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke cost viewer token: %v", err)
	}
	if _, err = service.GetBudget(ctx, viewer, projectID.String()); !costapp.IsCode(err, "unauthenticated") {
		t.Fatalf("revoked viewer read cost budget: %v", err)
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", viewerID).Update("token_version", 1).Error; err != nil {
		t.Fatalf("restore cost viewer token: %v", err)
	}
	if _, err = service.SetBudget(ctx, otherOwner, costapp.SetBudgetCommand{
		ProjectID: otherProjectID.String(), LimitAmount: "50", Currency: "CNY",
		ExpectedRevision: 0, IdempotencyKey: "other-cost-budget",
	}); err != nil {
		t.Fatalf("create other workspace budget: %v", err)
	}
	if _, err = service.GetBudget(ctx, viewer, otherProjectID.String()); !costapp.IsCode(err, "not_found") {
		t.Fatalf("cross-workspace budget read leaked existence: %v", err)
	}

	if err = database.Model(&model.CostBudgetPolicy{}).Where("id = ?", created.Policy.ID).
		Update("content_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject cost budget hash drift: %v", err)
	}
	if _, err = service.GetBudget(ctx, owner, projectID.String()); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("drifted budget policy passed read gate: %v", err)
	}

	var policyCount, receiptCount int64
	if err = database.Model(&model.CostBudgetPolicy{}).Where("workspace_id = ?", workspaceID).Count(&policyCount).Error; err != nil {
		t.Fatalf("count cost budget policies: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).Where(
		"workspace_id = ? AND operation = ?", workspaceID, "cost.budget.set",
	).Count(&receiptCount).Error; err != nil {
		t.Fatalf("count cost budget receipts: %v", err)
	}
	if policyCount != 1 || receiptCount != 2 {
		t.Fatalf("cost budget fact counts = policies %d receipts %d", policyCount, receiptCount)
	}
}

func costUser(id uuid.UUID, label string, now time.Time) model.UserAccount {
	return model.UserAccount{
		ID: id, EmailNormalized: "cost-" + label + "-" + id.String() + "@example.test",
		PasswordHash: "test", TokenVersion: 1, DisplayName: "Cost " + label,
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}
}
