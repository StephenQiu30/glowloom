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

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	providersecret "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/secretstore"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	"github.com/google/uuid"
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
	submissions   []generationapp.ProviderSubmission
}

type controlledBindingResolver struct {
	mu                sync.Mutex
	resolved          generationapp.ResolvedProjectProviderBinding
	err               error
	afterResolve      chan struct{}
	releaseResolution chan struct{}
	resolveOnce       sync.Once
}

type transactionalControlledBindingResolver struct {
	transactions generationapp.ProviderConfigurationTransactionManager
	resolved     generationapp.ResolvedProjectProviderBinding
}

func (resolver *transactionalControlledBindingResolver) ResolveProjectBinding(
	ctx context.Context,
	_ generationapp.Actor,
	projectID, purpose string,
) (generationapp.ResolvedProjectProviderBinding, error) {
	if resolver.resolved.Binding.ProjectID != projectID || resolver.resolved.Binding.Purpose != purpose {
		return generationapp.ResolvedProjectProviderBinding{}, generationapp.ErrProjectProviderBindingNotFound
	}
	err := resolver.transactions.WithinProviderConfigurationTransaction(ctx, func(
		repository generationapp.ProviderConfigurationRepository,
	) error {
		return repository.LockProviderWorkspace(ctx, resolver.resolved.Binding.WorkspaceID)
	})
	if err != nil {
		return generationapp.ResolvedProjectProviderBinding{}, err
	}
	return resolver.resolved, nil
}

func (resolver *controlledBindingResolver) ResolveProjectBinding(
	_ context.Context,
	_ generationapp.Actor,
	projectID, purpose string,
) (generationapp.ResolvedProjectProviderBinding, error) {
	resolver.mu.Lock()
	resolved, err := resolver.resolved, resolver.err
	afterResolve, releaseResolution := resolver.afterResolve, resolver.releaseResolution
	resolver.mu.Unlock()
	if err != nil {
		return generationapp.ResolvedProjectProviderBinding{}, err
	}
	if resolved.Binding.ProjectID != projectID || resolved.Binding.Purpose != purpose {
		return generationapp.ResolvedProjectProviderBinding{}, generationapp.ErrProjectProviderBindingNotFound
	}
	if afterResolve != nil {
		resolver.resolveOnce.Do(func() { close(afterResolve) })
	}
	if releaseResolution != nil {
		select {
		case <-releaseResolution:
		case <-time.After(5 * time.Second):
			return generationapp.ResolvedProjectProviderBinding{}, context.DeadlineExceeded
		}
	}
	return resolved, nil
}

func (resolver *controlledBindingResolver) set(resolved generationapp.ResolvedProjectProviderBinding) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.resolved, resolver.err = resolved, nil
}

func (resolver *controlledBindingResolver) pauseAfterResolve(afterResolve, releaseResolution chan struct{}) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.afterResolve, resolver.releaseResolution = afterResolve, releaseResolution
	resolver.resolveOnce = sync.Once{}
}

func (gateway *controlledProviderGateway) Submit(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	gateway.submitCalls++
	gateway.submissions = append(gateway.submissions, submission)
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
	gateway.submissions = append(gateway.submissions, submission)
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

func (gateway *controlledProviderGateway) lastSubmission() generationapp.ProviderSubmission {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.submissions) == 0 {
		return generationapp.ProviderSubmission{}
	}
	return gateway.submissions[len(gateway.submissions)-1]
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
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), now, "provider")
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
	bindingResolver := &controlledBindingResolver{err: generationapp.ErrProjectProviderBindingNotFound}
	gateway := &controlledProviderGateway{}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return currentTime }, NewID: uuid.NewString, Bindings: bindingResolver},
	)
	if _, err = providers.RequireProjectProviderBinding(
		ctx, fixture.owner, fixture.projectID.String(), generationdomain.ProviderPurposeReferenceAsset,
	); generationErrorCode(err) != "not_found" {
		t.Fatalf("missing configured Provider binding did not fail before generation: %v", err)
	}
	binding := seedControlledProjectProviderBinding(t, create, fixture, "controlled-image", "image-quality-v1", 1)
	bindingResolver.set(binding)
	var configuredBindingCount int64
	if err = database.Model(&model.ProjectProviderBindingVersion{}).
		Where("project_id = ?", fixture.projectID).Count(&configuredBindingCount).Error; err != nil || configuredBindingCount != 1 {
		t.Fatalf("configured image Provider binding count = %d: %v", configuredBindingCount, err)
	}
	configuredBinding, err := providers.RequireProjectProviderBinding(
		ctx, fixture.owner, fixture.projectID.String(), generationdomain.ProviderPurposeReferenceAsset,
	)
	if err != nil || configuredBinding.ID != binding.Binding.ID {
		t.Fatalf("require configured Provider binding: binding=%#v err=%v", configuredBinding, err)
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
	var unknownNode model.NodeRunProjection
	if err = database.First(&unknownNode, "id = ?", unknown.Intent.NodeRunID).Error; err != nil || unknownNode.InputHash == nil {
		t.Fatalf("load unknown Provider Workflow node: node=%#v err=%v", unknownNode, err)
	}
	progressedPreparation, err := preparations.PrepareImageGeneration(ctx, fixture.editor, generationapp.PrepareImageGenerationCommand{
		WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		WorkflowRunID: fixture.workflowRunID.String(), NodeRunID: unknown.Intent.NodeRunID,
		WorkflowInputHash: *unknownNode.InputHash, TargetID: unknown.Intent.TargetID, TargetHash: unknown.Intent.TargetHash,
		Units: unknown.Intent.Units, IdempotencyKey: "generation-prepare-provider-unknown",
	})
	if err != nil || progressedPreparation.Intent.ID != unknown.Intent.ID ||
		progressedPreparation.Intent.Status != generationdomain.IntentOutcomeUnknown ||
		progressedPreparation.Intent.ProviderJobID != unknown.Job.ID {
		t.Fatalf("replay preparation after Provider progress: result=%#v err=%v", progressedPreparation, err)
	}
	progressedClaim, err := preparations.AcquireExecutionClaim(ctx, generationapp.AcquireExecutionClaimCommand{
		IntentID: unknown.Intent.ID, Claimant: "model-gateway:provider-worker",
		IdempotencyKey: "generation-provider-claim-unknown",
	})
	if err != nil || progressedClaim.Intent.ID != unknown.Intent.ID ||
		progressedClaim.Intent.Status != generationdomain.IntentOutcomeUnknown ||
		progressedClaim.Authorization != unknownClaim.Authorization {
		t.Fatalf("replay execution claim after Provider progress: result=%#v err=%v", progressedClaim, err)
	}
	submission := gateway.lastSubmission()
	if submission.Target.ID != unknown.Request.TargetID || submission.TargetHash != unknown.Request.TargetHash ||
		submission.Target.TargetHash != unknown.Intent.TargetHash ||
		generationdomain.ValidateGenerationTarget(submission.Target) != nil {
		t.Fatalf("Provider did not receive the frozen GenerationTarget: %#v", submission)
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
	if err = database.Model(&model.GenerationTarget{}).Where("id = ?", unknown.Request.TargetID).
		UpdateColumn("target_hash", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatalf("inject persisted GenerationTarget drift: %v", err)
	}
	if _, err = providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: unknown.Job.ID, IdempotencyKey: "generation-provider-reconcile-target-drift",
	}); generationErrorCode(err) != "state_conflict" {
		t.Fatalf("GenerationTarget drift was accepted: %T %v", err, err)
	}
	if submitCalls, queryCalls := gateway.counts(); submitCalls != 1 || queryCalls != 0 {
		t.Fatalf("GenerationTarget drift reached Provider boundary: submit %d query %d", submitCalls, queryCalls)
	}
	if err = database.Model(&model.GenerationTarget{}).Where("id = ?", unknown.Request.TargetID).
		UpdateColumn("target_hash", unknown.Request.TargetHash).Error; err != nil {
		t.Fatalf("restore persisted GenerationTarget after drift test: %v", err)
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

	bindingV2 := seedControlledProjectProviderBinding(t, create, fixture, "controlled-image", "image-quality-v2", 2)
	bindingResolver.set(bindingV2)
	if bindingV2.Binding.Revision != 2 || terminalReplay.Request.BindingID != binding.Binding.ID {
		t.Fatalf("append Provider binding without rerouting old request: v2=%#v old=%#v", bindingV2, terminalReplay)
	}
	if configuredV2, configureErr := providers.RequireProjectProviderBinding(
		ctx, fixture.owner, fixture.projectID.String(), generationdomain.ProviderPurposeReferenceAsset,
	); configureErr != nil || configuredV2.ID != bindingV2.Binding.ID {
		t.Fatalf("activate configured Provider binding v2: binding=%#v err=%v", configuredV2, configureErr)
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

	singlePoolClaim := prepareAndClaimProviderIntent(
		t, ctx, preparations, fixture, 1, "single-pool", strings.Repeat("9", 64),
	)
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("load SQL pool for Provider self-deadlock regression: %v", err)
	}
	sqlDatabase.SetMaxIdleConns(1)
	sqlDatabase.SetMaxOpenConns(1)
	defer func() {
		sqlDatabase.SetMaxOpenConns(20)
		sqlDatabase.SetMaxIdleConns(20)
	}()
	singlePoolGateway := &controlledProviderGateway{
		submitOutcome: generationapp.ProviderOutcome{
			Status: generationapp.ProviderOutcomeAccepted, ProviderJobKey: "provider-job-single-pool",
		},
	}
	singlePoolProviders := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), singlePoolGateway,
		generationapp.ProviderConfig{
			Now: func() time.Time { return currentTime }, NewID: uuid.NewString,
			Bindings: &transactionalControlledBindingResolver{
				transactions: generationgorm.NewProviderConfigurationStore(database), resolved: bindingV2,
			},
		},
	)
	singlePoolContext, cancelSinglePool := context.WithTimeout(ctx, 3*time.Second)
	singlePoolResult, err := singlePoolProviders.SubmitImageRequest(
		singlePoolContext,
		singlePoolClaim.Authorization,
		generationapp.SubmitImageRequestCommand{
			IntentID: singlePoolClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-single-pool",
		},
	)
	cancelSinglePool()
	if err != nil || singlePoolResult.Request.ID == "" || singlePoolResult.Job.ProviderJobKey != "provider-job-single-pool" {
		t.Fatalf("Provider submission self-deadlocked with one DB connection: result=%#v err=%v", singlePoolResult, err)
	}
	sqlDatabase.SetMaxOpenConns(20)
	sqlDatabase.SetMaxIdleConns(20)

	gateway.mu.Lock()
	gateway.submitStarted, gateway.releaseSubmit = nil, nil
	gateway.mu.Unlock()
	gapClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, 1, "binding-gap", strings.Repeat("8", 64))
	afterResolve, releaseResolution := make(chan struct{}), make(chan struct{})
	bindingResolver.pauseAfterResolve(afterResolve, releaseResolution)
	beforeGapSubmit, beforeGapQuery := gateway.counts()
	gapDone := make(chan error, 1)
	go func() {
		_, submitErr := providers.SubmitImageRequest(ctx, gapClaim.Authorization, generationapp.SubmitImageRequestCommand{
			IntentID: gapClaim.Intent.ID, IdempotencyKey: "generation-provider-submit-binding-gap",
		})
		gapDone <- submitErr
	}()
	select {
	case <-afterResolve:
	case <-time.After(5 * time.Second):
		t.Fatal("Provider submission did not reach the post-resolver gap")
	}
	emptyRegistry, err := generationapp.NewMediaFactoryRegistry(nil)
	if err != nil {
		t.Fatalf("create empty Provider registry for execution fencing: %v", err)
	}
	emptyCatalog, err := generationapp.NewMediaPresetCatalog(generationdomain.MediaPresets{}, emptyRegistry)
	if err != nil {
		t.Fatalf("create empty Provider catalog for execution fencing: %v", err)
	}
	configuration := generationapp.NewProviderConfigurationService(
		generationgorm.NewProviderConfigurationStore(database), emptyCatalog, providersecret.Open(""),
		generationapp.ProviderConfigurationConfig{Now: func() time.Time { return currentTime }, NewID: uuid.NewString},
	)
	disabledConnection, err := configuration.SetConnectionState(ctx, fixture.owner, generationapp.SetProviderConnectionStateCommand{
		WorkspaceID: fixture.workspaceID.String(), ConnectionKey: bindingV2.Connection.ConnectionKey,
		State: generationdomain.ProviderStateDisabled, ExpectedRevision: bindingV2.Connection.Revision,
		ExpectedContentHash: bindingV2.Connection.ContentHash,
		IdempotencyKey:      "generation-provider-disable-in-resolver-gap",
	})
	if err != nil || disabledConnection.Connection.Revision != bindingV2.Connection.Revision+1 {
		t.Fatalf("disable Provider connection in resolver gap: result=%#v err=%v", disabledConnection, err)
	}
	close(releaseResolution)
	select {
	case submitErr := <-gapDone:
		if generationErrorCode(submitErr) != "state_conflict" {
			t.Fatalf("Provider request froze a connection disabled in the resolver gap: %T %v", submitErr, submitErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Provider submission did not leave the post-resolver gap")
	}
	if afterGapSubmit, afterGapQuery := gateway.counts(); afterGapSubmit != beforeGapSubmit || afterGapQuery != beforeGapQuery {
		t.Fatalf("resolver-gap drift reached Provider boundary: before=%d/%d after=%d/%d",
			beforeGapSubmit, beforeGapQuery, afterGapSubmit, afterGapQuery)
	}
	requestCount, jobCount = 0, 0
	if err = database.Model(&model.GenerationRequest{}).Where("intent_id = ?", gapClaim.Intent.ID).Count(&requestCount).Error; err != nil {
		t.Fatalf("count resolver-gap Generation requests: %v", err)
	}
	if err = database.Model(&model.GenerationProviderJob{}).Where("intent_id = ?", gapClaim.Intent.ID).Count(&jobCount).Error; err != nil {
		t.Fatalf("count resolver-gap Provider jobs: %v", err)
	}
	if requestCount != 0 || jobCount != 0 {
		t.Fatalf("resolver-gap drift persisted Provider facts: requests=%d jobs=%d", requestCount, jobCount)
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
		{&model.ProjectProviderBindingVersion{}, "uq_gen_project_provider_binding_revision"},
		{&model.ProviderConnectionVersion{}, "uq_gen_provider_connection_revision"},
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

func seedControlledProjectProviderBinding(
	t *testing.T,
	create func(any) error,
	fixture preparationFixture,
	providerKey, externalModelID string,
	revision int64,
) generationapp.ResolvedProjectProviderBinding {
	t.Helper()
	credentialID, connectionID, profileID, bindingID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	createdAt := fixture.now.Add(time.Duration(revision) * time.Second)
	connectionDomain := generationdomain.ProviderConnectionVersion{
		ID: connectionID.String(), WorkspaceID: fixture.workspaceID.String(), ConnectionKey: "controlled-primary",
		Revision: revision, SourcePresetKey: "controlled.image", SourcePresetVersion: 1,
		PresetSnapshotHash: strings.Repeat("b", 64), ProviderKey: providerKey, DisplayName: "Controlled image",
		CredentialVersionID: credentialID.String(), ResolvedConfig: map[string]any{},
		State: generationdomain.ProviderStateEnabled, AdapterContractVersion: "controlled-image-v1",
		CreatedBy: fixture.ownerID.String(), CreatedAt: createdAt,
	}
	connectionDomain.ContentHash = controlledProviderConnectionContentHash(t, connectionDomain)
	profileDomain := generationdomain.ProviderModelProfileVersion{
		ID: profileID.String(), WorkspaceID: fixture.workspaceID.String(),
		ProfileKey: "controlled-profile-" + externalModelID, Revision: revision,
		CreationSource: map[string]any{"kind": "preset"}, ConnectionKey: "controlled-primary",
		ProviderKey: providerKey, ExternalModelID: externalModelID,
		Modality: generationdomain.MediaModalityImage, Family: "controlled_image",
		AdapterTransportContract: "controlled-image-v1", CapabilitySchemaVersion: "controlled-image-v1",
		BillingMetric: "generation.image.call", Defaults: map[string]any{},
		State: generationdomain.ProviderStateEnabled, CreatedBy: fixture.ownerID.String(), CreatedAt: createdAt,
	}
	profileDomain.ContentHash = controlledProviderProfileContentHash(t, profileDomain)
	credential := model.ProviderCredentialVersion{
		ID: credentialID, WorkspaceID: fixture.workspaceID, ConnectionKey: "controlled-primary",
		Revision: revision, ProviderKey: providerKey, CipherSuite: generationdomain.ProviderCipherAES256GCM,
		KeyID: "controlled-key", Nonce: []byte("0123456789ab"), Ciphertext: []byte("0123456789abcdef"),
		SecretFingerprint: strings.Repeat("a", 64), CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	connection := model.ProviderConnectionVersion{
		ID: connectionID, WorkspaceID: fixture.workspaceID, ConnectionKey: "controlled-primary",
		Revision: revision, SourcePresetKey: "controlled.image", SourcePresetVersion: 1,
		PresetSnapshotHash: strings.Repeat("b", 64), ProviderKey: providerKey, DisplayName: "Controlled image",
		CredentialVersionID: credentialID, ResolvedConfig: []byte(`{}`),
		State: generationdomain.ProviderStateEnabled, AdapterContractVersion: "controlled-image-v1",
		ContentHash: connectionDomain.ContentHash, CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	profile := model.ProviderModelProfileVersion{
		ID: profileID, WorkspaceID: fixture.workspaceID, ProfileKey: "controlled-profile-" + externalModelID,
		Revision: revision, CreationSource: []byte(`{"kind":"preset"}`),
		ConnectionKey: "controlled-primary", ProviderKey: providerKey, ExternalModelID: externalModelID,
		Modality: generationdomain.MediaModalityImage, Family: "controlled_image",
		AdapterTransportContract: "controlled-image-v1", CapabilitySchemaVersion: "controlled-image-v1",
		BillingMetric: "generation.image.call", Defaults: []byte(`{}`),
		State: generationdomain.ProviderStateEnabled, ContentHash: profileDomain.ContentHash,
		CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	bindingDomain := generationdomain.ProjectProviderBindingVersion{
		ID: bindingID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		Purpose: generationdomain.ProviderPurposeReferenceAsset, Revision: revision,
		ConnectionVersionID: connectionID.String(), CredentialVersionID: credentialID.String(),
		ModelProfileVersionID: profileID.String(), ProviderKey: providerKey, Modality: generationdomain.MediaModalityImage,
		AdapterContractVersion: "controlled-image-v1", CreatedBy: fixture.ownerID.String(), CreatedAt: createdAt,
	}
	hashInput := bindingDomain
	hashInput.ID, hashInput.ContentHash, hashInput.CreatedBy, hashInput.CreatedAt = "", "", "", time.Time{}
	var err error
	bindingDomain.ContentHash, err = platformcommand.InputHash(hashInput)
	if err != nil {
		t.Fatalf("hash controlled Project Provider binding: %v", err)
	}
	binding := model.ProjectProviderBindingVersion{
		ID: bindingID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID,
		Purpose: bindingDomain.Purpose, Revision: revision, ConnectionVersionID: connectionID,
		CredentialVersionID: credentialID, ModelProfileVersionID: profileID, ProviderKey: providerKey,
		Modality: generationdomain.MediaModalityImage, AdapterContractVersion: "controlled-image-v1",
		ContentHash: bindingDomain.ContentHash, CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	for _, record := range []any{&credential, &connection, &profile, &binding} {
		if err = create(record); err != nil {
			t.Fatalf("seed controlled Provider configuration %T: %v", record, err)
		}
	}
	return generationapp.ResolvedProjectProviderBinding{
		Binding:    bindingDomain,
		Connection: connectionDomain,
		Credential: generationapp.ProviderCredentialView{ID: credentialID.String(), Revision: revision},
		Profile:    profileDomain,
	}
}

func controlledProviderConnectionContentHash(
	t *testing.T,
	value generationdomain.ProviderConnectionVersion,
) string {
	t.Helper()
	value.ID, value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", "", time.Time{}
	hash, err := platformcommand.InputHash(value)
	if err != nil {
		t.Fatalf("hash controlled Provider connection: %v", err)
	}
	return hash
}

func controlledProviderProfileContentHash(
	t *testing.T,
	value generationdomain.ProviderModelProfileVersion,
) string {
	t.Helper()
	value.ID, value.ContentHash, value.CreatedBy, value.CreatedAt = "", "", "", time.Time{}
	hash, err := platformcommand.InputHash(value)
	if err != nil {
		t.Fatalf("hash controlled Provider model profile: %v", err)
	}
	return hash
}
