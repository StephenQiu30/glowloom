package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func seedCopyMedia(t *testing.T, database *gorm.DB, objects *objectstorage.Client) (identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot, []string) {
	t.Helper()
	actor, project := mediaStoreProject(t, database)
	asset := uuid.New()
	key := "projects/" + project.String() + "/image/2026/10/" + asset.String() + ".png"
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	rkey := strings.TrimSuffix(key, ".png") + "/thumb_256.png"
	for _, objectKey := range []string{key, rkey} {
		if err := objects.Put(t.Context(), objectKey, bytes.NewReader(imageBytes.Bytes()), int64(imageBytes.Len()), "image/png"); err != nil {
			t.Fatal("write scoped synthetic PNG")
		}
	}
	// Source SHA and rendition size are intentionally missing legacy facts; the
	// copy must compute and durably bind real bytes before any target write.
	if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,width,height,moderation_status,aigc_marked) VALUES(?,?,'image','upload','ready',?,'内容.png','image/png',?,3,2,'passed',true)`, asset, project, key, imageBytes.Len()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO media.rendition(id,media_asset_id,kind,object_key,width,height) VALUES(?,?,'thumb_256',?,3,2)`, uuid.New(), asset, rkey).Error; err != nil {
		t.Fatal(err)
	}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: project, TargetProjectID: uuid.New()}
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,status) VALUES(?,?,'复制媒体私有目标','16:9','realistic','copying')`, binding.TargetProjectID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot mediaapp.ProjectCopySnapshot
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgmedia.NewProjectCopyStore(tx).Freeze(t.Context(), actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,1,?,'复制媒体私有目标','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, actor.OrgID, actor.ID, project, binding.TargetProjectID).Error
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := pgmedia.NewProjectCopyStore(database).Objects(t.Context(), actor, binding, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{key, rkey}
	for _, row := range rows {
		keys = append(keys, row.TargetObjectKey)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, objectKey := range keys {
			if err := objects.Remove(ctx, objectKey); err != nil {
				t.Error("exact scoped copy fixture cleanup")
			}
		}
	})
	return actor, binding, snapshot, keys
}

func TestProjectCopyMediaRealObjectsRegistrationAndCleanup(t *testing.T) {
	database := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, binding, snapshot, keys := seedCopyMedia(t, database, objects)
	repo := pgmedia.NewProjectCopyStore(database)
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("metadata registered before object receipts", err)
	}
	staging := t.TempDir()
	transfer := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), staging)
	if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("actual object transfer", err)
	}
	intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
	if err != nil || len(intents) != 2 {
		t.Fatal("original/rendition receipt incomplete", intents, err)
	}
	for _, intent := range intents {
		if intent.Status != "verified" || !intent.SourceVerified || intent.SHA256 == nil || intent.ByteSize == nil {
			t.Fatal("missing durable verified byte digest", intent)
		}
		reader, err := objects.Get(t.Context(), intent.TargetObjectKey)
		if err != nil {
			t.Fatal("private copy readback")
		}
		data, readErr := io.ReadAll(reader)
		if err := errors.Join(readErr, reader.Close()); err != nil {
			t.Fatal("readback close")
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != *intent.SHA256 || int64(len(data)) != *intent.ByteSize {
			t.Fatal("actual copied bytes differ")
		}
	}
	var receipt mediaapp.ProjectCopyReceipt
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		receipt, err = pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil || receipt.Assets != 1 || receipt.Renditions != 1 {
		t.Fatal("atomic real copy registration", receipt, err)
	}
	for _, targetAsset := range snapshot.AssetMapping {
		if _, err := pgmedia.NewStore(database).FindAsset(t.Context(), actor, binding.TargetProjectID, targetAsset); !errors.Is(err, mediaapp.ErrNotFound) {
			t.Fatal("unpublished copy media visible", err)
		}
		if ref, err := repo.ReferenceCopiedAsset(t.Context(), actor, binding, targetAsset); err != nil || ref.ID != targetAsset {
			t.Fatal("owning copy reader lost ready private media", ref, err)
		}
	}
	if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("verified exact-key replay", err)
	}
	if err := transfer.Cleanup(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("verified private cleanup", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return pgmedia.NewProjectCopyStore(tx).FinishCleanup(t.Context(), actor, binding, snapshot)
	}); err != nil {
		t.Fatal("owner metadata cleanup", err)
	}
	for i, key := range keys {
		exists, err := objects.Exists(t.Context(), key)
		if err != nil || exists != (i < 2) {
			t.Fatal("cleanup touched source or retained target object", i, exists, err)
		}
	}
}

type copyUnknownWrite struct {
	mediaapp.ProjectCopyObjects
	failed bool
}

func (o *copyUnknownWrite) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, digest string) error {
	err := o.ProjectCopyObjects.PutIfAbsent(ctx, key, reader, size, mime, digest)
	if err == nil && !o.failed {
		o.failed = true
		return io.ErrUnexpectedEOF
	}
	return err
}

func TestProjectCopyMediaRealUnknownWriteAndCollisionRecovery(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown after persisted object", true: "collision preserves unrelated bytes"}[collision], func(t *testing.T) {
			database := mediaStoreDB(t)
			objects := glbTestObjects(t)
			actor, binding, snapshot, _ := seedCopyMedia(t, database, objects)
			repo := pgmedia.NewProjectCopyStore(database)
			adapter := copyobjects.NewProjectCopyObjects(objects)
			intents, err := repo.Objects(t.Context(), actor, binding, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if collision {
				data := []byte("not the frozen source")
				if err := objects.Put(t.Context(), intents[0].TargetObjectKey, bytes.NewReader(data), int64(len(data)), "image/png"); err != nil {
					t.Fatal("create exact synthetic collision")
				}
			}
			transfer := mediaapp.NewProjectCopyTransfer(repo, &copyUnknownWrite{ProjectCopyObjects: adapter}, t.TempDir())
			var failure *mediaapp.ProjectCopyTransferError
			err = transfer.Transfer(t.Context(), actor, binding, snapshot)
			if !errors.As(err, &failure) || !failure.NeedsReconciliation {
				t.Fatal("unknown write/collision reported success", err)
			}
			intents, err = repo.Objects(t.Context(), actor, binding, snapshot)
			if err != nil || intents[0].Status != "pending" || !intents[0].SourceVerified {
				t.Fatal("unknown outcome lost durable pre-write receipt", intents, err)
			}
			if collision {
				err := mediaapp.NewProjectCopyTransfer(repo, adapter, t.TempDir()).Cleanup(t.Context(), actor, binding, snapshot)
				if err == nil {
					t.Fatal("unverified collision deleted")
				}
				exists, err := objects.Exists(t.Context(), intents[0].TargetObjectKey)
				if err != nil || !exists {
					t.Fatal("collision removed instead of preserved")
				}
				return
			}
			// This is an explicit owner-layer recovery fixture; the workspace
			// worker's claim/CAS and reconciliation boundary is tested separately.
			if err := mediaapp.NewProjectCopyTransfer(repo, adapter, t.TempDir()).Transfer(t.Context(), actor, binding, snapshot); err != nil {
				t.Fatal("same immutable keys not recoverable", err)
			}
		})
	}
}
