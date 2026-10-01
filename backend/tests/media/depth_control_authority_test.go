package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func depthControlAdmin(t *testing.T, f *depthHTTPFixture) identityapp.Principal {
	t.Helper()
	id := uuid.New()
	if err := f.db.Exec(`INSERT INTO identity."user" (id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,? ,?,'Depth controller','admin','test-hash',false)`, id, f.actor.OrgID, "depth-control-"+id.String()).Error; err != nil {
		t.Fatal(err)
	}
	return identityapp.Principal{ID: id, OrgID: f.actor.OrgID, Role: identitydomain.RoleAdmin}
}

func depthControlThroughHTTP(t *testing.T, f *depthHTTPFixture, j tooldomain.DepthJob, action string) tooldomain.DepthJob {
	t.Helper()
	r := f.request(t, "POST", "/api/media-depths/"+j.ID.String()+"/"+action, uuid.New(), toolapp.ControlInput{ProjectID: f.project, Revision: j.Revision})
	var result tooldomain.DepthJob
	if r.Code != 202 || json.Unmarshal(r.Body.Bytes(), &result) != nil {
		t.Fatalf("current controller %s: %d %s", action, r.Code, r.Body.String())
	}
	return result
}

func depthControlWorkflow(t *testing.T, f *depthHTTPFixture, worker *toolapp.DepthWorker, id toolapp.DepthWorkID) tooldomain.DepthJob {
	t.Helper()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetTestTimeout(2 * time.Minute)
	a := toolflow.NewDepthActivities(worker, f.store)
	env.RegisterActivityWithOptions(a.ProcessDepth, activity.RegisterOptions{Name: toolflow.DepthActivity})
	env.RegisterActivityWithOptions(a.InterruptDepth, activity.RegisterOptions{Name: toolflow.DepthFailureActivity})
	env.ExecuteWorkflow(toolflow.DepthWorkflow, id)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal("committed control under Temporal SDK", err)
	}
	var job tooldomain.DepthJob
	if err := env.GetWorkflowResult(&job); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestDepthJobPGAdministratorCancelsAfterCreatorChange(t *testing.T) {
	for _, change := range []string{"disabled", "role_changed"} {
		t.Run(change, func(t *testing.T) {
			f := depthHTTP(t)
			j := f.create(t)
			creator := f.actor
			admin := depthControlAdmin(t, f)
			q := `UPDATE identity."user" SET status='disabled' WHERE id=?`
			if change == "role_changed" {
				q = `UPDATE identity."user" SET role='admin' WHERE id=?`
			}
			if err := f.db.Exec(q, creator.ID).Error; err != nil {
				t.Fatal(err)
			}
			f.actor = admin
			depthControlThroughHTTP(t, f, j, "cancel")
			stopped := depthControlWorkflow(t, f, toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1})
			if stopped.Status != tooldomain.DepthCancelled || stopped.ExecutionUnconfirmed || stopped.NeedsReconciliation {
				t.Fatalf("administrator cancellation not complete: %+v", stopped)
			}
			if busy, err := f.store.HasInflightWork(t.Context(), admin, f.project); err != nil || busy {
				t.Fatal("completed queued cancel still blocks project", busy, err)
			}
		})
	}
}

func TestDepthJobEndedInterruptPreservesPhysicalTruth(t *testing.T) {
	w := uuid.New()
	s := tooldomain.DepthExecution{WorkerID: w, ProcessState: tooldomain.DepthProcessEnded, Job: tooldomain.DepthJob{Status: tooldomain.DepthRunning, Revision: 7}}
	if err := s.Interrupt(w); err != nil {
		t.Fatal(err)
	}
	if s.ProcessState != tooldomain.DepthProcessEnded || s.Job.ExecutionUnconfirmed || !s.Job.NeedsReconciliation || s.WorkerID != w || s.Job.Status != tooldomain.DepthFailed {
		t.Fatalf("orchestration loss erased confirmed cessation: %+v", s)
	}
}

func TestDepthJobPGEndedInterruptRetainsFenceWithoutInventingUnknown(t *testing.T) {
	f := depthHTTP(t)
	j := f.create(t)
	id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := f.store.Claim(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	// This tests durable protocol facts directly. Actual subprocess cessation is
	// independently covered by TestDepthJobActualRevocationCancelWaitsNativeAndTemporalFinish.
	if err := f.store.StartProcess(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := f.store.EndProcess(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Interrupt(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	var state struct {
		ProcessState                              string
		ActiveWorker                              *uuid.UUID
		ExecutionUnconfirmed, NeedsReconciliation bool
		FailureCode                               string
	}
	if err := f.db.Raw(`SELECT process_state,active_worker,execution_unconfirmed,needs_reconciliation,failure_code FROM mediatool.depth_job WHERE id=?`, j.ID).Scan(&state).Error; err != nil || state.ProcessState != "ended" || state.ActiveWorker == nil || *state.ActiveWorker != id.ExecutionID || state.ExecutionUnconfirmed || !state.NeedsReconciliation || state.FailureCode != "depth_object_unknown" {
		t.Fatal("interruption erased confirmed cessation or released owner", state, err)
	}
	if _, err := f.store.Claim(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true}); !errors.Is(err, toolapp.ErrWorkerBusy) {
		t.Fatal("ended process incorrectly released active consumer", err)
	}
	finished, err := f.store.Finish(t.Context(), id, true, true, "depth_cleanup_unknown")
	if err != nil || finished.ExecutionUnconfirmed || !finished.NeedsReconciliation || finished.Status != tooldomain.DepthFailed {
		t.Fatal("synchronous owner completion lost object uncertainty", finished, err)
	}
}

type depthRevokedNative struct {
	toolapp.DepthProcessor
	t        *testing.T
	f        *depthHTTPFixture
	creator  identityapp.Principal
	admin    identityapp.Principal
	job      uuid.UUID
	children []int
}

func (p *depthRevokedNative) Process(ctx context.Context, input *mediaapp.Downloaded, sha string, report func(toolapp.DepthPhase) error) (*toolapp.DepthOutput, error) {
	return p.DepthProcessor.Process(ctx, input, sha, func(phase toolapp.DepthPhase) error {
		if err := report(phase); err != nil {
			return err
		}
		if phase != toolapp.DepthInferring {
			return nil
		}
		if err := p.f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, p.creator.ID).Error; err != nil {
			return err
		}
		p.f.actor = p.admin
		current, err := p.f.store.Get(ctx, p.admin, p.f.project, p.job)
		if err != nil {
			return err
		}
		if _, err := p.f.store.Control(ctx, p.admin, p.f.project, p.job, uuid.New(), current.Revision, "cancel"); err != nil {
			return err
		}
		for _, child := range depthOwnedChildren(p.t, os.Getpid()) {
			if strings.Contains(strings.ToLower(child.program), "python") {
				p.children = append(p.children, child.pid)
			}
		}
		return toolapp.ErrCancelled
	})
}

func TestDepthJobActualRevocationCancelWaitsNativeAndTemporalFinish(t *testing.T) {
	native := depthNativeRunner(t)
	f := depthHTTP(t)
	j := f.create(t)
	p := &depthRevokedNative{DepthProcessor: native, t: t, f: f, creator: f.actor, admin: depthControlAdmin(t, f), job: j.ID}
	stopped := depthControlWorkflow(t, f, f.worker(t, p, toolflow.NewObjects(f.objects)), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1})
	if stopped.Status != tooldomain.DepthCancelled || stopped.ExecutionUnconfirmed || stopped.NeedsReconciliation || len(p.children) == 0 {
		t.Fatalf("revoked creator cancellation manufactured cessation: %+v", stopped)
	}
	depthAssertStopped(t, p.children)
	var state struct {
		ProcessState string
		ActiveWorker *uuid.UUID
	}
	if err := f.db.Raw(`SELECT process_state,active_worker FROM mediatool.depth_job WHERE id=?`, j.ID).Scan(&state).Error; err != nil || state.ProcessState != "ended" || state.ActiveWorker != nil {
		t.Fatal("Temporal completion lost physical stop evidence", state, err)
	}
}

type depthRemoveFault struct {
	toolapp.DepthObjects
	deny bool
}

func (o *depthRemoveFault) Remove(ctx context.Context, key string) error {
	if o.deny {
		return io.ErrUnexpectedEOF
	}
	return o.DepthObjects.Remove(ctx, key)
}

func TestDepthJobActualAdministratorCleanupFailureReconcilesOriginalObjects(t *testing.T) {
	native := depthNativeRunner(t)
	f := depthHTTP(t)
	j := f.create(t)
	p := &countedDepth{DepthProcessor: native}
	unknownObjects := &unknownDepthObjects{DepthObjects: toolflow.NewObjects(f.objects), lastOnly: true}
	unknownObjects.unknown.Store(true)
	unknown, err := f.worker(t, p, unknownObjects).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if err == nil || !unknown.NeedsReconciliation || unknown.ExecutionUnconfirmed {
		t.Fatal("real objects unknown admission missing", err)
	}
	creator := f.actor
	admin := depthControlAdmin(t, f)
	if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, creator.ID).Error; err != nil {
		t.Fatal(err)
	}
	f.actor = admin
	// A controller may read and confirm the original immutable objects, but it
	// cannot borrow that intent to publish for a revoked source creator.
	depthControlThroughHTTP(t, f, unknown, "reconcile")
	publication, err := toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true})
	if !errors.Is(err, identityapp.ErrForbidden) || !publication.NeedsReconciliation || publication.ExecutionUnconfirmed || publication.Status != tooldomain.DepthFailed || p.calls.Load() != 1 {
		t.Fatalf("control intent granted creator publication: %+v %v", publication, err)
	}
	var published int
	if err := f.db.Raw(`SELECT count(*) FROM media.media_asset WHERE id IN(SELECT asset_id FROM mediatool.depth_result_intent WHERE job_id=?)`, j.ID).Scan(&published).Error; err != nil || published != 0 {
		t.Fatal("unauthorized result metadata published", published, err)
	}
	requested := depthControlThroughHTTP(t, f, publication, "cancel")
	depthControlThroughHTTP(t, f, requested, "reconcile")
	fault := &depthRemoveFault{DepthObjects: toolflow.NewObjects(f.objects), deny: true}
	failed, err := toolapp.NewDepthWorker(f.store, fault, nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true})
	if err == nil || failed.Status != tooldomain.DepthFailed || !failed.NeedsReconciliation || failed.ExecutionUnconfirmed || p.calls.Load() != 1 {
		t.Fatalf("cleanup error manufactured cancellation or native uncertainty: %+v %v", failed, err)
	}
	replacement := depthControlAdmin(t, f)
	if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	f.actor = replacement
	depthControlThroughHTTP(t, f, failed, "reconcile")
	fault.deny = false
	stopped := depthControlWorkflow(t, f, toolapp.NewDepthWorker(f.store, fault, nil, nil, nil, nil), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, Reconcile: true})
	if stopped.Status != tooldomain.DepthCancelled || stopped.NeedsReconciliation || stopped.ExecutionUnconfirmed || p.calls.Load() != 1 {
		t.Fatalf("original objects not safely cleaned: %+v", stopped)
	}
	var remaining int
	if err := f.db.Raw(`SELECT count(*) FROM mediatool.depth_object WHERE job_id=? AND status<>'removed'`, j.ID).Scan(&remaining).Error; err != nil || remaining != 0 {
		t.Fatal("object absence not permanently confirmed", remaining, err)
	}
	if _, err := f.store.Create(t.Context(), creator, f.project, uuid.New(), f.input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("cleanup authority restored creator generation permission", err)
	}
	var auditActor string
	if err := f.db.Raw(`SELECT payload->'actor'->>'id' FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->'data'->>'action'='media.depth_cancelled' AND payload->'data'->'object'->>'id'=?`, j.ID.String()).Scan(&auditActor).Error; err != nil || auditActor != replacement.ID.String() {
		t.Fatal("cancelled fact attributed to revoked creator/controller", auditActor, err)
	}
}

func TestDepthJobPGCleanupRequiresExactLivePermanentControl(t *testing.T) {
	for _, fault := range []string{"disabled_controller", "actor", "request", "action", "attempt", "missing_outbox", "hash", "foreign_actor", "another_controller", "another_request"} {
		t.Run(fault, func(t *testing.T) {
			f := depthHTTP(t)
			j := f.create(t)
			admin := depthControlAdmin(t, f)
			f.actor = admin
			depthControlThroughHTTP(t, f, j, "cancel")
			var command struct{ RequestID, EventID uuid.UUID }
			if err := f.db.Raw(`SELECT request_id,event_id FROM mediatool.depth_command WHERE job_id=? AND event_action='cancel'`, j.ID).Scan(&command).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			switch fault {
			case "disabled_controller":
				err = f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, admin.ID).Error
			case "actor", "request":
				field := "actor_id"
				if fault == "request" {
					field = "request_id"
				}
				err = f.db.Exec(`UPDATE infra.outbox SET payload=jsonb_set(payload,ARRAY['data',?],to_jsonb(?::text)) WHERE id=?`, field, uuid.NewString(), command.EventID).Error
			case "action":
				err = f.db.Exec(`UPDATE infra.outbox SET payload=jsonb_set(payload,'{data,action}','"start"') WHERE id=?`, command.EventID).Error
			case "attempt":
				err = f.db.Exec(`UPDATE infra.outbox SET payload=jsonb_set(payload,'{data,attempt}','2') WHERE id=?`, command.EventID).Error
			case "missing_outbox":
				err = f.db.Exec(`DELETE FROM infra.outbox WHERE id=?`, command.EventID).Error
			case "hash":
				err = f.db.Exec(`UPDATE mediatool.depth_command SET request_hash=repeat('0',64) WHERE actor_id=? AND request_id=?`, admin.ID, command.RequestID).Error
			case "foreign_actor", "another_controller":
				var candidate identityapp.Principal
				if fault == "foreign_actor" {
					candidate, _ = mediaStoreProject(t, f.db)
				} else {
					candidate = depthControlAdmin(t, f)
				}
				err = f.db.Exec(`UPDATE mediatool.depth_command SET actor_id=? WHERE actor_id=? AND request_id=?`, candidate.ID, admin.ID, command.RequestID).Error
				if err == nil {
					err = f.db.Exec(`UPDATE infra.outbox SET payload=jsonb_set(payload,'{data,actor_id}',to_jsonb(?::text)) WHERE id=?`, candidate.ID.String(), command.EventID).Error
				}
			case "another_request":
				request := uuid.New()
				err = f.db.Exec(`UPDATE mediatool.depth_command SET request_id=? WHERE actor_id=? AND request_id=?`, request, admin.ID, command.RequestID).Error
				if err == nil {
					err = f.db.Exec(`UPDATE infra.outbox SET payload=jsonb_set(payload,'{data,request_id}',to_jsonb(?::text)) WHERE id=?`, request.String(), command.EventID).Error
				}
			}
			if err != nil {
				t.Fatal("isolated authority fault setup", err)
			}
			_, err = toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
			if err == nil {
				t.Fatal("invalid or revoked permanent control admitted cleanup")
			}
			var state struct {
				Status, ProcessState string
				ActiveWorker         *uuid.UUID
			}
			wantStatus := "cancel_requested"
			if fault == "disabled_controller" {
				wantStatus = "failed"
			}
			if err := f.db.Raw(`SELECT status,process_state,active_worker FROM mediatool.depth_job WHERE id=?`, j.ID).Scan(&state).Error; err != nil || state.Status != wantStatus || state.ProcessState != "none" || state.ActiveWorker != nil {
				t.Fatal("rejected control changed ownership or invented stop", state, err)
			}
		})
	}
}

func TestDepthJobPGControlRevocationReleasesOnlyStoppedCoordination(t *testing.T) {
	for _, phase := range []string{"finish", "pending_reconcile", "forged_pending_reconcile"} {
		t.Run(phase, func(t *testing.T) {
			f := depthHTTP(t)
			j := f.create(t)
			admin, replacement := depthControlAdmin(t, f), depthControlAdmin(t, f)
			id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
			if _, err := f.store.Claim(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			// Durable protocol fixture only: the real native stop and private
			// objects are exercised by the two Actual tests in this file.
			if err := f.store.StartProcess(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			if err := f.store.EndProcess(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			if phase != "finish" {
				if _, err := f.store.Finish(t.Context(), id, true, true, "depth_cleanup_unknown"); err != nil {
					t.Fatal(err)
				}
			}
			f.actor = admin
			current, err := f.store.Get(t.Context(), admin, f.project, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			requested := depthControlThroughHTTP(t, f, current, "cancel")
			if phase != "finish" {
				requested = depthControlThroughHTTP(t, f, requested, "reconcile")
			}
			if phase == "forged_pending_reconcile" {
				if err := f.db.Exec(`UPDATE infra.outbox SET payload=jsonb_set(payload,'{data,actor_id}',to_jsonb(?::text)) WHERE id IN(SELECT event_id FROM mediatool.depth_command WHERE job_id=? AND event_action='reconcile')`, uuid.NewString(), j.ID).Error; err != nil {
					t.Fatal(err)
				}
				_, err := f.store.Claim(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true})
				if !errors.Is(err, toolapp.ErrInvalidDepthInput) {
					t.Fatal("forged pending controller accepted", err)
				}
				unchanged, err := f.store.Get(t.Context(), admin, f.project, j.ID)
				if err != nil || unchanged.Revision != requested.Revision || !unchanged.ReconciliationRequested || unchanged.ExecutionUnconfirmed {
					t.Fatal("forged intent changed durable pending command", unchanged, err)
				}
				return
			}
			if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, admin.ID).Error; err != nil {
				t.Fatal(err)
			}
			if phase == "finish" {
				_, err = f.store.Finish(t.Context(), id, true, false, "")
			} else {
				_, err = toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true})
			}
			if !errors.Is(err, identityapp.ErrForbidden) {
				t.Fatal("revoked controller authorized completion", err)
			}
			f.actor = replacement
			failed, err := f.store.Get(t.Context(), replacement, f.project, j.ID)
			if err != nil || failed.Status != tooldomain.DepthFailed || !failed.NeedsReconciliation || failed.ExecutionUnconfirmed || failed.ReconciliationRequested {
				t.Fatalf("revoked control stranded coordination or fabricated stop: %+v %v", failed, err)
			}
			var state struct {
				ProcessState string
				ActiveWorker *uuid.UUID
			}
			if err := f.db.Raw(`SELECT process_state,active_worker FROM mediatool.depth_job WHERE id=?`, j.ID).Scan(&state).Error; err != nil || state.ProcessState != "ended" || state.ActiveWorker != nil {
				t.Fatal("stopped consumer fence not safely released", state, err)
			}
			depthControlThroughHTTP(t, f, failed, "reconcile")
			stopped := depthControlWorkflow(t, f, toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, Reconcile: true})
			if stopped.Status != tooldomain.DepthCancelled || stopped.ExecutionUnconfirmed || stopped.NeedsReconciliation {
				t.Fatal("current replacement control cannot recover original job", stopped)
			}
		})
	}
}
