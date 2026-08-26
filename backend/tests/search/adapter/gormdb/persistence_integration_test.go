package gormdb_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	projectgorm "github.com/StephenQiu30/lanverse/backend/internal/production/project/adapter/gormdb"
	projectapp "github.com/StephenQiu30/lanverse/backend/internal/production/project/application"
	searches "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/elasticsearch"
	searchgorm "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/gormdb"
	searchproject "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/projectaccess"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestPostgreSQLOwnerSnapshotsDriveAuthorizedRealElasticsearchSearchAndStaleness(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	elasticsearchURL := os.Getenv("LANVERSE_TEST_ELASTICSEARCH_URL")
	if databaseURL == "" || elasticsearchURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL and LANVERSE_TEST_ELASTICSEARCH_URL to run the Search journey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	fixture := seedSearchOwners(t, func(value any) error { return database.Create(value).Error })
	prefix := "lanverse-pg-search-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	index, err := searches.New(searches.Config{
		Addresses: []string{elasticsearchURL}, ScriptAlias: prefix + "-script-v1", StoryGraphAlias: prefix + "-storygraph-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		request, requestErr := http.NewRequest(http.MethodDelete, strings.TrimRight(elasticsearchURL, "/")+"/"+prefix+"-*", nil)
		if requestErr == nil {
			_, _ = http.DefaultClient.Do(request)
		}
	})
	snapshots := searchgorm.New(database)
	script, err := snapshots.CurrentScriptSnapshot(ctx, fixture.projectID.String())
	if err != nil || len(script.Documents) != 1 || script.Documents[0].OwnerVersionID != fixture.scriptVersionID.String() {
		t.Fatalf("PostgreSQL Script Owner snapshot is incomplete: %#v err=%v", script, err)
	}
	graph, err := snapshots.CurrentStoryGraphSnapshot(ctx, fixture.projectID.String())
	if err != nil || len(graph.Documents) != 1 || graph.Documents[0].StoryNodeKey != fixture.storyNodeKey {
		t.Fatalf("PostgreSQL StoryGraph Owner snapshot is incomplete: %#v err=%v", graph, err)
	}
	projectedAt := time.Date(2026, 8, 27, 21, 0, 0, 0, time.UTC)
	if err = index.Project(ctx, script, search.ProjectionSource{Kind: search.SourceEvent, ID: uuid.NewString()}, projectedAt); err != nil {
		t.Fatal(err)
	}
	if err = index.Project(ctx, graph, search.ProjectionSource{Kind: search.SourceEvent, ID: uuid.NewString()}, projectedAt); err != nil {
		t.Fatal(err)
	}
	projects := projectapp.NewService(projectgorm.New(database), func() time.Time { return projectedAt }, uuid.NewString)
	service := searchapp.NewService(searchproject.New(projects), snapshots, index)
	actor := searchapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	scriptResult, err := service.SearchScripts(ctx, actor, searchapp.Query{ProjectID: fixture.projectID.String(), Text: "雨夜", Limit: 10})
	if err != nil || scriptResult.Status != search.StatusFresh || len(scriptResult.Hits) != 1 || scriptResult.Hits[0].Evidence[0].Href == "" {
		t.Fatalf("authorized Script search failed: %#v err=%v", scriptResult, err)
	}
	graphResult, err := service.SearchStoryGraph(ctx, actor, searchapp.Query{ProjectID: fixture.projectID.String(), Text: "码头", Limit: 10})
	if err != nil || graphResult.Status != search.StatusFresh || len(graphResult.Hits) != 1 || !strings.Contains(graphResult.Hits[0].OwnerHref, fixture.storyNodeKey) {
		t.Fatalf("authorized StoryGraph search failed: %#v err=%v", graphResult, err)
	}

	secondVersionID := uuid.New()
	secondText := "雨夜追逐后，角色进入仓库。"
	if err = database.Create(&model.EpisodeScriptVersion{
		ID: secondVersionID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, EpisodeID: fixture.episodeID,
		VersionNo: 2, DocumentRevisionID: fixture.revisionID, SourceStart: 0, SourceEnd: len([]rune(secondText)),
		Content: secondText, ContentHash: searchHashText(secondText), Status: "published", CreatedBy: fixture.userID,
		CreatedAt: projectedAt.Add(time.Minute), UpdatedAt: projectedAt.Add(time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.Episode{}).Where("id = ?", fixture.episodeID).Updates(map[string]any{
		"current_script_version_id": secondVersionID, "revision": 2, "updated_at": projectedAt.Add(time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}
	stale, err := service.SearchScripts(ctx, actor, searchapp.Query{ProjectID: fixture.projectID.String(), Text: "雨夜", Limit: 10})
	if err != nil || stale.Status != search.StatusStale || !stale.Stale {
		t.Fatalf("PostgreSQL Owner advance did not expose index staleness: %#v err=%v", stale, err)
	}
	if _, err = service.SearchScripts(ctx, searchapp.Actor{UserID: uuid.NewString(), TokenVersion: 1}, searchapp.Query{
		ProjectID: fixture.projectID.String(), Text: "雨夜", Limit: 10,
	}); err == nil {
		t.Fatal("non-member reached the search index")
	}
}

type searchOwnerFixture struct {
	userID, workspaceID, projectID, documentID, revisionID, episodeID, scriptVersionID uuid.UUID
	storyNodeKey                                                                       string
}

func seedSearchOwners(t *testing.T, create func(any) error) searchOwnerFixture {
	t.Helper()
	now := time.Date(2026, 8, 27, 20, 30, 0, 0, time.UTC)
	fixture := searchOwnerFixture{
		userID: uuid.New(), workspaceID: uuid.New(), projectID: uuid.New(), documentID: uuid.New(),
		revisionID: uuid.New(), episodeID: uuid.New(), scriptVersionID: uuid.New(),
	}
	text := "雨夜码头，角色发现关键线索。"
	for _, value := range []any{
		&model.UserAccount{ID: fixture.userID, EmailNormalized: fixture.userID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Search Owner", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: fixture.workspaceID, Name: "Search", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: fixture.workspaceID, UserID: fixture.userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: fixture.projectID, WorkspaceID: fixture.workspaceID, Name: "Search", AspectRatio: "16:9", Language: "zh-CN", TargetDurationMS: 60_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: fixture.documentID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, Title: "原稿", SourceType: "text", Language: "zh-CN", RightsDeclaration: "owned", Status: "active", Revision: 1, CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: fixture.revisionID, WorkspaceID: fixture.workspaceID, DocumentID: fixture.documentID, VersionNo: 1, SourceType: "text", RawText: text, RawHash: searchHashText(text), NormalizedText: text, NormalizedHash: searchHashText(text), NormalizerVersion: "v1", NormalizationMap: datatypes.JSON([]byte(`{}`)), CodepointCount: len([]rune(text)), AnalysisStatus: "deterministic", AnalyzerVersion: "v1", Blocks: datatypes.JSON([]byte(`[]`)), Issues: datatypes.JSON([]byte(`[]`)), CreatedBy: fixture.userID, CreatedAt: now},
		&model.Episode{ID: fixture.episodeID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, Name: "第一集", Position: 1, TargetDurationMS: 60_000, Status: "active", Revision: 1, CurrentScriptVersionID: &fixture.scriptVersionID, CreatedAt: now, UpdatedAt: now},
		&model.EpisodeScriptVersion{ID: fixture.scriptVersionID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, EpisodeID: fixture.episodeID, VersionNo: 1, DocumentRevisionID: fixture.revisionID, SourceStart: 0, SourceEnd: len([]rune(text)), Content: text, ContentHash: searchHashText(text), Status: "published", CreatedBy: fixture.userID, CreatedAt: now, UpdatedAt: now},
	} {
		if err := create(value); err != nil {
			t.Fatal(err)
		}
	}
	owner := storygraph.OwnerRef{OwnerKind: "production/planning", OwnerLogicalID: fixture.episodeID.String(), OwnerVersionID: fixture.scriptVersionID.String(), OwnerRevision: 1, ContentHash: searchHashText(text)}
	fixture.storyNodeKey, _ = storygraph.DeriveStoryNodeKey(storygraph.NodeTypeScene, owner)
	node := storygraph.Node{
		StoryNodeKey: fixture.storyNodeKey, NodeType: storygraph.NodeTypeScene, OwnerRef: owner, Label: "雨夜码头",
		EvidenceRefs: []storygraph.EvidenceRef{{DocumentRevisionID: fixture.revisionID.String(), AbsoluteStart: 0, AbsoluteEnd: len([]rune(text)), TextHash: searchHashText(text)}},
		Payload:      json.RawMessage(`{"heading":"雨夜码头","summary":"发现关键线索"}`), ContentHash: searchHashText("node"),
	}
	nodes, _ := json.Marshal([]storygraph.Node{node})
	versionID := uuid.New()
	if err := create(&model.StoryGraphVersion{
		ID: versionID, WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, VersionNo: 1,
		SourceRevisionID: fixture.revisionID, SourceRevisionHash: searchHashText(text), OwnerHeadRefs: datatypes.JSON([]byte(`[]`)),
		OwnerSetHash: searchHashText("owners"), SchemaVersion: storygraph.SchemaVersion, Nodes: datatypes.JSON(nodes), Edges: datatypes.JSON([]byte(`[]`)),
		TopologyHash: searchHashText("topology"), ContentHash: searchHashText("graph"), Status: "published", PublishedAt: now, CreatedBy: fixture.userID, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := create(&model.StoryGraphHead{WorkspaceID: fixture.workspaceID, ProjectID: fixture.projectID, CurrentVersionID: versionID, CurrentContentHash: searchHashText("graph"), Revision: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func searchHashText(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
