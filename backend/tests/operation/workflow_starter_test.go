package operation_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"gorm.io/gorm"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	operationevent "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/event"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationflow "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

type fakeStarterStore struct {
	eligible      bool
	stale         []uuid.UUID
	batchEligible bool
	staleBatches  []uuid.UUID
	checked       int
	offsets       []int
}

func (s *fakeStarterStore) ConfirmedSingle(_ context.Context, _, _, _, _ uuid.UUID) (bool, error) {
	s.checked++
	return s.eligible, nil
}

func (s *fakeStarterStore) StaleConfirmedSingles(_ context.Context, _ time.Time, limit, offset int) ([]uuid.UUID, error) {
	s.offsets = append(s.offsets, offset)
	if offset >= len(s.stale) {
		return nil, nil
	}
	end := min(offset+limit, len(s.stale))
	return s.stale[offset:end], nil
}

func (s *fakeStarterStore) ConfirmedBatch(_ context.Context, _, _, _ uuid.UUID) (bool, error) {
	s.checked++
	return s.batchEligible, nil
}

func (s *fakeStarterStore) StaleConfirmedBatches(_ context.Context, _ time.Time, limit, offset int) ([]uuid.UUID, error) {
	if offset >= len(s.staleBatches) {
		return nil, nil
	}
	return s.staleBatches[offset:min(offset+limit, len(s.staleBatches))], nil
}

type fakeWorkflowStarter struct {
	started        []uuid.UUID
	startedBatches []uuid.UUID
	err            error
}

func (s *fakeWorkflowStarter) StartOperation(_ context.Context, id uuid.UUID) error {
	if s.err != nil {
		return s.err
	}
	s.started = append(s.started, id)
	return nil
}

func (s *fakeWorkflowStarter) StartBatch(_ context.Context, id uuid.UUID) error {
	if s.err != nil {
		return s.err
	}
	s.startedBatches = append(s.startedBatches, id)
	return nil
}

type fakeStarterInbox struct{ done map[string]bool }

func (i *fakeStarterInbox) ProcessExternalOnce(ctx context.Context, _, eventID string, effect func(context.Context) error) (bool, error) {
	if i.done[eventID] {
		return false, nil
	}
	if err := effect(ctx); err != nil {
		return false, err
	}
	i.done[eventID] = true
	return true, nil
}

func TestWorkflowStarterHandlesConfirmedEventOnceAndRetriesFailure(t *testing.T) {
	projectID, operationID, reservationID, eventID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := &fakeStarterStore{eligible: true}
	start := &fakeWorkflowStarter{err: errors.New("Temporal unavailable")}
	inboxStore := &fakeStarterInbox{done: map[string]bool{}}
	handler := operationevent.NewWorkflowStarterHandler(inboxStore, store, start)
	record := inbox.Record{
		Topic: operationevent.OperationConfirmedTopic, Key: []byte(projectID.String()),
		Value: []byte(fmt.Sprintf(`{"event_id":%q,"event_type":%q,"project_id":%q,"org_id":%q,"occurred_at":%q,"actor":{"kind":"user","id":%q},"aggregate":{"type":"operation","id":%q},"data":{"operation_id":%q,"reservation_id":%q,"quote_micros":0}}`,
			eventID, operationevent.OperationConfirmedTopic, projectID, uuid.New(), time.Now().UTC().Format(time.RFC3339Nano), uuid.New(), operationID, operationID, reservationID)),
	}
	if err := handler.Handle(t.Context(), record); err == nil || inboxStore.done[eventID.String()] {
		t.Fatalf("failed start must stay retryable: %v", err)
	}
	start.err = nil
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if len(start.started) != 1 || start.started[0] != operationID || store.checked != 2 {
		t.Fatalf("starts=%v checks=%d", start.started, store.checked)
	}
	bad := record
	bad.Key = []byte(uuid.NewString())
	if err := handler.Handle(t.Context(), bad); err == nil {
		t.Fatal("mismatched partition key accepted")
	}
}

func TestWorkflowStarterSkipsIneligibleAndSweepsStaleSingles(t *testing.T) {
	projectID, operationID, reservationID := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStarterStore{stale: []uuid.UUID{operationID}}
	start := &fakeWorkflowStarter{}
	handler := operationevent.NewWorkflowStarterHandler(&fakeStarterInbox{done: map[string]bool{}}, store, start)
	record := inbox.Record{
		Topic: operationevent.OperationConfirmedTopic, Key: []byte(projectID.String()),
		Value: []byte(fmt.Sprintf(`{"event_id":%q,"event_type":%q,"project_id":%q,"org_id":%q,"occurred_at":%q,"actor":{"kind":"user","id":%q},"aggregate":{"type":"operation","id":%q},"data":{"operation_id":%q,"reservation_id":%q,"quote_micros":1}}`,
			uuid.New(), operationevent.OperationConfirmedTopic, projectID, uuid.New(), time.Now().UTC().Format(time.RFC3339Nano), uuid.New(), operationID, operationID, reservationID)),
	}
	if err := handler.Handle(t.Context(), record); err != nil || len(start.started) != 0 {
		t.Fatalf("ineligible event started: %v, %v", err, start.started)
	}
	if err := handler.SweepOnce(t.Context(), time.Now().UTC()); err != nil || len(start.started) != 1 {
		t.Fatalf("sweep failed: %v, starts=%v", err, start.started)
	}
}

func TestWorkflowStarterSweepPaginatesPastAlreadyStartedOperations(t *testing.T) {
	store := &fakeStarterStore{stale: make([]uuid.UUID, 101)}
	for i := range store.stale {
		store.stale[i] = uuid.New()
	}
	start := &fakeWorkflowStarter{}
	handler := operationevent.NewWorkflowStarterHandler(&fakeStarterInbox{done: map[string]bool{}}, store, start)
	if err := handler.SweepOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(start.started) != 101 || len(store.offsets) != 2 || store.offsets[1] != 100 {
		t.Fatalf("sweep starts=%d offsets=%v", len(start.started), store.offsets)
	}
}

func TestWorkflowStarterHandlesBatchConfirmationAndSweep(t *testing.T) {
	projectID, batchID, eventID := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStarterStore{batchEligible: true, staleBatches: []uuid.UUID{batchID}}
	starter := &fakeWorkflowStarter{err: errors.New("Temporal unavailable")}
	inboxStore := &fakeStarterInbox{done: map[string]bool{}}
	handler := operationevent.NewWorkflowStarterHandler(inboxStore, store, starter)
	record := inbox.Record{
		Topic: operationevent.BatchConfirmedTopic, Key: []byte(projectID.String()),
		Value: []byte(fmt.Sprintf(`{"event_id":%q,"event_type":%q,"project_id":%q,"org_id":%q,"occurred_at":%q,"actor":{"kind":"user","id":%q},"aggregate":{"type":"batch","id":%q},"data":{"batch_id":%q,"total_count":2,"quote_total_micros":0}}`,
			eventID, operationevent.BatchConfirmedTopic, projectID, uuid.New(), time.Now().UTC().Format(time.RFC3339Nano), uuid.New(), batchID, batchID)),
	}
	if err := handler.Handle(t.Context(), record); err == nil || inboxStore.done[eventID.String()] {
		t.Fatalf("failed batch start must remain retryable: %v", err)
	}
	starter.err = nil
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if len(starter.startedBatches) != 1 || starter.startedBatches[0] != batchID || len(starter.started) != 0 {
		t.Fatalf("batch starts=%v operation starts=%v", starter.startedBatches, starter.started)
	}
	if err := handler.SweepBatchesOnce(t.Context(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(starter.startedBatches) != 2 {
		t.Fatalf("batch sweep starts=%v", starter.startedBatches)
	}
	bad := record
	bad.Key = []byte(uuid.NewString())
	if err := handler.Handle(t.Context(), bad); err == nil {
		t.Fatal("mismatched batch partition key accepted")
	}
}

type fakeTemporalExecutor struct {
	options    client.StartWorkflowOptions
	input      operationflow.OperationInput
	batchInput operationflow.BatchInput
	workflow   interface{}
	err        error
}

func (f *fakeTemporalExecutor) ExecuteWorkflow(_ context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	f.options = options
	f.workflow = workflow
	switch input := args[0].(type) {
	case operationflow.OperationInput:
		f.input = input
	case operationflow.BatchInput:
		f.batchInput = input
	}
	return nil, f.err
}

func TestOperationStarterRejectsDuplicateTemporalWorkflowID(t *testing.T) {
	id := uuid.New()
	executor := &fakeTemporalExecutor{}
	starter := operationflow.NewStarter(executor)
	if err := starter.StartOperation(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if executor.options.ID != "operation/"+id.String() || executor.options.TaskQueue != "flow" ||
		executor.options.WorkflowIDReusePolicy != enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE ||
		executor.options.WorkflowIDConflictPolicy != enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL ||
		!executor.options.WorkflowExecutionErrorWhenAlreadyStarted || executor.input.OperationID != id.String() {
		t.Fatalf("Temporal start contract = %+v, input=%+v", executor.options, executor.input)
	}
	executor.err = serviceerror.NewWorkflowExecutionAlreadyStarted("already started", executor.options.ID, "run-1")
	if err := starter.StartOperation(t.Context(), id); err != nil {
		t.Fatalf("duplicate start: %v", err)
	}
	executor.err = errors.New("Temporal unavailable")
	if err := starter.StartOperation(t.Context(), id); err == nil {
		t.Fatal("transient failure was acknowledged")
	}
}

func TestBatchStarterRejectsDuplicateTemporalWorkflowID(t *testing.T) {
	id := uuid.New()
	executor := &fakeTemporalExecutor{}
	starter := operationflow.NewStarter(executor)
	if err := starter.StartBatch(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if executor.options.ID != "batch/"+id.String() || executor.options.TaskQueue != "flow" ||
		executor.options.WorkflowIDReusePolicy != enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE ||
		executor.options.WorkflowIDConflictPolicy != enums.WORKFLOW_ID_CONFLICT_POLICY_FAIL ||
		!executor.options.WorkflowExecutionErrorWhenAlreadyStarted ||
		executor.workflow != "BatchWorkflow" || executor.batchInput.BatchID != id.String() {
		t.Fatalf("batch Temporal contract = %+v, input=%+v", executor.options, executor.batchInput)
	}
	executor.err = serviceerror.NewWorkflowExecutionAlreadyStarted("already started", executor.options.ID, "run-1")
	if err := starter.StartBatch(t.Context(), id); err != nil {
		t.Fatalf("duplicate batch start: %v", err)
	}
	executor.err = errors.New("Temporal unavailable")
	if err := starter.StartBatch(t.Context(), id); err == nil {
		t.Fatal("transient batch start failure was acknowledged")
	}
}

func TestWorkflowStarterReadsCommittedSingleAndSweepCandidates(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	quoted, _, _ := seedQuoteCatalog(t, database, projectID, time.Now().UTC())
	item := quotedSnapshotItem(projectID, 1)
	item.Operation = quoted
	item.Inputs[0].OperationID = quoted.ID
	store := pgoperation.NewStore(database)
	if err := store.CreateQuoteSnapshot(t.Context(), actor, nil, []pgoperation.QuoteItem{item}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.ConfirmSingleQuote(t.Context(), actor, operationapp.ConfirmSingleQuoteInput{
		ProjectID: projectID, OperationID: quoted.ID, RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		orgID, projectID, reservationID uuid.UUID
		want                            bool
	}{
		{actor.OrgID, projectID, confirmed.ReservationID, true},
		{actor.OrgID, projectID, uuid.New(), false},
		{actor.OrgID, uuid.New(), confirmed.ReservationID, false},
		{uuid.New(), projectID, confirmed.ReservationID, false},
	} {
		got, err := store.ConfirmedSingle(t.Context(), tc.orgID, tc.projectID, quoted.ID, tc.reservationID)
		if err != nil || got != tc.want {
			t.Fatalf("confirmed single = %v, %v; want %v", got, err, tc.want)
		}
	}
	tooYoung, err := store.StaleConfirmedSingles(t.Context(), time.Now().Add(-2*time.Minute), 100, 0)
	if err != nil || containsOperationID(tooYoung, quoted.ID) {
		t.Fatalf("young confirmation in sweep: %v, %v", tooYoung, err)
	}
	stale, err := store.StaleConfirmedSingles(t.Context(), time.Now().Add(3*time.Minute), 100, 0)
	if err != nil || !containsOperationID(stale, quoted.ID) {
		t.Fatalf("old confirmation missing from sweep: %v, %v", stale, err)
	}
	batchID := uuid.New()
	if err := database.Exec(`
		INSERT INTO operation.batch (id, project_id, kind, scope, status, total_count, quote_total_micros, workflow_id)
		VALUES (?::uuid, ?::uuid, 'mixed', '{}'::jsonb, 'confirmed', 1, 1, ?)
	`, batchID.String(), projectID.String(), "batch/"+batchID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE operation.operation SET batch_id = ?::uuid WHERE id = ?::uuid`,
		batchID.String(), quoted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	eligible, err := store.ConfirmedSingle(t.Context(), actor.OrgID, projectID, quoted.ID, confirmed.ReservationID)
	if err != nil || eligible {
		t.Fatalf("batch member may not start alone: %v, %v", eligible, err)
	}
	stale, err = store.StaleConfirmedSingles(t.Context(), time.Now().Add(3*time.Minute), 100, 0)
	if err != nil || containsOperationID(stale, quoted.ID) {
		t.Fatalf("batch member in single sweep: %v, %v", stale, err)
	}
	batchEligible, err := store.ConfirmedBatch(t.Context(), actor.OrgID, projectID, batchID)
	if err != nil || !batchEligible {
		t.Fatalf("confirmed batch eligibility: %v, %v", batchEligible, err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, batch_id, target_type, capability, mode,
		   model_profile_version_id, price_rule_version_id, params, output_count,
		   input_hash, origin, status, quote_micros, quote_expires_at, region)
		SELECT ?::uuid, project_id, ?::uuid, target_type, capability, mode,
		       model_profile_version_id, price_rule_version_id, params, output_count,
		       ?::text, origin, 'expired', quote_micros, quote_expires_at, region
		FROM operation.operation WHERE id = ?::uuid
	`, uuid.NewString(), batchID.String(), uuid.NewString(), quoted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	batchEligible, err = store.ConfirmedBatch(t.Context(), actor.OrgID, projectID, batchID)
	if err != nil || !batchEligible {
		t.Fatalf("excluded expired item blocked selected batch: %v, %v", batchEligible, err)
	}
	snapshot, err := store.LoadWorkflowBatch(t.Context(), batchID)
	if err != nil || snapshot.Batch.ID != batchID || len(snapshot.Items) != 1 ||
		snapshot.Items[0].OperationID != quoted.ID || snapshot.Items[0].Status != "confirmed" {
		t.Fatalf("confirmed batch snapshot: %+v, %v", snapshot, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		_, err := pgoperation.NewStore(tx).LoadWorkflowBatch(t.Context(), batchID)
		return err
	}); err != nil {
		t.Fatalf("runtime role batch snapshot: %v", err)
	}
	batchStale, err := store.StaleConfirmedBatches(t.Context(), time.Now().Add(3*time.Minute), 100, 0)
	if err != nil || !containsOperationID(batchStale, batchID) {
		t.Fatalf("confirmed batch missing from sweep: %v, %v", batchStale, err)
	}
	if err := store.MarkWorkflowBatchRunning(t.Context(), batchID); err != nil {
		t.Fatalf("mark batch running: %v", err)
	}
	if err := store.MarkWorkflowBatchRunning(t.Context(), batchID); err != nil {
		t.Fatalf("replay running batch transition: %v", err)
	}
	snapshot, err = store.LoadWorkflowBatch(t.Context(), batchID)
	if err != nil || snapshot.Batch.Status != "running" {
		t.Fatalf("running batch snapshot: %+v, %v", snapshot, err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		acquired, err := pgoperation.NewStore(tx).AcquireBatchLaunch(t.Context(), batchID, quoted.ID)
		if err == nil && !acquired {
			return errors.New("runtime role did not acquire batch slot")
		}
		return err
	}); err != nil {
		t.Fatalf("runtime role batch admission: %v", err)
	}
	acquired, err := store.AcquireBatchLaunch(t.Context(), batchID, quoted.ID)
	if err != nil || !acquired {
		t.Fatalf("batch admission replay: %v, %v", acquired, err)
	}
	var activeSlots int64
	if err := database.Raw(`SELECT count(*) FROM operation.batch_launch WHERE operation_id = ?::uuid AND state = 'active'`,
		quoted.ID.String()).Scan(&activeSlots).Error; err != nil || activeSlots != 1 {
		t.Fatalf("active batch slots = %d, %v", activeSlots, err)
	}
	batchStale, err = store.StaleConfirmedBatches(t.Context(), time.Now().Add(3*time.Minute), 100, 0)
	if err != nil || containsOperationID(batchStale, batchID) {
		t.Fatalf("running batch remained in sweep: %v, %v", batchStale, err)
	}
	if err := database.Exec(`UPDATE operation.operation SET reservation_id = NULL WHERE id = ?::uuid`,
		quoted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	batchEligible, err = store.ConfirmedBatch(t.Context(), actor.OrgID, projectID, batchID)
	if err != nil || batchEligible {
		t.Fatalf("batch with unreserved child eligible: %v, %v", batchEligible, err)
	}
	if _, err := store.LoadWorkflowBatch(t.Context(), batchID); err == nil {
		t.Fatal("batch snapshot accepted unreserved child")
	}
	if err := database.Exec(`
		UPDATE operation.operation
		SET batch_id = NULL, target_type = 'agent_session',
		    model_profile_version_id = NULL, price_rule_version_id = NULL
		WHERE id = ?::uuid
	`, quoted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	eligible, err = store.ConfirmedSingle(t.Context(), actor.OrgID, projectID, quoted.ID, confirmed.ReservationID)
	if err != nil || eligible {
		t.Fatalf("agent session may not start workflow: %v, %v", eligible, err)
	}
}

func containsOperationID(ids []uuid.UUID, target uuid.UUID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
