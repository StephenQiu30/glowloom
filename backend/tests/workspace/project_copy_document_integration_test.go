package workspace_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func copyDocumentDOCX(t *testing.T) []byte {
	t.Helper()
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>保持原始剧情</w:t></w:r></w:p></w:body></w:document>`,
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for name, content := range parts {
		part, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestProjectCopyDocumentRealNonownerCompleteOriginalPublicationAndIndependentRead(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	objects := projectCopyObjects(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	store := mediapg.NewStore(db)
	upload := mediaapp.NewUploadService(store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	samples := []struct {
		name string
		data []byte
	}{{"UTF16原件.txt", []byte{0xff, 0xfe, 0x2c, 0x7b, 0x00, 0x0a}}, {"原文.docx", copyDocumentDOCX(t)}}
	sourceAssets := make(map[uuid.UUID][]byte)
	var keys []string
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			if err := objects.Remove(cleanup, key); err != nil {
				t.Error("remove exact private document copy fixture")
			}
		}
	})
	for _, sample := range samples {
		file, err := mediaapp.ReadUpload(ctx, bytes.NewReader(sample.data), sample.name)
		if err != nil {
			t.Fatal(err)
		}
		result, err := upload.Upload(ctx, mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: source, Key: uuid.New(), RequestID: uuid.New(), FileName: sample.name}, File: file, LocalReviewConfirmed: true})
		_ = file.Close()
		if err != nil {
			t.Fatal("real source document upload", err)
		}
		asset, err := store.FindAsset(ctx, actor, source, result.Asset.ID)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, asset.ObjectKey)
		sourceAssets[asset.ID] = sample.data
	}
	copyStore := projectCopyStore(db)
	job, err := copyStore.Create(ctx, actor, copyInput(source), time.Now())
	if err != nil || job.Manifest.Assets != 2 || job.Manifest.Renditions != 0 {
		t.Fatal("whole project document admission", job.Manifest, err)
	}
	finished, err := actualCopyWorker(t, db, mediaobjects.NewProjectCopyObjects(objects)).Execute(ctx, workspaceapp.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: job.ID, WorkerID: uuid.New()})
	if err != nil || finished.Status != "succeeded" {
		t.Fatal("actual whole project copy publication", finished.Status, err)
	}
	reader := mediaapp.NewDocumentSources(mediapg.NewDocumentSourceStore(db), objects)
	query := mediaapp.NewAssetQuery(store, objects)
	page, err := query.List(ctx, actor, finished.TargetProjectID, "document", "", 100)
	if err != nil || len(page.Items) != 2 {
		t.Fatal("fresh published document listing", err)
	}
	defaultPage, err := query.List(ctx, actor, finished.TargetProjectID, "", "", 100)
	if err != nil || len(defaultPage.Items) != 0 {
		t.Fatal("copied documents entered canvas list", err)
	}
	for _, item := range page.Items {
		if _, shared := sourceAssets[item.ID]; shared {
			t.Fatal("copied document shared source ID")
		}
		asset, err := store.FindAsset(ctx, actor, finished.TargetProjectID, item.ID)
		if err != nil || asset.UploadID != nil || asset.SourceOperationID != nil {
			t.Fatal("published independent document facts", err)
		}
		keys = append(keys, asset.ObjectKey)
		var sourceID uuid.UUID
		for id, data := range sourceAssets {
			sha := sha256.Sum256(data)
			if asset.SHA256 != nil && *asset.SHA256 == hex.EncodeToString(sha[:]) {
				sourceID = id
				break
			}
		}
		if sourceID == uuid.Nil {
			t.Fatal("copied original SHA changed")
		}
		sourceAsset, err := store.FindAsset(ctx, actor, source, sourceID)
		if err != nil || sourceAsset.ObjectKey == asset.ObjectKey {
			t.Fatal("copied original key shared", err)
		}
		if err := objects.Remove(ctx, sourceAsset.ObjectKey); err != nil {
			t.Fatal("remove only synthetic source object", err)
		}
		frozen, err := reader.Freeze(ctx, actor, finished.TargetProjectID, []uuid.UUID{asset.ID})
		if err != nil {
			t.Fatal(err)
		}
		file, err := reader.Open(ctx, actor, finished.TargetProjectID, frozen[0])
		if err != nil {
			t.Fatal("independent copied original lost", err)
		}
		actual, err := io.ReadAll(file.File)
		_ = file.Close()
		if err != nil || !bytes.Equal(actual, sourceAssets[sourceID]) {
			t.Fatal("actual copied original bytes differ", err)
		}
		if _, err := query.Reference(ctx, actor, finished.TargetProjectID, asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
			t.Fatal("copied document got canvas reference", err)
		}
		rends, err := store.FindRenditions(ctx, actor, finished.TargetProjectID, asset.ID)
		if err != nil || len(rends) != 0 {
			t.Fatal("copy invented document renditions", err)
		}
	}
}
