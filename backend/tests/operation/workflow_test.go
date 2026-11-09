package operation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func workflowFixture() application.WorkflowOperation {
	operationID, projectID := uuid.New(), uuid.New()
	modelID, priceID := uuid.New(), uuid.New()
	quote, region := int64(100), "domestic"
	created := time.Now().UTC().Add(-time.Minute)
	expires := created.Add(15 * time.Minute)
	prompt := "A lantern in the rain"
	return application.WorkflowOperation{
		Operation: domain.Operation{
			ID: operationID, ProjectID: projectID, TargetType: "free",
			Capability: "image.generate", Mode: "text_to_image",
			ModelProfileVersionID: &modelID, PriceRuleVersionID: &priceID,
			Params:      json.RawMessage(`{"mock_moderation_status":"passed"}`),
			OutputCount: 1, InputHash: "frozen-hash", Origin: "canvas",
			Status: domain.StatusConfirmed, QuoteMicros: &quote,
			QuoteExpiresAt: &expires, Region: &region, CreateTime: created,
		},
		Inputs: []domain.OperationInput{{
			ID: uuid.New(), OperationID: operationID, SeqNo: 0,
			Role: "prompt", RefType: "text", TextValue: &prompt,
		}},
		Provider: application.WorkflowProvider{
			ProviderKey: "mock", AdapterKey: "mock", ProviderModelID: "mock-image",
			Queue: "agent.mock", SupportsQuery: true, ExpectedMaxMS: 60_000,
			Moderation: "platform",
		},
		ProviderRequestKey: "operation/" + operationID.String(),
	}
}

// stateStore records business facts across independent task invocations. It is a
// unit fixture; PostgreSQL worker/lease coverage lives in task_runtime_pg_test.
type stateStore struct {
	loaded                                                                application.WorkflowOperation
	evidence                                                              application.TaskEvidence
	transitions                                                           []domain.Status
	calls                                                                 []application.BeginProviderCallInput
	completes                                                             []application.CompleteProviderCallInput
	scheduled                                                             []string
	settled                                                               []task.SettlementInput
	cost                                                                  application.ProviderCost
	inputErr, transitionErr, completeErr, manualErr, reuseErr, controlErr error
	reused, revoked, recoveries                                           int
	batch                                                                 application.WorkflowBatch
	acquired, cancelled                                                   []uuid.UUID
	capacity                                                              bool
}

func newStateStore() *stateStore {
	return &stateStore{loaded: workflowFixture(), evidence: application.TaskEvidence{Admitted: true}, capacity: true, manualErr: application.ErrManualResolutionUnverified}
}
func (s *stateStore) LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error) {
	return s.loaded, nil
}
func (s *stateStore) TaskEvidence(context.Context, uuid.UUID) (application.TaskEvidence, error) {
	return s.evidence, nil
}
func (s *stateStore) CheckSubmissionInputs(context.Context, uuid.UUID) error { return s.inputErr }
func (s *stateStore) TransitionWorkflowOperation(_ context.Context, i application.TransitionInput) (domain.Status, error) {
	if s.transitionErr != nil {
		return "", s.transitionErr
	}
	if err := s.loaded.Operation.Status.CanTransitionTo(i.To); err != nil {
		return "", err
	}
	s.transitions = append(s.transitions, i.To)
	s.loaded.Operation.Status = i.To
	if i.ProviderTaskID != nil {
		s.loaded.ProviderTaskID = i.ProviderTaskID
	}
	return i.To, nil
}
func (s *stateStore) ScheduleOperationStep(_ context.Context, k string, _ uuid.UUID) error {
	s.scheduled = append(s.scheduled, k)
	return nil
}
func (s *stateStore) BeginProviderCall(_ context.Context, i application.BeginProviderCallInput) error {
	s.calls = append(s.calls, i)
	return nil
}
func (s *stateStore) CompleteProviderCall(_ context.Context, i application.CompleteProviderCallInput) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	s.completes = append(s.completes, i)
	switch i.Action {
	case "submit":
		s.evidence.SubmitState = i.State
		s.evidence.SubmitTaskID = i.ProviderTaskID
		s.evidence.FailureCode = i.ErrorCode
		s.evidence.DispatchCancelled = i.ErrorCode == "provider_dispatch_cancelled"
		if i.ErrorCode != "" {
			s.evidence.FailureCode = application.SafeProviderFailureCode(i.ErrorCode, string(i.State))
		}
	case "cancel":
		s.evidence.CancelState = i.State
	case "query":
		s.evidence.QueryState = i.State
		s.evidence.QueryFailureCode = application.SafeProviderFailureCode(i.ErrorCode, string(i.State))
		s.loaded.LastQueryAttempt = i.Attempt
	}
	return nil
}
func (s *stateStore) CheckManualNotExecuted(context.Context, uuid.UUID) error { return s.manualErr }
func (s *stateStore) LoadProviderCost(context.Context, uuid.UUID) (application.ProviderCost, error) {
	return s.cost, nil
}
func (s *stateStore) LoadProviderImageIdentity(context.Context, uuid.UUID) (application.ProviderDispatchIdentity, bool, error) {
	o := s.loaded.Operation
	return application.ProviderDispatchIdentity{OperationID: o.ID, ProjectID: o.ProjectID, ModelProfileVersionID: *o.ModelProfileVersionID, PriceRuleVersionID: *o.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: s.loaded.ProviderRequestKey}, true, nil
}
func (s *stateStore) RecoverImage(context.Context, application.ProviderDispatchIdentity) (*application.ProviderReceipt, error) {
	s.recoveries++
	return nil, nil
}
func (s *stateStore) ResolveRevokedOperation(context.Context, string) error { s.revoked++; return nil }
func (s *stateStore) FinalizeOperation(_ context.Context, i task.SettlementInput) error {
	s.settled = append(s.settled, i)
	s.loaded.Operation.Status = i.To
	return nil
}
func (s *stateStore) CompleteFromReuse(context.Context, string) error { s.reused++; return s.reuseErr }
func (s *stateStore) CancelQueuedBatchOperation(_ context.Context, _, id uuid.UUID) (bool, error) {
	s.cancelled = append(s.cancelled, id)
	return true, nil
}
func (s *stateStore) LoadWorkflowBatch(context.Context, uuid.UUID) (application.WorkflowBatch, error) {
	return s.batch, nil
}
func (s *stateStore) MarkWorkflowBatchRunning(context.Context, uuid.UUID) error {
	s.batch.Batch.Status = domain.BatchStatusRunning
	return nil
}
func (s *stateStore) MarkWorkflowBatchCancelRequested(context.Context, uuid.UUID) error {
	s.batch.CancelRequested = true
	return nil
}
func (s *stateStore) AcquireBatchLaunch(_ context.Context, _, id uuid.UUID) (bool, error) {
	if s.capacity {
		s.acquired = append(s.acquired, id)
	}
	return s.capacity, nil
}
func (s *stateStore) PauseWorkflowBatch(_ context.Context, _ uuid.UUID, reason string) error {
	s.batch.PausedReason = reason
	return nil
}
func (s *stateStore) ResumeWorkflowBatch(context.Context, uuid.UUID) error {
	s.batch.PausedReason = ""
	return nil
}
func (s *stateStore) FinishWorkflowBatch(context.Context, uuid.UUID) error {
	s.batch.Batch.Status = domain.BatchStatusFinished
	return nil
}
func (s *stateStore) OriginalDeliveryRecoveryRequired(context.Context, application.WorkflowControlDelivery) (bool, error) {
	return true, nil
}
func (s *stateStore) CanDeliverWorkflowControl(context.Context, application.WorkflowControlDelivery) (bool, error) {
	return s.controlErr == nil, s.controlErr
}
func (s *stateStore) RecoverOperationTasks(ctx context.Context, d application.WorkflowControlDelivery) error {
	return s.ScheduleOperationStep(ctx, "operation.run", d.TargetID)
}
func (s *stateStore) ScheduleOriginalDelivery(context.Context, application.WorkflowControlDelivery) error {
	s.scheduled = append(s.scheduled, "operation.original")
	return nil
}
func (s *stateStore) handlers() map[string]taskruntime.Handler {
	h := map[string]taskruntime.Handler{}
	task.RegisterFlow(h, s, s, s)
	task.RegisterProvider(h, s, nil)
	return h
}
func (s *stateStore) run(t *testing.T, kind string) {
	t.Helper()
	_, err := s.handlers()[kind](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
}
func TestBatchOperationRejectsStandaloneTaskStart(t *testing.T) {
	s := newStateStore()
	s.evidence.Admitted = false
	_, err := s.handlers()["operation.run"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.ErrorIs(t, err, application.ErrInvalidWorkflowBatch)
	require.Empty(t, s.calls)
}
func TestOperationTaskValidatesInputAndTarget(t *testing.T) {
	s := newStateStore()
	for _, id := range []string{"", uuid.Nil.String(), "invalid"} {
		_, err := s.handlers()["operation.run"](t.Context(), taskruntime.Job{BusinessID: id})
		require.Error(t, err)
	}
	s.loaded.Operation.TargetType = "agent_session"
	_, err := s.handlers()["operation.run"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.ErrorIs(t, err, task.ErrInvalidOperationInput)
}
func TestOperationTaskCompletesReuseWithoutProviderCall(t *testing.T) {
	s := newStateStore()
	id := uuid.New()
	s.loaded.Operation.ReusedFromID = &id
	zero := int64(0)
	s.loaded.Operation.QuoteMicros = &zero
	s.run(t, "operation.run")
	require.Equal(t, 1, s.reused)
	require.Empty(t, s.calls)
	require.Empty(t, s.scheduled)
}
func TestOperationTaskFailsAndSettlesWhenReuseSourceDisappears(t *testing.T) {
	s := newStateStore()
	id := uuid.New()
	s.loaded.Operation.ReusedFromID = &id
	zero := int64(0)
	s.loaded.Operation.QuoteMicros = &zero
	s.reuseErr = application.ErrReuseSourceUnavailable
	s.run(t, "operation.run")
	require.Equal(t, "reuse_source_unavailable", s.settled[0].FailureCode)
	require.Zero(t, s.settled[0].ActualCostMicros)
}
func TestOperationTaskUnknownReconcilesWithoutResubmitting(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Params = json.RawMessage(`{"mock_outcome":"unknown","mock_result_urls":["https://fixture/a.png"],"mock_moderation_status":"passed"}`)
	for _, k := range []string{"operation.run", "operation.mock", "operation.run", "operation.run", "operation.mock", "operation.mock", "operation.run", "operation.run"} {
		s.run(t, k)
	}
	require.Equal(t, domain.StatusIngesting, s.loaded.Operation.Status)
	n := 0
	for _, c := range s.calls {
		if c.Action == "submit" {
			n++
		}
	}
	require.Equal(t, 1, n)
	require.Contains(t, s.transitions, domain.StatusUnknown)
	require.Contains(t, s.transitions, domain.StatusReconciling)
}
func TestOperationTaskCancelsSubmittedProviderWithEvidence(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitted
	s.loaded.Provider.SupportsCancel = true
	id := "mock-original"
	s.loaded.ProviderTaskID = &id
	s.evidence.SubmitTaskID = &id
	s.evidence.CancelRequested = true
	s.run(t, "operation.run")
	s.run(t, "operation.mock")
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusCancelled, s.loaded.Operation.Status)
	require.Equal(t, application.ProviderCallStateCancelled, s.evidence.CancelState)
	require.Len(t, s.calls, 1)
	require.Equal(t, "cancel", s.calls[0].Action)
}
func TestOperationTaskContinuesAfterProviderRejectsCancellation(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusCancelling
	id := "original"
	s.evidence.SubmitTaskID = &id
	s.evidence.CancelState = application.ProviderCallStateNotCancelled
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusSubmitted, s.loaded.Operation.Status)
	require.Empty(t, s.settled)
}
func TestOperationTaskRecordsAttemptBeforeDispatchAndSettlement(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Params = json.RawMessage(`{"mock_outcome":"rejected","mock_moderation_status":"passed"}`)
	s.run(t, "operation.run")
	s.run(t, "operation.mock")
	require.Len(t, s.calls, 1)
	require.Len(t, s.completes, 1)
	require.Empty(t, s.settled)
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusFailed, s.loaded.Operation.Status)
	require.Equal(t, "provider:failed", s.settled[0].FailureCode)
	require.Zero(t, s.settled[0].ActualCostMicros)
}
func TestOperationTaskManualNotExecutedRequiresVerifiedEvidence(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusManual
	s.run(t, "operation.run")
	require.Empty(t, s.settled)
	s.manualErr = nil
	s.run(t, "operation.run")
	require.Equal(t, "provider_not_executed", s.settled[0].FailureCode)
}
func TestOperationTaskPreSubmitBusinessFailureSettlesWithoutProviderCall(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{application.ErrWorkflowInputNotReady, "input_not_ready"}, {application.ErrWorkflowConsentUnavailable, "consent_unavailable"}} {
		s := newStateStore()
		s.inputErr = tc.err
		s.run(t, "operation.run")
		require.Equal(t, tc.code, s.settled[0].FailureCode)
		require.Empty(t, s.calls)
	}
}
func TestOperationTaskIngestFailureKeepsOnlyMediaRetry(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusIngesting
	s.run(t, "operation.run")
	require.Equal(t, []string{"operation.ingest"}, s.scheduled)
	require.Empty(t, s.calls)
	s.evidence.IngestFailed = true
	s.cost.ActualCostMicros = 7
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusFailed, s.loaded.Operation.Status)
	require.EqualValues(t, 7, s.settled[0].ActualCostMicros)
	require.True(t, s.settled[0].Retryable)
}
func TestOperationTaskSubmittedFailureSettlesWithSupplierCode(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusSubmitted
	s.evidence.QueryState = application.ProviderCallStateFailed
	s.evidence.QueryFailureCode = application.SafeProviderFailureCode("quota_exceeded", "failed")
	s.run(t, "operation.run")
	require.Equal(t, domain.StatusFailed, s.loaded.Operation.Status)
	require.Equal(t, application.SafeProviderFailureCode("quota_exceeded", "failed"), s.settled[0].FailureCode)
}
func TestOperationTaskFinalizationUsesDurableCostAfterIngest(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusIngesting
	s.cost.ActualCostMicros = 73
	s.evidence.Ingested = true
	s.run(t, "operation.run")
	s.run(t, "operation.run")
	require.Len(t, s.settled, 1)
	require.EqualValues(t, 73, s.settled[0].ActualCostMicros)
}
func TestOperationTaskRoutesRevokedAuthorityToFinancialClosure(t *testing.T) {
	s := newStateStore()
	s.transitionErr = application.ErrExecutionAuthorityRevoked
	s.run(t, "operation.run")
	require.Equal(t, 1, s.revoked)
	require.Empty(t, s.calls)
}

var _ = errors.Is
