package storygraph_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	storygraphgorm "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/gormdb"
	storygraphhttp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/adapter/httpapi"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
)

func TestStoryGraphPostgreSQLQueriesReauthorizeAndNeverWrite(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL StoryGraph query journey")
	}
	database, err := platformdatabase.Open(context.Background(), databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	fixture := seedStoryGraphOwners(t, func(value any) error { return database.Create(value).Error }, "query")
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	store := storygraphgorm.New(database)
	compiler := storygraphapp.NewService(store, storygraphapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString})
	owner := storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	first, err := compiler.Compile(context.Background(), owner, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 0, IdempotencyKey: "query-version-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.Episode{}).Where("id = ?", fixture.episodeID).
		Updates(map[string]any{"name": "第二版标题", "revision": 2, "updated_at": now.Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	second, err := compiler.Compile(context.Background(), owner, storygraphapp.CompileCommand{
		ProjectID: fixture.projectID.String(), ExpectedHeadRevision: 1,
		ExpectedCurrentContentHash: first.Version.ContentHash, IdempotencyKey: "query-version-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	viewerID, outsiderID := uuid.New(), uuid.New()
	for _, record := range []any{
		&model.UserAccount{ID: viewerID, EmailNormalized: viewerID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Viewer", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.UserAccount{ID: outsiderID, EmailNormalized: outsiderID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Outsider", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: viewerID, Role: "viewer", Status: "active", JoinedAt: now},
	} {
		if err = database.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	count := func(value any, query string, args ...any) (int64, error) {
		var result int64
		errorFound := database.Model(value).Where(query, args...).Count(&result).Error
		return result, errorFound
	}
	before := queryFactCounts(t, count, fixture)
	var headBefore model.StoryGraphHead
	if err = database.First(&headBefore, "project_id = ?", fixture.projectID).Error; err != nil {
		t.Fatal(err)
	}

	queries := storygraphapp.NewQueryService(store)
	viewer := storygraphapp.Actor{UserID: viewerID.String(), TokenVersion: 1}
	current, err := queries.Version(context.Background(), viewer, storygraphapp.VersionQuery{ProjectID: fixture.projectID.String(), VersionRef: "current"})
	if err != nil || current.Version.ID != second.Version.ID || current.Stale || len(current.CompiledFrom) == 0 {
		t.Fatalf("viewer current query failed: %#v error=%v", current, err)
	}
	exact, err := queries.Version(context.Background(), viewer, storygraphapp.VersionQuery{ProjectID: fixture.projectID.String(), VersionRef: first.Version.ID})
	if err != nil || exact.Version.ID != first.Version.ID || exact.Stale || len(exact.CompiledFrom) == 0 {
		t.Fatalf("exact version query failed: %#v error=%v", exact, err)
	}
	lens, err := queries.Lens(context.Background(), viewer, storygraphapp.LensQuery{
		ProjectID: fixture.projectID.String(), VersionRef: "current", Lens: "outline",
		ScopeKind: "project", ScopeID: fixture.projectID.String(), Depth: 2, Limit: 20,
	})
	if err != nil || lens.VersionID != second.Version.ID || len(lens.Nodes) != 3 || len(lens.ResultHash) != 64 {
		t.Fatalf("persisted Lens query failed: %#v error=%v", lens, err)
	}
	diff, err := queries.Diff(context.Background(), viewer, storygraphapp.DiffQuery{
		ProjectID: fixture.projectID.String(), BaseVersionID: first.Version.ID, TargetVersionID: second.Version.ID, Limit: 20,
	})
	if err != nil || len(diff.NodeChanges) != 1 || diff.NodeChanges[0].ChangeType != "changed" {
		t.Fatalf("persisted diff failed: %#v error=%v", diff, err)
	}
	scene := findNode(t, second.Version.Nodes, "scene")
	trace, err := queries.Trace(context.Background(), viewer, storygraphapp.TraceQuery{
		ProjectID: fixture.projectID.String(), VersionRef: second.Version.ID,
		StoryNodeKey: scene.StoryNodeKey, Direction: "downstream", Depth: 1, Limit: 20,
	})
	if err != nil || len(trace.Nodes) != 3 || len(trace.Edges) != 2 {
		t.Fatalf("persisted trace failed: %#v error=%v", trace, err)
	}
	mux := http.NewServeMux()
	storygraphhttp.New(queries, storyGraphClaimsAuthenticator{claims: authentication.Claims{
		UserID: viewerID.String(), TokenVersion: 1,
	}}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/"+fixture.projectID.String()+"/storygraph/versions/current/lens?lens=outline&scope_kind=project&scope_id="+fixture.projectID.String()+"&depth=2&limit=20", nil))
	var envelope struct {
		Data struct {
			VersionID  string `json:"version_id"`
			ResultHash string `json:"result_hash"`
		} `json:"data"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK ||
		envelope.Data.VersionID != second.Version.ID || len(envelope.Data.ResultHash) != 64 {
		t.Fatalf("real HTTP and PostgreSQL Lens query failed: status=%d body=%s error=%v", response.Code, response.Body.String(), err)
	}
	if err = database.Model(&model.Episode{}).Where("id = ?", fixture.episodeID).
		Updates(map[string]any{"name": "尚未重编译的标题", "revision": 3, "updated_at": now.Add(2 * time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	stale, err := queries.Version(context.Background(), viewer, storygraphapp.VersionQuery{ProjectID: fixture.projectID.String(), VersionRef: "current"})
	if err != nil || stale.Version.ID != second.Version.ID || !stale.Stale {
		t.Fatalf("current version did not expose Owner drift: %#v error=%v", stale, err)
	}
	if err = database.Model(&model.Episode{}).Where("id = ?", fixture.episodeID).
		Update("current_script_version_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	incomplete, err := queries.Version(context.Background(), viewer, storygraphapp.VersionQuery{ProjectID: fixture.projectID.String(), VersionRef: "current"})
	if err != nil || incomplete.Version.ID != second.Version.ID || !incomplete.Stale {
		t.Fatalf("published version became unreadable after Owner incompleteness: %#v error=%v", incomplete, err)
	}

	_, err = queries.Version(context.Background(), storygraphapp.Actor{UserID: fixture.userID.String(), TokenVersion: 2}, storygraphapp.VersionQuery{ProjectID: fixture.projectID.String(), VersionRef: "current"})
	assertApplicationError(t, err, "unauthenticated")
	_, err = queries.Version(context.Background(), storygraphapp.Actor{UserID: outsiderID.String(), TokenVersion: 1}, storygraphapp.VersionQuery{ProjectID: fixture.projectID.String(), VersionRef: "current"})
	assertApplicationError(t, err, "not_found")

	after := queryFactCounts(t, count, fixture)
	if before != after {
		t.Fatalf("read-only queries changed fact counts: before=%#v after=%#v", before, after)
	}
	var headAfter model.StoryGraphHead
	if err = database.First(&headAfter, "project_id = ?", fixture.projectID).Error; err != nil ||
		headAfter.CurrentVersionID != headBefore.CurrentVersionID || headAfter.CurrentContentHash != headBefore.CurrentContentHash ||
		headAfter.Revision != headBefore.Revision || !headAfter.UpdatedAt.Equal(headBefore.UpdatedAt) {
		t.Fatalf("read-only queries changed StoryGraph head: before=%#v after=%#v error=%v", headBefore, headAfter, err)
	}
}

type storyGraphClaimsAuthenticator struct{ claims authentication.Claims }

func (authenticator storyGraphClaimsAuthenticator) Authenticate(*http.Request) (authentication.Claims, error) {
	return authenticator.claims, nil
}

type queryCounts struct{ versions, heads, receipts, events int64 }

func queryFactCounts(t *testing.T, count func(any, string, ...any) (int64, error), fixture storyGraphOwnerFixture) queryCounts {
	t.Helper()
	values := queryCounts{}
	checks := []struct {
		model any
		value *int64
	}{
		{&model.StoryGraphVersion{}, &values.versions},
		{&model.StoryGraphHead{}, &values.heads},
		{&model.CommandReceipt{}, &values.receipts},
		{&model.OutboxEvent{}, &values.events},
	}
	for _, check := range checks {
		value, err := count(check.model, "project_id = ?", fixture.projectID)
		if _, receipt := check.model.(*model.CommandReceipt); receipt {
			value, err = count(check.model, "workspace_id = ? AND operation = ?", fixture.workspaceID, "storygraph.compile")
		}
		if err != nil {
			t.Fatal(err)
		}
		*check.value = value
	}
	return values
}
