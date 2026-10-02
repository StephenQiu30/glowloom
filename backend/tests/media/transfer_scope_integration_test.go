package media_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestMediaTransferActualProjectToPersonalCreatesIndependentOriginalAndRenditions(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	objects := copyobjects.NewProjectCopyObjects(f.objects)
	projectResult, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(f.job))
	if err != nil || projectResult.Status != "succeeded" {
		t.Fatal(projectResult, err)
	}
	input := mediaapp.TransferInput{Source: f.input.Target, Target: f.input.Source, Items: []mediaapp.LibraryItemRevision{{ID: projectResult.Items[0].TargetItemID, Revision: 1}}, ExpectedSourceRevision: 1, ExpectedTargetRevision: 1, ExpectedProjectRevision: 2, Key: uuid.New()}
	job, err := f.repo.CreateTransfer(t.Context(), f.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var keys []string
		if err := f.db.WithContext(context.Background()).Raw(`SELECT target_object_key FROM media.transfer_object WHERE job_id=?`, job.ID).Scan(&keys).Error; err != nil {
			t.Error(err)
			return
		}
		for _, key := range keys {
			_ = f.objects.Remove(context.Background(), key)
		}
	})
	result, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(job))
	if err != nil || result.Status != "succeeded" {
		t.Fatal("actual opposite scope transfer", result, err)
	}
	file, err := pgmedia.NewLibraryStore(f.db, libraryTestAccess, time.Now).FindLibraryMedia(t.Context(), f.actor, input.Target, *result.Items[0].TargetAssetID)
	if err != nil || file.Asset.Personal == nil || file.Asset.Personal.ActorID != f.actor.ID || file.Asset.ProjectID != uuid.Nil || len(file.Renditions) != 2 || file.Asset.ID == f.input.Items[0].ID || file.Asset.ID == projectResult.Items[0].TargetItemID {
		t.Fatal("independent personal ownership missing", file, err)
	}
	reader, err := f.objects.Get(t.Context(), file.Asset.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || n != file.Asset.ByteSize {
		t.Fatal("actual personal original bytes", n, err, closeErr)
	}
	if _, err := pgmedia.NewStore(f.db).FindAsset(t.Context(), f.actor, *input.Source.ProjectID, file.Asset.ID); err == nil {
		t.Fatal("personal original leaked into ordinary project reference")
	}
	var projectRevision int64
	if err := f.db.Raw(`SELECT revision FROM workspace.project WHERE id=?`, *input.Source.ProjectID).Scan(&projectRevision).Error; err != nil || projectRevision != 2 {
		t.Fatal("personal transfer modified source project content", projectRevision, err)
	}
}

func TestMediaTransferQueuedSourceAndTargetFolderChangesFailBeforeAnyObjectWrite(t *testing.T) {
	for _, changeSource := range []bool{true, false} {
		t.Run(map[bool]string{true: "source metadata changed", false: "target folder renamed"}[changeSource], func(t *testing.T) {
			f := newTransferRecoveryFixture(t, 1)
			library := pgmedia.NewLibraryStore(f.db, libraryTestAccess, time.Now)
			job := f.job
			if changeSource {
				_, err := library.ApplyLibraryCommand(t.Context(), f.actor, mediaapp.LibraryCommand{Scope: f.input.Source, Key: uuid.New(), ExpectedRevision: 1, Action: "update_item", ItemID: &f.input.Items[0].ID, Metadata: &mediaapp.LibraryMetadata{Title: "新分类描述", Category: "other"}})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.repo.ControlTransfer(t.Context(), f.actor, job.ID, uuid.New(), job.Revision, "cancel"); err != nil {
					t.Fatal(err)
				}
				created, err := library.ApplyLibraryCommand(t.Context(), f.actor, mediaapp.LibraryCommand{Scope: f.input.Target, Key: uuid.New(), Action: "create_folder", Folder: &mediaapp.LibraryFolderInput{Name: "目标目录", Style: "cinema", Theme: "obsidian"}})
				if err != nil || created.Folder == nil {
					t.Fatal(created, err)
				}
				input := f.input
				input.Key, input.TargetFolderID, input.ExpectedFolderRevision, input.ExpectedTargetRevision, input.ExpectedProjectRevision = uuid.New(), &created.Folder.ID, 1, 1, 2
				job, err = f.repo.CreateTransfer(t.Context(), f.actor, input)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := library.ApplyLibraryCommand(t.Context(), f.actor, mediaapp.LibraryCommand{Scope: f.input.Target, Key: uuid.New(), ExpectedRevision: 1, Action: "update_folder", FolderID: &created.Folder.ID, ExpectedFolderRevision: 1, Folder: &mediaapp.LibraryFolderInput{Name: "已改名目录", Style: "cinema", Theme: "obsidian"}}); err != nil {
					t.Fatal(err)
				}
			}
			objects := &transferUnknownWrite{ProjectCopyObjects: copyobjects.NewProjectCopyObjects(f.objects), failAt: 100}
			result, err := mediaapp.NewTransferWorker(f.repo, objects, t.TempDir()).Execute(t.Context(), transferExecution(job))
			want := map[bool]string{true: "source_changed", false: "target_folder_changed"}[changeSource]
			if err != nil || result.Status != "failed" || result.NeedsReconciliation || result.Items[0].FailureCode == nil || *result.Items[0].FailureCode != want || objects.writes != 0 {
				t.Fatal("changed frozen source or folder admitted physical work", result, objects.writes, err)
			}
			var published int64
			if err := f.db.Raw(`SELECT count(*) FROM media.media_asset WHERE id=?`, *result.Items[0].TargetAssetID).Scan(&published).Error; err != nil || published != 0 {
				t.Fatal("changed source or folder target was published", published, err)
			}
		})
	}
}
