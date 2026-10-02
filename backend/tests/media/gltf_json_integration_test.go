package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestGLTFJSONActualHTTPPrivateOriginalAndIndependentProjectCopy(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	_, foreignProject := mediaStoreProject(t, db)
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	store := pgmedia.NewStore(db)
	service := mediaapp.NewScopedUploadService(store, store, mediaflow.NewUploadProber(validator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	query := mediaapp.NewAssetQuery(store, objects)
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewUploadHandler(service).Register(group)
	mediahttp.NewHandler(query).Register(group)
	body := gltfJSONBytes(t, gltfJSONDocument(t))
	key := uuid.New()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, documentUploadRequest(t, project, key, "实际JSON场景.gltf", body, true))
	var result mediaapp.UploadResult
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Asset.Kind != "model" || result.Asset.MIMEType != "model/gltf+json" {
		t.Fatal("formal JSON model upload failed", response.Code, response.Body.String())
	}
	original, err := store.FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil || original.Codec == nil || *original.Codec != "gltf2" || original.SHA256 == nil || original.ByteSize != int64(len(body)) || original.Width != nil || original.Height != nil {
		t.Fatal("JSON model acquired fake binary or image facts", original, err)
	}
	keys := []string{original.ObjectKey}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = objects.Remove(context.Background(), key)
		}
	})
	var review mediaapp.LocalUploadReview
	if json.Unmarshal(original.ModerationDetail, &review) != nil || review.PrincipalID != actor.ID || review.SHA256 != *original.SHA256 || !review.RightsConfirmed || review.Normalization != nil {
		t.Fatal("JSON original review did not bind its exact uploaded bytes", review)
	}
	preview, err := query.Preview(t.Context(), actor, project, original.ID)
	if err != nil || preview.Asset.MIMEType != "model/gltf+json" {
		t.Fatal("private model preview missing", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), "GET", preview.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	served, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := io.ReadAll(io.LimitReader(served.Body, int64(len(body))+1))
	closeErr := served.Body.Close()
	if served.StatusCode != 200 || readErr != nil || closeErr != nil || !bytes.Equal(actual, body) {
		t.Fatal("private JSON original changed format or bytes", served.StatusCode, readErr, closeErr)
	}
	if _, err := query.Reference(t.Context(), actor, foreignProject, original.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("foreign project obtained model reference", err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, project, original.ID)
	if err != nil || len(rends) != 0 {
		t.Fatal("JSON model acquired fabricated thumbnail", err)
	}
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, documentUploadRequest(t, project, key, "实际JSON场景.gltf", body, true))
	if replay.Code != 201 || !bytes.Equal(replay.Body.Bytes(), response.Body.Bytes()) {
		t.Fatal("permanent JSON model upload changed on replay", replay.Code)
	}
	external := gltfJSONDocument(t)
	external["buffers"].([]any)[0].(map[string]any)["uri"] = "geometry.bin"
	for _, tc := range []struct {
		body   []byte
		key    uuid.UUID
		review bool
		status int
	}{{body, uuid.New(), false, 422}, {gltfJSONBytes(t, external), uuid.New(), true, 415}, {gltfJSONBytes(t, external), key, true, 409}} {
		failed := httptest.NewRecorder()
		router.ServeHTTP(failed, documentUploadRequest(t, project, tc.key, "实际JSON场景.gltf", tc.body, tc.review))
		if failed.Code != tc.status {
			t.Fatal("bad JSON model entered formal upload", failed.Code, tc.status)
		}
	}
	var counts struct{ Assets, Receipts int }
	if err := db.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?) AS assets,(SELECT count(*) FROM media.upload_request WHERE project_id=?) AS receipts`, project, project).Scan(&counts).Error; err != nil || counts.Assets != 1 || counts.Receipts != 1 {
		t.Fatal("bad JSON model created durable partial output", counts, err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'JSON模型独立副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot mediaapp.ProjectCopySnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'JSON模型独立副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil || snapshot.Assets != 1 || snapshot.Renditions != 0 {
		t.Fatal("complete JSON model Copy snapshot failed", snapshot, err)
	}
	repo := pgmedia.NewProjectCopyStore(db)
	if err := mediaapp.NewProjectCopyTransfer(repo, mediaobjects.NewProjectCopyObjects(objects), t.TempDir()).Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
	if err != nil || len(intents) != 1 {
		t.Fatal("independent JSON model original incomplete", intents, err)
	}
	keys = append(keys, intents[0].TargetObjectKey)
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := objects.Remove(t.Context(), original.ObjectKey); err != nil {
		t.Fatal(err)
	}
	copied, err := objects.Get(t.Context(), intents[0].TargetObjectKey)
	if err != nil {
		t.Fatal("independent model depended on removed fixture source", err)
	}
	actual, readErr = io.ReadAll(copied)
	closeErr = copied.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(actual, body) {
		t.Fatal("copied JSON model bytes changed", readErr, closeErr)
	}
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(body), "个人JSON场景.gltf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	personal, err := service.UploadPersonal(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{Key: uuid.New(), RequestID: uuid.New(), FileName: "个人JSON场景.gltf"}, File: file, LocalReviewConfirmed: true})
	if err != nil || personal.Asset.MIMEType != "model/gltf+json" || personal.Asset.Kind != "model" {
		t.Fatal("same personal pipeline lost JSON model facts", personal, err)
	}
	p, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).FindLibraryMedia(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, personal.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys = append(keys, p.Asset.ObjectKey)
	if _, err := query.Reference(t.Context(), actor, project, personal.Asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("personal JSON original bypassed project Transfer", err)
	}
}

func TestGLTFJSONSchemaFactsAndRetainedOriginalContract(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(gltfJSONBytes(t, gltfJSONDocument(t))), "约束模型.gltf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	service := mediaapp.NewUploadService(pgmedia.NewStore(db), mediaflow.NewUploadProber(validator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	result, err := service.Upload(t.Context(), mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: project, Key: uuid.New(), RequestID: uuid.New(), FileName: "约束模型.gltf"}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := pgmedia.NewStore(db).FindAsset(t.Context(), actor, project, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = objects.Remove(context.Background(), asset.ObjectKey) })
	for _, sql := range []string{`UPDATE media.media_asset SET codec='glb2' WHERE id=?`, `UPDATE media.media_asset SET width=1 WHERE id=?`, `UPDATE media.media_asset SET byte_size=67108865 WHERE id=?`} {
		if err := owner.Exec(sql, asset.ID).Error; err == nil {
			t.Fatal("owner constraint accepted fake JSON model facts", sql)
		}
	}
	var definition string
	if err := db.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='media.media_asset'::regclass AND conname='media_asset_model_facts_check'`).Scan(&definition).Error; err != nil || !bytes.Contains([]byte(definition), []byte("model/gltf+json")) {
		t.Fatal("schema constraint lost formal JSON model facts", err)
	}
}
