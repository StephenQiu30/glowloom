package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	workspaceevent "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/event"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	domain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func secondaryWorkers(tx *gorm.DB) workspace.WorkerScopeResolver {
	return workspace.NewScopeAccess(pgworkspace.NewScopeStore(tx, func(tx *gorm.DB) workspace.ActorAccess { return pgidentity.NewStore(tx) }))
}

type secondaryWork struct{}

func (secondaryWork) HasInflightWork(context.Context, identity.Principal, uuid.UUID) (bool, error) {
	return false, nil
}

func TestSecondaryFoldersBindSelectedScope(t *testing.T) {
	verifySecondaryRollback(t)
	tx, actor, team, scope, project, requests := projectConsumerFixture(t)
	installSecondaryScope(t, tx)
	personal := actor.ScopeID
	actor.ScopeID = scope
	store := pgworkspace.NewScopedFolderStore(tx, requests, func(*gorm.DB) workspace.ProjectWorkGuard { return secondaryWork{} }, nil)
	name := "team classification"
	in := workspace.FolderChangeInput{Action: "create", Name: &name, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	saved, err := store.ChangeFolder(t.Context(), actor, in, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	other := actor
	other.ScopeID = personal
	if page, err := store.ListFolders(t.Context(), other, workspace.FolderListInput{Limit: 20}); err != nil || len(page.Items) != 0 {
		t.Fatalf("personal listed team folders: %+v %v", page, err)
	}
	if _, err := store.ChangeFolder(t.Context(), other, in, time.Now()); !errors.Is(err, workspace.ErrIdempotencyConflict) {
		t.Fatalf("cross-scope receipt: %v", err)
	}
	if _, err := pgworkspace.NewScopedStore(tx, requests, nil, nil).ListProjectsForActor(t.Context(), other, workspace.ListProjectsInput{Limit: 20, FolderID: &saved.Folder.ID}); !errors.Is(err, domain.ErrProjectFolderNotFound) {
		t.Fatalf("foreign folder filter: %v", err)
	}
	move := workspace.FolderChangeInput{Action: "move", FolderID: saved.Folder.ID, ExpectedRevision: 1, ProjectID: project, ExpectedProjectRevision: 1, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	if _, err := store.ChangeFolder(t.Context(), actor, move, time.Now()); err != nil {
		t.Fatalf("non-author team placement: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='archived' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListFolders(t.Context(), actor, workspace.FolderListInput{Limit: 20}); err != nil {
		t.Fatal(err)
	}
	in.IdempotencyKey = uuid.New()
	if _, err := store.ChangeFolder(t.Context(), actor, in, time.Now()); err == nil {
		t.Fatal("archived folder write")
	}
	if err := tx.Exec(`UPDATE workspace.team SET status='active' WHERE id=?`, team).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListFolders(t.Context(), actor, workspace.FolderListInput{Limit: 20}); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked folder session: %v", err)
	}
}

func secondaryCopyFixture(t *testing.T) (*gorm.DB, identity.Principal, uuid.UUID, uuid.UUID, domain.ProjectCopyJob, *pgworkspace.ProjectCopyStore, pgworkspace.RequestScopeFactory) {
	t.Helper()
	verifySecondaryRollback(t)
	tx, actor, team, scope, source, requests := projectConsumerFixture(t)
	installSecondaryScope(t, tx)
	actor.ScopeID = scope
	target := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,'Copy target','16:9','realistic','copying')`, target, actor.OrgID, actor.ID, scope).Error; err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	job := domain.ProjectCopyJob{ID: uuid.New(), OrgID: actor.OrgID, ActorID: actor.ID, SourceProjectID: source, TargetProjectID: target, SourceRevision: 1, Revision: 1, TargetName: "Copy target", Status: "queued", Stage: "media", RequestID: uuid.NewString(), Manifest: domain.ProjectCopyManifest{WorkspaceSHA256: digest, MediaSnapshotID: uuid.New(), MediaSHA256: digest, CanvasSnapshotID: uuid.New(), CanvasSHA256: digest}}
	manifest, _ := json.Marshal(job.Manifest)
	response, _ := json.Marshal(job)
	if err := tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,owner_scope_id,actor_id,request_id,idem_key,request_sha256,admission_response,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot) VALUES(?,?,?,?,?,?,?,?::jsonb,?,1,?,?,'queued','media',?::jsonb,'{}')`, job.ID, actor.OrgID, scope, actor.ID, job.RequestID, uuid.New(), digest, string(response), source, target, job.TargetName, string(manifest)).Error; err != nil {
		t.Fatal(err)
	}
	store := pgworkspace.NewScopedProjectCopyStoreWithBible(tx, requests, secondaryWorkers, nil, func(*gorm.DB) workspace.ProjectCopyOwners { return workspace.ProjectCopyOwners{} }, nil, nil)
	return tx, actor, team, scope, job, store, requests
}
func TestSecondaryCopySessionScopeAndWorkerFence(t *testing.T) {
	tx, actor, team, scope, job, store, requests := secondaryCopyFixture(t)
	if _, err := store.Find(t.Context(), actor, job.ID); err != nil {
		t.Fatal(err)
	}
	other := actor
	var personal struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM workspace.creation_scope WHERE owner_user_id=?`, actor.ID).Scan(&personal).Error; err != nil {
		t.Fatal(err)
	}
	other.ScopeID = personal.ID
	if _, err := requests(tx).Resolve(t.Context(), other, other.ScopeID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find(t.Context(), other, job.ID); !errors.Is(err, workspace.ErrProjectNotFound) {
		t.Fatalf("different valid scope copy read: %v", err)
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Find(t.Context(), actor, job.ID); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("revoked copy session: %v", err)
	}
	worker, err := store.WorkerActor(t.Context(), actor.OrgID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if worker.ScopeID != scope || worker.SessionID != uuid.Nil {
		t.Fatal("worker lost durable scope")
	}
	nonce := uuid.New()
	claimed, err := store.Claim(t.Context(), worker, job.ID, nonce, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UseAttempt(t.Context(), worker, job.ID, uuid.New(), "media", func(workspace.ProjectCopyOwners, domain.ProjectCopyJob) error { return nil }); !errors.Is(err, domain.ErrProjectCopyWorkerConflict) {
		t.Fatalf("stale worker fence: %v", err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	called := false
	if err := store.UseAttempt(t.Context(), worker, job.ID, nonce, "media", func(workspace.ProjectCopyOwners, domain.ProjectCopyJob) error { called = true; return nil }); !errors.Is(err, workspace.ErrScopeNotFound) || called {
		t.Fatalf("revoked callback: %v %v", err, called)
	}
	if _, err := store.Fail(t.Context(), worker, job.ID, nonce, "denied", false, false); !errors.Is(err, workspace.ErrScopeNotFound) {
		t.Fatalf("revoked writeback: %v", err)
	}
	stopped, err := store.StopAttempt(t.Context(), workspace.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: job.ID, WorkerID: nonce}, "revoked", false, false)
	if err != nil || stopped.Revision != claimed.Revision+1 {
		t.Fatalf("exact-worker cessation: %v", err)
	}
}
func TestSecondaryCopyDispatchRechecksMembership(t *testing.T) {
	tx, actor, team, _, job, store, _ := secondaryCopyFixture(t)
	d := workspace.ProjectCopyDelivery{EventID: uuid.New(), OrgID: actor.OrgID, JobID: job.ID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID, ActorID: actor.ID, Revision: 1, Action: "requested", Status: "queued", Stage: "media", OccurredAt: time.Now().UTC().Truncate(time.Microsecond)}
	body, _ := json.Marshal(map[string]any{"event_id": d.EventID, "event_type": workspaceevent.ProjectCopyTopic, "project_id": job.SourceProjectID, "aggregate": map[string]any{"type": "project_copy", "id": job.ID, "revision": 1}, "org_id": d.OrgID, "occurred_at": d.OccurredAt.Format(time.RFC3339Nano), "actor": map[string]any{"id": actor.ID, "kind": "user"}, "data": map[string]any{"copy_job_id": job.ID, "source_project_id": job.SourceProjectID, "target_project_id": job.TargetProjectID, "revision": 1, "action": "requested", "status": "queued", "stage": "media"}})
	if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload)VALUES(?,'glowloom.workspace.project_copy_requested.v1',?,?::jsonb)`, d.EventID, job.ID.String(), string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := store.VerifyDelivery(t.Context(), d); err != nil || !ok {
		t.Fatalf("valid delivery: %v %v", ok, err)
	}
	if err := tx.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := store.VerifyDelivery(t.Context(), d); err != nil || ok {
		t.Fatalf("revoked event must be acknowledged without dispatch: %v %v", ok, err)
	}

	started := &secondaryCopyStarter{}
	handler := workspaceevent.NewProjectCopyHandler(secondaryInbox{}, store, started)
	if err := handler.Handle(t.Context(), inbox.Record{Topic: workspaceevent.ProjectCopyTopic, Key: []byte(job.ID.String()), Value: body}); err != nil || started.calls != 0 {
		t.Fatalf("obsolete copy handler stopped consumer or dispatched: %v %d", err, started.calls)
	}
	forged := d
	forged.EventID = uuid.New()
	if _, err := store.VerifyDelivery(t.Context(), forged); !errors.Is(err, domain.ErrInvalidProjectCopy) {
		t.Fatalf("forged copy proof treated as obsolete: %v", err)
	}
	unavailable := errors.New("secondary authority unavailable")
	broken := pgworkspace.NewScopedProjectCopyStore(tx, nil, func(*gorm.DB) workspace.WorkerScopeResolver { return secondaryUnavailable{err: unavailable} }, nil, nil)
	if _, err := broken.VerifyDelivery(t.Context(), d); !errors.Is(err, unavailable) {
		t.Fatalf("authority outage silently acknowledged: %v", err)
	}
}

func verifySecondaryRollback(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("LV_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open secondary rollback verifier")
	}
	t.Cleanup(func() { _ = conn.Close() })
	read := func() [3]int64 {
		var v [3]int64
		queries := []string{`SELECT count(*) FROM information_schema.columns WHERE table_schema='workspace' AND table_name IN ('project_folder','project_folder_command','project_folder_placement','project_copy_job') AND column_name='owner_scope_id'`, `SELECT count(*) FROM workspace.project_folder WHERE name='team classification'`, `SELECT count(*) FROM workspace.project WHERE name='Copy target'`}
		for i, q := range queries {
			if err := conn.DB.Raw(q).Scan(&v[i]).Error; err != nil {
				t.Error("secondary rollback evidence unavailable")
			}
		}
		return v
	}
	before := read()
	t.Cleanup(func() {
		after := read()
		if before != after {
			t.Errorf("secondary rollback changed %v -> %v", before, after)
		} else {
			t.Logf("verified secondary rollback: columns/folders/targets %v unchanged", after)
		}
	})
}

type secondaryInbox struct{}

func (secondaryInbox) ProcessExternalOnce(ctx context.Context, _, _ string, call func(context.Context) error) (bool, error) {
	return true, call(ctx)
}

type secondaryCopyStarter struct{ calls int }

func (s *secondaryCopyStarter) Deliver(context.Context, workspace.ProjectCopyDelivery) error {
	s.calls++
	return nil
}

type secondaryUnavailable struct{ err error }

func (s secondaryUnavailable) Resolve(context.Context, uuid.UUID, uuid.UUID, bool) (domain.Scope, error) {
	return domain.Scope{}, s.err
}
