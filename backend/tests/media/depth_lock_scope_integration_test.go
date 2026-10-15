package media_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/postgres"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// depthLockFixture uses only rolled-back metadata. It never creates source bytes,
// starts inference, or contacts object storage or Temporal.
func depthLockFixture(t *testing.T) (*depthHTTPFixture, tooldomain.DepthJob) {
	t.Helper()
	tx := mediaStoreDB(t).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	actor, project := mediaStoreProject(t, tx)
	a := imageAsset(project, uuid.New())
	a.Kind, a.MimeType, a.FileName = mediadomain.KindVideo, "video/mp4", "lock-fixture.mp4"
	a.ObjectKey = "projects/" + project.String() + "/video/2026/10/" + a.ID.String() + ".mp4"
	a.Status, a.ModerationStatus = mediadomain.StatusProcessing, mediadomain.ModerationPending
	width, height, duration := int32(320), int32(180), int32(1000)
	sha := strings.Repeat("a", 64)
	a.Width, a.Height, a.DurationMS, a.SHA256 = &width, &height, &duration, &sha
	if err := mediaTestStore(tx).CreateAsset(t.Context(), actor, a); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	node := transcriptionNode(t, tx, actor, project, a.ID, "video")
	f := &depthHTTPFixture{db: tx, actor: actor, project: project, source: a.ID, store: depthStore(tx, true), workerStore: depthWorkerStore(tx, true)}
	j, err := f.store.Create(t.Context(), actor, project, uuid.New(), toolapp.DepthCreateInput{CanvasID: node.CanvasID, NodeID: node.NodeID, Revision: node.Revision})
	if err != nil {
		t.Fatal(err)
	}
	return f, j
}

type depthLockMedia struct {
	toolapp.DepthDerivedMedia
	events *[]string
	after  func(*gorm.DB) error
	tx     *gorm.DB
}

func (m *depthLockMedia) AuthorizeProject(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) error {
	err := m.DepthDerivedMedia.AuthorizeProject(ctx, actor, project, write)
	*m.events = append(*m.events, "authority")
	if err == nil && m.after != nil {
		return m.after(m.tx)
	}
	return err
}

func depthObserveLocks(t *testing.T, f *depthHTTPFixture, events *[]string, after func(*gorm.DB) error) *toolpg.DepthStore {
	t.Helper()
	name := "test:depth-lock-" + uuid.NewString()
	if err := f.db.Callback().Row().Before("gorm:row").Register(name, func(tx *gorm.DB) {
		q := strings.ToLower(tx.Statement.SQL.String())
		if strings.Contains(q, "from mediatool.depth_job") && strings.Contains(q, "for update") {
			*events = append(*events, "job")
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Row().Remove(name) })
	return toolpg.NewDepthStore(f.db, nil, func(tx *gorm.DB) toolapp.DepthDerivedMedia {
		// Keep the actual app worker scope factory; the wrapper only observes it.
		return &depthLockMedia{DepthDerivedMedia: mediaapp.NewDerivedService(pgmedia.NewScopedStore(tx, app.NewMediaWorkerScopeAccess)), events: events, after: after, tx: tx}
	})
}

func depthAssertAuthorityFirst(t *testing.T, events []string) {
	t.Helper()
	if len(events) < 2 || events[0] != "authority" || events[1] != "job" {
		t.Fatalf("worker must authorize before locking the job: %v", events)
	}
}

func TestDepthJobPGLocksAuthorityBeforeJob(t *testing.T) {
	for _, step := range []string{"claim", "phase", "cleanup", "reconcile", "finish"} {
		t.Run(step, func(t *testing.T) {
			f, j := depthLockFixture(t)
			id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
			if step != "claim" {
				if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			if step == "cleanup" || step == "finish" {
				current, err := f.store.Get(t.Context(), f.actor, f.project, j.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), current.Revision, "cancel"); err != nil {
					t.Fatal(err)
				}
			}
			if step == "reconcile" {
				if err := f.workerStore.EndProcess(t.Context(), id); err != nil {
					t.Fatal(err)
				}
				failed, err := f.workerStore.Finish(t.Context(), id, true, true, "depth_object_unknown")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, uuid.New(), failed.Revision, "reconcile"); err != nil {
					t.Fatal(err)
				}
				id.ExecutionID, id.Reconcile = uuid.New(), true
				if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			var events []string
			store := depthObserveLocks(t, f, &events, nil)
			var err error
			switch step {
			case "claim":
				_, err = store.Claim(t.Context(), id)
			case "phase":
				err = store.Phase(t.Context(), id, "checking")
			case "cleanup":
				err = store.BeginRemove(t.Context(), id, "missing-fixture-object")
				if !errors.Is(err, toolapp.ErrConflict) {
					t.Fatal("missing cleanup object admitted", err)
				}
				err = nil
			case "reconcile":
				_, err = store.Objects(t.Context(), id)
			case "finish":
				_, err = store.Finish(t.Context(), id, true, false, "")
			}
			if err != nil {
				t.Fatal(err)
			}
			depthAssertAuthorityFirst(t, events)
		})
	}
}

func TestDepthJobPGLockRevalidatesCandidateScope(t *testing.T) {
	for _, field := range []string{"actor_id", "org_id", "project_id", "revision"} {
		t.Run(field, func(t *testing.T) {
			f, j := depthLockFixture(t)
			other, project := mediaStoreProject(t, f.db)
			var events []string
			done := false
			store := depthObserveLocks(t, f, &events, func(tx *gorm.DB) error {
				if done {
					return nil
				}
				done = true
				values := map[string]any{"actor_id": other.ID, "org_id": other.OrgID, "project_id": project}
				if field == "revision" {
					return tx.Exec(`UPDATE mediatool.depth_job SET revision=revision+1 WHERE id=?`, j.ID).Error
				}
				return tx.Exec("UPDATE mediatool.depth_job SET "+field+"=? WHERE id=?", values[field], j.ID).Error
			})
			_, err := store.Claim(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
			if !errors.Is(err, toolapp.ErrConflict) {
				t.Fatal("candidate identity changed after authorization", err)
			}
			depthAssertAuthorityFirst(t, events)
		})
	}
}

func TestDepthJobPGLockRevalidatesCurrentControl(t *testing.T) {
	for _, step := range []string{"cleanup_claim", "cleanup_begin_remove", "reconcile_claim", "reconcile_objects"} {
		t.Run(step, func(t *testing.T) {
			f, j := depthLockFixture(t)
			controller := depthControlAdmin(t, f)
			replacement := depthControlAdmin(t, f)
			id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
			if step != "cleanup_claim" {
				if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(step, "reconcile") {
				if err := f.workerStore.EndProcess(t.Context(), id); err != nil {
					t.Fatal(err)
				}
				if _, err := f.workerStore.Finish(t.Context(), id, true, true, "depth_object_unknown"); err != nil {
					t.Fatal(err)
				}
			}
			current, err := f.store.Get(t.Context(), controller, f.project, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			action := "cancel"
			if strings.HasPrefix(step, "reconcile") {
				action = "reconcile"
			}
			if _, err := f.store.Control(t.Context(), controller, f.project, j.ID, uuid.New(), current.Revision, action); err != nil {
				t.Fatal(err)
			}
			if action == "reconcile" {
				id.ExecutionID, id.Reconcile = uuid.New(), true
				if step == "reconcile_objects" {
					if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
						t.Fatal(err)
					}
				}
			}
			var events []string
			done := false
			store := depthObserveLocks(t, f, &events, func(tx *gorm.DB) error {
				if done {
					return nil
				}
				done = true
				// Replace the complete valid permanent command proof without
				// changing the job revision. Rechecking only the job is insufficient.
				request := uuid.New()
				event := uuid.NewSHA1(request, []byte("media-depth/"+replacement.ID.String()+"/"+action))
				if err := tx.Exec(`UPDATE infra.outbox SET id=?,payload=jsonb_set(jsonb_set(jsonb_set(jsonb_set(jsonb_set(payload,'{event_id}',to_jsonb(?::text)),'{data,event_id}',to_jsonb(?::text)),'{data,request_id}',to_jsonb(?::text)),'{data,actor_id}',to_jsonb(?::text)),'{org_id}',to_jsonb(?::text)) WHERE id IN(SELECT event_id FROM mediatool.depth_command WHERE job_id=? AND event_action=?)`, event, event.String(), event.String(), request.String(), replacement.ID.String(), replacement.OrgID.String(), j.ID, action).Error; err != nil {
					return err
				}
				return tx.Exec(`UPDATE mediatool.depth_command SET actor_id=?,request_id=?,event_id=? WHERE job_id=? AND event_action=?`, replacement.ID, request, event, j.ID, action).Error
			})
			switch step {
			case "cleanup_claim", "reconcile_claim":
				_, err = store.Claim(t.Context(), id)
			case "cleanup_begin_remove":
				err = store.BeginRemove(t.Context(), id, "missing-fixture-object")
			case "reconcile_objects":
				_, err = store.Objects(t.Context(), id)
			}
			if !errors.Is(err, toolapp.ErrConflict) {
				t.Fatal("current control changed after authorization", err)
			}
			depthAssertAuthorityFirst(t, events)
		})
	}
}

func TestDepthJobPGLockFinishConvergesConcurrentCancel(t *testing.T) {
	for _, phase := range []string{"undispatched", "started", "ended", "cleanup_replaced", "revoked_replaced", "proof_read"} {
		t.Run(phase, func(t *testing.T) {
			f, j := depthLockFixture(t)
			controller := depthControlAdmin(t, f)
			replacement := depthControlAdmin(t, f)
			id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
			if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			if phase == "started" || phase == "ended" {
				if err := f.workerStore.StartProcess(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "ended" {
				if err := f.workerStore.EndProcess(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			current, err := f.store.Get(t.Context(), controller, f.project, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "cleanup_replaced" || phase == "revoked_replaced" || phase == "proof_read" {
				current, err = f.store.Control(t.Context(), controller, f.project, j.ID, uuid.New(), current.Revision, "cancel")
				if err != nil {
					t.Fatal(err)
				}
			}
			if phase == "revoked_replaced" {
				if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, controller.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			key := uuid.New()
			cancel := func() error {
				_, err := f.store.Control(t.Context(), replacement, f.project, j.ID, key, current.Revision, "cancel")
				return err
			}
			injected, restored := false, false
			name := "test:depth-finish-race-" + uuid.NewString()
			if err := f.db.Callback().Row().Before("gorm:row").Register(name, func(tx *gorm.DB) {
				q := strings.ToLower(tx.Statement.SQL.String())
				lockWindow := strings.Contains(q, "from mediatool.depth_job") && strings.Contains(q, "for update")
				proofWindow := phase == "proof_read" && strings.Contains(q, "from mediatool.depth_command")
				if injected || (!lockWindow && !proofWindow) || (phase == "proof_read" && !proofWindow) {
					return
				}
				injected = true
				if err := cancel(); err != nil {
					t.Fatal("inject accepted concurrent cancellation", err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			// All rows belong to the outer rollback fixture. Reapply the
			// accepted command after rolling back Finish's nested savepoint
			// to model a competing commit surviving its transaction rollback.
			if err := f.db.Callback().Raw().After("gorm:raw").Register(name, func(tx *gorm.DB) {
				if injected && !restored && strings.HasPrefix(strings.ToUpper(tx.Statement.SQL.String()), "ROLLBACK TO SAVEPOINT") {
					restored = true
					if err := cancel(); err != nil {
						t.Fatal("retain competing accepted cancellation", err)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = f.db.Callback().Row().Remove(name)
				_ = f.db.Callback().Raw().Remove(name)
			})
			var events []string
			store := depthObserveLocks(t, f, &events, nil)
			finished, err := store.Finish(t.Context(), id, true, false, "")
			if err != nil || !injected || !restored || finished.Status != tooldomain.DepthCancelled || finished.ExecutionUnconfirmed || finished.NeedsReconciliation {
				t.Fatal("accepted cancellation stranded a stopped owner", finished, injected, restored, err)
			}
			var state struct {
				ActiveWorker *uuid.UUID
				ProcessState string
			}
			if err := f.db.Raw(`SELECT active_worker,process_state FROM mediatool.depth_job WHERE id=?`, j.ID).Scan(&state).Error; err != nil || state.ActiveWorker != nil || (state.ProcessState != "none" && state.ProcessState != "ended") {
				t.Fatal("stopped execution retained an owner or lost cessation", state, err)
			}
			var actor string
			if err := f.db.Raw(`SELECT payload->'actor'->>'id' FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->'data'->>'action'='media.depth_cancelled' AND payload->'data'->'object'->>'id'=?`, j.ID.String()).Scan(&actor).Error; err != nil || actor != replacement.ID.String() {
				t.Fatal("completion borrowed the previous control actor", actor, err)
			}
		})
	}
}

func TestDepthJobPGLockPhysicalTruthSurvivesRevocation(t *testing.T) {
	f, j := depthLockFixture(t)
	id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
	if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := f.workerStore.StartProcess(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.workerStore.Claim(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New(), Reconcile: true}); !errors.Is(err, toolapp.ErrWorkerBusy) {
		t.Fatal("active owner lost precedence over missing reconcile intent", err)
	}
	if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	var events []string
	store := depthObserveLocks(t, f, &events, nil)
	if err := store.EndProcess(t.Context(), id); err != nil {
		t.Fatal("revocation blocked cessation fact", err)
	}
	if err := store.Interrupt(t.Context(), id); err != nil {
		t.Fatal("revocation blocked interrupted ownership fact", err)
	}
	failed, err := store.Finish(t.Context(), id, true, true, "depth_object_unknown")
	if err != nil || failed.ExecutionUnconfirmed || !failed.NeedsReconciliation || failed.Status != tooldomain.DepthFailed {
		t.Fatal("revocation erased physical stop evidence", failed, err)
	}
	for _, event := range events {
		if event == "authority" {
			t.Fatal("physical facts acquired a new permission gate", events)
		}
	}
}

func TestDepthJobPGLockRevokedControlRetainsRecovery(t *testing.T) {
	for _, step := range []string{"claim", "finish"} {
		t.Run(step, func(t *testing.T) {
			f, j := depthLockFixture(t)
			controller := depthControlAdmin(t, f)
			id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
			if step == "finish" {
				if _, err := f.workerStore.Claim(t.Context(), id); err != nil {
					t.Fatal(err)
				}
			}
			current, err := f.store.Get(t.Context(), controller, f.project, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.Control(t.Context(), controller, f.project, j.ID, uuid.New(), current.Revision, "cancel"); err != nil {
				t.Fatal(err)
			}
			if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, controller.ID).Error; err != nil {
				t.Fatal(err)
			}
			var events []string
			store := depthObserveLocks(t, f, &events, nil)
			if step == "claim" {
				_, err = store.Claim(t.Context(), id)
			} else {
				_, err = store.Finish(t.Context(), id, true, false, "")
			}
			if !errors.Is(err, identityapp.ErrForbidden) {
				t.Fatal("revoked control accepted", err)
			}
			depthAssertAuthorityFirst(t, events)
			failed, err := f.store.Get(t.Context(), f.actor, f.project, j.ID)
			if err != nil || failed.Status != tooldomain.DepthFailed || failed.CancellationRequested || failed.Retryable || failed.NeedsReconciliation || failed.ExecutionUnconfirmed || failed.FailureCode == nil || *failed.FailureCode != "depth_control_forbidden" {
				t.Fatal("undispatched permission rejection stranded recovery", failed, err)
			}
			var auditActor string
			if err := f.db.Raw(`SELECT payload->'actor'->>'id' FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->'data'->>'action'='media.depth_failed' AND payload->'data'->'object'->>'id'=?`, j.ID.String()).Scan(&auditActor).Error; err != nil || auditActor != controller.ID.String() {
				t.Fatal("rejected intent attributed to another actor", auditActor, err)
			}
		})
	}
}
