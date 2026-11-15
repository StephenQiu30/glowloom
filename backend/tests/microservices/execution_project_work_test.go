package microservices_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	executionpg "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/postgres"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func executionProjectWorkRequest(t *testing.T, f *frozenFixture) executionapp.ProjectWorkRequest {
	t.Helper()
	var org uuid.UUID
	if err := f.admin.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, f.request.ProjectID).Row().Scan(&org); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	for _, a := range f.request.Command.Authorities {
		if a.Kind == writefence.Project {
			epoch = a.Epoch
		}
	}
	return executionapp.ProjectWorkRequest{CloseID: uuid.New(), ProjectID: f.request.ProjectID, OrganizationID: org, ScopeID: f.request.ScopeID, ProjectEpoch: epoch, ManifestSHA256: strings.Repeat("a", 64)}
}
func assertExecutionProjectWork(t *testing.T, db *gorm.DB, request executionapp.ProjectWorkRequest, expected executionapp.ProjectWorkState) executionapp.ProjectWorkSnapshot {
	t.Helper()
	got, err := executionpg.NewProjectWorkStore(db).ReadProjectWork(t.Context(), request)
	if err != nil || got.BusinessState != expected {
		t.Fatalf("project responsibility=%+v want=%s err=%v", got, expected, err)
	}
	return got
}

func TestExecutionProjectWorkZeroCommandsStillReadsAdmittedAndAcceptedResponsibility(t *testing.T) {
	f := newFrozenFixture(t)
	request := executionProjectWorkRequest(t, f)
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkBusy)
	envelope := f.freeze(t)
	store := executionpg.NewFrozenInputStore(f.execution, localFrozenSource(f.store))
	if _, err := store.AcceptFrozenInput(t.Context(), executionKey(envelope)); err != nil {
		t.Fatal(err)
	}
	request.Commands = []writefence.Command{f.request.Command}
	got := assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkUnknown)
	if len(got.Commands) != 1 || got.Commands[0].State != writefence.Committed || len(got.Commands[0].ReceiptSHA256) != 64 {
		t.Fatalf("lost original technical receipt: %+v", got.Commands)
	}
	// The input ACK created no operation or money. Even an empty scheduler is not closure.
	if err := f.admin.Exec(`DELETE FROM infra.task_work WHERE service_owner='execution' AND business_id=?`, envelope.Input.HandoffID.String()).Error; err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkUnknown)
	changed := request
	changed.Commands = append([]writefence.Command(nil), request.Commands...)
	changed.Commands[0].SHA256 = strings.Repeat("f", 64)
	if _, err := executionpg.NewProjectWorkStore(f.execution).ReadProjectWork(t.Context(), changed); !errors.Is(err, writefence.ErrConflict) {
		t.Fatalf("changed original command accepted: %v", err)
	}
	cancel := f.cancelCommand(t)
	if _, err := f.store.BlockFrozenInput(t.Context(), envelope.Key(), cancel); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CancelFrozenInput(t.Context(), executionKey(envelope), cancel); err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkIdle)
	missing := f.request.Command
	missing.ID = uuid.New()
	request.Commands = []writefence.Command{missing}
	got = assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkUnknown)
	if got.Commands[0].State != writefence.State("unknown") {
		t.Fatal("missing command inferred idle", got)
	}
	request.Commands = nil
	request.ProjectEpoch++
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkUnknown)
	if _, err := executionpg.NewProjectWorkStore(f.creation).ReadProjectWork(t.Context(), request); !errors.Is(err, executionapp.ErrFrozenInputOwner) {
		t.Fatalf("Creation role accepted: %v", err)
	}
	if err := f.execution.Transaction(func(tx *gorm.DB) error {
		_, err := executionpg.NewProjectWorkStore(tx).ReadProjectWork(t.Context(), request)
		if !errors.Is(err, writefence.ErrTransaction) {
			t.Fatalf("outer SQL transaction accepted: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionProjectWorkRequiresActualMoneyAndOutputClosure(t *testing.T) {
	f := newExecutionOutputFixture(t)
	request := executionProjectWorkRequest(t, f.frozenFixture)
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkBusy)
	if _, err := f.store.PrepareOutputDelivery(t.Context(), f.req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.StartOutputDelivery(t.Context(), f.req); err != nil {
		t.Fatal(err)
	}
	// A terminal label and an absent scheduler cannot adopt Media's missing ACK.
	if err := f.admin.Exec(`UPDATE operation.operation SET status='completed',settled_micros=1234 WHERE id=?`, f.req.Publication.Output.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkBusy)
	if err := f.admin.Exec(`UPDATE operation.operation SET status='ingesting',settled_micros=NULL WHERE id=?`, f.req.Publication.Output.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.store.AdoptOutputDelivery(t.Context(), f.req, f.ack); err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkIdle)
	// A claimed settlement amount must still match the append-only money ledger.
	if err := f.admin.Exec(`UPDATE operation.operation SET settled_micros=1235 WHERE id=?`, f.req.Publication.Output.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkUnknown)
	if err := f.admin.Exec(`UPDATE operation.operation SET settled_micros=1234 WHERE id=?`, f.req.Publication.Output.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkIdle)
	// Historical cost cannot disappear behind the terminal label and closed reservation.
	if err := f.admin.Exec(`UPDATE operation.provider_call SET cost_micros=NULL WHERE operation_id=?`, f.req.Publication.Output.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkUnknown)
}

func TestExecutionProjectWorkUnadoptedRawArtifactBlocksTerminalOperation(t *testing.T) {
	for _, state := range []string{"collecting", "prepared"} {
		t.Run(state, func(t *testing.T) {
			f := newExecutionOutputFixture(t)
			request := executionProjectWorkRequest(t, f.frozenFixture)
			if _, err := f.store.PrepareOutputDelivery(t.Context(), f.req); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.StartOutputDelivery(t.Context(), f.req); err != nil {
				t.Fatal(err)
			}
			if err := f.store.AdoptOutputDelivery(t.Context(), f.req, f.ack); err != nil {
				t.Fatal(err)
			}
			assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkIdle)
			if err := f.admin.Exec(`INSERT INTO operation.provider_artifact_delivery(delivery_id,media_command_id,provider_call_id,provider_call_created_at,operation_id,project_id,call_action,call_attempt,request_key,artifact_seq,byte_size,sha256,manifest_sha256,request_sha256,request,chunk_count,state,attempted,sealed_at) SELECT ?,?,id,create_time,operation_id,project_id,'submit',1,request_key,1,12,?,?,?,'{}',1,?,?::boolean,CASE WHEN ?='prepared' THEN now() ELSE NULL END FROM operation.provider_call WHERE operation_id=? AND action='submit'`, uuid.New(), uuid.New(), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64), state, state == "prepared", state, f.req.Publication.Output.OperationID).Error; err != nil {
				t.Fatal(err)
			}
			assertExecutionProjectWork(t, f.execution, request, executionapp.ProjectWorkBusy)
		})
	}
}
