package workspace_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProjectRecycleListHidesExpiredAndAllowsAuthorizedReconciliation(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	expired := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	live := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	if err := db.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day' WHERE id=?`, expired).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=clock_timestamp(),purge_after=clock_timestamp()+interval '30 days' WHERE id=?`, live).Error; err != nil {
		t.Fatal(err)
	}
	router := lifecycleHTTPRouter(db, actor)
	for _, tc := range []struct {
		path string
		want int
	}{{"/api/projects?deleted=true", 1}, {"/api/projects?deleted=true&include_expired=true", 2}} {
		response := projectHTTPRequest(ctx, router, http.MethodGet, tc.path, "", nil)
		var page struct {
			Items []struct {
				ID         uuid.UUID  `json:"id"`
				DeleteTime *time.Time `json:"delete_time"`
				PurgeAfter *time.Time `json:"purge_after"`
			} `json:"items"`
			ServerTime time.Time `json:"server_time"`
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != tc.want || page.ServerTime.IsZero() {
			t.Fatalf("%s: %d %s", tc.path, response.Code, response.Body.String())
		}
		for _, item := range page.Items {
			if item.DeleteTime == nil || item.PurgeAfter == nil {
				t.Fatal("missing recovery deadline")
			}
		}
	}
	for _, path := range []string{"/api/projects?include_expired=true", "/api/projects?deleted=false&include_expired=true", "/api/projects?deleted=true&include_expired=bad", "/api/projects?deleted=true&include_expired=true&include_expired=false"} {
		response := projectHTTPRequest(ctx, router, http.MethodGet, path, "", nil)
		if response.Code != 422 {
			t.Fatalf("invalid mode accepted %s: %d", path, response.Code)
		}
	}
	other := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	response := projectHTTPRequest(ctx, lifecycleHTTPRouter(db, other), http.MethodGet, "/api/projects?deleted=true&include_expired=true", "", nil)
	var private struct {
		Items []json.RawMessage `json:"items"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &private) != nil || len(private.Items) != 0 {
		t.Fatal("expired reconciliation crossed scope", response.Code, response.Body.String())
	}
}

func TestProjectRecycleCursorBindsExplicitReconciliationMode(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	for range 2 {
		id := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
		lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,delete_time=clock_timestamp(),purge_after=clock_timestamp()+interval '30 days' WHERE id=?`, id)
	}
	router := lifecycleHTTPRouter(db, actor)
	first := projectHTTPRequest(ctx, router, http.MethodGet, "/api/projects?deleted=true&include_expired=true&limit=1", "", nil)
	var page struct {
		NextCursor *string `json:"next_cursor"`
	}
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &page) != nil || page.NextCursor == nil {
		t.Fatal("missing reconciliation cursor", first.Code, first.Body.String())
	}
	for _, mode := range []string{"", "&include_expired=false", "&include_expired=true&order=recent_content"} {
		response := projectHTTPRequest(ctx, router, http.MethodGet, "/api/projects?deleted=true&limit=1"+mode+"&cursor="+*page.NextCursor, "", nil)
		if response.Code != 422 {
			t.Fatalf("cursor crossed mode %q: %d", mode, response.Code)
		}
	}
	response := projectHTTPRequest(ctx, router, http.MethodGet, "/api/projects?deleted=true&include_expired=true&limit=1&cursor="+*page.NextCursor, "", nil)
	if response.Code != 200 {
		t.Fatal("valid cursor rejected", response.Code, response.Body.String())
	}
}
