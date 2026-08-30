package generation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	costapp "github.com/StephenQiu30/lanverse/backend/internal/cost/application"
	costdomain "github.com/StephenQiu30/lanverse/backend/internal/cost/domain"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	generationdomain "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	quotaapp "github.com/StephenQiu30/lanverse/backend/internal/quota/application"
	quotadomain "github.com/StephenQiu30/lanverse/backend/internal/quota/domain"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

const (
	providerTemporalRestartWorkerHelperFlag      = "LANVERSE_PROVIDER_TEMPORAL_RESTART_WORKER_HELPER"
	providerTemporalRestartWorkerAddress         = "LANVERSE_PROVIDER_TEMPORAL_RESTART_WORKER_ADDRESS"
	providerTemporalRestartWorkerTaskQueue       = "LANVERSE_PROVIDER_TEMPORAL_RESTART_WORKER_TASK_QUEUE"
	providerTemporalRestartWorkerMode            = "LANVERSE_PROVIDER_TEMPORAL_RESTART_WORKER_MODE"
	providerTemporalRestartWorkerAuditPath       = "LANVERSE_PROVIDER_TEMPORAL_RESTART_WORKER_AUDIT_PATH"
	providerTemporalRestartWorkerModeSubmit      = "submit"
	providerTemporalRestartWorkerModeReconcile   = "reconcile"
	providerTemporalRestartWorkflowName          = "lanverse.generation.provider.remote-identity-restart"
	providerTemporalRestartActivityName          = "lanverse.generation.provider.remote-identity-restart.activity"
	providerTemporalRestartSubmitActivityID      = "provider-submit"
	providerTemporalRestartReconcileActivityID   = "provider-reconcile"
	providerTemporalRestartPhaseSubmit           = "submit"
	providerTemporalRestartPhaseReconcile        = "reconcile"
	providerTemporalRestartStatusRetrying        = "RETRYING"
	providerTemporalRestartStatusNeedsAttention  = "NEEDS_ATTENTION"
	providerTemporalRestartRemoteRequestID       = "sg-vis-017-remote-request"
	providerTemporalRestartRemoteJobID           = "sg-vis-017-remote-task"
	providerTemporalRestartIdentityFailureReason = "remote task identity is unrecoverable"
)

type providerTemporalRestartWorkflowInput struct {
	Authorization   generationdomain.ExecutionAuthorization
	IntentID        string
	StartedAt       time.Time
	QueryDeadlineAt time.Time
	RemoteExpiresAt time.Time
	RemoteRequestID string
	RemoteJobID     string
}

type providerTemporalRestartActivityInput struct {
	WorkflowInput providerTemporalRestartWorkflowInput
	Phase         string
	ProviderJobID string
}

type providerTemporalRestartActivityResult struct {
	Status          string
	IntentID        string
	ProviderJobID   string
	ProviderCallID  string
	RemoteRequestID string
	RemoteJobID     string
	QueryDeadlineAt time.Time
	RemoteExpiresAt time.Time
}

type providerTemporalRestartActivities struct {
	database  *generationtestgorm.Database
	mode      string
	auditPath string
}

type providerTemporalRestartGateway struct {
	mode            string
	auditPath       string
	remoteRequestID string
	remoteJobID     string
	queryDeadlineAt time.Time
	remoteExpiresAt time.Time
}

type providerTemporalRestartAuditEvent struct {
	Operation       string    `json:"operation"`
	ProviderJobID   string    `json:"provider_job_id"`
	ProviderCallID  string    `json:"provider_call_id"`
	RemoteRequestID string    `json:"remote_request_id"`
	RemoteJobID     string    `json:"remote_job_id"`
	QueryDeadlineAt time.Time `json:"query_deadline_at"`
	RemoteExpiresAt time.Time `json:"remote_expires_at"`
}

func TestProviderRemoteIdentitySurvivesTemporalWorkerAndBackendRestart(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	temporalAddress := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if databaseURL == "" || temporalAddress == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL and LANVERSE_TEST_TEMPORAL_ADDRESS to run the Provider restart journey")
	}

	startedAt := time.Date(2026, time.August, 30, 20, 0, 0, 0, time.UTC)
	queryDeadlineAt := startedAt.Add(2 * time.Hour)
	remoteExpiresAt := startedAt.Add(26 * time.Hour)
	harness := newProviderFencingHarness(t, startedAt, "provider-temporal-restart")
	claim := prepareAndClaimProviderIntent(
		t,
		harness.ctx,
		harness.preparations,
		harness.fixture,
		"temporal-restart",
		"1717171717171717171717171717171717171717171717171717171717171717",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddress, Namespace: "default"})
	if err != nil {
		t.Fatalf("connect real Temporal service: %v", err)
	}
	t.Cleanup(temporalClient.Close)

	taskQueue := "lanverse-provider-identity-restart-" + uuid.NewString()
	workflowID := "lanverse:provider-identity-restart:" + uuid.NewString()
	auditPath := t.TempDir() + "/provider-boundaries.jsonl"
	workflowInput := providerTemporalRestartWorkflowInput{
		Authorization: claim.Authorization, IntentID: claim.Intent.ID, StartedAt: startedAt,
		QueryDeadlineAt: queryDeadlineAt, RemoteRequestID: providerTemporalRestartRemoteRequestID,
		RemoteExpiresAt: remoteExpiresAt, RemoteJobID: providerTemporalRestartRemoteJobID,
	}

	firstWorker := startProviderTemporalRestartWorkerProcess(
		t, databaseURL, temporalAddress, taskQueue, providerTemporalRestartWorkerModeSubmit, auditPath,
	)
	run, err := temporalClient.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID: workflowID, TaskQueue: taskQueue, WorkflowExecutionTimeout: 30 * time.Second,
		},
		providerTemporalRestartWorkflowName,
		workflowInput,
	)
	if err != nil {
		t.Fatalf("start Provider identity restart Workflow: %v", err)
	}
	waitForProviderTemporalRestartTimer(t, ctx, temporalClient, workflowID)
	stopProviderTemporalRestartWorkerProcess(t, firstWorker)

	firstIntent, firstJob, firstCalls := loadProviderTemporalRestartFacts(t, harness.database, claim.Intent.ID)
	assertProviderTemporalRestartFirstRound(
		t, firstIntent, firstJob, firstCalls, queryDeadlineAt, remoteExpiresAt,
		providerTemporalRestartRemoteRequestID, providerTemporalRestartRemoteJobID,
	)

	secondWorker := startProviderTemporalRestartWorkerProcess(
		t, databaseURL, temporalAddress, taskQueue, providerTemporalRestartWorkerModeReconcile, auditPath,
	)
	var workflowResult providerTemporalRestartActivityResult
	if err = run.Get(ctx, &workflowResult); err != nil {
		t.Fatalf("wait for restarted Provider identity Workflow: %v", err)
	}
	if workflowResult.Status != providerTemporalRestartStatusNeedsAttention ||
		workflowResult.IntentID != claim.Intent.ID || workflowResult.ProviderJobID != firstJob.ID.String() ||
		workflowResult.ProviderCallID != firstCalls[3].ID.String() ||
		workflowResult.RemoteRequestID != providerTemporalRestartRemoteRequestID ||
		workflowResult.RemoteJobID != providerTemporalRestartRemoteJobID ||
		!workflowResult.QueryDeadlineAt.Equal(queryDeadlineAt) ||
		!workflowResult.RemoteExpiresAt.Equal(remoteExpiresAt) {
		t.Fatalf("restarted Provider Workflow result drifted: %#v", workflowResult)
	}
	stopProviderTemporalRestartWorkerProcess(t, secondWorker)

	finalIntent, finalJob, finalCalls := loadProviderTemporalRestartFacts(t, harness.database, claim.Intent.ID)
	assertProviderTemporalRestartFinalFacts(
		t, finalIntent, finalJob, finalCalls, firstCalls, queryDeadlineAt, remoteExpiresAt,
		providerTemporalRestartRemoteRequestID, providerTemporalRestartRemoteJobID,
	)
	assertProviderTemporalRestartReservations(t, harness.database, claim.Intent)
	assertProviderTemporalRestartAudit(
		t, auditPath, firstJob.ID.String(), firstCalls[3].ID.String(), queryDeadlineAt, remoteExpiresAt,
		providerTemporalRestartRemoteRequestID, providerTemporalRestartRemoteJobID,
	)

	history, activityStarts, activityCompletions, timerStarts, timerFires := loadProviderTemporalRestartHistory(
		t, ctx, temporalClient, workflowID,
	)
	wantActivities := []string{
		providerTemporalRestartSubmitActivityID,
		providerTemporalRestartReconcileActivityID,
	}
	if len(activityStarts) != len(wantActivities) || len(activityCompletions) != len(wantActivities) {
		t.Fatalf("Provider restart activity sets: starts=%#v completions=%#v", activityStarts, activityCompletions)
	}
	for _, activityID := range wantActivities {
		if activityStarts[activityID] != 1 || activityCompletions[activityID] != 1 {
			t.Fatalf(
				"Provider restart activity %q counts: starts=%d completions=%d, want 1/1",
				activityID, activityStarts[activityID], activityCompletions[activityID],
			)
		}
	}
	if timerStarts != 1 || timerFires != 1 {
		t.Fatalf("Provider restart timer counts: started=%d fired=%d, want 1/1", timerStarts, timerFires)
	}

	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflowWithOptions(
		providerTemporalRestartWorkflow,
		temporalworkflow.RegisterOptions{Name: providerTemporalRestartWorkflowName},
	)
	if err = replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay restarted Provider identity Workflow history: %v", err)
	}
}

func TestProviderTemporalRestartWorkerHelper(t *testing.T) {
	if os.Getenv(providerTemporalRestartWorkerHelperFlag) != "1" {
		t.Skip("subprocess helper")
	}
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	address := os.Getenv(providerTemporalRestartWorkerAddress)
	taskQueue := os.Getenv(providerTemporalRestartWorkerTaskQueue)
	mode := os.Getenv(providerTemporalRestartWorkerMode)
	auditPath := os.Getenv(providerTemporalRestartWorkerAuditPath)
	if databaseURL == "" || address == "" || taskQueue == "" || auditPath == "" ||
		(mode != providerTemporalRestartWorkerModeSubmit && mode != providerTemporalRestartWorkerModeReconcile) {
		t.Fatal("Provider restart helper configuration is incomplete")
	}

	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal("open Provider restart helper database")
	}
	defer func() { _ = platformdatabase.Close(database) }()
	temporalClient, err := client.Dial(client.Options{HostPort: address, Namespace: "default"})
	if err != nil {
		t.Fatal("connect Provider restart helper to Temporal")
	}
	defer temporalClient.Close()

	activities := &providerTemporalRestartActivities{database: database, mode: mode, auditPath: auditPath}
	runtimeWorker := worker.New(temporalClient, taskQueue, worker.Options{})
	runtimeWorker.RegisterWorkflowWithOptions(
		providerTemporalRestartWorkflow,
		temporalworkflow.RegisterOptions{Name: providerTemporalRestartWorkflowName},
	)
	runtimeWorker.RegisterActivityWithOptions(
		activities.Execute,
		activity.RegisterOptions{Name: providerTemporalRestartActivityName},
	)
	if err = runtimeWorker.Start(); err != nil {
		t.Fatal("start Provider restart helper Worker")
	}
	defer runtimeWorker.Stop()
	select {}
}

func providerTemporalRestartWorkflow(
	ctx temporalworkflow.Context,
	input providerTemporalRestartWorkflowInput,
) (providerTemporalRestartActivityResult, error) {
	firstContext := temporalworkflow.WithActivityOptions(ctx, temporalworkflow.ActivityOptions{
		ActivityID: providerTemporalRestartSubmitActivityID, StartToCloseTimeout: 20 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	var first providerTemporalRestartActivityResult
	if err := temporalworkflow.ExecuteActivity(
		firstContext,
		providerTemporalRestartActivityName,
		providerTemporalRestartActivityInput{WorkflowInput: input, Phase: providerTemporalRestartPhaseSubmit},
	).Get(firstContext, &first); err != nil {
		return providerTemporalRestartActivityResult{}, err
	}
	if first.Status != providerTemporalRestartStatusRetrying || first.ProviderJobID == "" || first.ProviderCallID == "" {
		return providerTemporalRestartActivityResult{}, fmt.Errorf("Provider submit activity returned invalid retry state")
	}
	if err := temporalworkflow.Sleep(ctx, 3*time.Second); err != nil {
		return providerTemporalRestartActivityResult{}, err
	}

	secondContext := temporalworkflow.WithActivityOptions(ctx, temporalworkflow.ActivityOptions{
		ActivityID: providerTemporalRestartReconcileActivityID, StartToCloseTimeout: 20 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	var second providerTemporalRestartActivityResult
	if err := temporalworkflow.ExecuteActivity(
		secondContext,
		providerTemporalRestartActivityName,
		providerTemporalRestartActivityInput{
			WorkflowInput: input, Phase: providerTemporalRestartPhaseReconcile, ProviderJobID: first.ProviderJobID,
		},
	).Get(secondContext, &second); err != nil {
		return providerTemporalRestartActivityResult{}, err
	}
	if second.Status != providerTemporalRestartStatusNeedsAttention ||
		second.ProviderJobID != first.ProviderJobID || second.ProviderCallID != first.ProviderCallID {
		return providerTemporalRestartActivityResult{}, fmt.Errorf("Provider reconcile activity returned invalid attention state")
	}
	return second, nil
}

func (activities *providerTemporalRestartActivities) Execute(
	ctx context.Context,
	input providerTemporalRestartActivityInput,
) (providerTemporalRestartActivityResult, error) {
	if activities == nil || activities.database == nil ||
		input.WorkflowInput.IntentID == "" || input.WorkflowInput.Authorization.IntentID != input.WorkflowInput.IntentID {
		return providerTemporalRestartActivityResult{}, errors.New("Provider restart activity input is invalid")
	}
	gateway := &providerTemporalRestartGateway{
		mode: activities.mode, auditPath: activities.auditPath,
		remoteRequestID: input.WorkflowInput.RemoteRequestID,
		remoteJobID:     input.WorkflowInput.RemoteJobID,
		queryDeadlineAt: input.WorkflowInput.QueryDeadlineAt,
		remoteExpiresAt: input.WorkflowInput.RemoteExpiresAt,
	}
	now := input.WorkflowInput.StartedAt.UTC().Truncate(time.Microsecond)
	providers := generationapp.NewProviderService(
		generationgorm.NewProviderStore(
			activities.database,
			costapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
			quotaapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
		),
		gateway,
		generationapp.ProviderConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)

	switch input.Phase {
	case providerTemporalRestartPhaseSubmit:
		if activities.mode != providerTemporalRestartWorkerModeSubmit {
			return providerTemporalRestartActivityResult{}, errors.New("Provider submit activity reached the wrong Worker")
		}
		var result generationapp.ProviderExecutionResult
		for candidate := 1; candidate <= 4; candidate++ {
			var err error
			result, err = providers.SubmitImageRequest(
				ctx,
				input.WorkflowInput.Authorization,
				generationapp.SubmitImageRequestCommand{
					IntentID:       input.WorkflowInput.IntentID,
					IdempotencyKey: fmt.Sprintf("provider-temporal-restart-submit-%d", candidate),
				},
			)
			if err != nil {
				return providerTemporalRestartActivityResult{}, errors.New("Provider restart submission failed")
			}
		}
		if result.Intent.Status != generationdomain.IntentExecuting ||
			result.Job.Status != generationdomain.ProviderJobRunning || len(result.Calls) != 4 ||
			result.Calls[3].Status != generationdomain.ProviderCallSubmitted ||
			result.Calls[3].RemoteRequestID != input.WorkflowInput.RemoteRequestID ||
			result.Calls[3].RemoteJobID != input.WorkflowInput.RemoteJobID ||
			result.Calls[3].QueryDeadlineAt == nil ||
			!result.Calls[3].QueryDeadlineAt.Equal(input.WorkflowInput.QueryDeadlineAt) ||
			result.Calls[3].RemoteExpiresAt == nil ||
			!result.Calls[3].RemoteExpiresAt.Equal(input.WorkflowInput.RemoteExpiresAt) {
			return providerTemporalRestartActivityResult{}, errors.New("Provider restart submission did not persist identity")
		}
		return providerTemporalRestartResult(providerTemporalRestartStatusRetrying, result), nil

	case providerTemporalRestartPhaseReconcile:
		if activities.mode != providerTemporalRestartWorkerModeReconcile || input.ProviderJobID == "" {
			return providerTemporalRestartActivityResult{}, errors.New("Provider reconcile activity reached the wrong Worker")
		}
		result, err := providers.ReconcileProviderJob(ctx, generationapp.ReconcileProviderJobCommand{
			ProviderJobID: input.ProviderJobID, IdempotencyKey: "provider-temporal-restart-reconcile",
		})
		if err != nil {
			return providerTemporalRestartActivityResult{}, errors.New("Provider restart reconciliation failed")
		}
		if result.Intent.Status != generationdomain.IntentOutcomeUnknown ||
			result.Job.Status != generationdomain.ProviderJobOutcomeUnknown || len(result.Calls) != 4 ||
			result.Calls[3].Status != generationdomain.ProviderCallOutcomeUnknown ||
			result.Calls[3].RemoteRequestID != input.WorkflowInput.RemoteRequestID ||
			result.Calls[3].RemoteJobID != input.WorkflowInput.RemoteJobID ||
			result.Calls[3].QueryDeadlineAt == nil ||
			!result.Calls[3].QueryDeadlineAt.Equal(input.WorkflowInput.QueryDeadlineAt) ||
			result.Calls[3].RemoteExpiresAt == nil ||
			!result.Calls[3].RemoteExpiresAt.Equal(input.WorkflowInput.RemoteExpiresAt) {
			return providerTemporalRestartActivityResult{}, errors.New("Provider restart reconciliation did not preserve identity")
		}
		return providerTemporalRestartResult(providerTemporalRestartStatusNeedsAttention, result), nil

	default:
		return providerTemporalRestartActivityResult{}, errors.New("Provider restart activity phase is invalid")
	}
}

func providerTemporalRestartResult(
	status string,
	result generationapp.ProviderExecutionResult,
) providerTemporalRestartActivityResult {
	call := result.Calls[3]
	value := providerTemporalRestartActivityResult{
		Status: status, IntentID: result.Intent.ID, ProviderJobID: result.Job.ID, ProviderCallID: call.ID,
		RemoteRequestID: call.RemoteRequestID, RemoteJobID: call.RemoteJobID,
	}
	if call.QueryDeadlineAt != nil {
		value.QueryDeadlineAt = *call.QueryDeadlineAt
	}
	if call.RemoteExpiresAt != nil {
		value.RemoteExpiresAt = *call.RemoteExpiresAt
	}
	return value
}

func (gateway *providerTemporalRestartGateway) Preflight(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) error {
	if gateway.mode != providerTemporalRestartWorkerModeSubmit {
		_ = gateway.appendAudit("unexpected_preflight", submission)
		return errors.New("Provider resubmission after restart is forbidden")
	}
	if submission.CandidateIndex >= 1 && submission.CandidateIndex <= 3 {
		return controlledProviderFailure(fmt.Sprintf("provider.invalid_candidate_%d", submission.CandidateIndex))
	}
	if submission.CandidateIndex == 4 {
		return nil
	}
	return controlledProviderFailure("provider.invalid_candidate_index")
}

func (gateway *providerTemporalRestartGateway) Submit(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	if gateway.mode != providerTemporalRestartWorkerModeSubmit || submission.CandidateIndex != 4 {
		_ = gateway.appendAudit("unexpected_submit", submission)
		return generationapp.ProviderOutcome{}, errors.New("Provider Submit crossed an invalid restart boundary")
	}
	if err := gateway.appendAudit("submit", submission); err != nil {
		return generationapp.ProviderOutcome{}, errors.New("record Provider Submit boundary")
	}
	return generationapp.ProviderOutcome{
		Status: generationapp.ProviderOutcomeAccepted, RemoteRequestID: gateway.remoteRequestID,
		RemoteJobID: gateway.remoteJobID, QueryDeadlineAt: gateway.queryDeadlineAt,
		RemoteExpiresAt: gateway.remoteExpiresAt,
	}, nil
}

func (gateway *providerTemporalRestartGateway) Query(
	_ context.Context,
	submission generationapp.ProviderSubmission,
) (generationapp.ProviderOutcome, error) {
	if gateway.mode != providerTemporalRestartWorkerModeReconcile || submission.CandidateIndex != 4 {
		_ = gateway.appendAudit("unexpected_query", submission)
		return generationapp.ProviderOutcome{}, controlledProviderQueryFailure{
			kind: generationapp.ProviderQueryFailureRetryable, message: "Provider Query crossed an invalid restart boundary",
		}
	}
	if err := gateway.appendAudit("query", submission); err != nil {
		return generationapp.ProviderOutcome{}, controlledProviderQueryFailure{
			kind: generationapp.ProviderQueryFailureRetryable, message: "record Provider Query boundary",
		}
	}
	return generationapp.ProviderOutcome{}, controlledProviderQueryFailure{
		kind: generationapp.ProviderQueryFailureIdentityUnrecoverable, message: providerTemporalRestartIdentityFailureReason,
	}
}

func (gateway *providerTemporalRestartGateway) appendAudit(
	operation string,
	submission generationapp.ProviderSubmission,
) error {
	remoteRequestID, remoteJobID := submission.RemoteRequestID, submission.RemoteJobID
	queryDeadlineAt, remoteExpiresAt := time.Time{}, time.Time{}
	if operation == "submit" {
		remoteRequestID, remoteJobID = gateway.remoteRequestID, gateway.remoteJobID
		queryDeadlineAt, remoteExpiresAt = gateway.queryDeadlineAt, gateway.remoteExpiresAt
	} else {
		if submission.QueryDeadlineAt != nil {
			queryDeadlineAt = *submission.QueryDeadlineAt
		}
		if submission.RemoteExpiresAt != nil {
			remoteExpiresAt = *submission.RemoteExpiresAt
		}
	}
	return appendProviderTemporalRestartAudit(gateway.auditPath, providerTemporalRestartAuditEvent{
		Operation: operation, ProviderJobID: submission.ProviderJobID, ProviderCallID: submission.ProviderCallID,
		RemoteRequestID: remoteRequestID, RemoteJobID: remoteJobID, QueryDeadlineAt: queryDeadlineAt,
		RemoteExpiresAt: remoteExpiresAt,
	})
}

func appendProviderTemporalRestartAudit(path string, event providerTemporalRestartAuditEvent) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(file).Encode(event); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func startProviderTemporalRestartWorkerProcess(
	t *testing.T,
	databaseURL string,
	temporalAddress string,
	taskQueue string,
	mode string,
	auditPath string,
) *exec.Cmd {
	t.Helper()
	output := &bytes.Buffer{}
	command := exec.Command(os.Args[0], "-test.run=^TestProviderTemporalRestartWorkerHelper$", "-test.v")
	command.Env = append(
		os.Environ(),
		providerTemporalRestartWorkerHelperFlag+"=1",
		"LANVERSE_TEST_DATABASE_URL="+databaseURL,
		providerTemporalRestartWorkerAddress+"="+temporalAddress,
		providerTemporalRestartWorkerTaskQueue+"="+taskQueue,
		providerTemporalRestartWorkerMode+"="+mode,
		providerTemporalRestartWorkerAuditPath+"="+auditPath,
	)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatal("start Provider Temporal restart Worker subprocess")
	}
	t.Cleanup(func() {
		if command.ProcessState == nil && command.Process != nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})
	return command
}

func stopProviderTemporalRestartWorkerProcess(t *testing.T, command *exec.Cmd) {
	t.Helper()
	if command == nil || command.Process == nil || command.ProcessState != nil {
		t.Fatal("Provider Temporal restart Worker was not running at the restart boundary")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal("kill Provider Temporal restart Worker at the restart boundary")
	}
	_, _ = command.Process.Wait()
}

func waitForProviderTemporalRestartTimer(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	workflowID string,
) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		iterator := temporalClient.GetWorkflowHistory(
			ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
		)
		scheduled := make(map[int64]string)
		completedSubmitEventID := int64(0)
		timerStartedEventID := int64(0)
		timerFired := false
		for iterator.HasNext() {
			event, err := iterator.Next()
			if err != nil {
				t.Fatalf("read Provider restart history while waiting for TimerStarted: %v", err)
			}
			if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
				scheduled[event.GetEventId()] = attributes.GetActivityId()
			}
			if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil &&
				scheduled[attributes.GetScheduledEventId()] == providerTemporalRestartSubmitActivityID {
				completedSubmitEventID = event.GetEventId()
			}
			if event.GetTimerStartedEventAttributes() != nil &&
				completedSubmitEventID > 0 && event.GetEventId() > completedSubmitEventID {
				timerStartedEventID = event.GetEventId()
			}
			if event.GetTimerFiredEventAttributes() != nil && timerStartedEventID > 0 &&
				event.GetEventId() > timerStartedEventID {
				timerFired = true
			}
		}
		if timerFired {
			t.Fatal("Provider restart timer fired before the first Worker was killed")
		}
		if timerStartedEventID > 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("wait for Provider restart TimerStarted: %v", ctx.Err())
		}
	}
}

func loadProviderTemporalRestartFacts(
	t *testing.T,
	database *generationtestgorm.Database,
	intentID string,
) (model.GenerationIntent, model.GenerationProviderJob, []model.GenerationProviderCall) {
	t.Helper()
	var intent model.GenerationIntent
	if err := database.First(&intent, "id = ?", intentID).Error; err != nil {
		t.Fatalf("load Provider restart Intent: %v", err)
	}
	var job model.GenerationProviderJob
	if err := database.Where("intent_id = ?", intentID).First(&job).Error; err != nil {
		t.Fatalf("load Provider restart Job: %v", err)
	}
	var calls []model.GenerationProviderCall
	if err := database.Where("job_id = ?", job.ID).Order("candidate_index ASC").Find(&calls).Error; err != nil {
		t.Fatalf("load Provider restart Calls: %v", err)
	}
	return intent, job, calls
}

func assertProviderTemporalRestartFirstRound(
	t *testing.T,
	intent model.GenerationIntent,
	job model.GenerationProviderJob,
	calls []model.GenerationProviderCall,
	queryDeadlineAt time.Time,
	remoteExpiresAt time.Time,
	remoteRequestID string,
	remoteJobID string,
) {
	t.Helper()
	if intent.Status != generationdomain.IntentExecuting || job.Status != generationdomain.ProviderJobRunning ||
		job.CallCount != 4 || job.DispatchedCallCount != 1 || job.SucceededCallCount != 0 || job.FailedCallCount != 3 ||
		len(calls) != 4 {
		t.Fatalf("first Provider Worker did not persist one retrying remote task: intent=%#v job=%#v calls=%#v", intent, job, calls)
	}
	for index := 0; index < 3; index++ {
		wantFailureCode := fmt.Sprintf("provider.invalid_candidate_%d", index+1)
		if calls[index].CandidateIndex != index+1 || calls[index].Status != generationdomain.ProviderCallFailed ||
			calls[index].LocalFailureCode == nil || *calls[index].LocalFailureCode != wantFailureCode ||
			calls[index].DispatchBoundaryEnteredAt != nil || calls[index].RemoteRequestID != nil || calls[index].RemoteJobID != nil {
			t.Fatalf("Provider Candidate %d did not remain a local failure: %#v", index+1, calls[index])
		}
	}
	assertProviderTemporalRestartRemoteCall(
		t, calls[3], generationdomain.ProviderCallSubmitted, queryDeadlineAt, remoteExpiresAt,
		remoteRequestID, remoteJobID,
	)
}

func assertProviderTemporalRestartFinalFacts(
	t *testing.T,
	intent model.GenerationIntent,
	job model.GenerationProviderJob,
	calls []model.GenerationProviderCall,
	firstCalls []model.GenerationProviderCall,
	queryDeadlineAt time.Time,
	remoteExpiresAt time.Time,
	remoteRequestID string,
	remoteJobID string,
) {
	t.Helper()
	if intent.Status != generationdomain.IntentOutcomeUnknown || job.Status != generationdomain.ProviderJobOutcomeUnknown ||
		job.CallCount != 4 || job.DispatchedCallCount != 1 || job.SucceededCallCount != 0 || job.FailedCallCount != 3 ||
		len(calls) != 4 || len(firstCalls) != 4 {
		t.Fatalf("second Provider Worker did not converge to explicit attention: intent=%#v job=%#v calls=%#v", intent, job, calls)
	}
	for index := 0; index < 3; index++ {
		if calls[index].ID != firstCalls[index].ID || calls[index].Status != generationdomain.ProviderCallFailed ||
			calls[index].Revision != firstCalls[index].Revision || calls[index].ContentHash != firstCalls[index].ContentHash {
			t.Fatalf("restarted Provider Worker rewrote local Candidate %d: before=%#v after=%#v", index+1, firstCalls[index], calls[index])
		}
	}
	if calls[3].ID != firstCalls[3].ID || calls[3].DispatchBoundaryEnteredAt == nil ||
		firstCalls[3].DispatchBoundaryEnteredAt == nil ||
		!calls[3].DispatchBoundaryEnteredAt.Equal(*firstCalls[3].DispatchBoundaryEnteredAt) {
		t.Fatalf("restarted Provider Call crossed a second dispatch boundary: before=%#v after=%#v", firstCalls[3], calls[3])
	}
	assertProviderTemporalRestartRemoteCall(
		t, calls[3], generationdomain.ProviderCallOutcomeUnknown, queryDeadlineAt, remoteExpiresAt,
		remoteRequestID, remoteJobID,
	)
}

func assertProviderTemporalRestartRemoteCall(
	t *testing.T,
	call model.GenerationProviderCall,
	wantStatus string,
	queryDeadlineAt time.Time,
	remoteExpiresAt time.Time,
	remoteRequestID string,
	remoteJobID string,
) {
	t.Helper()
	if call.CandidateIndex != 4 || call.Status != wantStatus || call.DispatchBoundaryEnteredAt == nil ||
		call.RemoteRequestID == nil || *call.RemoteRequestID != remoteRequestID ||
		call.RemoteJobID == nil || *call.RemoteJobID != remoteJobID ||
		call.QueryDeadlineAt == nil || !call.QueryDeadlineAt.Equal(queryDeadlineAt) ||
		call.RemoteExpiresAt == nil || !call.RemoteExpiresAt.Equal(remoteExpiresAt) {
		t.Fatalf("Provider remote identity/retention window drifted at status %s: %#v", wantStatus, call)
	}
}

func assertProviderTemporalRestartReservations(
	t *testing.T,
	database *generationtestgorm.Database,
	intent generationdomain.Intent,
) {
	t.Helper()
	var costReservation model.CostReservation
	if err := database.First(&costReservation, "id = ?", intent.CostReservationID).Error; err != nil {
		t.Fatalf("load Provider restart Cost reservation: %v", err)
	}
	var quotaReservation model.QuotaReservation
	if err := database.First(&quotaReservation, "id = ?", intent.QuotaReservationID).Error; err != nil {
		t.Fatalf("load Provider restart Quota reservation: %v", err)
	}
	if costReservation.Status != costdomain.ReservationReserved || costReservation.SettledAt != nil ||
		costReservation.ReleasedAt != nil || quotaReservation.Status != quotadomain.ReservationReserved ||
		quotaReservation.ConsumedAt != nil || quotaReservation.ReleasedAt != nil {
		t.Fatalf(
			"OUTCOME_UNKNOWN settled or released an Owner reservation: cost=%#v quota=%#v",
			costReservation,
			quotaReservation,
		)
	}
}

func assertProviderTemporalRestartAudit(
	t *testing.T,
	path string,
	providerJobID string,
	providerCallID string,
	queryDeadlineAt time.Time,
	remoteExpiresAt time.Time,
	remoteRequestID string,
	remoteJobID string,
) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open Provider restart audit: %v", err)
	}
	defer func() { _ = file.Close() }()
	var events []providerTemporalRestartAuditEvent
	decoder := json.NewDecoder(file)
	for {
		var event providerTemporalRestartAuditEvent
		if err = decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode Provider restart audit: %v", err)
		}
		events = append(events, event)
	}
	if len(events) != 2 || events[0].Operation != "submit" || events[1].Operation != "query" {
		t.Fatalf("Provider restart remote boundaries = %#v, want exactly Submit then Query", events)
	}
	for _, event := range events {
		if event.ProviderJobID != providerJobID || event.ProviderCallID != providerCallID ||
			event.RemoteRequestID != remoteRequestID || event.RemoteJobID != remoteJobID ||
			!event.QueryDeadlineAt.Equal(queryDeadlineAt) || !event.RemoteExpiresAt.Equal(remoteExpiresAt) {
			t.Fatalf("Provider restart audit identity drifted: %#v", event)
		}
	}
}

func loadProviderTemporalRestartHistory(
	t *testing.T,
	ctx context.Context,
	temporalClient client.Client,
	workflowID string,
) (*historypb.History, map[string]int, map[string]int, int, int) {
	t.Helper()
	iterator := temporalClient.GetWorkflowHistory(
		ctx, workflowID, "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT,
	)
	history := &historypb.History{}
	scheduled := make(map[int64]string)
	activityStarts := make(map[string]int)
	activityCompletions := make(map[string]int)
	timerStarts, timerFires := 0, 0
	for iterator.HasNext() {
		event, err := iterator.Next()
		if err != nil {
			t.Fatalf("read Provider restart Workflow history: %v", err)
		}
		history.Events = append(history.Events, event)
		if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
			scheduled[event.GetEventId()] = attributes.GetActivityId()
		}
		if attributes := event.GetActivityTaskStartedEventAttributes(); attributes != nil {
			activityStarts[scheduled[attributes.GetScheduledEventId()]]++
		}
		if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
			activityCompletions[scheduled[attributes.GetScheduledEventId()]]++
		}
		if event.GetTimerStartedEventAttributes() != nil {
			timerStarts++
		}
		if event.GetTimerFiredEventAttributes() != nil {
			timerFires++
		}
	}
	return history, activityStarts, activityCompletions, timerStarts, timerFires
}

var _ generationapp.ProviderGateway = (*providerTemporalRestartGateway)(nil)
