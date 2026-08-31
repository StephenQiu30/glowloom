package quota_test

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	testgorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
)

func TestQuotaUsesProviderCallBillingMetrics(t *testing.T) {
	if !quotadomain.IsGenerationMetric(quotadomain.MetricGenerationImageCall) ||
		!quotadomain.IsGenerationMetric(quotadomain.MetricGenerationVideoCall) ||
		quotadomain.IsGenerationMetric("generation.image") {
		t.Fatal("quota accepted a legacy or unsupported generation metric")
	}
}

func TestDailyImageQuotaReservationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the daily quota journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open quota test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 26, 16, 30, 0, 0, time.UTC)
	workspaceID, projectID := uuid.New(), uuid.New()
	ownerID, editorID, viewerID := uuid.New(), uuid.New(), uuid.New()
	users := []model.UserAccount{
		{ID: ownerID, EmailNormalized: "quota-owner-" + ownerID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Quota Owner", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: editorID, EmailNormalized: "quota-editor-" + editorID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Quota Editor", Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: viewerID, EmailNormalized: "quota-viewer-" + viewerID.String() + "@example.test", PasswordHash: "test", TokenVersion: 1, DisplayName: "Quota Viewer", Status: "active", CreatedAt: now, UpdatedAt: now},
	}
	if err = database.Create(&users).Error; err != nil {
		t.Fatalf("seed quota users: %v", err)
	}
	if err = database.Create(&model.Workspace{ID: workspaceID, Name: "Quota", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed quota workspace: %v", err)
	}
	memberships := []model.Membership{
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: ownerID, Role: "owner", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: editorID, Role: "editor", Status: "active", JoinedAt: now},
		{ID: uuid.New(), WorkspaceID: workspaceID, UserID: viewerID, Role: "viewer", Status: "active", JoinedAt: now},
	}
	if err = database.Create(&memberships).Error; err != nil {
		t.Fatalf("seed quota memberships: %v", err)
	}
	if err = database.Create(&model.Project{
		ID: projectID, WorkspaceID: workspaceID, Name: "Quota Project", AspectRatio: "9:16", Language: "zh-CN",
		TargetDurationMS: 60000, Status: "active", Revision: 1,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed quota project: %v", err)
	}
	testgorm.RegisterOwnedWorkspaceFixtureCleanup(t, database, testgorm.OwnedWorkspaceFixture{
		UserIDs:     []string{ownerID.String(), editorID.String(), viewerID.String()},
		WorkspaceID: workspaceID.String(),
	})

	service := quotaapp.NewService(quotagorm.New(database), quotaapp.Config{
		Now: func() time.Time { return now }, NewID: uuid.NewString,
	})
	owner := quotaapp.Actor{UserID: ownerID.String(), TokenVersion: 1}
	editor := quotaapp.Actor{UserID: editorID.String(), TokenVersion: 1}
	viewer := quotaapp.Actor{UserID: viewerID.String(), TokenVersion: 1}
	setCommand := quotaapp.SetDailyPolicyCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		LimitUnits: 5, ExpectedRevision: 0, IdempotencyKey: "quota-policy-create",
	}
	if _, err = service.SetDailyPolicy(ctx, editor, setCommand); err == nil {
		t.Fatal("editor set a Workspace quota policy")
	}
	created, err := service.SetDailyPolicy(ctx, owner, setCommand)
	if err != nil || created.Policy.Revision != 1 || created.Policy.LimitUnits != 5 ||
		created.Policy.WindowKind != quotadomain.WindowUTCDay || created.Receipt.Operation != "quota.policy.set_daily" {
		t.Fatalf("create daily quota policy: result=%#v err=%v", created, err)
	}
	replayedPolicy, err := service.SetDailyPolicy(ctx, owner, setCommand)
	if err != nil || replayedPolicy.Policy != created.Policy || replayedPolicy.Receipt.ID != created.Receipt.ID {
		t.Fatalf("replay daily quota policy: result=%#v err=%v", replayedPolicy, err)
	}
	driftedPolicyCommand := setCommand
	driftedPolicyCommand.LimitUnits = 6
	if _, err = service.SetDailyPolicy(ctx, owner, driftedPolicyCommand); err == nil {
		t.Fatal("quota policy idempotency key accepted a different limit")
	}

	firstSourceID := uuid.NewString()
	firstCommand := quotaapp.ReserveCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		SourceType: "generation_intent", SourceID: firstSourceID, Units: 3, IdempotencyKey: "quota-reserve-first",
	}
	const callers = 8
	results := make(chan quotaapp.ReservationResult, callers)
	errorsFound := make(chan error, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, reserveErr := service.Reserve(ctx, editor, firstCommand)
			if reserveErr != nil {
				errorsFound <- reserveErr
				return
			}
			results <- result
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for reserveErr := range errorsFound {
		t.Fatalf("reserve the same quota source concurrently: %T %v", reserveErr, reserveErr)
	}
	var firstReservationID, firstReceiptID string
	for result := range results {
		if firstReservationID == "" {
			firstReservationID, firstReceiptID = result.Reservation.ID, result.Receipt.ID
		}
		if result.Reservation.ID != firstReservationID || result.Receipt.ID != firstReceiptID ||
			result.Reservation.Status != quotadomain.ReservationReserved || result.Reservation.Units != 3 ||
			result.Reservation.PolicyRevision != 1 || result.Reservation.LimitUnits != 5 {
			t.Fatalf("concurrent quota reservation drifted: %#v", result)
		}
	}

	secondSourceID := uuid.NewString()
	secondCommand := quotaapp.ReserveCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		SourceType: "generation_intent", SourceID: secondSourceID, Units: 3, IdempotencyKey: "quota-reserve-second-too-large",
	}
	if _, err = service.Reserve(ctx, editor, secondCommand); !quotaapp.IsCode(err, "quota_exceeded") {
		t.Fatalf("over-limit quota reserve error = %v", err)
	}
	secondCommand.Units = 2
	secondCommand.IdempotencyKey = "quota-reserve-second"
	second, err := service.Reserve(ctx, editor, secondCommand)
	if err != nil || second.Reservation.Status != quotadomain.ReservationReserved {
		t.Fatalf("reserve remaining daily quota: result=%#v err=%v", second, err)
	}
	redeliveredSecond, err := service.Reserve(ctx, editor, quotaapp.ReserveCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		SourceType: "generation_intent", SourceID: secondSourceID, Units: 2, IdempotencyKey: "quota-reserve-second-redelivery",
	})
	if err != nil || redeliveredSecond.Reservation.ID != second.Reservation.ID || redeliveredSecond.Receipt.ID == second.Receipt.ID {
		t.Fatalf("redeliver the same quota source: result=%#v err=%v", redeliveredSecond, err)
	}
	if _, err = service.Reserve(ctx, editor, quotaapp.ReserveCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		SourceType: "generation_intent", SourceID: secondSourceID, Units: 1, IdempotencyKey: "quota-reserve-second-drift",
	}); err == nil {
		t.Fatal("same quota source accepted different units")
	}

	if _, err = service.SetDailyPolicy(ctx, owner, quotaapp.SetDailyPolicyCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		LimitUnits: 4, ExpectedRevision: 1, IdempotencyKey: "quota-policy-below-usage",
	}); err == nil {
		t.Fatal("quota policy limit was lowered below current usage")
	}
	updated, err := service.SetDailyPolicy(ctx, owner, quotaapp.SetDailyPolicyCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		LimitUnits: 6, ExpectedRevision: 1, IdempotencyKey: "quota-policy-update",
	})
	if err != nil || updated.Policy.Revision != 2 || updated.Policy.LimitUnits != 6 {
		t.Fatalf("update daily quota policy: result=%#v err=%v", updated, err)
	}
	if _, err = service.SetDailyPolicy(ctx, owner, quotaapp.SetDailyPolicyCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		LimitUnits: 7, ExpectedRevision: 1, IdempotencyKey: "quota-policy-stale-update",
	}); err == nil {
		t.Fatal("stale quota policy revision was updated")
	}
	if _, err = service.Reserve(ctx, viewer, quotaapp.ReserveCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		SourceType: "generation_intent", SourceID: uuid.NewString(), Units: 1, IdempotencyKey: "quota-viewer-reserve",
	}); err == nil {
		t.Fatal("viewer reserved image quota")
	}

	if err = database.Model(&model.UserAccount{}).Where("id = ?", editorID).Update("token_version", 2).Error; err != nil {
		t.Fatalf("revoke quota editor token: %v", err)
	}
	if _, err = service.Consume(ctx, editor, quotaapp.TransitionCommand{ReservationID: firstReservationID, IdempotencyKey: "quota-consume-revoked"}); err == nil {
		t.Fatal("revoked editor consumed a quota reservation")
	}
	if err = database.Model(&model.UserAccount{}).Where("id = ?", editorID).Update("token_version", 1).Error; err != nil {
		t.Fatalf("restore quota editor token: %v", err)
	}

	consumed, err := service.Consume(ctx, editor, quotaapp.TransitionCommand{
		ReservationID: firstReservationID, IdempotencyKey: "quota-consume-first",
	})
	if err != nil || consumed.Reservation.Status != quotadomain.ReservationConsumed {
		t.Fatalf("consume quota reservation: result=%#v err=%v", consumed, err)
	}
	replayedConsume, err := service.Consume(ctx, editor, quotaapp.TransitionCommand{
		ReservationID: firstReservationID, IdempotencyKey: "quota-consume-first",
	})
	if err != nil || replayedConsume.Receipt.ID != consumed.Receipt.ID {
		t.Fatalf("replay quota consumption: result=%#v err=%v", replayedConsume, err)
	}
	released, err := service.Release(ctx, editor, quotaapp.TransitionCommand{
		ReservationID: second.Reservation.ID, IdempotencyKey: "quota-release-second",
	})
	if err != nil || released.Reservation.Status != quotadomain.ReservationReleased {
		t.Fatalf("release quota reservation: result=%#v err=%v", released, err)
	}
	if _, err = service.Release(ctx, editor, quotaapp.TransitionCommand{ReservationID: firstReservationID, IdempotencyKey: "quota-release-consumed"}); err == nil {
		t.Fatal("consumed quota reservation was released")
	}
	if _, err = service.Consume(ctx, editor, quotaapp.TransitionCommand{ReservationID: second.Reservation.ID, IdempotencyKey: "quota-consume-released"}); err == nil {
		t.Fatal("released quota reservation was consumed")
	}

	third, err := service.Reserve(ctx, editor, quotaapp.ReserveCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		SourceType: "generation_intent", SourceID: uuid.NewString(), Units: 3, IdempotencyKey: "quota-reserve-unknown",
	})
	if err != nil || third.Reservation.Status != quotadomain.ReservationReserved || third.Reservation.PolicyRevision != 2 {
		t.Fatalf("reserve quota for unknown outcome: result=%#v err=%v", third, err)
	}
	usage, err := service.GetDailyUsage(ctx, viewer, projectID.String(), "generation.image.call")
	if err != nil || usage.LimitUnits != 6 || usage.ReservedUnits != 3 || usage.ConsumedUnits != 3 ||
		usage.AvailableUnits != 0 || usage.PolicyRevision != 2 || !usage.WindowStart.Equal(now.Truncate(24*time.Hour)) {
		t.Fatalf("query reconciled daily quota: usage=%#v err=%v", usage, err)
	}

	var policyCount, counterCount, reservationCount int64
	if err = database.Model(&model.QuotaPolicy{}).Where("workspace_id = ?", workspaceID).Count(&policyCount).Error; err != nil {
		t.Fatalf("count quota policies: %v", err)
	}
	if err = database.Model(&model.QuotaCounter{}).Where("workspace_id = ?", workspaceID).Count(&counterCount).Error; err != nil {
		t.Fatalf("count quota counters: %v", err)
	}
	if err = database.Model(&model.QuotaReservation{}).Where("workspace_id = ?", workspaceID).Count(&reservationCount).Error; err != nil {
		t.Fatalf("count quota reservations: %v", err)
	}
	if policyCount != 1 || counterCount != 1 || reservationCount != 3 {
		t.Fatalf("quota fact counts = policy %d counter %d reservations %d", policyCount, counterCount, reservationCount)
	}
	if err = database.Model(&model.QuotaCounter{}).Where("workspace_id = ?", workspaceID).Update("reserved_units", 0).Error; err != nil {
		t.Fatalf("inject quota counter drift: %v", err)
	}
	if _, err = service.GetDailyUsage(ctx, viewer, projectID.String(), "generation.image.call"); err == nil {
		t.Fatal("drifted quota counter passed usage reconciliation")
	}
	if err = database.Model(&model.QuotaCounter{}).Where("workspace_id = ?", workspaceID).Update("reserved_units", 3).Error; err != nil {
		t.Fatalf("restore quota counter: %v", err)
	}
	if err = database.Model(&model.QuotaReservation{}).Where("id = ?", third.Reservation.ID).Update("binding_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject quota reservation drift: %v", err)
	}
	if _, err = service.GetDailyUsage(ctx, viewer, projectID.String(), "generation.image.call"); err == nil {
		t.Fatal("drifted quota reservation passed usage reconciliation")
	}
	if err = database.Model(&model.QuotaReservation{}).Where("id = ?", third.Reservation.ID).
		Update("binding_hash", third.Reservation.BindingHash).Error; err != nil {
		t.Fatalf("restore quota reservation binding: %v", err)
	}

	now = now.Add(24 * time.Hour)
	distinctResults := make(chan quotaapp.ReservationResult, callers)
	distinctErrors := make(chan error, callers)
	distinctStart := make(chan struct{})
	var distinctWorkers sync.WaitGroup
	for index := range callers {
		distinctWorkers.Add(1)
		sourceID := uuid.NewString()
		key := "quota-distinct-source-" + string(rune('a'+index))
		go func() {
			defer distinctWorkers.Done()
			<-distinctStart
			result, reserveErr := service.Reserve(ctx, editor, quotaapp.ReserveCommand{
				WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
				SourceType: "generation_intent", SourceID: sourceID, Units: 2, IdempotencyKey: key,
			})
			if reserveErr != nil {
				distinctErrors <- reserveErr
				return
			}
			distinctResults <- result
		}()
	}
	close(distinctStart)
	distinctWorkers.Wait()
	close(distinctResults)
	close(distinctErrors)
	distinctSuccesses, distinctExceeded := 0, 0
	for result := range distinctResults {
		if result.Reservation.Status != quotadomain.ReservationReserved || result.Reservation.Units != 2 ||
			result.Reservation.PolicyRevision != 2 {
			t.Fatalf("distinct concurrent reservation drifted: %#v", result)
		}
		distinctSuccesses++
	}
	for reserveErr := range distinctErrors {
		if !quotaapp.IsCode(reserveErr, "quota_exceeded") {
			t.Fatalf("distinct concurrent reserve error = %v", reserveErr)
		}
		distinctExceeded++
	}
	if distinctSuccesses != 3 || distinctExceeded != 5 {
		t.Fatalf("distinct concurrent quota results = successes %d exceeded %d", distinctSuccesses, distinctExceeded)
	}
	nextUsage, err := service.GetDailyUsage(ctx, viewer, projectID.String(), "generation.image.call")
	if err != nil || nextUsage.ReservedUnits != 6 || nextUsage.ConsumedUnits != 0 || nextUsage.AvailableUnits != 0 {
		t.Fatalf("query next-day concurrent quota: usage=%#v err=%v", nextUsage, err)
	}
	if err = database.Model(&model.QuotaCounter{}).Where("workspace_id = ?", workspaceID).Count(&counterCount).Error; err != nil {
		t.Fatalf("recount quota counters: %v", err)
	}
	if err = database.Model(&model.QuotaReservation{}).Where("workspace_id = ?", workspaceID).Count(&reservationCount).Error; err != nil {
		t.Fatalf("recount quota reservations: %v", err)
	}
	if counterCount != 2 || reservationCount != 6 {
		t.Fatalf("two-window quota facts = counters %d reservations %d", counterCount, reservationCount)
	}

	thirdPolicy, err := service.SetDailyPolicy(ctx, owner, quotaapp.SetDailyPolicyCommand{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(), Metric: "generation.image.call",
		LimitUnits: 7, ExpectedRevision: 2, IdempotencyKey: "quota-policy-next-day-update",
	})
	if err != nil || thirdPolicy.Policy.Revision != 3 || thirdPolicy.Policy.LimitUnits != 7 {
		t.Fatalf("update quota policy in the next window: result=%#v err=%v", thirdPolicy, err)
	}
	historicalRelease, err := service.Release(ctx, editor, quotaapp.TransitionCommand{
		ReservationID: third.Reservation.ID, IdempotencyKey: "quota-release-previous-window",
	})
	if err != nil || historicalRelease.Reservation.Status != quotadomain.ReservationReleased {
		t.Fatalf("release a previous-window reservation after policy update: result=%#v err=%v", historicalRelease, err)
	}
	nextUsage, err = service.GetDailyUsage(ctx, viewer, projectID.String(), "generation.image.call")
	if err != nil || nextUsage.PolicyRevision != 3 || nextUsage.LimitUnits != 7 ||
		nextUsage.ReservedUnits != 6 || nextUsage.ConsumedUnits != 0 || nextUsage.AvailableUnits != 1 {
		t.Fatalf("query current usage after policy update: usage=%#v err=%v", nextUsage, err)
	}
	var previousCounter model.QuotaCounter
	if err = database.First(&previousCounter, "id = ?", third.Reservation.CounterID).Error; err != nil {
		t.Fatalf("load previous-window quota counter: %v", err)
	}
	if previousCounter.PolicyRevision != 2 || previousCounter.LimitUnits != 6 ||
		previousCounter.ReservedUnits != 0 || previousCounter.ConsumedUnits != 3 {
		t.Fatalf("previous-window quota counter drifted: %#v", previousCounter)
	}
}
