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
	costgormtest "github.com/StephenQiu30/lanverse/backend/tests/cost/adapter/gormdb"
	testgorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
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
	projectID, videoProjectID, otherProjectID := uuid.New(), uuid.New(), uuid.New()
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
		{ID: videoProjectID, WorkspaceID: workspaceID, Name: "Video Pricing Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherProjectID, WorkspaceID: otherWorkspaceID, Name: "Other Pricing Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&projects).Error; err != nil {
		t.Fatalf("seed pricing projects: %v", err)
	}
	testgorm.RegisterOwnedWorkspaceFixtureCleanup(t, database, testgorm.OwnedWorkspaceFixture{
		UserIDs:     []string{ownerID.String(), editorID.String(), viewerID.String()},
		WorkspaceID: workspaceID.String(),
	})
	testgorm.RegisterOwnedWorkspaceFixtureCleanup(t, database, testgorm.OwnedWorkspaceFixture{
		UserIDs:     []string{otherOwnerID.String()},
		WorkspaceID: otherWorkspaceID.String(),
	})
	providerFacts := costgormtest.SeedProviderFacts(
		t, database, workspaceID, projectID, ownerID, now, "pricing",
	)
	crossProjectProviderFacts := costgormtest.SeedProviderFacts(
		t, database, workspaceID, videoProjectID, ownerID, now, "pricing-cross-project",
	)
	crossWorkspaceProviderFacts := costgormtest.SeedProviderFacts(
		t, database, otherWorkspaceID, otherProjectID, otherOwnerID, now, "pricing-cross-workspace",
	)
	profileID, bindingID := providerFacts.ProfileID, providerFacts.BindingID
	profileHash, bindingHash := providerFacts.ProfileHash, providerFacts.BindingHash
	videoProfileID := uuid.New()
	if err = database.Create(&model.ProviderModelProfileVersion{
		ID: videoProfileID, WorkspaceID: workspaceID, ProfileKey: "video-pricing", Revision: 1,
		CreationSource: []byte(`{}`), ConnectionKey: "video-pricing", ProviderKey: "controlled",
		ExternalModelID: "controlled-video", Modality: "video", Family: "controlled",
		AdapterTransportContract: "controlled", CapabilitySchemaVersion: "controlled",
		BillingMetric: "generation.video.call", Defaults: []byte(`{}`), State: "enabled",
		ContentHash: strings.Repeat("6", 64), CreatedBy: ownerID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed exact video profile: %v", err)
	}
	crossWorkspaceProfileID := uuid.New()
	if err = database.Create(&model.ProviderModelProfileVersion{
		ID: crossWorkspaceProfileID, WorkspaceID: otherWorkspaceID, ProfileKey: "cross-workspace-pricing", Revision: 1,
		CreationSource: []byte(`{}`), ConnectionKey: "cross-workspace-pricing", ProviderKey: "controlled",
		ExternalModelID: "controlled-image", Modality: "image", Family: "controlled",
		AdapterTransportContract: "controlled", CapabilitySchemaVersion: "controlled",
		BillingMetric: "generation.image.call", Defaults: []byte(`{}`), State: "enabled",
		ContentHash: strings.Repeat("7", 64), CreatedBy: otherOwnerID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed cross-workspace profile: %v", err)
	}

	service := costapp.NewService(costgorm.New(database), costapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	owner := costapp.Actor{UserID: ownerID.String(), TokenVersion: 1}
	editor := costapp.Actor{UserID: editorID.String(), TokenVersion: 1}
	viewer := costapp.Actor{UserID: viewerID.String(), TokenVersion: 1}
	otherOwner := costapp.Actor{UserID: otherOwnerID.String(), TokenVersion: 1}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: videoProjectID.String(), LimitAmount: "50", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: "video-budget-create",
	}); err != nil {
		t.Fatalf("create video-only budget: %v", err)
	}
	videoQuote, videoErr := service.SetPriceQuote(ctx, owner, costapp.SetPriceQuoteCommand{
		ProjectID: videoProjectID.String(), ModelProfileVersionID: videoProfileID.String(),
		ReservationUnitAmount: "2.5", Currency: "USD", ExpectedRevision: 0, IdempotencyKey: "video-price-create",
	})
	if videoErr != nil || videoQuote.Quote.BillingMetric != "generation.video.call" {
		t.Fatalf("create exact video price: result=%#v err=%v", videoQuote, videoErr)
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: videoProjectID.String(), LimitAmount: "50", Currency: "CNY",
		ExpectedRevision: 1, IdempotencyKey: "video-budget-currency-change",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("video-only quote did not freeze budget currency: %v", err)
	}

	priceCommand := costapp.SetPriceQuoteCommand{
		ProjectID: projectID.String(), ModelProfileVersionID: profileID.String(),
		ReservationUnitAmount: "0.125", Currency: "USD",
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
	crossWorkspacePrice := priceCommand
	crossWorkspacePrice.ModelProfileVersionID = crossWorkspaceProfileID.String()
	crossWorkspacePrice.IdempotencyKey = "price-cross-workspace"
	if _, err = service.SetPriceQuote(ctx, owner, crossWorkspacePrice); !costapp.IsCode(err, "not_found") {
		t.Fatalf("cross-workspace price quote leaked profile existence: %v", err)
	}
	if _, err = service.GetCurrentPriceQuote(ctx, viewer, projectID.String(), crossWorkspaceProfileID.String()); !costapp.IsCode(err, "not_found") {
		t.Fatalf("cross-workspace current price leaked profile existence: %v", err)
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
	if err != nil || createdPrice.Quote.Revision != 1 || createdPrice.Quote.ReservationUnitAmount.StringFixed(6) != "0.125000" ||
		createdPrice.Quote.ModelProfileVersionID != profileID.String() || createdPrice.Quote.BillingMetric != "generation.image.call" ||
		createdPrice.Quote.Currency != "USD" || createdPrice.Receipt.Operation != "cost.price_quote.set" {
		t.Fatalf("create image price quote: result=%#v err=%v", createdPrice, err)
	}
	replayedPrice, err := service.SetPriceQuote(ctx, owner, priceCommand)
	if err != nil || replayedPrice.Quote.ID != createdPrice.Quote.ID || replayedPrice.Receipt.ID != createdPrice.Receipt.ID {
		t.Fatalf("replay image price quote: result=%#v err=%v", replayedPrice, err)
	}
	driftedPriceCommand := priceCommand
	driftedPriceCommand.ReservationUnitAmount = "0.126"
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
	secondPriceCommand.ReservationUnitAmount = "0.250000"
	secondPriceCommand.ExpectedRevision = 1
	secondPriceCommand.IdempotencyKey = "price-update"
	secondPrice, err := service.SetPriceQuote(ctx, owner, secondPriceCommand)
	if err != nil || secondPrice.Quote.ID == createdPrice.Quote.ID || secondPrice.Quote.Revision != 2 ||
		secondPrice.Quote.ReservationUnitAmount.StringFixed(6) != "0.250000" {
		t.Fatalf("append image price quote: result=%#v err=%v", secondPrice, err)
	}
	stalePrice := secondPriceCommand
	stalePrice.ReservationUnitAmount = "0.300000"
	stalePrice.IdempotencyKey = "price-stale"
	if _, err = service.SetPriceQuote(ctx, owner, stalePrice); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("stale price revision error = %v", err)
	}
	currentPrice, err := service.GetCurrentPriceQuote(ctx, viewer, projectID.String(), profileID.String())
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
		ProjectID: projectID.String(), ProviderBindingVersionID: bindingID.String(),
		ProviderBindingRevision: 1, ProviderBindingContentHash: bindingHash,
		ModelProfileVersionID: profileID.String(), ModelProfileRevision: 1, ModelProfileContentHash: profileHash,
		PriceQuoteID: secondPrice.Quote.ID, PriceQuoteRevision: 2, PriceQuoteContentHash: secondPrice.Quote.ContentHash,
		Metric: "generation.image.call", SourceType: "generation_intent", SourceID: sourceID,
		Units: 3, IdempotencyKey: "estimate-create",
	}
	concealedBindings := []struct {
		name, bindingID, bindingHash string
	}{
		{name: "cross-project", bindingID: crossProjectProviderFacts.BindingID.String(), bindingHash: crossProjectProviderFacts.BindingHash},
		{name: "cross-workspace", bindingID: crossWorkspaceProviderFacts.BindingID.String(), bindingHash: crossWorkspaceProviderFacts.BindingHash},
		{name: "missing", bindingID: uuid.NewString(), bindingHash: bindingHash},
	}
	for _, concealed := range concealedBindings {
		command := estimateCommand
		command.ProviderBindingVersionID = concealed.bindingID
		command.ProviderBindingContentHash = concealed.bindingHash
		command.SourceID = uuid.NewString()
		command.IdempotencyKey = "estimate-concealed-binding-" + concealed.name
		if _, createErr := service.CreateEstimate(ctx, editor, command); !costapp.IsCode(createErr, "not_found") {
			t.Fatalf("%s Provider binding existence was not concealed: %v", concealed.name, createErr)
		}
	}
	var concealedEstimateCount, concealedReceiptCount int64
	if err = database.Model(&model.CostEstimate{}).Where("project_id = ?", projectID).Count(&concealedEstimateCount).Error; err != nil {
		t.Fatalf("count estimates after concealed binding requests: %v", err)
	}
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ?", workspaceID, "cost.estimate.create").
		Count(&concealedReceiptCount).Error; err != nil {
		t.Fatalf("count receipts after concealed binding requests: %v", err)
	}
	if concealedEstimateCount != 0 || concealedReceiptCount != 0 {
		t.Fatalf("concealed binding requests wrote estimates=%d receipts=%d", concealedEstimateCount, concealedReceiptCount)
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
	thirdPriceCommand.ReservationUnitAmount = "0.500000"
	thirdPriceCommand.ExpectedRevision = 2
	thirdPriceCommand.IdempotencyKey = "price-update-after-estimate"
	thirdPrice, err := service.SetPriceQuote(ctx, owner, thirdPriceCommand)
	if err != nil {
		t.Fatalf("append price after estimate: %v", err)
	}
	historicalPriceForNewSource := estimateCommand
	historicalPriceForNewSource.SourceID = uuid.NewString()
	historicalPriceForNewSource.IdempotencyKey = "estimate-historical-price-new-source"
	if _, err = service.CreateEstimate(ctx, editor, historicalPriceForNewSource); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("new source accepted a historical price quote: %v", err)
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
	if _, err = service.GetCurrentPriceQuote(ctx, viewer, projectID.String(), profileID.String()); !costapp.IsCode(err, "unauthenticated") {
		t.Fatalf("revoked viewer read current price: %v", err)
	}
	costgormtest.AssertEstimateImmutable(t, database, estimateID)
	costgormtest.AssertPriceQuoteImmutable(t, database, thirdPrice.Quote.ID)

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
