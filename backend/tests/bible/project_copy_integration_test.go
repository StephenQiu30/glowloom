package bible_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type bibleCopyAccess struct {
	owner *workspacepg.ProjectCopyAccessStore
}

func (a bibleCopyAccess) Authorize(ctx context.Context, actor identityapp.Principal, b app.ProjectCopyBinding, target bool) error {
	return a.owner.Authorize(ctx, actor, workspaceapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, target)
}
func bibleCopyStore(db *gorm.DB, authority workspaceapp.ProjectCopyAuthority) *pg.ProjectCopyStore {
	return pg.NewProjectCopyStore(db, func(tx *gorm.DB) app.ProjectCopyAccess {
		return bibleCopyAccess{owner: workspacepg.NewProjectCopyAccessStore(tx, authority)}
	}, nil, nil)
}

// This isolated owning fixture validates real workspace SQL authorization, not whole-copy admission.
func bibleCopyFixture(t *testing.T, owner *gorm.DB, actor identityapp.Principal, source uuid.UUID) (app.ProjectCopyBinding, workspaceapp.ProjectCopyAuthority) {
	t.Helper()
	b := app.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: source, TargetProjectID: uuid.New()}
	var revision int64
	if err := owner.Raw(`SELECT revision FROM workspace.project WHERE id=?`, source).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO workspace.project(id,org_id,name,description,aspect_ratio,style_type,resolution,status,revision,default_models) VALUES(?,?,'复制未发布','','16:9','realistic','1080p','copying',1,'{}')`, b.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	worker := uuid.New()
	manifest := workspacedomain.ProjectCopyManifest{WorkspaceSHA256: digest, CanvasSnapshotID: uuid.New(), CanvasSHA256: digest, MediaSnapshotID: uuid.New(), MediaSHA256: digest, Script: &workspacedomain.ProjectCopyScriptSnapshot{ID: uuid.New(), ManifestSHA256: digest, ContentSHA256: digest}}
	media := workspacedomain.ProjectCopyReceipt{ManifestSHA256: digest, ContentSHA256: digest}
	m, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	r, err := json.Marshal(media)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,request_id,idem_key,request_sha256,admission_response,source_project_id,source_revision,target_project_id,target_name,status,stage,revision,attempt,worker_id,started_at,manifest,workspace_snapshot,media_receipt) VALUES(?,?,?,?,?,?,'{}',?,?,?,'复制未发布','running','script',2,1,?,?,?::jsonb,'{}',?::jsonb)`, b.JobID, b.OrgID, actor.ID, uuid.New(), uuid.New(), digest, b.SourceProjectID, revision, b.TargetProjectID, worker, time.Now(), string(m), string(r)).Error; err != nil {
		t.Fatal(err)
	}
	return b, workspaceapp.ProjectCopyAuthority{Binding: workspaceapp.ProjectCopyBinding{JobID: b.JobID, OrgID: b.OrgID, SourceProjectID: b.SourceProjectID, TargetProjectID: b.TargetProjectID}, ActorID: actor.ID, SourceRevision: revision, WorkerID: worker, Phase: "transfer"}
}

func freezeBibleCopy(t *testing.T, db *gorm.DB, actor identityapp.Principal, b app.ProjectCopyBinding, authority workspaceapp.ProjectCopyAuthority) app.ProjectCopySnapshot {
	t.Helper()
	admission := authority
	admission.Phase = "freeze"
	admission.WorkerID = uuid.Nil
	var snapshot app.ProjectCopySnapshot
	if err := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = bibleCopyStore(tx, admission).Freeze(t.Context(), actor, b, nil, time.Now())
		return err
	}); err != nil {
		t.Fatal("own admission", err)
	}
	return snapshot
}

func TestBibleCopyPGCompleteHistoryIndependentRereadAndRuntimeACL(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, source := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(source))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Change(t.Context(), actor, app.Command{ProjectID: source, Kind: domain.KindCharacter, Action: "update", EntryID: first.EntryID, ExpectedRevision: first.Revision, Key: uuid.New(), RequestID: uuid.New(), Character: &app.CharacterInput{Name: "历史角色", Description: " 原文\n👩🏽‍🚀 "}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: source, Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: second.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []domain.Kind{domain.KindLocation, domain.KindProp} {
		c := app.Command{ProjectID: source, Kind: kind, Action: "create", Key: uuid.New(), RequestID: uuid.New()}
		if kind == domain.KindLocation {
			c.Location = &domain.LocationContent{Name: "街角", Prompt: "原始场景"}
		} else {
			c.Prop = &domain.PropContent{Name: "铜钥匙", Prompt: "原始物件"}
		}
		if _, err := service.Change(t.Context(), actor, c); err != nil {
			t.Fatal(err)
		}
	}
	b, authority := bibleCopyFixture(t, owner, actor, source)
	snapshot := freezeBibleCopy(t, db, actor, b, authority)
	if snapshot.Counts.Characters != 1 || snapshot.Counts.CharacterVersions != 2 || snapshot.Counts.Locations != 1 || snapshot.Counts.Props != 1 || snapshot.Counts.LookVersions != 2 || len(snapshot.Versions) != 4 {
		t.Fatal("partial history", snapshot.Counts)
	}
	if err := app.NewProjectCopy(bibleCopyStore(db, authority), nil).Transfer(t.Context(), actor, b, snapshot); err != nil {
		t.Fatal("empty-media proof", err)
	}
	register := authority
	register.Phase = "register"
	apply := func() error {
		return db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`SET LOCAL TIME ZONE 'Asia/Shanghai'`).Error; err != nil {
				return err
			}
			_, err := bibleCopyStore(tx, register).Register(t.Context(), actor, b, snapshot)
			return err
		})
	}
	if err := apply(); err != nil {
		t.Fatal("full registration", err)
	}
	if err := apply(); err != nil {
		t.Fatal("exact reread replay", err)
	}
	if _, err := service.Find(t.Context(), actor, b.TargetProjectID, domain.KindCharacter, snapshot.Identities[0].TargetID); err == nil {
		t.Fatal("unpublished content became ordinary readable")
	}
	if err := db.Exec(`UPDATE bible.copy_snapshot SET content_sha256=? WHERE id=?`, strings.Repeat("b", 64), snapshot.ID).Error; err == nil {
		t.Fatal("runtime can rewrite frozen plan")
	}
	if err := db.Exec(`DELETE FROM bible.copy_receipt WHERE snapshot_id=?`, snapshot.ID).Error; err == nil {
		t.Fatal("runtime can remove receipt")
	}
	if err := owner.Exec(`UPDATE bible.character_version SET content_sha256=? WHERE project_id=? AND version_no=1`, strings.Repeat("b", 64), b.TargetProjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := apply(); !errors.Is(err, domain.ErrCorruptHistory) {
		t.Fatal("changed target hash escaped final reread", err)
	}
}

func TestBibleCopyPGSourceCorruptionWorkerFenceAndZeroRowRollback(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, source := bibleActorProject(t, owner)
	first, err := app.NewService(bibleStore(db), time.Now).Change(t.Context(), actor, createCharacter(source))
	if err != nil {
		t.Fatal(err)
	}
	b, authority := bibleCopyFixture(t, owner, actor, source)
	snapshot := freezeBibleCopy(t, db, actor, b, authority)
	stale := authority
	stale.WorkerID = uuid.New()
	if err := app.NewProjectCopy(bibleCopyStore(db, stale), nil).Transfer(t.Context(), actor, b, snapshot); !errors.Is(err, workspacedomain.ErrProjectCopyWorkerConflict) {
		t.Fatal("stale worker accepted", err)
	}
	if err := app.NewProjectCopy(bibleCopyStore(db, authority), nil).Transfer(t.Context(), actor, b, snapshot); err != nil {
		t.Fatal(err)
	}
	register := authority
	register.Phase = "register"
	function := "bible_copy_zero_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := owner.Exec(`CREATE FUNCTION bible.` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + function + ` BEFORE INSERT ON bible.copy_receipt FOR EACH ROW EXECUTE FUNCTION bible.` + function + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER IF EXISTS ` + function + ` ON bible.copy_receipt`).Error; err != nil {
			t.Error(err)
		}
		if err := owner.Exec(`DROP FUNCTION IF EXISTS bible.` + function + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	err = db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		_, err := bibleCopyStore(tx, register).Register(t.Context(), actor, b, snapshot)
		return err
	})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatal("zero row receipt published", err)
	}
	var n int64
	if err := owner.Table("bible.character").Where("project_id=?", b.TargetProjectID).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("target rows survived rollback", n, err)
	}
	if err := owner.Exec(`UPDATE bible.character_version SET content_sha256=? WHERE id=?`, strings.Repeat("c", 64), first.VersionID).Error; err != nil {
		t.Fatal(err)
	}
	other, otherAuthority := bibleCopyFixture(t, owner, actor, source)
	admission := otherAuthority
	admission.Phase = "freeze"
	admission.WorkerID = uuid.Nil
	err = db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		_, err := bibleCopyStore(tx, admission).Freeze(t.Context(), actor, other, nil, time.Now())
		return err
	})
	if !errors.Is(err, domain.ErrCorruptHistory) {
		t.Fatal("corrupt source rehashed", err)
	}
	if err := owner.Table("bible.copy_snapshot").Where("job_id=?", other.JobID).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("corrupt source created intent", n, err)
	}
}
