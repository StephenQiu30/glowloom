package search_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	searches "github.com/StephenQiu30/lanverse/backend/internal/search/adapter/elasticsearch"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

func TestRealElasticsearchProjectsFencesAndAtomicallyReindexesBothAliases(t *testing.T) {
	address := os.Getenv("LANVERSE_TEST_ELASTICSEARCH_URL")
	if address == "" {
		t.Skip("set LANVERSE_TEST_ELASTICSEARCH_URL to run the real Elasticsearch journey")
	}
	prefix := "lanverse-test-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	username := os.Getenv("LANVERSE_TEST_ELASTICSEARCH_USERNAME")
	password := os.Getenv("LANVERSE_TEST_ELASTICSEARCH_PASSWORD")
	if (username == "") != (password == "") {
		t.Fatal("LANVERSE_TEST_ELASTICSEARCH_USERNAME and LANVERSE_TEST_ELASTICSEARCH_PASSWORD must be configured together")
	}
	index, err := searches.New(searches.Config{
		Addresses: []string{address}, Username: username, Password: password,
		ScriptAlias: prefix + "-script-v1", StoryGraphAlias: prefix + "-storygraph-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = index.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		request, requestErr := http.NewRequest(http.MethodDelete, strings.TrimRight(address, "/")+"/"+prefix+"-*", nil)
		if requestErr == nil {
			if username != "" {
				request.SetBasicAuth(username, password)
			}
			_, _ = http.DefaultClient.Do(request)
		}
	})

	script := scriptSnapshot()
	storyGraph := storyGraphSnapshot()
	now := time.Date(2026, 8, 27, 19, 0, 0, 0, time.UTC)
	if err = index.Project(ctx, script, search.ProjectionSource{Kind: search.SourceEvent, ID: uuid.NewString()}, now); err != nil {
		t.Fatal(err)
	}
	storySource := search.ProjectionSource{Kind: search.SourceEvent, ID: uuid.NewString()}
	if err = index.Project(ctx, storyGraph, storySource, now); err != nil {
		t.Fatal(err)
	}
	result, err := index.Search(ctx, search.IndexQuery{Kind: search.KindStoryGraph, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID, Text: "雨夜", Limit: 10})
	if err != nil || len(result.Hits) != 1 || result.SnapshotHash != storyGraph.ContentHash || result.Source.ID != storySource.ID || result.IndexVersion == "" {
		t.Fatalf("real StoryGraph search did not preserve projection metadata: %#v err=%v", result, err)
	}
	crossTenant, err := index.Search(ctx, search.IndexQuery{Kind: search.KindStoryGraph, WorkspaceID: uuid.NewString(), ProjectID: searchProjectID, Text: "雨夜", Limit: 10})
	if err != nil || len(crossTenant.Hits) != 0 {
		t.Fatalf("workspace filter leaked a search hit: %#v err=%v", crossTenant, err)
	}

	older := storyGraph
	older.Revision = storyGraph.Revision - 1
	older.ContentHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	older.Documents = append([]search.Document(nil), storyGraph.Documents...)
	older.Documents[0].Label = "旧快照"
	older.Documents[0].SearchText = "旧快照"
	if err = index.Project(ctx, older, search.ProjectionSource{Kind: search.SourceEvent, ID: uuid.NewString()}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	stillCurrent, err := index.Search(ctx, search.IndexQuery{Kind: search.KindStoryGraph, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID, Text: "雨夜", Limit: 10})
	if err != nil || len(stillCurrent.Hits) != 1 || stillCurrent.SnapshotHash != storyGraph.ContentHash {
		t.Fatalf("older StoryGraph revision overwrote current projection: %#v err=%v", stillCurrent, err)
	}

	before := result.IndexVersion
	storyGraph.Documents[0].Label = "码头重建"
	storyGraph.Documents[0].SearchText = "码头重建"
	storyGraph.ContentHash = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	reindexed, err := index.Rebuild(ctx, search.KindStoryGraph, []search.Snapshot{storyGraph}, search.ProjectionSource{Kind: search.SourceReindex, ID: uuid.NewString()}, now.Add(2*time.Minute))
	if err != nil || reindexed.IndexVersion == before || reindexed.Alias == "" {
		t.Fatalf("real Elasticsearch alias was not switched: %#v err=%v", reindexed, err)
	}
	after, err := index.Search(ctx, search.IndexQuery{Kind: search.KindStoryGraph, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID, Text: "码头", Limit: 10})
	if err != nil || len(after.Hits) != 1 || after.IndexVersion != reindexed.IndexVersion {
		t.Fatalf("reindexed alias did not serve the rebuilt snapshot: %#v err=%v", after, err)
	}
}
