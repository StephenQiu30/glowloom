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
	"github.com/shopspring/decimal"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestCostReservationAndLedgerPreserveBudget(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the cost reservation journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open cost reservation test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 26, 22, 0, 0, 0, time.UTC)
	currentTime := now
	workspaceID, otherWorkspaceID := uuid.New(), uuid.New()
	projectID, unknownProjectID, raceProjectID, otherProjectID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ownerID, editorID, viewerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	users := []model.UserAccount{
		costUser(ownerID, "reservation-owner", now), costUser(editorID, "reservation-editor", now),
		costUser(viewerID, "reservation-viewer", now), costUser(otherOwnerID, "reservation-other-owner", now),
	}
	if err = database.Create(&users).Error; err != nil {
		t.Fatalf("seed reservation users: %v", err)
	}
	workspaces := []model.Workspace{
		{ID: workspaceID, Name: "Reservation", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherWorkspaceID, Name: "Other Reservation", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&workspaces).Error; err != nil {
		t.Fatalf("seed reservation workspaces: %v", err)
	}
	memberships := []model.Membership{
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: ownerID, Role: "owner", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: editorID, Role: "editor", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: viewerID, Role: "viewer", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: otherWorkspaceID, UserID: otherOwnerID, Role: "owner", Status: "active", JoinedAt: now},
	}
	if err = database.Create(&memberships).Error; err != nil {
		t.Fatalf("seed reservation memberships: %v", err)
	}
	projects := []model.Project{
		{ID: projectID, WorkspaceID: workspaceID, Name: "Reservation Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: unknownProjectID, WorkspaceID: workspaceID, Name: "Unknown Reservation", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: raceProjectID, WorkspaceID: workspaceID, Name: "Transition Race", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		{ID: otherProjectID, WorkspaceID: otherWorkspaceID, Name: "Other Reservation", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&projects).Error; err != nil {
		t.Fatalf("seed reservation projects: %v", err)
	}

	service := costapp.NewService(costgorm.New(database), costapp.Config{
		Now: func() time.Time { return currentTime }, NewID: uuid.NewString,
	})
	owner := costapp.Actor{UserID: ownerID.String(), TokenVersion: 1}
	editor := costapp.Actor{UserID: editorID.String(), TokenVersion: 1}
	viewer := costapp.Actor{UserID: viewerID.String(), TokenVersion: 1}
	otherOwner := costapp.Actor{UserID: otherOwnerID.String(), TokenVersion: 1}

	prepareCostProject(t, ctx, service, owner, projectID.String(), "reservation")
	estimateA := createCostEstimate(t, ctx, service, editor, projectID.String(), uuid.NewString(), 6, "estimate-a")
	estimateB := createCostEstimate(t, ctx, service, editor, projectID.String(), uuid.NewString(), 6, "estimate-b")
	commands := []costapp.ReserveEstimateCommand{
		{EstimateID: estimateA.ID, IdempotencyKey: "reserve-a"},
		{EstimateID: estimateB.ID, IdempotencyKey: "reserve-b"},
	}
	type reserveAttempt struct {
		index  int
		result costapp.ReservationResult
		err    error
	}
	attempts := make(chan reserveAttempt, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for index := range commands {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			result, reserveErr := service.ReserveEstimate(ctx, editor, commands[index])
			attempts <- reserveAttempt{index: index, result: result, err: reserveErr}
		}(index)
	}
	close(start)
	workers.Wait()
	close(attempts)
	var winner reserveAttempt
	loserIndex := -1
	successes, exceeded := 0, 0
	for attempt := range attempts {
		if attempt.err == nil {
			successes++
			winner = attempt
			continue
		}
		if costapp.IsCode(attempt.err, "budget_exceeded") {
			exceeded++
			loserIndex = attempt.index
			continue
		}
		t.Fatalf("unexpected concurrent reservation error: %v", attempt.err)
	}
	if successes != 1 || exceeded != 1 || loserIndex < 0 || winner.result.Reservation.Status != "reserved" ||
		winner.result.Reservation.ReservedAmount.StringFixed(6) != "60.000000" ||
		winner.result.LedgerEntry.EntryType != "reservation_created" ||
		winner.result.Receipt.Operation != "cost.reservation.reserve" {
		t.Fatalf("concurrent budget gate = successes %d exceeded %d winner %#v", successes, exceeded, winner)
	}

	const replayCallers = 8
	replays := make(chan costapp.ReservationResult, replayCallers)
	replayErrors := make(chan error, replayCallers)
	startReplay := make(chan struct{})
	for range replayCallers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-startReplay
			result, replayErr := service.ReserveEstimate(ctx, editor, commands[winner.index])
			if replayErr != nil {
				replayErrors <- replayErr
				return
			}
			replays <- result
		}()
	}
	close(startReplay)
	workers.Wait()
	close(replays)
	close(replayErrors)
	for replayErr := range replayErrors {
		t.Fatalf("replay reserved estimate concurrently: %v", replayErr)
	}
	for replay := range replays {
		if replay.Reservation.ID != winner.result.Reservation.ID || replay.Receipt.ID != winner.result.Receipt.ID {
			t.Fatalf("reservation replay drifted: %#v", replay)
		}
	}

	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "59", Currency: "USD",
		ExpectedRevision: 1, IdempotencyKey: "budget-below-reserved",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("budget lowered below reserved amount: %v", err)
	}
	usageReceiptID := uuid.NewString()
	settleCommand := costapp.SettleReservationCommand{
		ReservationID: winner.result.Reservation.ID, UsageReceiptID: usageReceiptID,
		SettledUnits: 5, IdempotencyKey: "settle-winner",
	}
	settled, err := service.SettleReservation(ctx, editor, settleCommand)
	if err != nil || settled.Reservation.Status != "settled" || settled.Reservation.Revision != 2 ||
		settled.Reservation.SettledUnits != 5 || settled.Reservation.SettledAmount.StringFixed(6) != "50.000000" ||
		settled.LedgerEntry.EntryType != "reservation_settled" ||
		settled.LedgerEntry.ReservedDelta.StringFixed(6) != "-60.000000" ||
		settled.LedgerEntry.SettledDelta.StringFixed(6) != "50.000000" {
		t.Fatalf("settle cost reservation: result=%#v err=%v", settled, err)
	}
	settledReplay, err := service.SettleReservation(ctx, editor, settleCommand)
	if err != nil || settledReplay.Receipt.ID != settled.Receipt.ID || settledReplay.LedgerEntry.ID != settled.LedgerEntry.ID {
		t.Fatalf("replay cost settlement: result=%#v err=%v", settledReplay, err)
	}
	settleWithNewKey := settleCommand
	settleWithNewKey.IdempotencyKey = "settle-winner-new-key"
	settledAgain, err := service.SettleReservation(ctx, editor, settleWithNewKey)
	if err != nil || settledAgain.Reservation.ID != settled.Reservation.ID || settledAgain.LedgerEntry.ID != settled.LedgerEntry.ID {
		t.Fatalf("settle terminal fact with new key: result=%#v err=%v", settledAgain, err)
	}
	driftedSettle := settleWithNewKey
	driftedSettle.SettledUnits = 4
	driftedSettle.IdempotencyKey = "settle-winner-drift"
	if _, err = service.SettleReservation(ctx, editor, driftedSettle); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("settlement accepted different actual units: %v", err)
	}
	if _, err = service.ReleaseReservation(ctx, editor, costapp.ReleaseReservationCommand{
		ReservationID: settled.Reservation.ID, IdempotencyKey: "release-settled",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("settled reservation was released: %v", err)
	}

	if _, err = service.ReserveEstimate(ctx, editor, commands[loserIndex]); !costapp.IsCode(err, "budget_exceeded") {
		t.Fatalf("losing 60-unit estimate bypassed settled budget: %v", err)
	}
	estimateC := createCostEstimate(t, ctx, service, editor, projectID.String(), uuid.NewString(), 5, "estimate-c")
	reservedC, err := service.ReserveEstimate(ctx, editor, costapp.ReserveEstimateCommand{
		EstimateID: estimateC.ID, IdempotencyKey: "reserve-c",
	})
	if err != nil || reservedC.Reservation.ReservedAmount.StringFixed(6) != "50.000000" {
		t.Fatalf("reserve exact remaining budget: result=%#v err=%v", reservedC, err)
	}
	released, err := service.ReleaseReservation(ctx, editor, costapp.ReleaseReservationCommand{
		ReservationID: reservedC.Reservation.ID, IdempotencyKey: "release-c",
	})
	if err != nil || released.Reservation.Status != "released" || released.Reservation.Revision != 2 ||
		released.LedgerEntry.EntryType != "reservation_released" ||
		released.LedgerEntry.ReservedDelta.StringFixed(6) != "-50.000000" || !released.LedgerEntry.SettledDelta.IsZero() {
		t.Fatalf("release cost reservation: result=%#v err=%v", released, err)
	}
	if _, err = service.SettleReservation(ctx, editor, costapp.SettleReservationCommand{
		ReservationID: released.Reservation.ID, UsageReceiptID: uuid.NewString(), SettledUnits: 1,
		IdempotencyKey: "settle-released",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("released reservation was settled: %v", err)
	}

	lowered, err := service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "50", Currency: "USD",
		ExpectedRevision: 1, IdempotencyKey: "budget-to-settled",
	})
	if err != nil || lowered.Policy.Revision != 2 {
		t.Fatalf("lower budget to settled amount: result=%#v err=%v", lowered, err)
	}
	if _, err = service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID.String(), LimitAmount: "49.999999", Currency: "USD",
		ExpectedRevision: 2, IdempotencyKey: "budget-below-settled",
	}); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("budget lowered below settled amount: %v", err)
	}
	estimateD := createCostEstimate(t, ctx, service, editor, projectID.String(), uuid.NewString(), 1, "estimate-d")
	if _, err = service.ReserveEstimate(ctx, editor, costapp.ReserveEstimateCommand{
		EstimateID: estimateD.ID, IdempotencyKey: "reserve-over-settled-budget",
	}); !costapp.IsCode(err, "budget_exceeded") {
		t.Fatalf("reservation exceeded fully settled budget: %v", err)
	}

	prepareCostProject(t, ctx, service, owner, unknownProjectID.String(), "unknown")
	unknownEstimate := createCostEstimate(t, ctx, service, editor, unknownProjectID.String(), uuid.NewString(), 1, "unknown-estimate")
	unknownReservation, err := service.ReserveEstimate(ctx, editor, costapp.ReserveEstimateCommand{
		EstimateID: unknownEstimate.ID, IdempotencyKey: "unknown-reserve",
	})
	if err != nil {
		t.Fatalf("reserve unknown-result operation: %v", err)
	}
	currentTime = currentTime.Add(72 * time.Hour)
	unknownView, err := service.GetReservation(ctx, viewer, unknownReservation.Reservation.ID)
	if err != nil || unknownView.Reservation.Status != "reserved" || len(unknownView.LedgerEntries) != 1 {
		t.Fatalf("unknown result reservation was released automatically: view=%#v err=%v", unknownView, err)
	}

	prepareCostProject(t, ctx, service, owner, raceProjectID.String(), "transition-race")
	raceEstimate := createCostEstimate(t, ctx, service, editor, raceProjectID.String(), uuid.NewString(), 1, "race-estimate")
	raceReservation, err := service.ReserveEstimate(ctx, editor, costapp.ReserveEstimateCommand{
		EstimateID: raceEstimate.ID, IdempotencyKey: "race-reserve",
	})
	if err != nil {
		t.Fatalf("reserve transition race cost: %v", err)
	}
	type transitionAttempt struct {
		status string
		err    error
	}
	transitionAttempts := make(chan transitionAttempt, 2)
	startTransition := make(chan struct{})
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-startTransition
		result, transitionErr := service.SettleReservation(ctx, editor, costapp.SettleReservationCommand{
			ReservationID: raceReservation.Reservation.ID, UsageReceiptID: uuid.NewString(), SettledUnits: 1,
			IdempotencyKey: "race-settle",
		})
		transitionAttempts <- transitionAttempt{status: result.Reservation.Status, err: transitionErr}
	}()
	go func() {
		defer workers.Done()
		<-startTransition
		result, transitionErr := service.ReleaseReservation(ctx, editor, costapp.ReleaseReservationCommand{
			ReservationID: raceReservation.Reservation.ID, IdempotencyKey: "race-release",
		})
		transitionAttempts <- transitionAttempt{status: result.Reservation.Status, err: transitionErr}
	}()
	close(startTransition)
	workers.Wait()
	close(transitionAttempts)
	transitionSuccesses, transitionConflicts := 0, 0
	var terminalStatus string
	for attempt := range transitionAttempts {
		if attempt.err == nil {
			transitionSuccesses++
			terminalStatus = attempt.status
			continue
		}
		if costapp.IsCode(attempt.err, "state_conflict") {
			transitionConflicts++
			continue
		}
		t.Fatalf("unexpected settle/release race error: %v", attempt.err)
	}
	raceView, err := service.GetReservation(ctx, viewer, raceReservation.Reservation.ID)
	if err != nil || transitionSuccesses != 1 || transitionConflicts != 1 ||
		raceView.Reservation.Status != terminalStatus || len(raceView.LedgerEntries) != 2 {
		t.Fatalf("settle/release race = successes %d conflicts %d status %q view=%#v err=%v",
			transitionSuccesses, transitionConflicts, terminalStatus, raceView, err)
	}

	view, err := service.GetReservation(ctx, viewer, settled.Reservation.ID)
	if err != nil || view.Reservation.Status != "settled" || len(view.LedgerEntries) != 2 {
		t.Fatalf("read settled reservation ledger: view=%#v err=%v", view, err)
	}
	if _, err = service.GetReservation(ctx, otherOwner, settled.Reservation.ID); !costapp.IsCode(err, "not_found") {
		t.Fatalf("cross-workspace reservation read leaked existence: %v", err)
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", viewerID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke reservation viewer token: %v", err)
	}
	if _, err = service.GetReservation(ctx, viewer, settled.Reservation.ID); !costapp.IsCode(err, "unauthenticated") {
		t.Fatalf("revoked viewer read reservation ledger: %v", err)
	}

	var reservations []model.CostReservation
	if err = database.Where("project_id = ?", projectID).Find(&reservations).Error; err != nil {
		t.Fatalf("load project cost reservations: %v", err)
	}
	var entries []model.CostLedgerEntry
	if err = database.Where("project_id = ?", projectID).Find(&entries).Error; err != nil {
		t.Fatalf("load project cost ledger: %v", err)
	}
	reservedDelta, settledDelta := decimal.Zero, decimal.Zero
	for _, entry := range entries {
		reservedDelta = reservedDelta.Add(entry.ReservedDelta)
		settledDelta = settledDelta.Add(entry.SettledDelta)
	}
	if len(reservations) != 2 || len(entries) != 4 || !reservedDelta.IsZero() ||
		settledDelta.StringFixed(6) != "50.000000" {
		t.Fatalf("cost ledger reconciliation = reservations %d entries %d reserved %s settled %s",
			len(reservations), len(entries), reservedDelta.StringFixed(6), settledDelta.StringFixed(6))
	}
	if err = database.Model(&model.CostLedgerEntry{}).Where(
		"reservation_id = ? AND entry_type = ?", released.Reservation.ID, "reservation_released",
	).Update("content_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject cost ledger hash drift: %v", err)
	}
	if _, err = service.GetReservation(ctx, owner, released.Reservation.ID); !costapp.IsCode(err, "state_conflict") {
		t.Fatalf("drifted cost ledger passed read gate: %v", err)
	}
}

func prepareCostProject(
	t *testing.T,
	ctx context.Context,
	service *costapp.Service,
	owner costapp.Actor,
	projectID, keyPrefix string,
) {
	t.Helper()
	if _, err := service.SetBudget(ctx, owner, costapp.SetBudgetCommand{
		ProjectID: projectID, LimitAmount: "100", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: keyPrefix + "-budget",
	}); err != nil {
		t.Fatalf("create %s cost budget: %v", keyPrefix, err)
	}
	if _, err := service.SetPriceQuote(ctx, owner, costapp.SetPriceQuoteCommand{
		ProjectID: projectID, Metric: "generation.image", UnitAmount: "10", Currency: "USD",
		ExpectedRevision: 0, IdempotencyKey: keyPrefix + "-price",
	}); err != nil {
		t.Fatalf("create %s image price: %v", keyPrefix, err)
	}
}

func createCostEstimate(
	t *testing.T,
	ctx context.Context,
	service *costapp.Service,
	actor costapp.Actor,
	projectID, sourceID string,
	units int64,
	key string,
) costdomain.Estimate {
	t.Helper()
	result, err := service.CreateEstimate(ctx, actor, costapp.CreateEstimateCommand{
		ProjectID: projectID, Metric: "generation.image", SourceType: "generation_intent",
		SourceID: sourceID, Units: units, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("create cost estimate %s: %v", key, err)
	}
	return result.Estimate
}
