package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	toolflow "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func TestDepthJobPGRejectedUndispatchedCancelHasPublicRecovery(t *testing.T) {
	for _, phase := range []string{"before_claim", "before_finish", "consumer_cleanup"} {
		t.Run(phase, func(t *testing.T) {
			f := depthHTTP(t)
			j := f.create(t)
			controller, replacement := depthControlAdmin(t, f), depthControlAdmin(t, f)
			id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
			if phase == "before_finish" {
				if _, err := f.store.Claim(t.Context(), id); err != nil {
					t.Fatal(err)
				}
				// Only the persisted undispatched owner is exercised here. No
				// native process, result intent or object Put has occurred.
				j = depthPublicGet(t, f, j.ID)
			}
			f.actor = controller
			requested := depthControlThroughHTTP(t, f, j, "cancel")
			var receipt []byte
			if err := f.db.Raw(`SELECT response FROM mediatool.depth_command WHERE actor_id=? AND job_id=? AND event_action='cancel'`, controller.ID, j.ID).Row().Scan(&receipt); err != nil || len(receipt) == 0 {
				t.Fatal("permanent cancel receipt missing", err)
			}
			if phase != "consumer_cleanup" {
				if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, controller.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			var err error
			switch phase {
			case "consumer_cleanup":
				store := &depthCleanupRevocation{DepthWorkerStore: f.store, fixture: f, controller: controller.ID}
				_, err = toolapp.NewDepthWorker(store, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), id)
				if !store.revoked {
					t.Fatal("consumer did not enter its actual cleanup path")
				}
			case "before_claim":
				_, err = toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil).Execute(t.Context(), id)
			case "before_finish":
				_, err = f.store.Finish(t.Context(), id, true, false, "")
			}
			if !errors.Is(err, identityapp.ErrForbidden) {
				t.Fatal("revoked controller was not rejected", err)
			}
			f.actor = replacement
			failed := depthPublicGet(t, f, j.ID)
			wantRevision := requested.Revision + 1
			if phase == "consumer_cleanup" {
				wantRevision++ // Actual Claim and rejected Finish each advance once.
			}
			if failed.Status != tooldomain.DepthFailed || failed.Revision != wantRevision || failed.CancellationRequested || failed.Retryable || failed.NeedsReconciliation || failed.ReconciliationRequested || failed.ExecutionUnconfirmed {
				t.Fatalf("GET leaves no safe public cancellation recovery: %+v", failed)
			}
			if failed.FailureCode == nil || *failed.FailureCode != "depth_control_forbidden" {
				t.Fatal("permission rejection was mislabeled as native or object failure", failed.FailureCode)
			}
			var rejectedActor string
			if err := f.db.Raw(`SELECT payload->'actor'->>'id' FROM infra.outbox WHERE topic='lanverse.audit.recorded.v1' AND payload->'data'->>'action'='media.depth_failed' AND payload->'data'->'after'->>'failure_code'='depth_control_forbidden' AND payload->'data'->'object'->>'id'=?`, j.ID.String()).Scan(&rejectedActor).Error; err != nil || rejectedActor != controller.ID.String() {
				t.Fatal("rejected control intent attributed to a different principal", rejectedActor, err)
			}
			var preserved []byte
			if err := f.db.Raw(`SELECT response FROM mediatool.depth_command WHERE actor_id=? AND job_id=? AND event_action='cancel'`, controller.ID, j.ID).Row().Scan(&preserved); err != nil || !bytes.Equal(receipt, preserved) {
				t.Fatal("rejecting current intent rewrote its original accepted receipt", err)
			}
			if r := f.request(t, "POST", "/api/media-depths/"+j.ID.String()+"/retry", uuid.New(), toolapp.ControlInput{ProjectID: f.project, Revision: failed.Revision}); r.Code != 409 {
				t.Fatal("rejected cancel unexpectedly permits fresh inference", r.Code, r.Body.String())
			}
			depthControlThroughHTTP(t, f, failed, "cancel")
			stopped := depthControlWorkflow(t, f, toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1})
			if stopped.Status != tooldomain.DepthCancelled || stopped.NeedsReconciliation || stopped.ExecutionUnconfirmed {
				t.Fatalf("replacement current controller cannot complete existing public cancel: %+v", stopped)
			}
			var state struct {
				ProcessState string
				ActiveWorker *uuid.UUID
				Objects      int
			}
			if err := f.db.Raw(`SELECT process_state,active_worker,(SELECT count(*) FROM mediatool.depth_object WHERE job_id=j.id) AS objects FROM mediatool.depth_job j WHERE id=?`, j.ID).Scan(&state).Error; err != nil || state.ProcessState != "none" || state.ActiveWorker != nil || state.Objects != 0 {
				t.Fatal("public recovery started a process or produced objects", state, err)
			}
		})
	}
}

func TestDepthJobPGUndispatchedRejectionRetainsUnknownEvidence(t *testing.T) {
	for _, phase := range []string{"before_claim", "before_finish"} {
		for _, evidence := range []string{"result_intent", "object", "unreadable_proof"} {
			t.Run(phase+"/"+evidence, func(t *testing.T) {
				f := depthHTTP(t)
				j := f.create(t)
				id := toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()}
				if phase == "before_finish" {
					if _, err := f.store.Claim(t.Context(), id); err != nil {
						t.Fatal(err)
					}
					j = depthPublicGet(t, f, j.ID)
				}
				controller := depthControlAdmin(t, f)
				f.actor = controller
				depthControlThroughHTTP(t, f, j, "cancel")
				if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, controller.ID).Error; err != nil {
					t.Fatal(err)
				}
				if evidence != "unreadable_proof" {
					// Deliberately inject inconsistent private facts into this
					// isolated task database. They must never grant fence release.
					if err := f.db.Exec(`INSERT INTO mediatool.depth_result_intent(job_id,attempt,asset_id,output_sha256,artifact_sha256,artifact,created_at) VALUES(?,1,?,?,?,'{}',statement_timestamp())`, j.ID, uuid.New(), strings.Repeat("1", 64), strings.Repeat("2", 64)).Error; err != nil {
						t.Fatal(err)
					}
					if evidence == "object" {
						if err := f.db.Exec(`INSERT INTO mediatool.depth_object(job_id,attempt,kind,object_key,sha256,byte_size,mime_type,updated_at) VALUES(?,1,'original',?,?,1,'video/mp4',statement_timestamp())`, j.ID, "depth-proof/"+uuid.NewString(), strings.Repeat("1", 64)).Error; err != nil {
							t.Fatal(err)
						}
					}
				}
				var before, after []byte
				if err := f.db.Raw(`SELECT row_to_json(j) FROM mediatool.depth_job j WHERE id=?`, j.ID).Row().Scan(&before); err != nil {
					t.Fatal(err)
				}
				invoke := func(store toolapp.DepthWorkerStore) error {
					if phase == "before_claim" {
						_, err := store.Claim(t.Context(), id)
						return err
					}
					_, err := store.Finish(t.Context(), id, true, true, "depth_cleanup_unknown")
					return err
				}
				var err error
				if evidence == "unreadable_proof" {
					if err := f.db.Exec(`REVOKE SELECT ON mediatool.depth_result_intent FROM lanverse_app`).Error; err != nil {
						t.Fatal(err)
					}
					// Permission changes are restricted to the isolated test
					// database and restored before inspecting the result.
					err = f.db.Transaction(func(tx *gorm.DB) error {
						if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
							return err
						}
						return invoke(depthStore(tx, true))
					})
					if restoreErr := f.db.Exec(`GRANT SELECT ON mediatool.depth_result_intent TO lanverse_app`).Error; restoreErr != nil {
						t.Fatal("restore isolated permission", restoreErr)
					}
					var pgErr *pgconn.PgError
					if !errors.As(err, &pgErr) || pgErr.Code != "42501" || pgErr.Message != "permission denied for table depth_result_intent" {
						t.Fatal("missing proof read did not fail closed", err)
					}
				} else {
					err = invoke(f.store)
					if !errors.Is(err, identityapp.ErrForbidden) {
						t.Fatal("existing private result obligations were discarded", err)
					}
				}
				if err := f.db.Raw(`SELECT row_to_json(j) FROM mediatool.depth_job j WHERE id=?`, j.ID).Row().Scan(&after); err != nil || !bytes.Equal(before, after) {
					t.Fatal("unproved absence changed job or released its owner", err)
				}
			})
		}
	}
}

func TestDepthJobPGRejectedRedeliveryPreservesCompletedCancel(t *testing.T) {
	f := depthHTTP(t)
	j := f.create(t)
	controller, reader := depthControlAdmin(t, f), depthControlAdmin(t, f)
	f.actor = controller
	depthControlThroughHTTP(t, f, j, "cancel")
	completed := depthControlWorkflow(t, f, toolapp.NewDepthWorker(f.store, toolflow.NewObjects(f.objects), nil, nil, nil, nil), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1})
	if completed.Status != tooldomain.DepthCancelled {
		t.Fatal("fixture did not complete actual cancellation", completed)
	}
	if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, controller.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, err := f.store.Claim(t.Context(), toolapp.DepthWorkID{JobID: j.ID, Attempt: 1, ExecutionID: uuid.New()})
	if !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("redelivery did not reject revoked current controller", err)
	}
	f.actor = reader
	current := depthPublicGet(t, f, j.ID)
	if current.Status != tooldomain.DepthCancelled || current.Revision != completed.Revision || current.CancellationRequested != completed.CancellationRequested || current.FailureCode != nil {
		t.Fatalf("rejected redelivery rewrote completed cancellation: %+v", current)
	}
}

type depthCleanupRevocation struct {
	toolapp.DepthWorkerStore
	fixture    *depthHTTPFixture
	controller uuid.UUID
	revoked    bool
}

func (s *depthCleanupRevocation) Objects(ctx context.Context, id toolapp.DepthWorkID) ([]toolapp.DepthObject, error) {
	if !s.revoked {
		if err := s.fixture.db.WithContext(ctx).Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, s.controller).Error; err != nil {
			return nil, err
		}
		s.revoked = true
	}
	return s.DepthWorkerStore.Objects(ctx, id)
}

func depthPublicGet(t *testing.T, f *depthHTTPFixture, id uuid.UUID) tooldomain.DepthJob {
	t.Helper()
	r := f.request(t, "GET", "/api/media-depths/"+id.String()+"?project_id="+f.project.String(), uuid.Nil, nil)
	var j tooldomain.DepthJob
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &j) != nil {
		t.Fatal("current authorized public GET", r.Code, r.Body.String())
	}
	return j
}
