package workspace_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// This exact field order was captured by running the fixed 29ce5664 Go implementation.
type legacyCopyWorkspace struct {
	SourceProjectID uuid.UUID
	SourceRevision  int64
	Target          domain.Project
	DefaultModels   map[string]string
	AIGCMarkStyle   json.RawMessage
	Presets         []domain.StylePreset
}

func legacyWorkspaceBytes(t *testing.T, body []byte) ([]byte, string) {
	t.Helper()
	var old legacyCopyWorkspace
	if err := json.Unmarshal(body, &old); err != nil {
		t.Fatal(err)
	}
	var mark any
	if err := json.Unmarshal(old.AIGCMarkStyle, &mark); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(mark)
	if err != nil {
		t.Fatal(err)
	}
	old.AIGCMarkStyle = canonical
	out, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(out)
	return out, hex.EncodeToString(digest[:])
}

func TestProjectCopyPlacementLegacyWorkspaceGolden(t *testing.T) {
	body, err := os.ReadFile("testdata/project_copy_legacy_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if json.Unmarshal(body, &golden) != nil {
		t.Fatal("invalid fixed old Go golden")
	}
	actual, digest := legacyWorkspaceBytes(t, []byte(golden["workspace_bytes"]))
	if string(actual) != golden["workspace_bytes"] || digest != golden["workspace_sha256"] {
		t.Fatal("legacy workspace fixture changed pinned old serialization")
	}
}

func TestProjectCopyPlacementPGLegacyManifestPublicationAndReceipt(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	store := projectCopyStore(db)
	service := workspaceapp.NewProjectCopyService(store, time.Now)
	in := copyInput(source)
	job, err := service.Create(ctx, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	inputBytes, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	inputDigest := sha256.Sum256(append([]byte("project-copy/v1\x00"), inputBytes...))
	var row struct {
		RequestSHA256     string
		WorkspaceSnapshot []byte
	}
	if err = db.Raw(`SELECT request_sha256,workspace_snapshot FROM workspace.project_copy_job WHERE id=?`, job.ID).Scan(&row).Error; err != nil || row.RequestSHA256 != hex.EncodeToString(inputDigest[:]) {
		t.Fatalf("persisted legacy fingerprint %s %v", row.RequestSHA256, err)
	}
	legacyBody, legacyDigest := legacyWorkspaceBytes(t, row.WorkspaceSnapshot)
	job.Manifest.WorkspaceSHA256 = legacyDigest
	manifest, err := json.Marshal(job.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	// Owner-only fixture reconstructs a historical job before folders existed.
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project_copy_job SET workspace_snapshot=?::jsonb,manifest=?::jsonb,admission_response=?::jsonb WHERE id=?`, string(legacyBody), string(manifest), string(admission), job.ID)
	lifecycleFixtureSQL(t, owner, `DELETE FROM workspace.project_folder_placement WHERE actor_id=? AND project_id=?`, actor.ID, job.TargetProjectID)
	folders := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, folders, actor, "历史受理后的分类")
	moveTestProject(ctx, t, folders, actor, source, f, 0)
	replay, err := service.Create(ctx, actor, in)
	if err != nil || !reflect.DeepEqual(replay, job) {
		t.Fatalf("historical receipt recomputed current classification %+v %v", replay, err)
	}
	worker := uuid.New()
	if _, err = store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteMedia(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteCanvases(ctx, actor, job.ID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Publish(ctx, actor, job.ID, worker); err != nil {
		t.Fatalf("historical nil-placement digest rejected publication %v", err)
	}
	root := uuid.Nil
	page, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{FolderID: &root, Limit: 50})
	if err != nil || len(page.Projects) != 1 || page.Projects[0].ID != job.TargetProjectID || page.Projects[0].PlacementRevision != 0 {
		t.Fatalf("historical target acquired current source folder %+v %v", page, err)
	}
}
