package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	billingpg "github.com/StephenQiu30/lanverse/backend/internal/billing/adapter/postgres"
	canvaspg "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// This contract fixture tests actual coordinator SQL/fences, not script content ownership.
type scriptCopyContractOwner struct {
	tx             *gorm.DB
	authority      workspaceapp.ProjectCopyAuthority
	corrupt        *bool
	cleanupBlocked *bool
}

func (o scriptCopyContractOwner) ReferencedMedia(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding) ([]uuid.UUID, error) {
	if err := workspacepg.NewProjectCopyAccessStore(o.tx, o.authority).Authorize(ctx, actor, b, false); err != nil {
		return nil, err
	}
	return []uuid.UUID{}, nil
}

func TestProjectCopyScriptHTTPReportsExactSafeHistoricalProgress(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	store := scriptCopyContractStore(db, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
		return scriptCopyContractOwner{tx: tx, authority: a}
	})
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	workspacehttp.NewProjectCopyHandler(workspaceapp.NewProjectCopyService(store, time.Now)).Register(g)
	path := "/api/project-copies/" + job.ID.String()
	read := func(completed bool) {
		response := copyHTTPRequest(t, router, "GET", path, "", uuid.New())
		decodeCopyHTTP(t, response, 200)
		var payload struct {
			Script *struct {
				Counts          map[string]int  `json:"counts"`
				CompletedCounts *map[string]int `json:"completed_counts"`
			} `json:"script"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Script == nil || len(payload.Script.Counts) != 13 || payload.Script.Counts["version_sources"] != 3 || payload.Script.Counts["sources"] != 2 || payload.Script.Counts["objects"] != 8 {
			t.Fatal("incomplete safe historical progress", response.Body.String(), err)
		}
		if (payload.Script.CompletedCounts != nil) != completed {
			t.Fatal("unregistered history claimed completed", response.Body.String())
		}
		if completed && (*payload.Script.CompletedCounts)["objects"] != 8 {
			t.Fatal("completed original-object count lost")
		}
	}
	read(false)
	worker := uuid.New()
	if _, err := store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteMedia(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	read(false)
	if _, err := store.CompleteScript(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	read(true)
}

func (o scriptCopyContractOwner) Freeze(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, _ map[uuid.UUID]uuid.UUID, _ time.Time) (domain.ProjectCopyScriptSnapshot, error) {
	if err := workspacepg.NewProjectCopyAccessStore(o.tx, o.authority).Authorize(ctx, actor, b, false); err != nil {
		return domain.ProjectCopyScriptSnapshot{}, err
	}
	return domain.ProjectCopyScriptSnapshot{ID: uuid.NewSHA1(b.JobID, []byte("script-contract")), ManifestSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ContentSHA256: "fedcba98fedcba98fedcba98fedcba98fedcba98fedcba98fedcba98fedcba98", Counts: domain.ProjectCopyScriptCounts{Sources: 2, Versions: 2, VersionHeads: 2, VersionSources: 3, Objects: 8}}, nil
}
func (o scriptCopyContractOwner) Register(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, s domain.ProjectCopyScriptSnapshot) (domain.ProjectCopyScriptReceipt, error) {
	if err := workspacepg.NewProjectCopyAccessStore(o.tx, o.authority).Authorize(ctx, actor, b, true); err != nil {
		return domain.ProjectCopyScriptReceipt{}, err
	}
	r := domain.ProjectCopyScriptReceipt{ManifestSHA256: s.ManifestSHA256, ContentSHA256: s.ContentSHA256, Counts: s.Counts}
	if o.corrupt != nil && *o.corrupt {
		r.Counts.Objects--
	}
	return r, nil
}
func (o scriptCopyContractOwner) FinishCleanup(ctx context.Context, actor identityapp.Principal, b workspaceapp.ProjectCopyBinding, _ domain.ProjectCopyScriptSnapshot) error {
	if err := workspacepg.NewProjectCopyAccessStore(o.tx, o.authority).Authorize(ctx, actor, b, true); err != nil {
		return err
	}
	if o.cleanupBlocked != nil && *o.cleanupBlocked {
		return workspaceapp.ErrProjectDependencyUnavailable
	}
	return nil
}
func scriptCopyContractStore(db *gorm.DB, factory workspacepg.ProjectCopyScriptOwnerFactory) *workspacepg.ProjectCopyStore {
	return workspacepg.NewProjectCopyStoreWithScript(db, folderWork, func(tx *gorm.DB) workspaceapp.ProjectCopyOwners {
		return workspaceapp.ProjectCopyOwners{Media: mediapg.NewProjectCopyStore(tx), Budget: billingpg.NewStore(tx), Cover: actualProjectCopyCoverOwner{tx: tx}, Canvas: canvaspg.NewProjectCopyStore(tx, func(tx *gorm.DB) canvasapp.MediaReader { return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil) }, func(tx *gorm.DB, b canvasapp.ProjectCopyBinding) canvasapp.MediaReader {
			return privateCopyReference{owner: mediapg.NewProjectCopyStore(tx), binding: mediaapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}}
		})}
	}, factory)
}
func TestProjectCopyScriptPGOwnStageReceiptAndPublicationFence(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	corrupt, cleanupBlocked := false, false
	store := scriptCopyContractStore(db, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
		return scriptCopyContractOwner{tx: tx, authority: a, corrupt: &corrupt, cleanupBlocked: &cleanupBlocked}
	})
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil || job.Manifest.Script == nil {
		t.Fatal("frozen owning history", job, err)
	}
	worker := uuid.New()
	if _, err := store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	job, err = store.CompleteMedia(ctx, actor, job.ID, worker)
	if err != nil || job.Stage != "script" {
		t.Fatal("own script stage", job.Stage, err)
	}
	authority := workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}, ActorID: actor.ID, WorkerID: worker, SourceRevision: 1, Phase: "transfer"}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := workspacepg.NewProjectCopyAccessStore(tx, authority).Authorize(ctx, actor, authority.Binding, true); err != nil {
			t.Fatal("current transfer scope", err)
		}
		stale := authority
		stale.WorkerID = uuid.New()
		if err := workspacepg.NewProjectCopyAccessStore(tx, stale).Authorize(ctx, actor, authority.Binding, true); !errors.Is(err, domain.ErrProjectCopyWorkerConflict) {
			t.Fatal("stale private transfer", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectCopyStore(db).CompleteScript(ctx, actor, job.ID, worker); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("missing owning module bypassed", err)
	}
	corrupt = true
	if _, err := store.CompleteScript(ctx, actor, job.ID, worker); !errors.Is(err, domain.ErrInvalidProjectCopy) {
		t.Fatal("partial historical receipt advanced", err)
	}
	corrupt = false
	if _, err := store.CompleteScript(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	job, err = store.Find(ctx, actor, job.ID)
	if err != nil || job.Stage != "canvases" || job.ScriptReceipt == nil {
		t.Fatal("durable historical checkpoint", job, err)
	}
	if _, err := store.CompleteCanvases(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	corrupt = true
	if _, err := store.Publish(ctx, actor, job.ID, worker); err == nil {
		t.Fatal("changed actual own history published")
	}
	if job, err = store.Find(ctx, actor, job.ID); err != nil || job.Stage != "finalizing" || job.Status != "running" {
		t.Fatal("failed publication changed job", job, err)
	}
	corrupt = false
	if job, err = store.Publish(ctx, actor, job.ID, worker); err != nil || job.Status != "succeeded" {
		t.Fatal("complete historical publication", job, err)
	}
}

func TestProjectCopyScriptPGMissingOwnerRollsBackAdmissionAndCleanupWaits(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	if _, err := scriptCopyContractStore(db, nil).Create(ctx, actor, copyInput(source), time.Now()); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("nil configured script owner accepted", err)
	}
	var counts struct{ Targets, Jobs int64 }
	if err := owner.Raw(`SELECT(SELECT count(*) FROM workspace.project WHERE org_id=? AND status='copying')AS targets,(SELECT count(*) FROM workspace.project_copy_job WHERE org_id=?)AS jobs`, actor.OrgID, actor.OrgID).Scan(&counts).Error; err != nil || counts.Targets != 0 || counts.Jobs != 0 {
		t.Fatal("owner failure leaked target/job", counts, err)
	}
	blocked := true
	store := scriptCopyContractStore(db, func(tx *gorm.DB, a workspaceapp.ProjectCopyAuthority) workspaceapp.ProjectCopyScriptOwner {
		return scriptCopyContractOwner{tx: tx, authority: a, cleanupBlocked: &blocked}
	})
	job, err := store.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	job, err = store.Claim(ctx, actor, job.ID, worker, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	job, err = store.Change(ctx, actor, job.ID, "cancel", job.Revision, uuid.New(), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishCancelled(ctx, actor, job.ID, worker); err == nil {
		t.Fatal("script cleanup incomplete but cancelled")
	}
	if job, err = store.Find(ctx, actor, job.ID); err != nil || job.Status != "cancel_requested" || job.WorkerID != worker {
		t.Fatal("missing cleanup discarded fence", job, err)
	}
	blocked = false
	if job, err = store.FinishCancelled(ctx, actor, job.ID, worker); err != nil || job.Status != "cancelled" {
		t.Fatal("all owner cleanup rejected", job, err)
	}
}
