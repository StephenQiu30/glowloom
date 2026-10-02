package workspace_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func coverTestDB(t *testing.T) (context.Context, *gorm.DB, *gorm.DB) {
	t.Helper()
	runtime, owner := os.Getenv("LV_TEST_COVER_DB_DSN"), os.Getenv("LV_TEST_COVER_OWNER_DSN")
	if runtime == "" || owner == "" {
		t.Skip("set isolated LV_TEST_COVER_DB_DSN and LV_TEST_COVER_OWNER_DSN")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	t.Cleanup(cancel)
	open := func(dsn string) *gorm.DB {
		c, err := platformdb.Open(ctx, dsn, noop.NewTracerProvider())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c.DB.WithContext(ctx)
	}
	r, o := open(runtime), open(owner)
	var row struct{ Actor, Database string }
	if err := r.Raw(`SELECT current_user AS actor,current_database() AS database`).Scan(&row).Error; err != nil || row.Actor != "lanverse_app" || row.Database != "lanverse_cover" {
		t.Fatal("test must use isolated cover database/runtime role", row, err)
	}
	return ctx, r, o
}

func coverStore(db *gorm.DB) *workspacepg.Store {
	return workspacepg.NewStoreWithProjectCover(db, folderWork, func(tx *gorm.DB) workspaceapp.ProjectCoverReference {
		return mediaapp.NewAssetQuery(mediapg.NewStore(tx), nil)
	})
}
func coverRouter(db *gorm.DB, actor identityapp.Principal) *gin.Engine {
	r := gin.New()
	r.Use(httpapi.Middleware(workspaceTestOrigin))
	api := r.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	s := coverStore(db)
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(s), nil, nil).Register(api)
	workspacehttp.NewProjectLifecycleHandler(workspaceapp.NewProjectLifecycle(s, time.Now)).Register(api)
	return r
}
func seedCoverAsset(t *testing.T, owner *gorm.DB, project uuid.UUID, kind string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if kind == "image" {
		lifecycleFixtureSQL(t, owner, `INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,codec,moderation_status)VALUES(?,?,'image','upload','ready',?,'synthetic.png','image/png',12,repeat('a',64),2,2,'png','passed')`, id, project, "projects/"+project.String()+"/"+kind+"/2026/10/"+id.String()+map[string]string{"image": ".png", "audio": ".wav"}[kind])
	} else {
		lifecycleFixtureSQL(t, owner, `INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,duration_ms,codec,moderation_status)VALUES(?,?,'audio','upload','ready',?,'synthetic.wav','audio/wav',12,repeat('a',64),1000,'pcm_s16le','passed')`, id, project, "projects/"+project.String()+"/"+kind+"/2026/10/"+id.String()+map[string]string{"image": ".png", "audio": ".wav"}[kind])
	}
	return id
}
func coverChange(project uuid.UUID, revision int64, asset *uuid.UUID) workspaceapp.ProjectChangeInput {
	i := projectChange(project, revision, "patch")
	i.Patch.SetCover, i.Patch.CoverAssetID = true, asset
	return i
}

func TestProjectCoverPGCurrentOwnershipReplayAndUnavailable(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	asset := seedCoverAsset(t, owner, project, "image")
	s := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	set := coverChange(project, 1, &asset)
	first, err := s.Change(ctx, actor, set)
	if err != nil || first.Project.Revision != 2 || first.Project.CoverAssetID == nil || *first.Project.CoverAssetID != asset || first.CoverUnavailable {
		t.Fatal("cover set", first, err)
	}
	nop, err := s.Change(ctx, actor, coverChange(project, 2, &asset))
	if err != nil || nop.Project.Revision != 2 {
		t.Fatal("valid cover no-op", err)
	}
	cleared, err := s.Change(ctx, actor, coverChange(project, 2, nil))
	if err != nil || cleared.Project.Revision != 3 || cleared.Project.CoverAssetID != nil {
		t.Fatal("cover clear", err)
	}
	replay, err := s.Change(ctx, actor, set)
	if err != nil || replay.Project.Revision != 2 || replay.Project.CoverAssetID == nil || *replay.Project.CoverAssetID != asset {
		t.Fatal("old cover permanent request reassigned current selection", err)
	}
	if _, err := s.Change(ctx, actor, coverChange(project, 2, nil)); !errors.Is(err, domain.ErrProjectRevisionConflict) {
		t.Fatal("stale cover", err)
	}
	if _, err := s.Change(ctx, actor, coverChange(project, 3, &asset)); err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE media.media_asset SET status='rejected',moderation_status='rejected',revision=revision+1 WHERE id=?`, asset)
	read, err := s.Get(ctx, actor, project)
	if err != nil || !read.CoverUnavailable || read.Project.CoverAssetID == nil || *read.Project.CoverAssetID != asset {
		t.Fatal("revoked cover binding vanished", err)
	}
	page, err := workspaceapp.NewListProjectsQuery(coverStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{Limit: 50})
	if err != nil || len(page.Projects) != 1 || !page.Projects[0].CoverUnavailable || page.Projects[0].CoverAssetID == nil {
		t.Fatal("list unavailable cover", page, err)
	}
	if _, err := s.Change(ctx, actor, coverChange(project, 4, &asset)); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
		t.Fatal("invalid cover no-op accepted", err)
	}
	if _, err := workspaceapp.NewProjectLifecycle(workspacepg.NewStore(db), time.Now).Get(ctx, actor, project); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("missing cover owner presented empty", err)
	}
	if _, err := workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)).Execute(ctx, actor, workspaceapp.ListProjectsInput{Limit: 50}); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatal("list missing owner", err)
	}
	if _, err := s.Change(ctx, actor, coverChange(project, 4, nil)); err != nil {
		t.Fatal("cannot explicitly clear unusable binding", err)
	}
}

func TestProjectCoverPGClosedMediaScopesCurrentActorAndArchived(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	_, foreign := lifecycleActorProject(ctx, t, owner)
	otherProject := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	for _, id := range []uuid.UUID{uuid.New(), seedCoverAsset(t, owner, foreign, "image"), seedCoverAsset(t, owner, otherProject, "image"), seedCoverAsset(t, owner, project, "audio")} {
		if _, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Change(ctx, actor, coverChange(project, 1, &id)); !errors.Is(err, workspaceapp.ErrProjectCoverUnavailable) {
			t.Fatal("invalid cover accepted", err)
		}
	}
	asset := seedCoverAsset(t, owner, project, "image")
	if _, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Change(ctx, actor, coverChange(project, 1, &asset)); err != nil {
		t.Fatal(err)
	}
	s := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	if _, err := s.Change(ctx, actor, projectChange(project, 2, "archive")); err != nil {
		t.Fatal(err)
	}
	read, err := s.Get(ctx, actor, project)
	if err != nil || read.CoverUnavailable || read.Project.CoverAssetID == nil {
		t.Fatal("archived cover lost read", err)
	}
	if _, err := s.Change(ctx, actor, coverChange(project, 3, nil)); !errors.Is(err, domain.ErrProjectStateConflict) {
		t.Fatal("archived cover write", err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID)
	if _, err := s.Get(ctx, actor, project); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked actor cover exposed", err)
	}
}

func TestProjectCoverHTTPActualReviewedUploadNullableAndRefresh(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	objects := projectCopyObjects(t)
	data := copyPNG(t)
	file, err := mediaapp.ReadUpload(ctx, bytes.NewReader(data), "正式主图.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	mediaStore := mediapg.NewStore(db)
	upload := mediaapp.NewUploadService(mediaStore, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	result, err := upload.Upload(ctx, mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "正式主图.png"}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("actual reviewed source upload", err)
	}
	asset, err := mediaStore.FindAsset(ctx, actor, project, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = objects.Remove(cleanup, asset.ObjectKey)
	})
	path := "/api/projects/" + project.String()
	key := uuid.NewString()
	body := []byte(`{"expected_revision":1,"cover_asset_id":"` + asset.ID.String() + `"}`)
	r := coverRouter(db, actor)
	first := projectHTTPRequest(ctx, r, "PATCH", path, key, body)
	if first.Code != 200 || !bytes.Contains(first.Body.Bytes(), []byte(`"cover_asset_id":"`+asset.ID.String()+`"`)) {
		t.Fatal("HTTP set cover", first.Code, first.Body.String())
	}
	for _, route := range []string{path, "/api/projects?limit=1"} {
		read := projectHTTPRequest(ctx, coverRouter(db, actor), "GET", route, "", nil)
		if read.Code != 200 || !bytes.Contains(read.Body.Bytes(), []byte(asset.ID.String())) || bytes.Contains(read.Body.Bytes(), []byte(asset.ObjectKey)) || bytes.Contains(read.Body.Bytes(), []byte("url")) {
			t.Fatal("HTTP safe refresh", read.Code, read.Body.String())
		}
	}
	replay := projectHTTPRequest(ctx, coverRouter(db, actor), "PATCH", path, key, body)
	if replay.Code != 200 || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("HTTP repeated key changed result", replay.Code, replay.Body.String())
	}
	for _, raw := range []string{`{"expected_revision":2,"cover_asset_id":"00000000-0000-0000-0000-000000000000"}`, `{"expected_revision":2,"cover_asset_id":{}}`, `{"expected_revision":2,"cover_asset_id":null,"url":"private"}`} {
		bad := projectHTTPRequest(ctx, r, "PATCH", path, uuid.NewString(), []byte(raw))
		if bad.Code != 422 {
			t.Fatal("HTTP unclosed cover", bad.Code, bad.Body.String())
		}
	}
	cleared := projectHTTPRequest(ctx, r, "PATCH", path, uuid.NewString(), []byte(`{"expected_revision":2,"cover_asset_id":null}`))
	if cleared.Code != 200 || !bytes.Contains(cleared.Body.Bytes(), []byte(`"cover_asset_id":null`)) {
		t.Fatal("explicit null clear", cleared.Code, cleared.Body.String())
	}
}

func TestProjectCoverPGLegacyFingerprintAndReceiptExactCompatibility(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	read, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Get(ctx, actor, project)
	if err != nil {
		t.Fatal(err)
	}
	name := read.Project.Name
	in := projectChange(project, 1, "patch")
	in.Patch.Name = &name
	legacy, err := json.Marshal(struct {
		Contract          string
		Org, Project      uuid.UUID
		Action            string
		Revision          int64
		Name, Description *string
		Preset            *uuid.UUID
		Overseas          *bool
	}{"project.lifecycle.v1", actor.OrgID, project, "patch", 1, &name, nil, nil, nil})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(legacy)
	result, _ := json.Marshal(read)
	lifecycleFixtureSQL(t, owner, `INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at)VALUES(?,?,?,?,200,?::jsonb,clock_timestamp()+interval '24 hours')`, uuid.New(), actor.ID, in.IdempotencyKey.String(), hex.EncodeToString(hash[:]), string(result))
	replayed, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Change(ctx, actor, in)
	encoded, _ := json.Marshal(replayed)
	if err != nil || !bytes.Equal(result, encoded) {
		t.Fatal("legacy fingerprint/receipt changed", err, string(encoded))
	}
	if bytes.Contains(encoded, []byte("CoverAssetID")) || bytes.Contains(encoded, []byte("CoverUnavailable")) {
		t.Fatal("nil cover changed historical JSON")
	}
}
