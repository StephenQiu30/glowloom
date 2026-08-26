package generation_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
)

type controlledProviderGateway struct {
	mu            sync.Mutex
	submitCalls   int
	queryCalls    int
	submitOutcome generationapp.ProviderOutcome
	queryOutcome  generationapp.ProviderOutcome
	submitError   error
	queryError    error
	submitStarted chan struct{}
	releaseSubmit chan struct{}
}

func (gateway *controlledProviderGateway) Submit(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	gateway.submitCalls++
	outcome, err := gateway.submitOutcome, gateway.submitError
	started, release := gateway.submitStarted, gateway.releaseSubmit
	gateway.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		<-release
	}
	return controlledProviderOutcome(submission, outcome), err
}

func (gateway *controlledProviderGateway) Query(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.queryCalls++
	return controlledProviderOutcome(submission, gateway.queryOutcome), gateway.queryError
}

func controlledProviderOutcome(
	submission generationapp.ProviderSubmission,
	outcome generationapp.ProviderOutcome,
) generationapp.ProviderOutcome {
	outcome.Outputs = append([]generationapp.ProviderOutput(nil), outcome.Outputs...)
	for index := range outcome.Outputs {
		outcome.Outputs[index].StagingObjectKey = "staging/" + submission.WorkspaceID + "/" +
			submission.ProviderJobID + "/" + outcome.Outputs[index].OutputKey + ".png"
	}
	return outcome
}

func (gateway *controlledProviderGateway) setSubmit(outcome generationapp.ProviderOutcome, err error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.submitOutcome, gateway.submitError = outcome, err
}

func (gateway *controlledProviderGateway) setQuery(outcome generationapp.ProviderOutcome, err error) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.queryOutcome, gateway.queryError = outcome, err
}

func (gateway *controlledProviderGateway) counts() (int, int) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	return gateway.submitCalls, gateway.queryCalls
}

func TestProviderSubmissionAndReconcileUseOneRequestKeyAndAtomicOwnerTransitions(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Generation Provider journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Generation Provider test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Generation Provider GORM catalog: %v", err)
	}
	assertProviderSchema(t,
		func(constraint string) bool {
			return database.Migrator().HasConstraint(&model.GenerationProviderJob{}, constraint)
		},
		func(value any, index string) bool { return database.Migrator().HasIndex(value, index) },
	)

	now := time.Date(2026, time.August, 26, 21, 0, 0, 0, time.UTC)
	currentTime := now
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedPreparationFixture(t, create, now, "provider")
	t.Cleanup(func() {
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return database.Where(query, arguments...).Delete(value).Error
		}, fixture)
	})

	costConfig := costapp.Config{Now: func() time.Time { return currentTime }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return currentTime }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{
			Now: func() time.Time { return currentTime }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL,
		},
	)
	gateway := &controlledProviderGateway{}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig),
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return currentTime }, NewID: uuid.NewString},
	)

	binding, err := providers.PublishImageProviderBinding(ctx, fixture.owner, generationapp.PublishProviderBindingCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		ProviderKey: "controlled-image", ModelKey: "image-quality-v1", CredentialRef: "provider/image-primary",
		IdempotencyKey: "generation-provider-binding-v1",
	})
	if err != nil || binding.Binding.Revision != 1 || binding.Binding.Capability != costdomain.MetricGenerationImage {
		t.Fatalf("publish image Provider binding: result=%#v err=%v", binding, err)
	}

	unknownClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 2, "unknown", strings.Repeat("2", 64))
	gateway.setSubmit(generationapp.ProviderOutcome{Status: generationapp.ProviderOutcomeUnknown}, nil)
	unknown, err := providers.SubmitImageRequest(ctx, unknownClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: unknownClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-unknown",
	})
	if err != nil || unknown.Job.Status != generationdomain.ProviderJobUnknown ||
		unknown.Intent.Status != generationdomain.IntentOutcomeUnknown || unknown.Request.RequestKey == "" ||
		unknown.Request.BindingID != binding.Binding.ID {
		t.Fatalf("persist unknown Provider submission: result=%#v err=%v", unknown, err)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, unknown.Intent, costdomain.ReservationReserved, quotadomain.ReservationReserved)
	if submitCalls, queryCalls := gateway.counts(); submitCalls != 1 || queryCalls != 0 {
		t.Fatalf("initial Provider submission calls = submit %d query %d, want 1/0", submitCalls, queryCalls)
	}

	replayedUnknown, err := providers.SubmitImageRequest(ctx, unknownClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: unknownClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-unknown",
	})
	if err != nil || replayedUnknown.Receipt.ID != unknown.Receipt.ID ||
		replayedUnknown.Request.RequestKey != unknown.Request.RequestKey {
		t.Fatalf("replay unknown Provider submission: result=%#v err=%v", replayedUnknown, err)
	}
	if submitCalls, queryCalls := gateway.counts(); submitCalls != 1 || queryCalls != 0 {
		t.Fatalf("idempotent Provider replay called remote boundary: submit %d query %d", submitCalls, queryCalls)
	}

	output := generationapp.ProviderOutput{
		OutputKey: "image-1", StagingObjectKey: "provider/unknown/image-1.png",
		SHA256: strings.Repeat("a", 64), Bytes: 128, MediaType: "image/png", Width: 1024, Height: 1024,
	}
	gateway.setQuery(generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeSucceeded, ProviderJobKey: "provider-job-unknown",
		ProviderEventID: "provider-event-success", ActualUnits: 1, Outputs: []generationapp.ProviderOutput{output},
	}, nil)
	reconciled, err := providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: unknown.Job.ID, IdempotencyKey: "generation-provider-reconcile-success",
	})
	output.StagingObjectKey = "staging/" + fixture.workspaceID.String() + "/" + unknown.Job.ID + "/image-1.png"
	if err != nil || reconciled.Job.Status != generationdomain.ProviderJobSucceeded ||
		reconciled.Intent.Status != generationdomain.IntentSucceeded || reconciled.ProviderReceipt.ID == "" ||
		reconciled.ProviderReceipt.ProviderEventID != "provider-event-success" ||
		len(reconciled.ProviderReceipt.Outputs) != 1 || reconciled.ProviderReceipt.Outputs[0] != output {
		t.Fatalf("reconcile late Provider success: result=%#v err=%v", reconciled, err)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, reconciled.Intent, costdomain.ReservationSettled, quotadomain.ReservationConsumed)
	if submitCalls, queryCalls := gateway.counts(); submitCalls != 1 || queryCalls != 1 {
		t.Fatalf("Provider reconcile calls = submit %d query %d, want 1/1", submitCalls, queryCalls)
	}

	terminalReplay, err := providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: unknown.Job.ID, IdempotencyKey: "generation-provider-reconcile-terminal-replay",
	})
	if err != nil || terminalReplay.ProviderReceipt.ID != reconciled.ProviderReceipt.ID ||
		terminalReplay.Request.RequestKey != unknown.Request.RequestKey {
		t.Fatalf("replay terminal Provider result: result=%#v err=%v", terminalReplay, err)
	}
	if submitCalls, queryCalls := gateway.counts(); submitCalls != 1 || queryCalls != 1 {
		t.Fatalf("terminal Provider replay queried remote boundary: submit %d query %d", submitCalls, queryCalls)
	}

	bindingV2, err := providers.PublishImageProviderBinding(ctx, fixture.owner, generationapp.PublishProviderBindingCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		ProviderKey: "controlled-image", ModelKey: "image-quality-v2", CredentialRef: "provider/image-primary",
		IdempotencyKey: "generation-provider-binding-v2",
	})
	if err != nil || bindingV2.Binding.Revision != 2 || terminalReplay.Request.BindingID != binding.Binding.ID {
		t.Fatalf("append Provider binding without rerouting old request: v2=%#v old=%#v err=%v", bindingV2, terminalReplay, err)
	}

	jobKeyDriftClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 1, "job-key-drift", strings.Repeat("6", 64))
	gateway.setSubmit(generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeUnknown, ProviderJobKey: "provider-job-key-original",
	}, nil)
	jobKeyUnknown, err := providers.SubmitImageRequest(ctx, jobKeyDriftClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: jobKeyDriftClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-job-key-drift",
	})
	if err != nil || jobKeyUnknown.Job.ProviderJobKey != "provider-job-key-original" {
		t.Fatalf("freeze first known Provider job key: result=%#v err=%v", jobKeyUnknown, err)
	}
	gateway.setQuery(generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeSucceeded, ProviderJobKey: "provider-job-key-replaced",
		ProviderEventID: "provider-event-job-key-drift", ActualUnits: 1,
		Outputs: []generationapp.ProviderOutput{{
			OutputKey: "image-1", StagingObjectKey: "provider/job-key-drift/image-1.png",
			SHA256: strings.Repeat("c", 64), Bytes: 64, MediaType: "image/png", Width: 512, Height: 512,
		}},
	}, nil)
	if _, err = providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: jobKeyUnknown.Job.ID, IdempotencyKey: "generation-provider-reconcile-job-key-drift",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider external job key drift was accepted: %T %v", err, err)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, jobKeyUnknown.Intent, costdomain.ReservationReserved, quotadomain.ReservationReserved)

	duplicateEventClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 1, "duplicate-event", strings.Repeat("7", 64))
	gateway.setSubmit(generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeSucceeded, ProviderJobKey: "provider-job-duplicate-event",
		ProviderEventID: "provider-event-success", ActualUnits: 1,
		Outputs: []generationapp.ProviderOutput{{
			OutputKey: "image-1", StagingObjectKey: "provider/duplicate-event/image-1.png",
			SHA256: strings.Repeat("d", 64), Bytes: 64, MediaType: "image/png", Width: 512, Height: 512,
		}},
	}, nil)
	if _, err = providers.SubmitImageRequest(ctx, duplicateEventClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: duplicateEventClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-duplicate-event",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("duplicate Provider event was accepted by another job: %T %v", err, err)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, duplicateEventClaim.Intent, costdomain.ReservationReserved, quotadomain.ReservationReserved)

	failedClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 1, "failed", strings.Repeat("3", 64))
	gateway.setSubmit(generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeFailed, ProviderJobKey: "provider-job-failed",
		ProviderEventID: "provider-event-failed", FailureCode: "content_rejected",
	}, nil)
	failed, err := providers.SubmitImageRequest(ctx, failedClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: failedClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-failed",
	})
	if err != nil || failed.Job.Status != generationdomain.ProviderJobFailed ||
		failed.Intent.Status != generationdomain.IntentFailed || failed.ProviderReceipt.FailureCode != "content_rejected" ||
		failed.Request.BindingID != bindingV2.Binding.ID {
		t.Fatalf("apply known Provider failure: result=%#v err=%v", failed, err)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, failed.Intent, costdomain.ReservationReleased, quotadomain.ReservationReleased)

	rollbackClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 1, "owner-rollback", strings.Repeat("4", 64))
	gateway.setSubmit(generationapp.ProviderOutcome{Status: generationapp.ProviderOutcomeUnknown}, errors.New("connection reset after write"))
	rollbackUnknown, err := providers.SubmitImageRequest(ctx, rollbackClaim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: rollbackClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-owner-rollback",
	})
	if err != nil || rollbackUnknown.Job.Status != generationdomain.ProviderJobUnknown {
		t.Fatalf("persist transport-unknown Provider result: result=%#v err=%v", rollbackUnknown, err)
	}
	if _, err = quotas.Release(ctx, quotaapp.Actor{UserID: fixture.editor.UserID, TokenVersion: 1}, quotaapp.TransitionCommand{
		ReservationID: rollbackUnknown.Intent.QuotaReservationID, IdempotencyKey: "generation-provider-injected-quota-release",
	}); err != nil {
		t.Fatalf("inject terminal Quota drift: %v", err)
	}
	gateway.setQuery(generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeSucceeded, ProviderJobKey: "provider-job-owner-rollback",
		ProviderEventID: "provider-event-owner-rollback", ActualUnits: 1,
		Outputs: []generationapp.ProviderOutput{{
			OutputKey: "image-1", StagingObjectKey: "provider/rollback/image-1.png",
			SHA256: strings.Repeat("b", 64), Bytes: 64, MediaType: "image/png", Width: 512, Height: 512,
		}},
	}, nil)
	if _, err = providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: rollbackUnknown.Job.ID, IdempotencyKey: "generation-provider-reconcile-owner-rollback",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("Provider terminal Owner drift did not fail closed: %T %v", err, err)
	}
	costView, err := costs.GetReservation(ctx, costapp.Actor{UserID: fixture.editor.UserID, TokenVersion: 1}, rollbackUnknown.Intent.CostReservationID)
	if err != nil || costView.Reservation.Status != costdomain.ReservationReserved {
		t.Fatalf("failed terminal transaction did not roll back Cost settlement: view=%#v err=%v", costView, err)
	}
	var providerReceiptCount int64
	if err = database.Model(&model.GenerationProviderResultReceipt{}).
		Where("provider_event_id = ?", "provider-event-owner-rollback").Count(&providerReceiptCount).Error; err != nil {
		t.Fatalf("count rolled-back Provider receipt: %v", err)
	}
	if providerReceiptCount != 0 {
		t.Fatalf("failed terminal transaction kept %d Provider result receipts", providerReceiptCount)
	}

	concurrentClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 1, "concurrent", strings.Repeat("5", 64))
	started, release := make(chan struct{}, 1), make(chan struct{})
	gateway.mu.Lock()
	gateway.submitStarted, gateway.releaseSubmit = started, release
	gateway.submitOutcome, gateway.submitError = generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeAccepted, ProviderJobKey: "provider-job-concurrent",
	}, nil
	gateway.queryOutcome, gateway.queryError = generationapp.ProviderOutcome{Status: generationapp.ProviderOutcomeUnknown}, nil
	gateway.mu.Unlock()
	beforeSubmit, _ := gateway.counts()
	firstDone := make(chan error, 1)
	go func() {
		_, submitErr := providers.SubmitImageRequest(ctx, concurrentClaim.Authorization, generationapp.SubmitImageRequestCommand{
			IntentID: concurrentClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-concurrent-0",
		})
		firstDone <- submitErr
	}()
	<-started
	const followers = 7
	followerErrors := make(chan error, followers)
	var followersDone sync.WaitGroup
	for index := 1; index <= followers; index++ {
		followersDone.Add(1)
		go func(index int) {
			defer followersDone.Done()
			_, submitErr := providers.SubmitImageRequest(ctx, concurrentClaim.Authorization, generationapp.SubmitImageRequestCommand{
				IntentID:       concurrentClaim.Intent.ID,
				IdempotencyKey: "generation-provider-submit-concurrent-" + string(rune('0'+index)),
			})
			followerErrors <- submitErr
		}(index)
	}
	followersDone.Wait()
	close(release)
	if err = <-firstDone; err != nil {
		t.Fatalf("first concurrent Provider submission: %v", err)
	}
	for range followers {
		if followerErr := <-followerErrors; followerErr != nil {
			t.Fatalf("follower concurrent Provider submission: %v", followerErr)
		}
	}
	afterSubmit, _ := gateway.counts()
	if afterSubmit-beforeSubmit != 1 {
		t.Fatalf("concurrent Provider submission called Submit %d times, want 1", afterSubmit-beforeSubmit)
	}
	var requestCount, jobCount int64
	if err = database.Model(&model.GenerationRequest{}).Where("intent_id = ?", concurrentClaim.Intent.ID).Count(&requestCount).Error; err != nil {
		t.Fatalf("count concurrent Generation requests: %v", err)
	}
	if err = database.Model(&model.GenerationProviderJob{}).Where("intent_id = ?", concurrentClaim.Intent.ID).Count(&jobCount).Error; err != nil {
		t.Fatalf("count concurrent Provider jobs: %v", err)
	}
	if requestCount != 1 || jobCount != 1 {
		t.Fatalf("concurrent Provider facts = requests %d jobs %d, want 1/1", requestCount, jobCount)
	}
}

func prepareAndClaimProviderIntent(
	t *testing.T,
	ctx context.Context,
	preparations *generationapp.PreparationService,
	fixture preparationFixture,
	units int64,
	suffix, hash string,
) generationapp.ExecutionClaimResult {
	t.Helper()
	prepared := prepareDistinctIntent(t, ctx, preparations, fixture, units, "provider-"+suffix, hash)
	claimed, err := preparations.AcquireExecutionClaim(ctx, generationapp.AcquireExecutionClaimCommand{
		IntentID: prepared.Intent.ID, Claimant: "model-gateway:provider-worker",
		IdempotencyKey: "generation-provider-claim-" + suffix,
	})
	if err != nil {
		t.Fatalf("claim Provider intent %s: %v", suffix, err)
	}
	return claimed
}

func assertProviderReservations(
	t *testing.T,
	ctx context.Context,
	costs *costapp.Service,
	quotas *quotaapp.Service,
	fixture preparationFixture,
	intent generationdomain.Intent,
	wantCost, wantQuota string,
) {
	t.Helper()
	costView, err := costs.GetReservation(ctx, costapp.Actor{
		UserID: fixture.editor.UserID, TokenVersion: fixture.editor.TokenVersion,
	}, intent.CostReservationID)
	if err != nil || costView.Reservation.Status != wantCost {
		t.Fatalf("Cost reservation status = %#v err=%v, want %s", costView.Reservation, err, wantCost)
	}
	quotaReservation, err := quotas.GetReservation(ctx, quotaapp.Actor{
		UserID: fixture.editor.UserID, TokenVersion: fixture.editor.TokenVersion,
	}, intent.QuotaReservationID)
	if err != nil || quotaReservation.Status != wantQuota {
		t.Fatalf("Quota reservation status = %#v err=%v, want %s", quotaReservation, err, wantQuota)
	}
}

func assertProviderSchema(
	t *testing.T,
	hasJobConstraint func(string) bool,
	hasIndex func(any, string) bool,
) {
	t.Helper()
	for _, constraint := range []string{"ck_gen_provider_job_state", "ck_gen_provider_job_terminal"} {
		if !hasJobConstraint(constraint) {
			t.Fatalf("Generation Provider schema is missing constraint %s", constraint)
		}
	}
	checks := []struct {
		model any
		index string
	}{
		{&model.GenerationProviderBindingVersion{}, "uq_gen_provider_binding_revision"},
		{&model.GenerationRequest{}, "uq_gen_request_intent"},
		{&model.GenerationRequest{}, "uq_gen_request_key"},
		{&model.GenerationProviderJob{}, "uq_gen_provider_job_request"},
		{&model.GenerationProviderResultReceipt{}, "uq_gen_provider_receipt_event"},
	}
	for _, check := range checks {
		if !hasIndex(check.model, check.index) {
			t.Fatalf("Generation Provider schema is missing index %s", check.index)
		}
	}
}

func cleanupProviderFixture(
	t *testing.T,
	deleteRecords func(any, string, ...any) error,
	fixture preparationFixture,
) {
	t.Helper()
	deletions := []struct {
		name string
		err  error
	}{
		{"Provider result receipts", deleteRecords(&model.GenerationProviderResultReceipt{}, "workspace_id = ?", fixture.workspaceID)},
		{"Provider jobs", deleteRecords(&model.GenerationProviderJob{}, "workspace_id = ?", fixture.workspaceID)},
		{"Generation requests", deleteRecords(&model.GenerationRequest{}, "workspace_id = ?", fixture.workspaceID)},
		{"Generation intents", deleteRecords(&model.GenerationIntent{}, "workspace_id = ?", fixture.workspaceID)},
		{"Provider bindings", deleteRecords(&model.GenerationProviderBindingVersion{}, "workspace_id = ?", fixture.workspaceID)},
		{"Workflow node runs", deleteRecords(&model.NodeRunProjection{}, "workflow_run_id = ?", fixture.workflowRunID)},
		{"Workflow run", deleteRecords(&model.WorkflowRun{}, "id = ?", fixture.workflowRunID)},
		{"Workflow input snapshot", deleteRecords(&model.RunInputSnapshot{}, "id = ?", fixture.runInputSnapshotID)},
		{"Workflow definition", deleteRecords(&model.WorkflowDefinitionVersion{}, "id = ?", fixture.workflowDefinitionID)},
	}
	for _, deletion := range deletions {
		if deletion.err != nil {
			t.Errorf("clean test-owned %s: %v", deletion.name, deletion.err)
		}
	}
}
