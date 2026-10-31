package media_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestLibraryPickerFullFilteredPaginationAndProjectIsolation(t *testing.T) {
	db := mediaStoreDB(t)
	owner := projectOwnerDB(t)
	var name, role string
	if err := db.Raw(`SELECT current_database(),current_user`).Row().Scan(&name, &role); err != nil || name != "glowloom_canvas_director_tests" || role != "glowloom_app" {
		t.Fatal("requires isolated picker database and restricted runtime", name, role, err)
	}
	actor, project := mediaStoreProject(t, owner)
	other, foreign := mediaStoreProject(t, owner)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	for i, dims := range [][2]int32{{1024, 576}, {1024, 576}, {1024, 1024}, {2048, 1152}, {1024, 576}, {0, 0}} {
		asset := imageAsset(project, uuid.New())
		asset.Width = &dims[0]
		asset.Height = &dims[1]
		if i == 5 {
			asset.Width = nil
			asset.Height = nil
		}
		asset.FileName = "needle fixture.png"
		asset.Status = domain.StatusProcessing
		asset.ModerationStatus = domain.ModerationPending
		asset.CreateTime = start.Add(time.Duration(i) * time.Hour)
		asset.UpdateTime = asset.CreateTime
		if i == 4 {
			asset.CreateTime = end
			asset.UpdateTime = end
		}
		if err := mediaTestStore(db).CreateAsset(t.Context(), actor, asset); err != nil {
			t.Fatal(err)
		}
		if err := owner.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, asset.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := mediaLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	q := libraryQuery()
	q.Kind = "image"
	q.PageSize = 1
	q.Search = "needle"
	q.Origin = "upload"
	q.CreatedFrom = &start
	q.CreatedBefore = &end
	q.Resolutions = []string{"1K"}
	q.Ratios = []string{"16:9"}
	first, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || first.Total != 2 || len(first.Items) != 1 {
		t.Fatalf("full filters before pagination %+v %v", first, err)
	}
	q.Page = 2
	second, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second filtered page %+v %v", second, err)
	}
	q.Page = 3
	last, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || last.Total != 2 || len(last.Items) != 0 {
		t.Fatalf("filtered end %+v %v", last, err)
	}
	q.Page = 1
	q.Resolutions = []string{"1K", "2K"}
	q.Ratios = []string{"16:9", "1:1"}
	combined, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || combined.Total != 4 {
		t.Fatalf("OR within dimensions; unknown dimensions excluded %+v %v", combined, err)
	}
	q.Origin = "generated"
	empty, err := store.ListLibrary(t.Context(), actor, scope, q)
	if err != nil || empty.Total != 0 {
		t.Fatalf("origin inferred from title instead of asset %+v %v", empty, err)
	}
	if _, err := store.ListLibrary(t.Context(), other, scope, q); err == nil {
		t.Fatal("foreign actor queried project")
	}
	scope.ProjectID = &foreign
	if _, err := store.ListLibrary(t.Context(), actor, scope, q); err == nil {
		t.Fatal("current actor queried foreign project")
	}
}
