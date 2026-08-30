package search_test

import (
	"context"
	"errors"
	"testing"
	"time"

	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	searchapp "github.com/StephenQiu30/lanverse/backend/internal/search/application"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

func TestSearchServiceAuthorizesBeforeQueryAndReportsFreshness(t *testing.T) {
	snapshot := scriptSnapshot()
	authorizer := &searchAuthorizer{project: projectdomain.Project{ID: searchProjectID, WorkspaceID: searchWorkspaceID}}
	reader := &searchSnapshotReader{script: snapshot}
	index := &searchIndex{result: search.IndexResult{
		IndexVersion: "lanverse-script-search", SnapshotHash: snapshot.ContentHash,
		IndexedAt: time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC), Hits: []search.Hit{{
			Score: 4.2, Document: snapshot.Documents[0], Snippet: "<em>雨夜</em>追逐",
		}},
	}}
	service := searchapp.NewService(authorizer, reader, index)
	result, err := service.SearchScripts(context.Background(), searchapp.Actor{UserID: searchVersionID, TokenVersion: 3}, searchapp.Query{
		ProjectID: searchProjectID, Text: "雨夜", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorizer.calls != 1 || reader.calls != 1 || index.calls != 1 || result.Status != search.StatusFresh || result.Stale || len(result.Hits) != 1 {
		t.Fatalf("unexpected authorized search result: %#v authorizer=%d reader=%d index=%d", result, authorizer.calls, reader.calls, index.calls)
	}
	if result.Hits[0].OwnerHref == "" || result.Hits[0].VersionHref == "" || result.Hits[0].Evidence[0].Href == "" {
		t.Fatalf("search hit did not expose Owner/Version/Evidence deep links: %#v", result.Hits[0])
	}

	index.result.SnapshotHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	stale, err := service.SearchScripts(context.Background(), searchapp.Actor{UserID: searchVersionID, TokenVersion: 3}, searchapp.Query{
		ProjectID: searchProjectID, Text: "雨夜", Limit: 10,
	})
	if err != nil || stale.Status != search.StatusStale || !stale.Stale {
		t.Fatalf("index lag was not explicit: %#v err=%v", stale, err)
	}
}

func TestSearchServiceReturnsDegradedWithoutWeakeningOwnerAuthorization(t *testing.T) {
	authorizer := &searchAuthorizer{project: projectdomain.Project{ID: searchProjectID, WorkspaceID: searchWorkspaceID}}
	reader := &searchSnapshotReader{storygraph: storyGraphSnapshot()}
	index := &searchIndex{err: errors.New("elastic unavailable")}
	service := searchapp.NewService(authorizer, reader, index)
	result, err := service.SearchStoryGraph(context.Background(), searchapp.Actor{UserID: searchVersionID, TokenVersion: 3}, searchapp.Query{
		ProjectID: searchProjectID, Text: "码头", Limit: 10,
	})
	if err != nil || result.Status != search.StatusDegraded || !result.Stale || result.ErrorCode != "search_unavailable" || len(result.Hits) != 0 {
		t.Fatalf("Elasticsearch outage was not returned as degraded: %#v err=%v", result, err)
	}

	authorizer.err = errors.New("forbidden")
	index.calls = 0
	if _, err = service.SearchStoryGraph(context.Background(), searchapp.Actor{}, searchapp.Query{ProjectID: searchProjectID, Text: "码头", Limit: 10}); err == nil || index.calls != 0 {
		t.Fatalf("authorization failure reached Elasticsearch: err=%v calls=%d", err, index.calls)
	}
}

func TestSearchServiceRejectsDslShapedOrUnboundedInput(t *testing.T) {
	service := searchapp.NewService(&searchAuthorizer{}, &searchSnapshotReader{}, &searchIndex{})
	for _, query := range []searchapp.Query{
		{ProjectID: searchProjectID, Text: `{\"query\":{\"match_all\":{}}}`, Limit: 10},
		{ProjectID: searchProjectID, Text: "rain", Limit: 0},
		{ProjectID: searchProjectID, Text: "rain", Limit: 51},
	} {
		if _, err := service.SearchScripts(context.Background(), searchapp.Actor{}, query); err == nil {
			t.Fatalf("unsafe or unbounded query was accepted: %#v", query)
		}
	}
}

type searchAuthorizer struct {
	project projectdomain.Project
	err     error
	calls   int
}

func (value *searchAuthorizer) Get(context.Context, searchapp.Actor, string) (projectdomain.Project, error) {
	value.calls++
	return value.project, value.err
}

type searchSnapshotReader struct {
	script, storygraph search.Snapshot
	err                error
	calls              int
}

func (value *searchSnapshotReader) CurrentScriptSnapshot(context.Context, string) (search.Snapshot, error) {
	value.calls++
	return value.script, value.err
}

func (value *searchSnapshotReader) CurrentStoryGraphSnapshot(context.Context, string) (search.Snapshot, error) {
	value.calls++
	return value.storygraph, value.err
}

func (*searchSnapshotReader) AllScriptSnapshots(context.Context) ([]search.Snapshot, error) {
	return nil, nil
}

func (*searchSnapshotReader) AllStoryGraphSnapshots(context.Context) ([]search.Snapshot, error) {
	return nil, nil
}

type searchIndex struct {
	result search.IndexResult
	err    error
	calls  int
}

func (value *searchIndex) Search(context.Context, search.IndexQuery) (search.IndexResult, error) {
	value.calls++
	return value.result, value.err
}

func (*searchIndex) Project(context.Context, search.Snapshot, search.ProjectionSource, time.Time) error {
	return nil
}

func (*searchIndex) Rebuild(context.Context, search.Kind, []search.Snapshot, search.ProjectionSource, time.Time) (search.ReindexResult, error) {
	return search.ReindexResult{}, nil
}

func scriptSnapshot() search.Snapshot {
	return search.Snapshot{
		Kind: search.KindScript, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID,
		VersionID: searchVersionID, Revision: 2, ContentHash: searchHash,
		Documents: []search.Document{{
			ID: "script:episode-1", Kind: search.KindScript, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID,
			OwnerKind: "production/script", OwnerLogicalID: "episode-1", OwnerVersionID: searchVersionID,
			OwnerRevision: 2, OwnerContentHash: searchHash, ProjectionVersionID: searchVersionID,
			Label: "第一集", SearchText: "雨夜追逐", Evidence: []search.Evidence{{
				DocumentRevisionID: searchVersionID, Start: 0, End: 4, TextHash: searchHash,
			}},
		}},
	}
}

func storyGraphSnapshot() search.Snapshot {
	value := scriptSnapshot()
	value.Kind = search.KindStoryGraph
	value.Documents[0].Kind = search.KindStoryGraph
	value.Documents[0].StoryNodeKey = "sgn_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	value.Documents[0].NodeType = "scene"
	return value
}
