package workspace_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"

	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediapg "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

type actualProjectCopyCoverOwner struct{ tx *gorm.DB }

func (o actualProjectCopyCoverOwner) Freeze(ctx context.Context, actor identityapp.Principal, project uuid.UUID, asset *uuid.UUID) error {
	return workspaceapp.FreezeProjectCover(ctx, mediaapp.NewAssetQuery(mediapg.NewStore(o.tx), nil), actor, project, asset)
}

func (o actualProjectCopyCoverOwner) Verify(ctx context.Context, actor identityapp.Principal, binding mediaapp.ProjectCopyBinding, expected, actual *uuid.UUID) error {
	return workspaceapp.VerifyProjectCover(ctx, privateCopyReference{owner: mediapg.NewProjectCopyStore(o.tx), binding: binding}, actor, binding.TargetProjectID, expected, actual)
}

func TestProjectCopyCoverRealPrivateImageMappedAndIndependent(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	objects := projectCopyObjects(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	data := copyPNG(t)
	file, err := mediaapp.ReadUpload(ctx, bytes.NewReader(data), "复制主图.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	media := mediapg.NewStore(db)
	upload := mediaapp.NewUploadService(media, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	result, err := upload.Upload(ctx, mediaapp.UploadInput{Actor: actor, Request: mediaapp.UploadRequest{ProjectID: source, Key: uuid.New(), RequestID: uuid.New(), FileName: "复制主图.png"}, File: file, LocalReviewConfirmed: true})
	if err != nil {
		t.Fatal("actual reviewed cover image", err)
	}
	asset, err := media.FindAsset(ctx, actor, source, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{asset.ObjectKey}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			if err := objects.Remove(cleanup, key); err != nil {
				t.Error("remove exact private cover fixture")
			}
		}
	})
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET cover_asset_id=?,revision=2 WHERE id=?`, asset.ID, source)
	input := copyInput(source)
	input.ExpectedRevision = 2
	job, err := projectCopyStore(db).Create(ctx, actor, input, time.Now())
	if err != nil || job.Manifest.Assets != 1 {
		t.Fatal("admit complete project with cover", err)
	}
	var frozen struct{ CoverAssetID uuid.UUID }
	if err := db.Raw(`SELECT (workspace_snapshot->'Target'->>'CoverAssetID')::uuid AS cover_asset_id FROM workspace.project_copy_job WHERE id=?`, job.ID).Scan(&frozen).Error; err != nil || frozen.CoverAssetID == uuid.Nil || frozen.CoverAssetID == asset.ID {
		t.Fatal("copy admission lost independent frozen cover binding", err)
	}
	worker := uuid.New()
	copyStore := projectCopyStore(db)
	if _, err := copyStore.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal("claim exact cover copy", err)
	}
	// A later explicit edit must not change the already frozen target binding.
	if _, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Change(ctx, actor, coverChange(source, 2, nil)); err != nil {
		t.Fatal("clear current source after copy freeze", err)
	}
	transfer := mediaapp.NewProjectCopyTransfer(workspacepg.NewProjectCopyMediaObjects(copyStore, job.ID, worker, false), mediaobjects.NewProjectCopyObjects(objects), t.TempDir())
	binding := mediaapp.ProjectCopyBinding{JobID: job.ID, OrgID: job.OrgID, SourceProjectID: job.SourceProjectID, TargetProjectID: job.TargetProjectID}
	snapshot := mediaapp.ProjectCopySnapshot{ID: job.Manifest.MediaSnapshotID, ManifestSHA256: job.Manifest.MediaSHA256, Assets: job.Manifest.Assets, Renditions: job.Manifest.Renditions}
	if err := transfer.Transfer(ctx, actor, binding, snapshot); err != nil {
		t.Fatal("actual fenced private cover bytes", err)
	}
	if _, err := copyStore.CompleteMedia(ctx, actor, job.ID, worker); err != nil {
		t.Fatal("register copied image and cover in same checkpoint", err)
	}
	if _, err := copyStore.CompleteCanvases(ctx, actor, job.ID, worker); err != nil {
		t.Fatal("complete actual empty graph set", err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET cover_asset_id=NULL WHERE id=?`, job.TargetProjectID)
	if _, err := copyStore.Publish(ctx, actor, job.ID, worker); err == nil {
		t.Fatal("target with missing frozen cover published")
	}
	lifecycleFixtureSQL(t, owner, `UPDATE workspace.project SET cover_asset_id=? WHERE id=?`, frozen.CoverAssetID, job.TargetProjectID)
	lifecycleFixtureSQL(t, owner, `UPDATE media.media_asset SET status='rejected',moderation_status='rejected' WHERE id=?`, frozen.CoverAssetID)
	if _, err := copyStore.Publish(ctx, actor, job.ID, worker); err == nil {
		t.Fatal("revoked copied cover published from frozen receipt alone")
	}
	current, err := copyStore.Find(ctx, actor, job.ID)
	if err != nil || current.Status != "running" || current.Stage != "finalizing" || current.WorkerID != worker {
		t.Fatal("rejected cover publication changed job fence", err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, frozen.CoverAssetID)
	finished, err := copyStore.Publish(ctx, actor, job.ID, worker)
	if err != nil || finished.Status != "succeeded" {
		t.Fatal("publish actual copied project cover", finished.Status, err)
	}
	var targetRow struct{ CoverAssetID *uuid.UUID }
	if err := db.Raw(`SELECT cover_asset_id FROM workspace.project WHERE id=?`, job.TargetProjectID).Scan(&targetRow).Error; err != nil || targetRow.CoverAssetID == nil || *targetRow.CoverAssetID != frozen.CoverAssetID {
		t.Fatal("published copy lost frozen cover binding", err)
	}
	targetCover := targetRow.CoverAssetID
	read, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Get(ctx, actor, finished.TargetProjectID)
	if err != nil || read.CoverUnavailable || read.Project.CoverAssetID == nil || *read.Project.CoverAssetID != *targetCover {
		t.Fatal("fresh target cover unavailable or changed", err)
	}
	target, err := media.FindAsset(ctx, actor, finished.TargetProjectID, *targetCover)
	if err != nil || target.ObjectKey == asset.ObjectKey || target.ID == asset.ID {
		t.Fatal("copied cover shared private source identity", err)
	}
	keys = append(keys, target.ObjectKey)
	if err := objects.Remove(ctx, asset.ObjectKey); err != nil {
		t.Fatal("remove only synthetic source cover bytes", err)
	}
	reader, err := objects.Get(ctx, target.ObjectKey)
	if err != nil {
		t.Fatal("target cover depended on removed source bytes", err)
	}
	actual, err := io.ReadAll(io.LimitReader(reader, int64(len(data))+1))
	closeErr := reader.Close()
	digest := sha256.Sum256(actual)
	if err != nil || closeErr != nil || !bytes.Equal(actual, data) || target.SHA256 == nil || *target.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("independent target cover changed actual bytes", err)
	}
	var sourceRow struct{ CoverAssetID *uuid.UUID }
	if err := db.Raw(`SELECT cover_asset_id FROM workspace.project WHERE id=?`, source).Scan(&sourceRow).Error; err != nil || sourceRow.CoverAssetID != nil {
		t.Fatal("copy overwrote later original project cover edit", err)
	}
}
