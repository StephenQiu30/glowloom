package microservices_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
)

func supplierObservationFixture(t *testing.T) (*executionOutputFixture, executionapp.SupplierObservation) {
	t.Helper()
	f := newExecutionOutputFixture(t)
	id := f.req.Publication.Output.OperationID
	for _, q := range []string{
		`UPDATE operation.operation SET status='submitting' WHERE id=?`,
		`UPDATE operation.provider_call SET outcome='unknown',response_summary='{}',cost_micros=NULL,usage=NULL WHERE operation_id=?`,
		`UPDATE catalog.price_rule_version SET unit='per_request',rule='{"base_micros":1234}' WHERE id=(SELECT price_rule_version_id FROM operation.operation WHERE id=?)`,
	} {
		if err := f.admin.Exec(q, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	var original executionapp.ManagedResult
	original.Identity = executionapp.ProviderDispatchIdentity{ProjectID: f.req.Publication.Output.ProjectID, OperationID: id, Action: "submit", Attempt: 1, RequestKey: f.req.Publication.Output.Provider.RequestKey}
	if err := f.execution.Raw(`SELECT model_profile_version_id,price_rule_version_id FROM operation.operation WHERE id=?`, id).Row().Scan(&original.Identity.ModelProfileVersionID, &original.Identity.PriceRuleVersionID); err != nil {
		t.Fatal(err)
	}
	usage := []byte("{\n\"billable_requests\":0,\"original\":9007199254740993\n}")
	o := executionapp.SupplierObservation{Identity: original.Identity, CallAction: "submit", CallAttempt: 1, ReceivedAt: time.Now().UTC().Truncate(time.Microsecond), Outcome: executionapp.ProviderOutcomeCompleted, State: executionapp.ProviderTaskSucceeded, ImageCount: 1, SupplierUsage: usage}
	return f, o
}

func TestSupplierObservationPreservesKnownCostThroughArchiveFailure(t *testing.T) {
	f, o := supplierObservationFixture(t)
	id, usage := o.Identity.OperationID, o.SupplierUsage
	store := executionpg.NewStore(f.execution)
	if err := store.RecordSupplierObservation(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	assert := func() {
		t.Helper()
		var cost *int64
		var raw []byte
		if err := f.execution.Raw(`SELECT cost_micros,response_summary FROM operation.provider_call WHERE operation_id=? AND action='submit'`, id).Row().Scan(&cost, &raw); err != nil {
			t.Fatal(err)
		}
		var record struct {
			Observation struct {
				Original executionapp.SupplierObservation `json:"original"`
			} `json:"supplier_observation"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		if cost == nil || *cost != 1234 || !bytes.Equal(record.Observation.Original.SupplierUsage, usage) || !record.Observation.Original.ReceivedAt.Equal(o.ReceivedAt) {
			t.Fatalf("lost original liability: cost=%v record=%s", cost, raw)
		}
	}
	assert()
	// Archiving failed after the supplier returned. Workflow completion records
	// uncertainty, but must retain the already committed original liability.
	unknown := executionapp.CompleteProviderCallInput{OperationID: id, Action: "submit", Attempt: 1, Outcome: executionapp.ProviderCallOutcomeUnknown, State: executionapp.ProviderCallStateUnknown, ErrorCode: "archive_unavailable"}
	if err := store.CompleteProviderCall(t.Context(), unknown); err != nil {
		t.Fatal(err)
	}
	assert()
	if err := store.CompleteProviderCall(t.Context(), unknown); err != nil {
		t.Fatal("repeat unknown", err)
	}
	if err := store.RecordSupplierObservation(t.Context(), o); err != nil {
		t.Fatal("original recovery", err)
	}
	assert()
	changed := o
	changed.ReceivedAt = changed.ReceivedAt.Add(time.Second)
	if err := store.RecordSupplierObservation(t.Context(), changed); !errors.Is(err, executionapp.ErrProviderCallConflict) {
		t.Fatalf("changed original accepted: %v", err)
	}
	assert()
}

func TestSupplierObservationUnknownUsageRemainsNullAndQueryTaskIsExact(t *testing.T) {
	f, o := supplierObservationFixture(t)
	store := executionpg.NewStore(f.execution)
	o.SupplierUsage = []byte(`{"input_tokens":"untrusted"}`)
	if err := f.admin.Exec(`UPDATE catalog.price_rule_version SET unit='per_1k_tokens',rule='{"input_micros_per_1k":1,"output_micros_per_1k":1}' WHERE id=?`, o.Identity.PriceRuleVersionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSupplierObservation(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	unknown := executionapp.CompleteProviderCallInput{OperationID: o.Identity.OperationID, Action: "submit", Attempt: 1, Outcome: executionapp.ProviderCallOutcomeUnknown, State: executionapp.ProviderCallStateUnknown}
	if err := store.CompleteProviderCall(t.Context(), unknown); err != nil {
		t.Fatal(err)
	}
	var cost *int64
	if err := f.execution.Raw(`SELECT cost_micros FROM operation.provider_call WHERE operation_id=?`, o.Identity.OperationID).Row().Scan(&cost); err != nil {
		t.Fatal(err)
	}
	if cost != nil {
		t.Fatalf("unknown usage invented charge %d", *cost)
	}
	taskID := "original-task"
	if err := f.admin.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,provider_task_id,request_summary,outcome,region,request_key,model_profile_version_id,price_rule_version_id) SELECT gen_random_uuid(),project_id,operation_id,1,'query',provider_key,?,'{}','unknown',region,request_key,model_profile_version_id,price_rule_version_id FROM operation.provider_call WHERE operation_id=? AND action='submit'`, taskID, o.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	query := o
	query.CallAction = "query"
	query.TaskID = "another-task"
	if err := store.RecordSupplierObservation(t.Context(), query); !errors.Is(err, executionapp.ErrProviderCallConflict) {
		t.Fatalf("wrong task accepted: %v", err)
	}
	query.TaskID = taskID
	if err := store.RecordSupplierObservation(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}

func TestSupplierObservationRepeatedQueriesChargeOnceAndReplay(t *testing.T) {
	f, o := supplierObservationFixture(t)
	store := executionpg.NewStore(f.execution)
	taskID := "original-task"
	for attempt := int32(1); attempt <= 2; attempt++ {
		if err := f.admin.Exec(`INSERT INTO operation.provider_call(id,project_id,operation_id,attempt,action,provider_key,provider_task_id,request_summary,outcome,region,request_key,model_profile_version_id,price_rule_version_id) SELECT gen_random_uuid(),project_id,operation_id,?,'query',provider_key,?,'{}','unknown',region,request_key,model_profile_version_id,price_rule_version_id FROM operation.provider_call WHERE operation_id=? AND action='submit'`, attempt, taskID, o.Identity.OperationID).Error; err != nil {
			t.Fatal(err)
		}
		query := o
		query.CallAction = "query"
		query.CallAttempt = attempt
		query.TaskID = taskID
		if err := store.RecordSupplierObservation(t.Context(), query); err != nil {
			t.Fatal(err)
		}
		if err := store.RecordSupplierObservation(t.Context(), query); err != nil {
			t.Fatal("replay", err)
		}
	}
	first := o
	first.CallAction = "query"
	first.CallAttempt = 1
	first.TaskID = taskID
	if err := store.RecordSupplierObservation(t.Context(), first); err != nil {
		t.Fatal("late original ACK", err)
	}
	var cost int64
	if err := f.execution.Raw(`SELECT sum(cost_micros) FROM operation.provider_call WHERE operation_id=? AND action='query'`, o.Identity.OperationID).Row().Scan(&cost); err != nil {
		t.Fatal(err)
	}
	if cost != 1234 {
		t.Fatalf("duplicate queries charged %d", cost)
	}
}
