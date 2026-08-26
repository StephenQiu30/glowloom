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
)

func TestImagePriceAndEstimateFreezeServerFacts(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the cost price and estimate journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open cost pricing test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 26, 20, 0, 0, 0, time.UTC)
	workspaceID, otherWorkspaceID := uuid.New(), uuid.New()
	projectID, otherProjectID := uuid.New(), uuid.New()
	ownerID, editorID, viewerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	users := []model.UserAccount{
		costUser(ownerID, "price-owner", now), costUser(editorID, "price-editor", now),
		costUser(viewerID, "price-viewer", now), costUser(otherOwnerID, "price-other-owner", now),
	}
	if err = database.Create(&users).Error; err != nil {
		t.Fatalf("seed pricing users: %v", err)
	}
	workspaces := []model.Workspace{
		{ID: workspaceID, Name: "Pricing", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherWorkspaceID, Name: "Other Pricing", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&workspaces).Error; err != nil {
		t.Fatalf("seed pricing workspaces: %v", err)
	}
	memberships := []model.Membership{
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: ownerID, Role: "owner", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: editorID, Role: "editor", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: viewerID, Role: "viewer", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: otherWorkspaceID, UserID: otherOwnerID, Role: "owner", Status: "active", JoinedAt: now},
	}
	if err = database.Create(&memberships).Error; err != nil {
		t.Fatalf("seed pricing memberships: %v", err)
	}
	projects := []model.Project{
		{ID: projectID, WorkspaceID: workspaceID, Name: "Pricing Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherProjectID, WorkspaceID: otherWorkspaceID, Name: "Other Pricing Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&projects).Error; err != nil {
		t.Fatalf("seed pricing projects: %v", err)
	}

	service := costapp.NewService(costgorm.New(database), costapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	owner := costapp.Actor{UserID: ownerID.String(), TokenVersion: 1}
	editor := costapp.Actor{UserID: editorID.String(), TokenVersion: 1}
	viewer := costapp.Actor{UserID: viewerID.String(), TokenVersion: 1}
	otherOwner := costapp.Actor{UserID: otherOwnerID.String(), TokenVersion: 1}

	priceCommand := costapp.SetPriceQuoteCommand{
		ProjectID: projectID.String(), Metric: "generation.image", UnitAmount: "0.125", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "price-create",
	}
	if _, err = service.SetPriceQuote(ctx, owner, priceCommand); !costapp.IsCode(err, "not_found") {
		t.Fatalf("price without budget error = %v", err)
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "100", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "price-budget-create",
	}); err != nil {
		t.Fatalf("create pricing budget: %v", err)
	}
	if _, err = service.SetPriceQuote(ctx, editor, priceCommand); !costapp.IsCode(err, "forbidden") {
		t.Fatalf("editor set price quote: %v", err)
	}
	driftedCurrency := priceCommand
	driftedCurrency.Currency = "CNY"
	driftedCurrency.IdempotencyKey = "price-currency-drift"
	if _, err = service.SetPriceQuote(ctx, owner, driftedCurrency); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("price quote accepted budget currency drift: %v", err)
	}
	createdPrice, err := service.SetPriceQuote(ctx, owner, priceCommand)
	if err != nil || createdPrice.Quote.Revision != 1 || createdPrice.Quote.UnitAmount.StringFixed(6) != "0.125000" ||
		createdPrice.Quote.Currency != "USD" || createdPrice.Receipt.Operation != "cost.price_quote.set" {
		t.Fatalf("create image price quote: result=%#v err=%v", createdPrice, err)
	}
	replayedPrice, err := service.SetPriceQuote(ctx, owner, priceCommand)
	if err != nil || replayedPrice.Quote.ID != createdPrice.Quote.ID || replayedPrice.Receipt.ID != createdPrice.Receipt.ID {
		t.Fatalf("replay image price quote: result=%#v err=%v", replayedPrice, err)
	}
	driftedPriceCommand := priceCommand
	driftedPriceCommand.UnitAmount = "0.126"
	if _, err = service.SetPriceQuote(ctx, owner, driftedPriceCommand); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("price idempotency key accepted different amount: %v", err)
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "100", Currency: "CNY",
		ExpectedRevision: 1, IdempotencyKey: "price-budget-currency-change",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("budget currency changed after price quote existed: %v", err)
	}
	secondPriceCommand := priceCommand
	secondPriceCommand.UnitAmount = "0.250000"
	secondPriceCommand.ExpectedRevision = 1
	secondPriceCommand.IdempotencyKey = "price-update"
	secondPrice, err := service.SetPriceQuote(ctx, owner, secondPriceCommand)
	if err != nil || secondPrice.Quote.ID == createdPrice.Quote.ID || secondPrice.Quote.Revision != 2 ||
		secondPrice.Quote.UnitAmount.StringFixed(6) != "0.250000" {
		t.Fatalf("append image price quote: result=%#v err=%v", secondPrice, err)
	}
	stalePrice := secondPriceCommand
	stalePrice.UnitAmount = "0.300000"
	stalePrice.IdempotencyKey = "price-stale"
	if _, err = service.SetPriceQuote(ctx, owner, stalePrice); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("stale price revision error = %v", err)
	}
	currentPrice, err := service.GetCurrentPriceQuote(ctx, viewer, projectID.String(), "generation.image")
	if err != nil || currentPrice.ID != secondPrice.Quote.ID || currentPrice.Revision != 2 {
		t.Fatalf("read current image price: quote=%#v err=%v", currentPrice, err)
	}
	updatedBudget, err := service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "120", Currency: "USD",
		ExpectedRevision: 1, IdempotencyKey: "price-budget-limit-update",
	})
	if err != nil || updatedBudget.Policy.Revision != 2 {
		t.Fatalf("update pricing budget limit: result=%#v err=%v", updatedBudget, err)
	}

	sourceID := uuid.NewString()
	estimateCommand := costapp.CreateEstimateCommand{
		ProjectID: projectID.String(), Metric: "generation.image", SourceType: "generation_intent",
		SourceID: sourceID, Units: 3, IdempotencyKey: "estimate-create",
	}
	viewerEstimate := estimateCommand
	viewerEstimate.SourceID = uuid.NewString()
	viewerEstimate.IdempotencyKey = "estimate-viewer-forbidden"
	if _, err = service.CreateEstimate(ctx, viewer, viewerEstimate); !costapp.IsCode(err, "forbidden") {
		t.Fatalf("viewer created image estimate: %v", err)
	}
	const callers = 8
	results := make(chan costapp.EstimateResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, estimateErr := service.CreateEstimate(ctx, editor, estimateCommand)
			if estimateErr != nil {
				errorsFound <- estimateErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for estimateErr := range errorsFound {
		t.Fatalf("create the same estimate concurrently: %v", estimateErr)
	}
	var estimateID, estimateReceiptID string
	for result := range results {
		if estimateID == "" {
			estimateID, estimateReceiptID = result.Estimate.ID, result.Receipt.ID
		}
		if result.Estimate.ID != estimateID || result.Receipt.ID != estimateReceiptID ||
			result.Estimate.PriceQuoteID != secondPrice.Quote.ID || result.Estimate.PriceQuoteRevision != 2 ||
			result.Estimate.BudgetPolicyID == "" || result.Estimate.BudgetPolicyRevision != 2 ||
			result.Estimate.BudgetLimit.StringFixed(6) != "120.000000" ||
			result.Estimate.UnitAmount.StringFixed(6) != "0.250000" ||
			result.Estimate.TotalAmount.StringFixed(6) != "0.750000" || result.Estimate.Currency != "USD" {
			t.Fatalf("concurrent image estimate drifted: %#v", result)
		}
	}
	if estimateID == "" {
		t.Fatal("concurrent image estimate returned no result")
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "200", Currency: "USD",
		ExpectedRevision: 2, IdempotencyKey: "price-budget-after-estimate",
	}); err != nil {
		t.Fatalf("update budget after estimate: %v", err)
	}

	thirdPriceCommand := secondPriceCommand
	thirdPriceCommand.UnitAmount = "0.500000"
	thirdPriceCommand.ExpectedRevision = 2
	thirdPriceCommand.IdempotencyKey = "price-update-after-estimate"
	thirdPrice, err := service.SetPriceQuote(ctx, owner, thirdPriceCommand)
	if err != nil {
		t.Fatalf("append price after estimate: %v", err)
	}
	replayBySource := estimateCommand
	replayBySource.IdempotencyKey = "estimate-same-source-new-command"
	frozen, err := service.CreateEstimate(ctx, editor, replayBySource)
	if err != nil || frozen.Estimate.ID != estimateID || frozen.Estimate.PriceQuoteRevision != 2 ||
		frozen.Estimate.BudgetPolicyRevision != 2 || frozen.Estimate.BudgetLimit.StringFixed(6) != "120.000000" ||
		frozen.Estimate.TotalAmount.StringFixed(6) != "0.750000" {
		t.Fatalf("estimate did not preserve first source snapshot: result=%#v err=%v", frozen, err)
	}
	driftedEstimate := replayBySource
	driftedEstimate.Units = 4
	driftedEstimate.IdempotencyKey = "estimate-source-drift"
	if _, err = service.CreateEstimate(ctx, editor, driftedEstimate); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("estimate source accepted different units: %v", err)
	}
	driftedEstimate.IdempotencyKey = estimateCommand.IdempotencyKey
	if _, err = service.CreateEstimate(ctx, editor, driftedEstimate); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("estimate idempotency key accepted different units: %v", err)
	}
	readEstimate, err := service.GetEstimate(ctx, viewer, estimateID)
	if err != nil || readEstimate.ID != estimateID || readEstimate.TotalAmount.StringFixed(6) != "0.750000" {
		t.Fatalf("read frozen image estimate: estimate=%#v err=%v", readEstimate, err)
	}
	if _, err = service.GetEstimate(ctx, otherOwner, estimateID); !costapp.IsCode(err, "not_found") {
		t.Fatalf("cross-workspace estimate read leaked existence: %v", err)
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", viewerID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke pricing viewer token: %v", err)
	}
	if _, err = service.GetCurrentPriceQuote(ctx, viewer, projectID.String(), "generation.image"); !costapp.IsCode(err, "unauthenticated") {
		t.Fatalf("revoked viewer read current price: %v", err)
	}
	if err = database.Model(&model.CostEstimate{}).Where("id = ?", estimateID).
		Update("content_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject estimate content hash drift: %v", err)
	}
	if _, err = service.GetEstimate(ctx, owner, estimateID); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("drifted estimate passed read gate: %v", err)
	}
	if err = database.Model(&model.CostPriceQuote{}).Where("id = ?", thirdPrice.Quote.ID).
		Update("content_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject current price content hash drift: %v", err)
	}
	if _, err = service.GetCurrentPriceQuote(ctx, owner, projectID.String(), "generation.image"); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("drifted current price quote passed read gate: %v", err)
	}

	var quoteCount, estimateCount int64
	if err = database.Model(&model.CostPriceQuote{}).Where("project_id = ?", projectID).Count(&quoteCount).Error; err != nil {
		t.Fatalf("count price quotes: %v", err)
	}
	if err = database.Model(&model.CostEstimate{}).Where("project_id = ?", projectID).Count(&estimateCount).Error; err != nil {
		t.Fatalf("count cost estimates: %v", err)
	}
	if quoteCount != 3 || estimateCount != 1 {
		t.Fatalf("pricing fact counts = quotes %d estimates %d", quoteCount, estimateCount)
	}
}
