package media_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestLibraryActualReviewedProjectImageUsesLegacyIdentityAndSeparateMetadata(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	upload := documentUpload(t, db, objects, actor, project, "actual-source.png", data.Bytes())
	media := pgmedia.NewStore(db)
	before, err := media.FindAsset(t.Context(), actor, project, upload.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	rends, err := media.FindRenditions(t.Context(), actor, project, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, rendition := range rends {
			if err := objects.Remove(ctx, rendition.ObjectKey); err != nil {
				t.Error("remove exact actual image fixture rendition")
			}
		}
	})
	store := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	page, err := store.ListLibrary(t.Context(), actor, scope, libraryQuery())
	if err != nil || page.Total != 1 || page.Items[0].ID != before.ID || page.Items[0].Revision != 0 || page.Items[0].Media == nil || *page.Items[0].Media.Width != 3 {
		t.Fatal("legacy image not shown with its real identity", page, err)
	}
	changed, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "update_item", ItemID: &before.ID, Metadata: &mediaapp.LibraryMetadata{Title: "目录展示标题", Category: "prop", Tags: []string{"道具"}, SourceLabel: "用户填写来源", Note: "可编辑备注"}})
	if err != nil || changed.Items[0].Revision != 1 {
		t.Fatal("classify real image", changed, err)
	}
	after, err := media.FindAsset(t.Context(), actor, project, before.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("catalog metadata changed original/review/execution facts", err)
	}
	trash, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 1, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: before.ID, Revision: 1}}})
	if err != nil || trash.Revision != 2 {
		t.Fatal("catalog trash", err)
	}
	active, err := store.ListLibrary(t.Context(), actor, scope, libraryQuery())
	if err != nil || active.Total != 0 {
		t.Fatal("trashed catalog item remained visible", active, err)
	}
	q := libraryQuery()
	q.State = "trashed"
	trashed, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || trashed.Total != 1 || trashed.Items[0].TrashedAt == nil {
		t.Fatal("trash page lost original", trashed, err)
	}
	if _, err := media.FindAsset(t.Context(), actor, project, before.ID); err != nil {
		t.Fatal("catalog trash destroyed historic business media", err)
	}
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 2, Action: "restore_items", Items: []mediaapp.LibraryItemRevision{{ID: before.ID, Revision: 2}}}); err != nil {
		t.Fatal("restore catalog", err)
	}
}
