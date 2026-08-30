package application

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	projectdomain "github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

const searchTimeout = 3 * time.Second

type Actor struct {
	UserID       string
	TokenVersion int
}

type Query struct {
	ProjectID string
	Text      string
	Limit     int
}

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (value *Error) Error() string { return value.Message }

type ProjectAuthorizer interface {
	Get(context.Context, Actor, string) (projectdomain.Project, error)
}

type SnapshotReader interface {
	CurrentScriptSnapshot(context.Context, string) (search.Snapshot, error)
	CurrentStoryGraphSnapshot(context.Context, string) (search.Snapshot, error)
	AllScriptSnapshots(context.Context) ([]search.Snapshot, error)
	AllStoryGraphSnapshots(context.Context) ([]search.Snapshot, error)
}

type Index interface {
	Search(context.Context, search.IndexQuery) (search.IndexResult, error)
	Project(context.Context, search.Snapshot, search.ProjectionSource, time.Time) error
	Rebuild(context.Context, search.Kind, []search.Snapshot, search.ProjectionSource, time.Time) (search.ReindexResult, error)
}

type Service struct {
	authorizer ProjectAuthorizer
	snapshots  SnapshotReader
	index      Index
}

func NewService(authorizer ProjectAuthorizer, snapshots SnapshotReader, index Index) *Service {
	return &Service{authorizer: authorizer, snapshots: snapshots, index: index}
}

func (service *Service) SearchScripts(ctx context.Context, actor Actor, query Query) (search.Result, error) {
	return service.search(ctx, actor, query, search.KindScript)
}

func (service *Service) SearchStoryGraph(ctx context.Context, actor Actor, query Query) (search.Result, error) {
	return service.search(ctx, actor, query, search.KindStoryGraph)
}

func (service *Service) search(ctx context.Context, actor Actor, query Query, kind search.Kind) (search.Result, error) {
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	query.Text = strings.TrimSpace(query.Text)
	if _, err := uuid.Parse(query.ProjectID); err != nil || query.Text == "" || len([]rune(query.Text)) > 200 ||
		query.Limit < 1 || query.Limit > 50 || looksLikeDSL(query.Text) {
		return search.Result{}, invalid()
	}
	project, err := service.authorizer.Get(ctx, actor, query.ProjectID)
	if err != nil {
		return search.Result{}, err
	}
	var snapshot search.Snapshot
	if kind == search.KindScript {
		snapshot, err = service.snapshots.CurrentScriptSnapshot(ctx, project.ID)
	} else {
		snapshot, err = service.snapshots.CurrentStoryGraphSnapshot(ctx, project.ID)
	}
	if err != nil {
		if errors.Is(err, search.ErrSnapshotNotFound) {
			return search.Result{}, &Error{Code: "not_found", Message: "Search Owner snapshot not found", Status: 404}
		}
		return search.Result{}, err
	}
	searchContext, cancelSearch := context.WithTimeout(ctx, searchTimeout)
	defer cancelSearch()
	indexed, err := service.index.Search(searchContext, search.IndexQuery{
		Kind: kind, WorkspaceID: project.WorkspaceID, ProjectID: project.ID, Text: query.Text, Limit: query.Limit,
	})
	if err != nil {
		return search.Result{
			Kind: kind, Status: search.StatusDegraded, Stale: true, ErrorCode: "search_unavailable",
			ExpectedSnapshotHash: snapshot.ContentHash, Hits: []search.Hit{},
		}, nil
	}
	status := search.StatusFresh
	stale := indexed.SnapshotHash != snapshot.ContentHash
	if stale {
		status = search.StatusStale
	}
	hits := make([]search.Hit, len(indexed.Hits))
	for index, hit := range indexed.Hits {
		hits[index] = deepLinks(kind, project.ID, hit)
	}
	return search.Result{
		Kind: kind, Status: status, Stale: stale,
		ExpectedSnapshotHash: snapshot.ContentHash, IndexedSnapshotHash: indexed.SnapshotHash,
		IndexVersion: indexed.IndexVersion, Source: &indexed.Source, IndexedAt: &indexed.IndexedAt, Hits: hits,
	}, nil
}

func looksLikeDSL(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func invalid() error {
	return &Error{Code: "validation_failed", Message: "Request validation failed", Status: 422}
}

func deepLinks(kind search.Kind, projectID string, hit search.Hit) search.Hit {
	document := hit.Document
	hit.OwnerKind, hit.OwnerLogicalID, hit.OwnerVersionID = document.OwnerKind, document.OwnerLogicalID, document.OwnerVersionID
	hit.OwnerRevision, hit.OwnerContentHash = document.OwnerRevision, document.OwnerContentHash
	hit.StoryNodeKey, hit.NodeType = document.StoryNodeKey, document.NodeType
	if kind == search.KindScript {
		hit.OwnerHref = "/api/episodes/" + url.PathEscape(document.OwnerLogicalID)
		hit.VersionHref = hit.OwnerHref
	} else {
		hit.OwnerHref = "/api/projects/" + url.PathEscape(projectID) + "/storygraph/versions/" + url.PathEscape(document.ProjectionVersionID)
		hit.VersionHref = hit.OwnerHref
		if document.StoryNodeKey != "" {
			hit.OwnerHref += "/nodes/" + url.PathEscape(document.StoryNodeKey) + "/trace?direction=upstream&depth=1&limit=20"
		}
	}
	hit.Evidence = make([]search.EvidenceHit, len(document.Evidence))
	for index, evidence := range document.Evidence {
		hit.Evidence[index] = search.EvidenceHit{Evidence: evidence,
			Href: "/api/document-revisions/" + url.PathEscape(evidence.DocumentRevisionID) + "#range=" + integer(evidence.Start) + ":" + integer(evidence.End),
		}
	}
	return hit
}

func integer(value int) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	buffer := make([]byte, 0, 20)
	for value > 0 {
		buffer = append(buffer, digits[value%10])
		value /= 10
	}
	for left, right := 0, len(buffer)-1; left < right; left, right = left+1, right-1 {
		buffer[left], buffer[right] = buffer[right], buffer[left]
	}
	return string(buffer)
}

func IsApplicationError(err error) bool {
	var value *Error
	return errors.As(err, &value)
}
