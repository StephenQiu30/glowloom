package generation_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	costgorm "github.com/StephenQiu30/lanverse/backend/internal/cost/adapter/gormdb"
	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	quotagorm "github.com/StephenQiu30/lanverse/backend/internal/quota/adapter/gormdb"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
)

type controlledProviderFailure string

func (failure controlledProviderFailure) Error() string { return string(failure) }

func (failure controlledProviderFailure) ProviderFailureCode() string { return string(failure) }

type controlledProviderQueryFailure struct {
	kind    string
	message string
}

func (failure controlledProviderQueryFailure) Error() string { return failure.message }

func (failure controlledProviderQueryFailure) ProviderQueryFailureKind() string { return failure.kind }

type controlledProviderSubmitFailure struct {
	kind    string
	message string
}

func (failure controlledProviderSubmitFailure) Error() string { return failure.message }

func (failure controlledProviderSubmitFailure) ProviderSubmitFailureKind() string {
	return failure.kind
}

type controlledProviderGateway struct {
	mu sync.Mutex

	preflightFailures map[int]error
	submitOutcomes    map[int]generationapp.ProviderOutcome
	submitFailures    map[int]error
	queryOutcomes     map[int][]generationapp.ProviderOutcome
	queryFailures     map[int][]error
	preflightCalls    []generationapp.ProviderSubmission
	submitCalls       []generationapp.ProviderSubmission
	queryCalls        []generationapp.ProviderSubmission

	submitStarted chan struct{}
	releaseSubmit chan struct{}
	queryStarted  chan struct{}
	releaseQuery  chan struct{}
}

func (gateway *controlledProviderGateway) Preflight(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) error {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	gateway.preflightCalls = append(gateway.preflightCalls, submission)
	return gateway.preflightFailures[submission.CandidateIndex]
}

func (gateway *controlledProviderGateway) Submit(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	gateway.submitCalls = append(gateway.submitCalls, submission)
	outcome := gateway.submitOutcomes[submission.CandidateIndex]
	failure := gateway.submitFailures[submission.CandidateIndex]
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
	return controlledProviderOutcome(submission, outcome), failure
}

func (gateway *controlledProviderGateway) Query(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	gateway.mu.Lock()
	gateway.queryCalls = append(gateway.queryCalls, submission)
	started, release := gateway.queryStarted, gateway.releaseQuery
	failures := gateway.queryFailures[submission.CandidateIndex]
	if len(failures) != 0 {
		failure := failures[0]
		gateway.queryFailures[submission.CandidateIndex] = failures[1:]
		gateway.mu.Unlock()
		waitForControlledProviderCall(started, release)
		return generationapp.ProviderOutcome{}, failure
	}
	outcomes := gateway.queryOutcomes[submission.CandidateIndex]
	if len(outcomes) == 0 {
		gateway.mu.Unlock()
		return generationapp.ProviderOutcome{Status: generationapp.ProviderOutcomeUnknown}, nil
	}
	outcome := outcomes[0]
	gateway.queryOutcomes[submission.CandidateIndex] = outcomes[1:]
	gateway.mu.Unlock()
	waitForControlledProviderCall(started, release)
	return controlledProviderOutcome(submission, outcome), nil
}

func waitForControlledProviderCall(started, release chan struct{}) {
	if started != nil {
		started <- struct{}{}
	}
	if release != nil {
		<-release
	}
}

func controlledProviderOutcome(
	submission generationapp.ProviderSubmission,
	outcome generationapp.ProviderOutcome,
) generationapp.ProviderOutcome {
	if outcome.Output == nil {
		return outcome
	}
	cloned := *outcome.Output
	if cloned.StagingObjectKey == "" {
		cloned.StagingObjectKey = "staging/" + submission.WorkspaceID + "/" + submission.ProviderJobID + "/" +
			submission.ProviderCallID + "/" + cloned.OutputKey + ".png"
	}
	outcome.Output = &cloned
	return outcome
}

func (gateway *controlledProviderGateway) counts() (preflight, submit, query int) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	return len(gateway.preflightCalls), len(gateway.submitCalls), len(gateway.queryCalls)
}

func (gateway *controlledProviderGateway) submissions() (submit, query []generationapp.ProviderSubmission) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	return append([]generationapp.ProviderSubmission(nil), gateway.submitCalls...),
		append([]generationapp.ProviderSubmission(nil), gateway.queryCalls...)
}

var (
	_ generationapp.ProviderGateway       = (*controlledProviderGateway)(nil)
	_ generationapp.ProviderLocalFailure  = controlledProviderFailure("")
	_ generationapp.ProviderQueryFailure  = controlledProviderQueryFailure{}
	_ generationapp.ProviderSubmitFailure = controlledProviderSubmitFailure{}
)

func TestProviderCallsUseOneDispatchBoundaryPerCandidateAndSettleExactUsage(t *testing.T) {
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
	assertProviderSchema(t, database.Migrator().HasConstraint, database.Migrator().HasIndex)

	now := time.Date(2026, time.August, 30, 15, 0, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), now, "provider-calls")
	fixture.provider = seedControlledProjectProviderBinding(t, create, fixture, "controlled-image", "image-quality", 1)
	t.Cleanup(func() {
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return generationtestgorm.DeleteWithoutHooks(database, value, query, arguments...)
		}, fixture)
	})

	costConfig := costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL},
	)

	gateway := &controlledProviderGateway{
		preflightFailures: map[int]error{2: controlledProviderFailure("provider.invalid_prompt")},
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			1: providerSuccess("candidate-1", "event-1", strings.Repeat("1", 64)),
			3: {
				Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "remote-request-3",
				RemoteJobID: "remote-task-3", QueryDeadlineAt: now.Add(6 * time.Hour),
				RemoteExpiresAt: now.Add(24 * time.Hour),
			},
			4: {
				Status: generationapp.ProviderOutcomeFailed, RemoteRequestID: "remote-request-4",
				ProviderEventID: "event-4", FailureCode: "provider.content_rejected",
			},
		},
		queryOutcomes: map[int][]generationapp.ProviderOutcome{
			3: {
				{Status: generationapp.ProviderOutcomeRunning, RemoteRequestID: "remote-request-3", RemoteJobID: "remote-task-3"},
				providerSuccess("candidate-3", "event-3", strings.Repeat("3", 64)),
			},
		},
		queryFailures: map[int][]error{
			3: {controlledProviderQueryFailure{
				kind:    generationapp.ProviderQueryFailureRetryable,
				message: "temporary upstream failure containing should-not-leak-secret",
			}},
		},
	}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	claim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, "four-calls", strings.Repeat("a", 64))
	assertGenerationIntentDatabaseState(t, database, claim.Intent)

	first := submitProvider(t, ctx, providers, claim, "provider-submit-1")
	if first.Job.CallCount != 4 || len(first.Calls) != 4 || first.Calls[0].Status != generationdomain.ProviderCallSucceeded {
		t.Fatalf("first Candidate did not create four exact Calls and complete Call 1: %#v", first)
	}
	second := submitProvider(t, ctx, providers, claim, "provider-submit-2")
	if second.Calls[1].Status != generationdomain.ProviderCallFailed || second.Calls[1].LocalFailureCode != "provider.invalid_prompt" ||
		second.Calls[1].DispatchBoundaryEnteredAt != nil {
		t.Fatalf("local preflight failure crossed the dispatch boundary: %#v", second.Calls[1])
	}
	thirdSubmitted := submitProvider(t, ctx, providers, claim, "provider-submit-3")
	if thirdSubmitted.Calls[2].Status != generationdomain.ProviderCallSubmitted ||
		thirdSubmitted.Calls[2].RemoteJobID != "remote-task-3" {
		t.Fatalf("asynchronous Provider task was not persisted: %#v", thirdSubmitted.Calls[2])
	}
	transientKey := "provider-query-3-transient"
	transient, transientErr := providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: thirdSubmitted.Job.ID, IdempotencyKey: transientKey,
	})
	if !generationapp.IsCode(transientErr, "provider_query_temporarily_unavailable") ||
		strings.Contains(transientErr.Error(), "should-not-leak-secret") ||
		transient.Calls[2].Status != generationdomain.ProviderCallSubmitted ||
		transient.Calls[2].RemoteJobID != "remote-task-3" {
		t.Fatalf("temporary Provider query was not safely retryable: result=%#v err=%v", transient, transientErr)
	}
	thirdRunning := reconcileProvider(t, ctx, providers, thirdSubmitted.Job.ID, transientKey)
	if thirdRunning.Calls[2].Status != generationdomain.ProviderCallRunning ||
		thirdRunning.Calls[2].RemoteJobID != "remote-task-3" {
		t.Fatalf("asynchronous Provider task did not remain bound while running: %#v", thirdRunning.Calls[2])
	}
	thirdSucceeded := reconcileProvider(t, ctx, providers, thirdSubmitted.Job.ID, "provider-query-3-succeeded")
	if thirdSucceeded.Calls[2].Status != generationdomain.ProviderCallSucceeded {
		t.Fatalf("asynchronous Provider task did not reach terminal success: %#v", thirdSucceeded.Calls[2])
	}
	terminal := submitProvider(t, ctx, providers, claim, "provider-submit-4")
	if terminal.Job.Status != generationdomain.ProviderJobPartialSucceeded ||
		terminal.Intent.Status != generationdomain.IntentPartialSucceeded || terminal.Job.CallCount != 4 ||
		terminal.Job.DispatchedCallCount != 3 || terminal.Job.SucceededCallCount != 2 || terminal.Job.FailedCallCount != 2 ||
		len(terminal.Calls) != 4 || len(terminal.Receipts) != 3 {
		t.Fatalf("four-Call partial Provider aggregate drifted: %#v", terminal)
	}

	for index, call := range terminal.Calls {
		if call.CandidateIndex != index+1 || call.RequestedOutputCount != 1 || call.CallKey != "generation-call:"+call.ID ||
			len(call.RequestHash) != 64 || len(call.ContentHash) != 64 {
			t.Fatalf("Call %d lost its exact per-Candidate request boundary: %#v", index+1, call)
		}
		if index != 1 && call.DispatchBoundaryEnteredAt == nil {
			t.Fatalf("Call %d did not record its paid dispatch boundary: %#v", index+1, call)
		}
	}
	assertProviderReceiptOwnership(t, terminal)
	assertProviderImmutableFacts(t, database, terminal)
	preflightCount, submitCount, queryCount := gateway.counts()
	if preflightCount != 4 || submitCount != 3 || queryCount != 3 {
		t.Fatalf("Provider boundary counts = %d/%d/%d, want preflight=4 submit=3 query=3",
			preflightCount, submitCount, queryCount)
	}
	remoteSubmits, remoteQueries := gateway.submissions()
	if len(remoteSubmits) != 3 || len(remoteQueries) != 3 ||
		remoteQueries[0].ProviderCallID != terminal.Calls[2].ID ||
		remoteQueries[1].ProviderCallID != terminal.Calls[2].ID ||
		remoteQueries[2].ProviderCallID != terminal.Calls[2].ID ||
		remoteQueries[0].RemoteJobID != "remote-task-3" ||
		remoteQueries[1].RemoteJobID != "remote-task-3" ||
		remoteQueries[2].RemoteJobID != "remote-task-3" {
		t.Fatalf("remote task query binding drifted: submits=%#v queries=%#v", remoteSubmits, remoteQueries)
	}
	for _, submission := range append(remoteSubmits, remoteQueries...) {
		call := terminal.Calls[submission.CandidateIndex-1]
		if submission.CallRequestHash != call.RequestHash || submission.BindingID != terminal.Request.BindingID ||
			submission.BindingRevision != terminal.Request.BindingRevision ||
			submission.BindingContentHash != terminal.Request.BindingContentHash ||
			submission.ModelProfileVersionID != terminal.Request.ModelProfileVersionID ||
			submission.ModelProfileRevision != terminal.Request.ModelProfileRevision ||
			submission.ModelProfileContentHash != terminal.Request.ModelProfileContentHash ||
			submission.PriceQuoteID != terminal.Request.PriceQuoteID ||
			submission.PriceQuoteRevision != terminal.Request.PriceQuoteRevision ||
			submission.PriceQuoteContentHash != terminal.Request.PriceQuoteContentHash ||
			submission.BillingMetric != terminal.Request.BillingMetric ||
			submission.EstimatedUnits != terminal.Request.EstimatedUnits {
			t.Fatalf("Provider submission did not carry the exact frozen execution facts: %#v", submission)
		}
	}
	for _, submission := range remoteQueries {
		if submission.QueryDeadlineAt == nil || !submission.QueryDeadlineAt.Equal(now.Add(6*time.Hour)) ||
			submission.RemoteExpiresAt == nil || !submission.RemoteExpiresAt.Equal(now.Add(24*time.Hour)) {
			t.Fatalf("Provider query lost its persisted retention window: %#v", submission)
		}
	}

	var terminalOwnerReceipt model.CommandReceipt
	if err = database.Where("operation = ? AND resource_id = ?", "generation.provider.terminal", terminal.Job.ID).
		First(&terminalOwnerReceipt).Error; err != nil {
		t.Fatalf("load dedicated Provider terminal receipt: %v", err)
	}
	if terminalOwnerReceipt.ID.String() == terminal.Receipt.ID || terminal.Receipt.Operation == "generation.provider.terminal" {
		t.Fatalf("Provider command receipt was confused with the terminal owner receipt: command=%#v terminal=%#v",
			terminal.Receipt, terminalOwnerReceipt)
	}
	costView, err := costs.GetReservation(ctx, costapp.Actor{
		UserID: fixture.editor.UserID, TokenVersion: fixture.editor.TokenVersion,
	}, terminal.Intent.CostReservationID)
	if err != nil || costView.Reservation.Status != costdomain.ReservationSettled ||
		costView.Reservation.EstimatedUnits != 4 || costView.Reservation.SettledUnits != 3 ||
		costView.Reservation.UsageReceiptID == nil ||
		*costView.Reservation.UsageReceiptID != terminalOwnerReceipt.ID.String() {
		t.Fatalf("Cost did not settle by the three dispatch boundaries: view=%#v err=%v", costView, err)
	}
	quotaReservation, err := quotas.GetReservation(ctx, quotaapp.Actor{
		UserID: fixture.editor.UserID, TokenVersion: fixture.editor.TokenVersion,
	}, terminal.Intent.QuotaReservationID)
	if err != nil || quotaReservation.Status != quotadomain.ReservationConsumed || quotaReservation.Units != 4 {
		t.Fatalf("Quota did not consume the frozen four-Call Target once: reservation=%#v err=%v", quotaReservation, err)
	}
	quotaUsage, err := quotas.GetDailyUsage(ctx, quotaapp.Actor{
		UserID: fixture.editor.UserID, TokenVersion: fixture.editor.TokenVersion,
	}, fixture.projectID.String(), quotadomain.MetricGenerationImageCall)
	if err != nil || quotaUsage.ConsumedUnits != 4 || quotaUsage.ReservedUnits != 0 {
		t.Fatalf("Quota usage did not consume the full frozen Target: usage=%#v err=%v", quotaUsage, err)
	}

	replayed := submitProvider(t, ctx, providers, claim, "provider-submit-4")
	if replayed.Job.ID != terminal.Job.ID || replayed.Receipt.ID != terminal.Receipt.ID {
		t.Fatalf("terminal Provider submission did not replay: first=%#v replay=%#v", terminal, replayed)
	}
	afterPreflight, afterSubmit, afterQuery := gateway.counts()
	if afterPreflight != preflightCount || afterSubmit != submitCount || afterQuery != queryCount {
		t.Fatalf("terminal replay reached Provider again: before=%d/%d/%d after=%d/%d/%d",
			preflightCount, submitCount, queryCount, afterPreflight, afterSubmit, afterQuery)
	}

	allLocalGateway := &controlledProviderGateway{preflightFailures: map[int]error{}}
	for candidate := 1; candidate <= 4; candidate++ {
		allLocalGateway.preflightFailures[candidate] = controlledProviderFailure("provider.invalid_candidate_" + strconv.Itoa(candidate))
	}
	allLocalProviders := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), allLocalGateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	allLocalClaim := prepareAndClaimProviderIntent(
		t, ctx, preparations, fixture, "all-local-failures", strings.Repeat("b", 64),
	)
	var allLocal generationapp.ProviderExecutionResult
	for candidate := 1; candidate <= 4; candidate++ {
		allLocal = submitProvider(t, ctx, allLocalProviders, allLocalClaim, "provider-all-local-"+strconv.Itoa(candidate))
	}
	if allLocal.Job.Status != generationdomain.ProviderJobFailed || allLocal.Intent.Status != generationdomain.IntentFailed ||
		allLocal.Job.DispatchedCallCount != 0 || allLocal.Job.FailedCallCount != 4 || len(allLocal.Receipts) != 0 {
		t.Fatalf("all-local Provider failures did not terminally release before dispatch: %#v", allLocal)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, allLocal.Intent,
		costdomain.ReservationReleased, quotadomain.ReservationReleased)
	allLocalPreflight, allLocalSubmit, allLocalQuery := allLocalGateway.counts()
	if allLocalPreflight != 4 || allLocalSubmit != 0 || allLocalQuery != 0 {
		t.Fatalf("all-local failures crossed Provider boundary: %d/%d/%d", allLocalPreflight, allLocalSubmit, allLocalQuery)
	}
}

func TestAsyncProviderDeadlineBecomesOutcomeUnknownWithoutAnotherRemoteQuery(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the asynchronous Provider deadline journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open asynchronous Provider deadline database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize asynchronous Provider deadline GORM catalog: %v", err)
	}

	startedAt := time.Date(2026, time.August, 30, 15, 30, 0, 0, time.UTC)
	currentTime := startedAt
	deadlineAt := startedAt.Add(2 * time.Hour)
	remoteExpiresAt := startedAt.Add(26 * time.Hour)
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), startedAt, "provider-deadline")
	fixture.provider = seedControlledProjectProviderBinding(t, create, fixture, "controlled-image", "image-quality", 1)
	t.Cleanup(func() {
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return generationtestgorm.DeleteWithoutHooks(database, value, query, arguments...)
		}, fixture)
	})
	costConfig := costapp.Config{Now: func() time.Time { return startedAt }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return startedAt }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return startedAt }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL},
	)

	gateway := &controlledProviderGateway{
		preflightFailures: map[int]error{
			1: controlledProviderFailure("provider.invalid_candidate_1"),
			2: controlledProviderFailure("provider.invalid_candidate_2"),
			3: controlledProviderFailure("provider.invalid_candidate_3"),
		},
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			4: {
				Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "remote-request-deadline",
				RemoteJobID: "remote-task-deadline", QueryDeadlineAt: deadlineAt,
				RemoteExpiresAt: remoteExpiresAt,
			},
		},
		queryOutcomes: map[int][]generationapp.ProviderOutcome{
			4: {
				{Status: generationapp.ProviderOutcomeAccepted},
				{Status: generationapp.ProviderOutcomeRunning},
				{Status: generationapp.ProviderOutcomeRunning},
			},
		},
	}
	newProviderService := func() *generationapp.ProviderService {
		return generationapp.NewProviderService(
			generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
			generationapp.ProviderConfig{Now: func() time.Time { return currentTime }, NewID: uuid.NewString},
		)
	}
	providers := newProviderService()
	claim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, "deadline", strings.Repeat("e", 64))
	var submitted generationapp.ProviderExecutionResult
	for candidate := 1; candidate <= 4; candidate++ {
		submitted = submitProvider(t, ctx, providers, claim, "provider-deadline-submit-"+strconv.Itoa(candidate))
	}
	if submitted.Job.Status != generationdomain.ProviderJobRunning ||
		submitted.Calls[3].Status != generationdomain.ProviderCallSubmitted ||
		submitted.Calls[3].QueryDeadlineAt == nil || !submitted.Calls[3].QueryDeadlineAt.Equal(deadlineAt) ||
		submitted.Calls[3].RemoteExpiresAt == nil || !submitted.Calls[3].RemoteExpiresAt.Equal(remoteExpiresAt) {
		t.Fatalf("asynchronous Provider deadline was not persisted: %#v", submitted)
	}
	invalidRetention := database.Begin()
	if invalidRetention.Error != nil {
		t.Fatalf("begin invalid Provider retention transaction: %v", invalidRetention.Error)
	}
	invalidRetentionErr := invalidRetention.Model(&model.GenerationProviderCall{}).
		Where("id = ?", submitted.Calls[3].ID).
		Update("remote_expires_at", deadlineAt).Error
	if rollbackErr := invalidRetention.Rollback().Error; rollbackErr != nil {
		t.Fatalf("rollback invalid Provider retention transaction: %v", rollbackErr)
	}
	if invalidRetentionErr == nil || !strings.Contains(invalidRetentionErr.Error(), "ck_gen_provider_call_boundary") {
		t.Fatalf("PostgreSQL accepted query_deadline_at >= remote_expires_at: %v", invalidRetentionErr)
	}
	pollKey := "provider-deadline-poll"
	accepted := reconcileProvider(t, ctx, providers, submitted.Job.ID, pollKey)
	if accepted.Calls[3].Status != generationdomain.ProviderCallSubmitted || accepted.Receipt.ID != "" {
		t.Fatalf("repeated ACCEPTED observation became an immutable replay point: %#v", accepted)
	}
	running := reconcileProvider(t, ctx, providers, submitted.Job.ID, pollKey)
	if running.Calls[3].Status != generationdomain.ProviderCallRunning || running.Receipt.ID == "" {
		t.Fatalf("RUNNING state advance did not persist its command receipt: %#v", running)
	}
	noProgressKey := "provider-deadline-running-no-progress"
	noProgress := reconcileProvider(t, ctx, providers, submitted.Job.ID, noProgressKey)
	if noProgress.Calls[3].Status != generationdomain.ProviderCallRunning || noProgress.Receipt.ID != "" {
		t.Fatalf("repeated RUNNING observation became an immutable replay point: %#v", noProgress)
	}

	currentTime = deadlineAt
	restarted := newProviderService()
	expired := reconcileProvider(t, ctx, restarted, submitted.Job.ID, noProgressKey)
	if expired.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
		expired.Intent.Status != generationdomain.IntentOutcomeUnknown ||
		expired.Calls[3].Status != generationdomain.ProviderCallOutcomeUnknown ||
		expired.Calls[3].RemoteJobID != "remote-task-deadline" ||
		expired.Calls[3].RemoteExpiresAt == nil || !expired.Calls[3].RemoteExpiresAt.Equal(remoteExpiresAt) ||
		len(expired.Receipts) != 0 {
		t.Fatalf("expired asynchronous Provider task did not become explicit OUTCOME_UNKNOWN: %#v", expired)
	}
	preflightCount, submitCount, queryCount := gateway.counts()
	if preflightCount != 4 || submitCount != 1 || queryCount != 3 {
		t.Fatalf("expired asynchronous task crossed another remote boundary: %d/%d/%d",
			preflightCount, submitCount, queryCount)
	}
	_, deadlineQueries := gateway.submissions()
	if len(deadlineQueries) != 3 {
		t.Fatalf("asynchronous Provider query count drifted: %#v", deadlineQueries)
	}
	for _, query := range deadlineQueries {
		if query.RemoteRequestID != "remote-request-deadline" || query.RemoteJobID != "remote-task-deadline" ||
			query.QueryDeadlineAt == nil || !query.QueryDeadlineAt.Equal(deadlineAt) ||
			query.RemoteExpiresAt == nil || !query.RemoteExpiresAt.Equal(remoteExpiresAt) {
			t.Fatalf("asynchronous Provider query lost its frozen remote retention binding: %#v", query)
		}
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, expired.Intent,
		costdomain.ReservationReserved, quotadomain.ReservationReserved)

	replayed := reconcileProvider(t, ctx, newProviderService(), expired.Job.ID, "provider-deadline-replay")
	if replayed.Job.Status != generationdomain.ProviderJobOutcomeUnknown || replayed.Calls[3].Status != generationdomain.ProviderCallOutcomeUnknown {
		t.Fatalf("expired asynchronous Provider result did not replay after restart: %#v", replayed)
	}
	_, _, replayQueryCount := gateway.counts()
	if replayQueryCount != 3 {
		t.Fatalf("expired asynchronous Provider result made another remote query on replay: %d", replayQueryCount)
	}

	currentTime = startedAt
	identityGateway := &controlledProviderGateway{
		preflightFailures: map[int]error{
			1: controlledProviderFailure("provider.invalid_candidate_1"),
			2: controlledProviderFailure("provider.invalid_candidate_2"),
			3: controlledProviderFailure("provider.invalid_candidate_3"),
		},
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			4: {
				Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "remote-request-lost",
				RemoteJobID: "remote-task-lost", QueryDeadlineAt: startedAt.Add(3 * time.Hour),
				RemoteExpiresAt: startedAt.Add(27 * time.Hour),
			},
		},
		queryFailures: map[int][]error{
			4: {controlledProviderQueryFailure{
				kind: generationapp.ProviderQueryFailureIdentityUnrecoverable, message: "remote task identity is gone",
			}},
		},
	}
	identityProviders := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), identityGateway,
		generationapp.ProviderConfig{Now: func() time.Time { return currentTime }, NewID: uuid.NewString},
	)
	identityClaim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, "identity-lost", strings.Repeat("1", 64))
	var identitySubmitted generationapp.ProviderExecutionResult
	for candidate := 1; candidate <= 4; candidate++ {
		identitySubmitted = submitProvider(
			t, ctx, identityProviders, identityClaim, "provider-identity-submit-"+strconv.Itoa(candidate),
		)
	}
	identityUnknown := reconcileProvider(
		t, ctx, identityProviders, identitySubmitted.Job.ID, "provider-identity-query",
	)
	if identityUnknown.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
		identityUnknown.Intent.Status != generationdomain.IntentOutcomeUnknown ||
		identityUnknown.Calls[3].Status != generationdomain.ProviderCallOutcomeUnknown ||
		identityUnknown.Calls[3].RemoteJobID != "remote-task-lost" {
		t.Fatalf("unrecoverable Provider identity did not become explicit OUTCOME_UNKNOWN: %#v", identityUnknown)
	}
	_, identitySubmitCount, identityQueryCount := identityGateway.counts()
	if identitySubmitCount != 1 || identityQueryCount != 1 {
		t.Fatalf("unrecoverable Provider identity crossed the wrong remote boundaries: submit=%d query=%d",
			identitySubmitCount, identityQueryCount)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, identityUnknown.Intent,
		costdomain.ReservationReserved, quotadomain.ReservationReserved)
}

func TestConcurrentTerminalProviderOutcomesFenceTheLosingResult(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Provider terminal outcome fencing journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Provider terminal outcome fencing database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Provider terminal outcome fencing GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 30, 15, 45, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), now, "provider-terminal-race")
	fixture.provider = seedControlledProjectProviderBinding(t, create, fixture, "controlled-image", "image-quality", 1)
	t.Cleanup(func() {
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return generationtestgorm.DeleteWithoutHooks(database, value, query, arguments...)
		}, fixture)
	})
	costConfig := costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL},
	)

	queryStarted, releaseQuery := make(chan struct{}, 2), make(chan struct{})
	gateway := &controlledProviderGateway{
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			1: {
				Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: "remote-request-race",
				RemoteJobID: "remote-task-race", QueryDeadlineAt: now.Add(2 * time.Hour),
				RemoteExpiresAt: now.Add(26 * time.Hour),
			},
		},
		queryOutcomes: map[int][]generationapp.ProviderOutcome{
			1: {
				providerSuccess("candidate-race", "event-race-success", strings.Repeat("f", 64)),
				{
					Status: generationapp.ProviderOutcomeFailed, ProviderEventID: "event-race-failure",
					FailureCode: "provider.content_rejected",
				},
			},
		},
		queryStarted: queryStarted,
		releaseQuery: releaseQuery,
	}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	claim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, "terminal-race", strings.Repeat("f", 64))
	submitted := submitProvider(t, ctx, providers, claim, "provider-terminal-race-submit")

	type reconcileResult struct {
		result generationapp.ProviderExecutionResult
		err    error
	}
	results := make(chan reconcileResult, 2)
	for index := 1; index <= 2; index++ {
		key := "provider-terminal-race-query-" + strconv.Itoa(index)
		go func() {
			result, reconcileErr := providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
				ProviderJobID: submitted.Job.ID, IdempotencyKey: key,
			})
			results <- reconcileResult{result: result, err: reconcileErr}
		}()
	}
	for index := 0; index < 2; index++ {
		select {
		case <-queryStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent Provider queries did not both cross the remote boundary")
		}
	}
	close(releaseQuery)

	succeeded, conflicted := 0, 0
	for index := 0; index < 2; index++ {
		select {
		case value := <-results:
			if value.err == nil {
				succeeded++
				if value.result.Calls[0].Status != generationdomain.ProviderCallSucceeded &&
					value.result.Calls[0].Status != generationdomain.ProviderCallFailed {
					t.Fatalf("winning terminal Provider result did not persist: %#v", value.result)
				}
			} else if generationapp.IsCode(value.err, "provider_outcome_conflict") {
				conflicted++
			} else {
				t.Fatalf("losing terminal Provider result returned the wrong error: %v", value.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent Provider terminal reconciliation did not finish")
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("terminal Provider fencing results = success:%d conflict:%d, want 1/1", succeeded, conflicted)
	}
	var receiptCount int64
	if err = database.Model(&model.GenerationProviderResultReceipt{}).
		Where("call_id = ?", submitted.Calls[0].ID).Count(&receiptCount).Error; err != nil || receiptCount != 1 {
		t.Fatalf("terminal Provider fencing persisted %d receipts: %v", receiptCount, err)
	}
	_, _, queryCount := gateway.counts()
	if queryCount != 2 {
		t.Fatalf("terminal Provider fencing made %d remote queries, want 2 concurrent in-flight queries", queryCount)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, submitted.Intent,
		costdomain.ReservationReserved, quotadomain.ReservationReserved)
}

func TestDispatchingRecoveryBecomesOutcomeUnknownWithoutQueryOrResubmit(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Generation Provider recovery journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Generation Provider recovery database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Generation Provider recovery GORM catalog: %v", err)
	}

	now := time.Date(2026, time.August, 30, 16, 0, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedPreparationFixture(t, create, generationgorm.NewTargetStore(database), now, "provider-recovery")
	fixture.provider = seedControlledProjectProviderBinding(t, create, fixture, "controlled-image", "image-quality", 1)
	t.Cleanup(func() {
		cleanupProviderFixture(t, func(value any, query string, arguments ...any) error {
			return generationtestgorm.DeleteWithoutHooks(database, value, query, arguments...)
		}, fixture)
	})
	costConfig := costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	quotaConfig := quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString}
	costs := costapp.NewService(costgorm.New(database), costConfig)
	quotas := quotaapp.NewService(quotagorm.New(database), quotaConfig)
	configurePreparationLimits(t, ctx, costs, quotas, fixture, "1000.000000", "10.000000", 100)
	preparations := generationapp.NewPreparationService(
		generationgorm.NewPreparationStore(database, costConfig, quotaConfig),
		generationapp.PreparationConfig{Now: func() time.Time { return now }, NewID: uuid.NewString, ClaimTTL: preparationClaimTTL},
	)

	started, release := make(chan struct{}, 1), make(chan struct{})
	gateway := &controlledProviderGateway{
		submitOutcomes: map[int]generationapp.ProviderOutcome{
			1: providerSuccess("candidate-1", "event-late", strings.Repeat("c", 64)),
		},
		submitStarted: started,
		releaseSubmit: release,
	}
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	claim := prepareAndClaimProviderIntent(t, ctx, preparations, fixture, "dispatch-recovery", strings.Repeat("d", 64))
	type submitResult struct {
		result generationapp.ProviderExecutionResult
		err    error
	}
	submitDone := make(chan submitResult, 1)
	go func() {
		result, submitErr := providers.SubmitImageRequest(ctx, claim.Authorization, generationapp.SubmitImageRequestCommand{
			IntentID: claim.Intent.ID, IdempotencyKey: "provider-recovery-submit",
		})
		submitDone <- submitResult{result: result, err: submitErr}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Provider submission did not enter the remote dispatch gap")
	}
	var job model.GenerationProviderJob
	if err = database.Where("intent_id = ?", claim.Intent.ID).First(&job).Error; err != nil {
		t.Fatalf("load dispatching Provider job: %v", err)
	}
	var dispatching model.GenerationProviderCall
	if err = database.Where("job_id = ? AND candidate_index = ?", job.ID, 1).
		First(&dispatching).Error; err != nil || dispatching.Status != generationdomain.ProviderCallDispatching ||
		dispatching.DispatchBoundaryEnteredAt == nil {
		t.Fatalf("dispatch fence was not committed before remote Submit: call=%#v err=%v", dispatching, err)
	}
	restarted := generationapp.NewProviderService(
		generationgorm.NewProviderStore(database, costConfig, quotaConfig), gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	recovered := reconcileProvider(t, ctx, restarted, job.ID.String(), "provider-recovery-reconcile")
	if recovered.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
		recovered.Intent.Status != generationdomain.IntentOutcomeUnknown || len(recovered.Calls) != 4 ||
		recovered.Calls[0].Status != generationdomain.ProviderCallOutcomeUnknown ||
		recovered.Calls[1].Status != generationdomain.ProviderCallPending ||
		recovered.Calls[2].Status != generationdomain.ProviderCallPending ||
		recovered.Calls[3].Status != generationdomain.ProviderCallPending || len(recovered.Receipts) != 0 {
		t.Fatalf("DISPATCHING recovery did not become explicit OUTCOME_UNKNOWN: %#v", recovered)
	}
	preflightCount, submitCount, queryCount := gateway.counts()
	if preflightCount != 1 || submitCount != 1 || queryCount != 0 {
		t.Fatalf("DISPATCHING recovery queried or resubmitted remote work: %d/%d/%d",
			preflightCount, submitCount, queryCount)
	}
	assertProviderReservations(t, ctx, costs, quotas, fixture, recovered.Intent,
		costdomain.ReservationReserved, quotadomain.ReservationReserved)

	close(release)
	select {
	case late := <-submitDone:
		if late.err != nil || late.result.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
			late.result.Calls[0].Status != generationdomain.ProviderCallOutcomeUnknown || len(late.result.Receipts) != 0 {
			t.Fatalf("late Provider success crossed the recovery fence: result=%#v err=%v", late.result, late.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late Provider submission did not leave the recovery gap")
	}
	resubmitted := submitProvider(t, ctx, restarted, claim, "provider-recovery-submit-after-unknown")
	reconciled := reconcileProvider(t, ctx, restarted, recovered.Job.ID, "provider-recovery-reconcile-after-unknown")
	if resubmitted.Job.Status != generationdomain.ProviderJobOutcomeUnknown ||
		reconciled.Job.Status != generationdomain.ProviderJobOutcomeUnknown {
		t.Fatalf("OUTCOME_UNKNOWN was not terminal for automatic execution: submit=%#v reconcile=%#v", resubmitted, reconciled)
	}
	afterPreflight, afterSubmit, afterQuery := gateway.counts()
	if afterPreflight != 1 || afterSubmit != 1 || afterQuery != 0 {
		t.Fatalf("OUTCOME_UNKNOWN retried remote work: %d/%d/%d", afterPreflight, afterSubmit, afterQuery)
	}
}

func providerSuccess(outputKey, eventID, sha string) generationapp.ProviderOutcome {
	return generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeSucceeded, ProviderEventID: eventID,
		Output: &generationapp.ProviderOutput{
			OutputKey: outputKey, SHA256: sha, MediaType: "image/png", Bytes: 128, Width: 1536, Height: 1024,
		},
		ProviderUsageObservation: generationdomain.ProviderUsageObservation{ImageCount: 1},
	}
}

func submitProvider(
	t *testing.T,
	ctx context.Context,
	providers *generationapp.ProviderService,
	claim generationapp.ExecutionClaimResult,
	idempotencyKey string,
) generationapp.ProviderExecutionResult {
	t.Helper()
	result, err := providers.SubmitImageRequest(ctx, claim.Authorization, generationapp.SubmitImageRequestCommand{
		IntentID: claim.Intent.ID, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("submit Provider Call %s: %v", idempotencyKey, err)
	}
	return result
}

func reconcileProvider(
	t *testing.T,
	ctx context.Context,
	providers *generationapp.ProviderService,
	providerJobID, idempotencyKey string,
) generationapp.ProviderExecutionResult {
	t.Helper()
	result, err := providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
		ProviderJobID: providerJobID, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("reconcile Provider Job %s: %v", idempotencyKey, err)
	}
	return result
}

func assertProviderReceiptOwnership(t *testing.T, result generationapp.ProviderExecutionResult) {
	t.Helper()
	calls := make(map[string]generationdomain.ProviderCall, len(result.Calls))
	for _, call := range result.Calls {
		calls[call.ID] = call
	}
	for _, receipt := range result.Receipts {
		call, exists := calls[receipt.CallID]
		if !exists || receipt.WorkspaceID != call.WorkspaceID || receipt.ProjectID != call.ProjectID ||
			(receipt.Status == generationdomain.ProviderResultSucceeded &&
				(receipt.Output == nil || receipt.OutputCount != 1 ||
					!strings.HasPrefix(receipt.Output.StagingObjectKey,
						"staging/"+call.WorkspaceID+"/"+result.Job.ID+"/"+call.ID+"/"))) ||
			(receipt.Status == generationdomain.ProviderResultFailed && (receipt.Output != nil || receipt.OutputCount != 0)) {
			t.Fatalf("Provider receipt is not owned by one exact Call: call=%#v receipt=%#v", call, receipt)
		}
	}
}

func prepareAndClaimProviderIntent(
	t *testing.T,
	ctx context.Context,
	preparations *generationapp.PreparationService,
	fixture preparationFixture,
	suffix, hash string,
) generationapp.ExecutionClaimResult {
	t.Helper()
	prepared := prepareDistinctIntent(t, ctx, preparations, fixture, "provider-"+suffix, hash)
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
	hasConstraint func(any, string) bool,
	hasIndex func(any, string) bool,
) {
	t.Helper()
	constraints := []struct {
		value any
		name  string
	}{
		{&model.GenerationIntent{}, "ck_gen_intent_state"},
		{&model.GenerationIntent{}, "ck_gen_intent_owner_refs"},
		{&model.GenerationIntent{}, "ck_gen_intent_terminal_refs"},
		{&model.GenerationIntent{}, "ck_gen_intent_provider_refs"},
		{&model.GenerationProviderJob{}, "ck_gen_provider_job_state"},
		{&model.GenerationProviderCall{}, "ck_gen_provider_call_state"},
		{&model.GenerationProviderCall{}, "ck_gen_provider_call_boundary"},
		{&model.GenerationProviderCall{}, "ck_gen_provider_call_output_count"},
		{&model.GenerationProviderResultReceipt{}, "ck_gen_provider_receipt_status"},
		{&model.GenerationProviderResultReceipt{}, "ck_gen_provider_receipt_output_count"},
		{&model.GenerationProviderResultReceipt{}, "ck_gen_provider_receipt_result"},
	}
	for _, constraint := range constraints {
		if !hasConstraint(constraint.value, constraint.name) {
			t.Fatalf("Generation Provider schema is missing constraint %s", constraint.name)
		}
	}
	indexes := []struct {
		value any
		name  string
	}{
		{&model.GenerationRequest{}, "uq_gen_request_intent"},
		{&model.GenerationProviderJob{}, "uq_gen_provider_job_request"},
		{&model.GenerationProviderCall{}, "uq_gen_provider_call_candidate"},
		{&model.GenerationProviderCall{}, "uq_gen_provider_call_key"},
		{&model.GenerationProviderResultReceipt{}, "uq_gen_provider_receipt_call"},
	}
	for _, index := range indexes {
		if !hasIndex(index.value, index.name) {
			t.Fatalf("Generation Provider schema is missing index %s", index.name)
		}
	}
}

func assertGenerationIntentDatabaseState(
	t *testing.T,
	database *generationtestgorm.Database,
	claimed generationdomain.Intent,
) {
	t.Helper()
	var record model.GenerationIntent
	if err := database.Where("id = ?", claimed.ID).First(&record).Error; err != nil {
		t.Fatalf("load claimed Generation intent for database state checks: %v", err)
	}
	if record.Status != generationdomain.IntentClaimed || record.CostEstimateReceiptID == nil ||
		record.CostReservationReceiptID == nil || record.QuotaReservationReceiptID == nil {
		t.Fatalf("claimed Generation intent fixture is incomplete: %#v", record)
	}
	requestID, jobID := uuid.New(), uuid.New()
	providerHash := strings.Repeat("f", 64)
	tests := []struct {
		name       string
		constraint string
		values     map[string]any
	}{
		{
			name: "preparing rejects an orphan Owner receipt", constraint: "ck_gen_intent_owner_refs",
			values: map[string]any{
				"status": generationdomain.IntentPreparing, "revision": int64(1),
				"cost_estimate_id": nil, "cost_reservation_id": nil, "quota_reservation_id": nil,
				"cost_estimate_receipt_id":    *record.CostEstimateReceiptID,
				"cost_reservation_receipt_id": nil, "quota_reservation_receipt_id": nil,
				"claimant": nil, "claim_token": nil, "claim_expires_at": nil, "claim_fencing_version": int64(0),
			},
		},
		{
			name: "prepared rejects the claimed revision", constraint: "ck_gen_intent_state",
			values: map[string]any{
				"status": generationdomain.IntentPrepared, "revision": int64(3),
				"claimant": nil, "claim_token": nil, "claim_expires_at": nil, "claim_fencing_version": int64(0),
			},
		},
		{
			name: "executing rejects a missing initial Owner receipt", constraint: "ck_gen_intent_owner_refs",
			values: map[string]any{
				"status": generationdomain.IntentExecuting, "revision": int64(4),
				"cost_estimate_receipt_id": nil,
				"generation_request_id":    requestID, "provider_job_id": jobID, "provider_call_set_hash": providerHash,
			},
		},
		{
			name: "executing rejects an incomplete Provider identity", constraint: "ck_gen_intent_provider_refs",
			values: map[string]any{
				"status": generationdomain.IntentExecuting, "revision": int64(4),
				"generation_request_id": requestID, "provider_job_id": nil, "provider_call_set_hash": providerHash,
			},
		},
		{
			name: "succeeded rejects a missing core Owner reference", constraint: "ck_gen_intent_owner_refs",
			values: map[string]any{
				"status": generationdomain.IntentSucceeded, "revision": int64(5),
				"cost_estimate_id":      nil,
				"generation_request_id": requestID, "provider_job_id": jobID, "provider_call_set_hash": providerHash,
				"cost_settlement_receipt_id":   *record.CostEstimateReceiptID,
				"quota_consumption_receipt_id": *record.QuotaReservationReceiptID,
			},
		},
		{
			name: "failed rejects an unmatched terminal receipt pair", constraint: "ck_gen_intent_terminal_refs",
			values: map[string]any{
				"status": generationdomain.IntentFailed, "revision": int64(5),
				"generation_request_id": requestID, "provider_job_id": jobID, "provider_call_set_hash": providerHash,
				"cost_release_receipt_id":  *record.CostReservationReceiptID,
				"quota_release_receipt_id": nil,
			},
		},
		{
			name: "cancelled rejects a missing initial Owner receipt", constraint: "ck_gen_intent_owner_refs",
			values: map[string]any{
				"status": generationdomain.IntentCancelled, "revision": int64(3),
				"quota_reservation_receipt_id": nil,
				"cost_release_receipt_id":      *record.CostReservationReceiptID,
				"quota_release_receipt_id":     *record.QuotaReservationReceiptID,
				"claimant":                     nil, "claim_token": nil, "claim_expires_at": nil, "claim_fencing_version": int64(0),
				"cancelled_at": record.UpdatedAt,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transaction := database.Begin()
			if transaction.Error != nil {
				t.Fatalf("begin invalid Generation intent transaction: %v", transaction.Error)
			}
			err := transaction.Model(&model.GenerationIntent{}).Where("id = ?", record.ID).
				Updates(test.values).Error
			if rollbackErr := transaction.Rollback().Error; rollbackErr != nil {
				t.Fatalf("rollback invalid Generation intent transaction: %v", rollbackErr)
			}
			if err == nil || !strings.Contains(err.Error(), test.constraint) {
				t.Fatalf("PostgreSQL accepted impossible Generation intent state; constraint=%s err=%v", test.constraint, err)
			}
		})
	}
}

func assertProviderImmutableFacts(
	t *testing.T,
	database *generationtestgorm.Database,
	result generationapp.ProviderExecutionResult,
) {
	t.Helper()
	if len(result.Receipts) == 0 {
		t.Fatal("Provider immutability fixture has no terminal ResultReceipt")
	}
	receiptID := result.Receipts[0].ID
	checks := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "update GenerationRequest",
			err: database.Model(&model.GenerationRequest{}).Where("id = ?", result.Request.ID).
				Update("provider_key", "tampered-provider").Error,
			want: model.ErrImmutableGenerationRequest,
		},
		{
			name: "delete GenerationRequest",
			err:  database.Where("id = ?", result.Request.ID).Delete(&model.GenerationRequest{}).Error,
			want: model.ErrImmutableGenerationRequest,
		},
		{
			name: "update GenerationProviderResultReceipt",
			err: database.Model(&model.GenerationProviderResultReceipt{}).Where("id = ?", receiptID).
				Update("status", generationdomain.ProviderResultFailed).Error,
			want: model.ErrImmutableGenerationProviderResultReceipt,
		},
		{
			name: "delete GenerationProviderResultReceipt",
			err:  database.Where("id = ?", receiptID).Delete(&model.GenerationProviderResultReceipt{}).Error,
			want: model.ErrImmutableGenerationProviderResultReceipt,
		},
	}
	for _, check := range checks {
		if !errors.Is(check.err, check.want) {
			t.Fatalf("%s returned %v, want immutable error %v", check.name, check.err, check.want)
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
		{"Provider Calls", deleteRecords(&model.GenerationProviderCall{}, "workspace_id = ?", fixture.workspaceID)},
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
		State: generationdomain.ProviderStateEnabled, AdapterContractVersion: "controlled-image",
		CreatedBy: fixture.ownerID.String(), CreatedAt: createdAt,
	}
	connectionDomain.ContentHash = controlledProviderConnectionContentHash(t, connectionDomain)
	profileDomain := generationdomain.ProviderModelProfileVersion{
		ID: profileID.String(), WorkspaceID: fixture.workspaceID.String(),
		ProfileKey: "controlled-profile-" + externalModelID, Revision: revision,
		CreationSource: map[string]any{"kind": "preset"}, ConnectionKey: "controlled-primary",
		ProviderKey: providerKey, ExternalModelID: externalModelID,
		Modality: generationdomain.MediaModalityImage, Family: "controlled_image",
		AdapterTransportContract: "controlled-image", CapabilitySchemaVersion: "controlled-image",
		BillingMetric: costdomain.MetricGenerationImageCall, Defaults: map[string]any{},
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
		State: generationdomain.ProviderStateEnabled, AdapterContractVersion: "controlled-image",
		ContentHash: connectionDomain.ContentHash, CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	profile := model.ProviderModelProfileVersion{
		ID: profileID, WorkspaceID: fixture.workspaceID, ProfileKey: "controlled-profile-" + externalModelID,
		Revision: revision, CreationSource: []byte(`{"kind":"preset"}`),
		ConnectionKey: "controlled-primary", ProviderKey: providerKey, ExternalModelID: externalModelID,
		Modality: generationdomain.MediaModalityImage, Family: "controlled_image",
		AdapterTransportContract: "controlled-image", CapabilitySchemaVersion: "controlled-image",
		BillingMetric: costdomain.MetricGenerationImageCall, Defaults: []byte(`{}`),
		State: generationdomain.ProviderStateEnabled, ContentHash: profileDomain.ContentHash,
		CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	bindingDomain := generationdomain.ProjectProviderBindingVersion{
		ID: bindingID.String(), WorkspaceID: fixture.workspaceID.String(), ProjectID: fixture.projectID.String(),
		Purpose: generationdomain.ProviderPurposeReferenceAsset, Revision: revision,
		ConnectionVersionID: connectionID.String(), CredentialVersionID: credentialID.String(),
		ModelProfileVersionID: profileID.String(), ProviderKey: providerKey, Modality: generationdomain.MediaModalityImage,
		AdapterContractVersion: "controlled-image", CreatedBy: fixture.ownerID.String(), CreatedAt: createdAt,
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
		Modality: generationdomain.MediaModalityImage, AdapterContractVersion: "controlled-image",
		ContentHash: bindingDomain.ContentHash, CreatedBy: fixture.ownerID, CreatedAt: createdAt,
	}
	for _, record := range []any{&credential, &connection, &profile, &binding} {
		if err = create(record); err != nil {
			t.Fatalf("seed controlled Provider configuration %T: %v", record, err)
		}
	}
	return generationapp.ResolvedProjectProviderBinding{
		Binding: bindingDomain, Connection: connectionDomain,
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
