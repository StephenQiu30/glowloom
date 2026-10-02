package media_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func libraryQuery() mediaapp.LibraryQuery {
	return mediaapp.LibraryQuery{Page: 1, PageSize: 40, State: "active", Order: "updated_desc"}
}

func TestLibraryPersonalCommandsPermanentReplayAndCurrentIdentity(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	store := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	initial, err := store.ListLibrary(t.Context(), actor, scope, libraryQuery())
	if err != nil || initial.Revision != 0 || len(initial.Items) != 0 || initial.CurrentActorID != actor.ID {
		t.Fatal("read must be scoped and must not create an empty library", initial, err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM media.library WHERE id=?`, initial.LibraryID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("read created a business row", count, err)
	}
	command := mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "create_folder", Folder: &mediaapp.LibraryFolderInput{Name: "角色"}}
	folder, err := store.ApplyLibraryCommand(t.Context(), actor, command)
	if err != nil || folder.Folder == nil || folder.Revision != 1 || folder.ProjectRevision != nil {
		t.Fatal("create personal classification", folder, err)
	}
	text := "保留原始文字"
	created, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 1, Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, FolderID: &folder.Folder.ID, Title: "角色设定", Category: "character", Tags: []string{"主角"}, Favorite: true}})
	if err != nil || len(created.Items) != 1 || created.Revision != 2 {
		t.Fatal("create plain text classification", created, err)
	}
	replay, err := store.ApplyLibraryCommand(t.Context(), actor, command)
	want, _ := json.Marshal(folder)
	actual, _ := json.Marshal(replay)
	if err != nil || !bytes.Equal(want, actual) {
		t.Fatal("later mutations broke original permanent replay", err)
	}
	command.Folder.Name = "换了请求"
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, command); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("permanent key accepted a different body", err)
	}
	removed, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 2, Action: "delete_folder", FolderID: &folder.Folder.ID, ExpectedFolderRevision: 1})
	if err != nil || removed.Revision != 3 {
		t.Fatal("delete personal classification without deleting its text", err)
	}
	detail, err := store.LibraryDetail(t.Context(), actor, scope, created.Items[0].ID)
	if err != nil || detail.FolderID != nil || detail.PlainText == nil || *detail.PlainText != text || detail.Revision != 2 {
		t.Fatal("deleting folder lost text or failed to return it to root", detail, err)
	}
	foreign, _ := mediaStoreProject(t, db)
	if _, err := store.LibraryDetail(t.Context(), foreign, scope, detail.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("another personal owner discovered text", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	command.Folder.Name = "角色"
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, command); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("receipt bypassed current identity revocation", err)
	}
}

func TestLibraryProjectHierarchyCASAndAtomicMove(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project := mediaStoreProject(t, db)
	store := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	var parent *uuid.UUID
	var first uuid.UUID
	for level := int64(0); level < 8; level++ {
		result, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: level, Action: "create_folder", Folder: &mediaapp.LibraryFolderInput{ParentID: parent, Name: fmt.Sprintf("第%d层", level+1), Style: "cinema", Theme: "obsidian"}})
		if err != nil || result.Folder == nil || result.ProjectRevision == nil || *result.ProjectRevision != level+2 {
			t.Fatal("same-transaction project content revision", result, err)
		}
		id := result.Folder.ID
		if level == 0 {
			first = id
		}
		parent = &id
	}
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 8, Action: "create_folder", Folder: &mediaapp.LibraryFolderInput{ParentID: parent, Name: "第九层", Style: "cinema", Theme: "obsidian"}}); !errors.Is(err, domain.ErrInvalidLibrary) {
		t.Fatal("over-depth folder admitted", err)
	}
	text := "镜头提示"
	item, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 8, Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "镜头", Category: "material"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 9, Action: "move_items", Items: []mediaapp.LibraryItemRevision{{ID: item.Items[0].ID, Revision: 1}, {ID: uuid.New(), Revision: 1}}, TargetFolderID: &first})
	if !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("mixed invalid batch admitted", err)
	}
	detail, err := store.LibraryDetail(t.Context(), actor, scope, item.Items[0].ID)
	if err != nil || detail.FolderID != nil || detail.Revision != 1 {
		t.Fatal("batch partly moved before failure", detail, err)
	}
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 8, Action: "move_items", Items: []mediaapp.LibraryItemRevision{{ID: detail.ID, Revision: 1}}, TargetFolderID: &first}); !errors.Is(err, mediaapp.ErrLibraryConflict) {
		t.Fatal("stale full library revision accepted", err)
	}
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 9, Action: "delete_folder", FolderID: &first, ExpectedFolderRevision: 1}); !errors.Is(err, mediaapp.ErrLibraryFolderNotEmpty) {
		t.Fatal("nonempty project folder deleted", err)
	}
}

func TestLibraryFullQueryFiltersBeforePageAndStableThreeOrders(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	store := pgmedia.NewLibraryStore(db, libraryTestAccess, func() time.Time { return domainNow })
	for i := int64(0); i < 43; i++ {
		text := fmt.Sprintf("原文%d", i)
		category := "material"
		if i%2 == 0 {
			category = "character"
		}
		if _, err := store.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: i, Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: fmt.Sprintf("标题%03d", 42-i), Category: category, Tags: []string{"源标签"}, Favorite: i%2 == 0}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, order := range []string{"updated_desc", "updated_asc", "name_asc"} {
		q := libraryQuery()
		q.Order, q.PageSize = order, 20
		seen := make(map[uuid.UUID]bool)
		for page := 1; page <= 3; page++ {
			q.Page = page
			result, err := store.ListLibrary(t.Context(), actor, scope, q)
			if err != nil || result.Total != 43 || result.CategoryCounts["character"] != 22 {
				t.Fatal("full scoped total/facets", result, err)
			}
			for _, item := range result.Items {
				if seen[item.ID] {
					t.Fatal("page tie repeated an item", order, item.ID)
				}
				seen[item.ID] = true
			}
		}
		if len(seen) != 43 {
			t.Fatal("stable page missed an item", order, len(seen))
		}
	}
	q := libraryQuery()
	q.PageSize, q.FavoriteOnly, q.Search, q.Order = 20, true, "标题000", "name_asc"
	result, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].Title != "标题000" {
		t.Fatal("search/favorite applied after page", result, err)
	}
}
