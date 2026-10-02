package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image/gif"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestGIFActualPrivateUploadReviewThumbnailsAndCompleteIndependentCopy(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	data := uploadGIF(t, 3)
	asset := documentUpload(t, db, objects, actor, project, "完整动画.gif", data)
	store := pgmedia.NewStore(db)
	original, err := store.FindAsset(t.Context(), actor, project, asset.Asset.ID)
	if err != nil || original.MimeType != "image/gif" || original.Codec == nil || *original.Codec != "gif" || original.SHA256 == nil || original.Width == nil || *original.Width != 3 || original.Height == nil || *original.Height != 4 || original.DurationMS != nil {
		t.Fatal("formal GIF original facts", original, err)
	}
	var review mediaapp.LocalUploadReview
	if json.Unmarshal(original.ModerationDetail, &review) != nil || review.PrincipalID != actor.ID || review.SHA256 != *original.SHA256 || !review.RightsConfirmed || !review.NoAuthorizationRequiredRealPerson || review.Normalization != nil {
		t.Fatal("GIF original approval was replaced by conversion metadata", review)
	}
	file, err := objects.Get(t.Context(), original.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || !bytes.Equal(actual, data) {
		t.Fatal("animation original changed into a still image", err, closeErr)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(actual))
	if err != nil || len(decoded.Image) != 3 {
		t.Fatal("actual stored GIF is not complete animation", err)
	}
	rends, err := store.FindRenditions(t.Context(), actor, project, original.ID)
	if err != nil || len(rends) != 2 {
		t.Fatal("GIF real first-frame thumbnails missing", err)
	}
	keys := []string{original.ObjectKey}
	for _, r := range rends {
		keys = append(keys, r.ObjectKey)
		file, err := objects.Get(t.Context(), r.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
		thumb, err := png.Decode(file)
		closeErr := file.Close()
		if err != nil || closeErr != nil || thumb.Bounds().Dx() != int(*r.Width) || thumb.Bounds().Dy() != int(*r.Height) {
			t.Fatal("GIF derivative is not an actual PNG", err, closeErr)
		}
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'动画完整副本','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot mediaapp.ProjectCopySnapshot
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'动画完整副本','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil || snapshot.Assets != 1 || snapshot.Renditions != 2 {
		t.Fatal("complete GIF owning snapshot", snapshot, err)
	}
	repo := pgmedia.NewProjectCopyStore(db)
	if err := mediaapp.NewProjectCopyTransfer(repo, mediaobjects.NewProjectCopyObjects(objects), t.TempDir()).Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("GIF full private bytes transfer", err)
	}
	intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
	if err != nil || len(intents) != 3 {
		t.Fatal("complete independent GIF and previews", intents, err)
	}
	for _, intent := range intents {
		keys = append(keys, intent.TargetObjectKey)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, key := range keys {
			_ = objects.Remove(ctx, key)
		}
	})
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal("register real independent GIF assets", err)
	}
	if err := objects.Remove(t.Context(), original.ObjectKey); err != nil {
		t.Fatal("remove only synthetic source original", err)
	}
	for _, intent := range intents {
		if intent.RenditionKind != "" {
			continue
		}
		file, err := objects.Get(t.Context(), intent.TargetObjectKey)
		if err != nil {
			t.Fatal("independent copied GIF is unavailable after source removal", err)
		}
		copied, err := io.ReadAll(file)
		closeErr := file.Close()
		if err != nil || closeErr != nil || !bytes.Equal(copied, data) {
			t.Fatal("copied animation bytes differ", err, closeErr)
		}
	}
	personal := libraryPersonalFixture(t, db, objects, actor, "个人动画.gif", data)
	if personal.Asset.MIMEType != "image/gif" || personal.Asset.Kind != "image" {
		t.Fatal("same scoped pipeline lost the original GIF type", personal)
	}
}
