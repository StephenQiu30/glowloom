package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type purgeTestReferences struct{ tx *gorm.DB }

func (r purgeTestReferences) HasMediaReferences(ctx context.Context, _ identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	var used bool
	err := r.tx.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project WHERE id=? AND cover_asset_id=?)`, project, asset).Scan(&used).Error
	return used, err
}
func purgeTestReferenceFactory(tx *gorm.DB) mediaapp.LibraryReferences {
	return purgeTestReferences{tx}
}

func TestMediaPurgeCurrentCoverBlockedAndPermanentReplayNoRawDeletion(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	upload := documentUpload(t, db, objects, actor, project, "cover-purge.png", uploadPNG(t))
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "update_item", ItemID: &upload.Asset.ID, Metadata: &mediaapp.LibraryMetadata{Title: "明确保留的封面", Category: "material", Tags: []string{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "recycle_items", ExpectedRevision: 1, Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE workspace.project SET cover_asset_id=? WHERE id=?`, upload.Asset.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	request := mediaapp.PurgeInput{Scope: scope, Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID, Revision: 2}}, ExpectedRevision: 2, ExpectedProjectRevision: 3, PermanentDeleteConfirmed: true, Key: uuid.New()}
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, request)
	if err != nil || job.Status != "failed" || len(job.Items) != 1 || job.Items[0].Status != "blocked" || job.Items[0].FailureCode == nil || *job.Items[0].FailureCode != "in_use" {
		t.Fatal("actual current cover did not block purge", job, err)
	}
	var deleted bool
	if err := db.Raw(`SELECT is_delete FROM media.media_asset WHERE project_id=? AND id=?`, project, upload.Asset.ID).Scan(&deleted).Error; err != nil || deleted {
		t.Fatal("blocked item was tombstoned before references were cleared", deleted, err)
	}
	if err := owner.Exec(`UPDATE workspace.project SET cover_asset_id=NULL,revision=revision+1 WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := repo.CreatePurge(t.Context(), actor, request)
	want, _ := json.Marshal(job)
	got, _ := json.Marshal(replay)
	if err != nil || !bytes.Equal(want, got) {
		t.Fatal("later cover/catalog state rewrote permanent result", err)
	}
	request.PermanentDeleteConfirmed = false
	if _, err := repo.CreatePurge(t.Context(), actor, request); err == nil {
		t.Fatal("unconfirmed permanent deletion accepted")
	}
	request.PermanentDeleteConfirmed = true
	request.ExpectedProjectRevision = 4
	if _, err := repo.CreatePurge(t.Context(), actor, request); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("same permanent key accepted changed input", err)
	}
}

func TestMediaPurgePersonalTextAdmissionNeverInventsProjectAndUnavailableReferenceFailsClosed(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project := mediaStoreProject(t, db)
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	text := "必须真正清空的个人原文"
	created, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{Title: "私有标题", Category: "material", Tags: []string{}, PlainText: &text}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Items[0].ID
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 1, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: id, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, nil, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, mediaapp.PurgeInput{Scope: scope, Items: []mediaapp.LibraryItemRevision{{ID: id, Revision: 2}}, ExpectedRevision: 2, PermanentDeleteConfirmed: true, Key: uuid.New()})
	if err != nil || job.Scope.ProjectID != nil || job.Status != "queued" {
		t.Fatal("personal text incorrectly depended on foreign project", job, err)
	}
	foreign, _ := mediaStoreProject(t, db)
	if _, err := repo.GetPurge(t.Context(), foreign, job.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("foreign actor discovered permanent purge", err)
	}
	objects := glbTestObjects(t)
	upload := documentUpload(t, db, objects, actor, project, "closed.png", uploadPNG(t))
	projectScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: projectScope, Key: uuid.New(), Action: "update_item", ItemID: &upload.Asset.ID, Metadata: &mediaapp.LibraryMetadata{Title: "原图", Category: "material", Tags: []string{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: projectScope, Key: uuid.New(), Action: "recycle_items", ExpectedRevision: 1, Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreatePurge(t.Context(), actor, mediaapp.PurgeInput{Scope: projectScope, Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID, Revision: 2}}, ExpectedRevision: 2, ExpectedProjectRevision: 3, PermanentDeleteConfirmed: true, Key: uuid.New()})
	if !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing installed reference owner was treated as zero references", err)
	}
}
