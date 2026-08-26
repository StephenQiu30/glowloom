package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/google/uuid"

	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

const maximumResponseBytes = 8 << 20

var indexNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,199}$`)

type Config struct {
	Addresses       []string
	Username        string
	Password        string
	ScriptAlias     string
	StoryGraphAlias string
}

type Index struct {
	client          *elasticsearch.BaseClient
	scriptAlias     string
	storyGraphAlias string
}

type storedDocument struct {
	search.Document
	SnapshotHash         string    `json:"snapshot_hash"`
	ProjectionSourceKind string    `json:"projection_source_kind"`
	SourceEventID        string    `json:"source_event_id"`
	IndexedAt            time.Time `json:"indexed_at"`
}

type snapshotMarker struct {
	DocumentKind         string    `json:"document_kind"`
	WorkspaceID          string    `json:"workspace_id"`
	ProjectID            string    `json:"project_id"`
	SnapshotHash         string    `json:"snapshot_hash"`
	Revision             int64     `json:"revision"`
	ProjectionVersionID  string    `json:"projection_version_id"`
	ProjectionSourceKind string    `json:"projection_source_kind"`
	SourceEventID        string    `json:"source_event_id"`
	IndexedAt            time.Time `json:"indexed_at"`
}

func New(config Config) (*Index, error) {
	if len(config.Addresses) == 0 || !indexNamePattern.MatchString(config.ScriptAlias) ||
		!indexNamePattern.MatchString(config.StoryGraphAlias) || config.ScriptAlias == config.StoryGraphAlias {
		return nil, errors.New("Elasticsearch search index configuration is invalid")
	}
	for _, address := range config.Addresses {
		parsed, err := url.Parse(strings.TrimSpace(address))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" {
			return nil, errors.New("Elasticsearch address is invalid")
		}
	}
	client, err := elasticsearch.NewBaseClient(elasticsearch.Config{
		Addresses: config.Addresses, Username: config.Username, Password: config.Password,
		DisableRetry: true, AutoDrainBody: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create Elasticsearch client: %w", err)
	}
	return &Index{client: client, scriptAlias: config.ScriptAlias, storyGraphAlias: config.StoryGraphAlias}, nil
}

func (index *Index) Ping(ctx context.Context) error {
	_, status, err := index.perform(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("Elasticsearch ping returned status %d", status)
	}
	return nil
}

func (index *Index) Ensure(ctx context.Context) error {
	if err := index.Ping(ctx); err != nil {
		return err
	}
	for _, kind := range []search.Kind{search.KindScript, search.KindStoryGraph} {
		alias := index.alias(kind)
		_, status, err := index.perform(ctx, http.MethodGet, "/_alias/"+url.PathEscape(alias), nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK {
			continue
		}
		if status != http.StatusNotFound {
			return fmt.Errorf("inspect Elasticsearch alias %s: status %d", alias, status)
		}
		backing := alias + "-bootstrap"
		if err = index.createBacking(ctx, backing, alias); err != nil {
			return err
		}
	}
	return nil
}

func (index *Index) Project(ctx context.Context, snapshot search.Snapshot, source search.ProjectionSource, at time.Time) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return err
	}
	if at.IsZero() {
		return errors.New("search projection timestamp is required")
	}
	alias := index.alias(snapshot.Kind)
	if snapshot.Kind == search.KindStoryGraph {
		marker, found, err := index.marker(ctx, alias, snapshot.ProjectID)
		if err != nil {
			return err
		}
		if found && marker.Revision > snapshot.Revision {
			return nil
		}
		if found && marker.Revision == snapshot.Revision {
			if marker.SnapshotHash != snapshot.ContentHash {
				return errors.New("StoryGraph search revision has conflicting content")
			}
			return nil
		}
	}
	return index.projectTo(ctx, alias, snapshot, source, at.UTC())
}

func (index *Index) Search(ctx context.Context, query search.IndexQuery) (search.IndexResult, error) {
	if (query.Kind != search.KindScript && query.Kind != search.KindStoryGraph) || strings.TrimSpace(query.Text) == "" ||
		query.Limit < 1 || query.Limit > 50 {
		return search.IndexResult{}, errors.New("Elasticsearch search query is invalid")
	}
	alias := index.alias(query.Kind)
	backing, err := index.resolveAlias(ctx, alias)
	if err != nil {
		return search.IndexResult{}, err
	}
	marker, found, err := index.marker(ctx, alias, query.ProjectID)
	if err != nil {
		return search.IndexResult{}, err
	}
	result := search.IndexResult{IndexVersion: backing, Hits: []search.Hit{}}
	if !found || marker.WorkspaceID != query.WorkspaceID || marker.ProjectID != query.ProjectID {
		return result, nil
	}
	result.SnapshotHash = marker.SnapshotHash
	result.Source = search.ProjectionSource{Kind: marker.ProjectionSourceKind, ID: marker.SourceEventID}
	result.IndexedAt = marker.IndexedAt
	body := map[string]any{
		"size": query.Limit,
		"query": map[string]any{"bool": map[string]any{
			"filter": []any{
				map[string]any{"term": map[string]any{"workspace_id": query.WorkspaceID}},
				map[string]any{"term": map[string]any{"project_id": query.ProjectID}},
				map[string]any{"term": map[string]any{"document_kind": string(query.Kind)}},
				map[string]any{"term": map[string]any{"snapshot_hash": marker.SnapshotHash}},
			},
			"must": map[string]any{"multi_match": map[string]any{
				"query": query.Text, "fields": []string{"label^2", "search_text"}, "type": "best_fields",
			}},
		}},
		"highlight": map[string]any{
			"pre_tags": []string{"<em>"}, "post_tags": []string{"</em>"},
			"encoder": "html",
			"fields":  map[string]any{"search_text": map[string]any{"fragment_size": 240, "number_of_fragments": 1}, "label": map[string]any{}},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return search.IndexResult{}, err
	}
	raw, status, err := index.perform(ctx, http.MethodPost, "/"+url.PathEscape(alias)+"/_search", encoded)
	if err != nil {
		return search.IndexResult{}, err
	}
	if status != http.StatusOK {
		return search.IndexResult{}, responseError("search Elasticsearch index", status, raw)
	}
	var response struct {
		Hits struct {
			Hits []struct {
				Score     float64             `json:"_score"`
				Source    storedDocument      `json:"_source"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return search.IndexResult{}, fmt.Errorf("decode Elasticsearch search response: %w", err)
	}
	result.Hits = make([]search.Hit, len(response.Hits.Hits))
	for position, item := range response.Hits.Hits {
		snippet := firstHighlight(item.Highlight)
		if snippet == "" {
			snippet = truncate(item.Source.SearchText, 240)
		}
		result.Hits[position] = search.Hit{Score: item.Score, Snippet: snippet, Document: item.Source.Document}
	}
	return result, nil
}

func (index *Index) Rebuild(ctx context.Context, kind search.Kind, snapshots []search.Snapshot, source search.ProjectionSource, at time.Time) (result search.ReindexResult, returnErr error) {
	if (kind != search.KindScript && kind != search.KindStoryGraph) || at.IsZero() {
		return search.ReindexResult{}, errors.New("Elasticsearch reindex request is invalid")
	}
	if err := source.Validate(); err != nil || source.Kind != search.SourceReindex {
		return search.ReindexResult{}, errors.New("Elasticsearch reindex source is invalid")
	}
	alias := index.alias(kind)
	backing := alias + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := index.createBacking(ctx, backing, ""); err != nil {
		return search.ReindexResult{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			cleanupContext, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancelCleanup()
			_, _, _ = index.perform(cleanupContext, http.MethodDelete, "/"+url.PathEscape(backing), nil)
		}
	}()
	sorted := append([]search.Snapshot(nil), snapshots...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left].ProjectID < sorted[right].ProjectID })
	documents := 0
	for _, snapshot := range sorted {
		if snapshot.Kind != kind {
			return search.ReindexResult{}, errors.New("Elasticsearch reindex snapshot kind is inconsistent")
		}
		if err := index.projectTo(ctx, backing, snapshot, source, at.UTC()); err != nil {
			return search.ReindexResult{}, err
		}
		documents += len(snapshot.Documents)
	}
	current, err := index.aliasIndices(ctx, alias)
	if err != nil {
		return search.ReindexResult{}, err
	}
	actions := make([]map[string]any, 0, len(current)+1)
	for _, name := range current {
		actions = append(actions, map[string]any{"remove": map[string]any{"index": name, "alias": alias}})
	}
	actions = append(actions, map[string]any{"add": map[string]any{"index": backing, "alias": alias, "is_write_index": true}})
	encoded, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		return search.ReindexResult{}, err
	}
	raw, status, err := index.perform(ctx, http.MethodPost, "/_aliases", encoded)
	if err != nil {
		return search.ReindexResult{}, err
	}
	if status != http.StatusOK {
		return search.ReindexResult{}, responseError("switch Elasticsearch alias", status, raw)
	}
	cleanup = false
	return search.ReindexResult{Kind: kind, IndexVersion: backing, Alias: alias, Documents: documents}, nil
}

func (index *Index) projectTo(ctx context.Context, target string, snapshot search.Snapshot, source search.ProjectionSource, at time.Time) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := source.Validate(); err != nil {
		return err
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	for _, document := range snapshot.Documents {
		identifier := document.ID + ":" + snapshot.ContentHash
		if err := encoder.Encode(map[string]any{"index": map[string]any{"_index": target, "_id": identifier}}); err != nil {
			return err
		}
		if err := encoder.Encode(storedDocument{
			Document: document, SnapshotHash: snapshot.ContentHash, ProjectionSourceKind: source.Kind,
			SourceEventID: source.ID, IndexedAt: at,
		}); err != nil {
			return err
		}
	}
	if payload.Len() > 0 {
		raw, status, err := index.perform(ctx, http.MethodPost, "/_bulk?refresh=wait_for", payload.Bytes())
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return responseError("bulk project Elasticsearch documents", status, raw)
		}
		var response struct {
			Errors bool `json:"errors"`
		}
		if err = json.Unmarshal(raw, &response); err != nil {
			return fmt.Errorf("decode Elasticsearch bulk response: %w", err)
		}
		if response.Errors {
			return errors.New("Elasticsearch bulk projection contained rejected documents")
		}
	}
	marker := snapshotMarker{
		DocumentKind: "_snapshot", WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
		SnapshotHash: snapshot.ContentHash, Revision: snapshot.Revision, ProjectionVersionID: snapshot.VersionID,
		ProjectionSourceKind: source.Kind, SourceEventID: source.ID, IndexedAt: at,
	}
	encoded, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	raw, status, err := index.perform(ctx, http.MethodPut, "/"+url.PathEscape(target)+"/_doc/"+url.PathEscape(markerID(snapshot.ProjectID))+"?refresh=wait_for", encoded)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return responseError("write Elasticsearch snapshot marker", status, raw)
	}
	return nil
}

func (index *Index) marker(ctx context.Context, target, projectID string) (snapshotMarker, bool, error) {
	raw, status, err := index.perform(ctx, http.MethodGet, "/"+url.PathEscape(target)+"/_doc/"+url.PathEscape(markerID(projectID)), nil)
	if err != nil {
		return snapshotMarker{}, false, err
	}
	if status == http.StatusNotFound {
		return snapshotMarker{}, false, nil
	}
	if status != http.StatusOK {
		return snapshotMarker{}, false, responseError("read Elasticsearch snapshot marker", status, raw)
	}
	var response struct {
		Source snapshotMarker `json:"_source"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return snapshotMarker{}, false, fmt.Errorf("decode Elasticsearch snapshot marker: %w", err)
	}
	return response.Source, true, nil
}

func (index *Index) createBacking(ctx context.Context, backing, alias string) error {
	body := map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": map[string]any{
			"dynamic": "strict",
			"properties": map[string]any{
				"document_kind": map[string]any{"type": "keyword"}, "workspace_id": map[string]any{"type": "keyword"},
				"project_id": map[string]any{"type": "keyword"}, "owner_kind": map[string]any{"type": "keyword"},
				"owner_logical_id": map[string]any{"type": "keyword"}, "owner_version_id": map[string]any{"type": "keyword"},
				"owner_revision": map[string]any{"type": "long"}, "owner_content_hash": map[string]any{"type": "keyword"},
				"projection_version_id": map[string]any{"type": "keyword"}, "story_node_key": map[string]any{"type": "keyword"},
				"node_type": map[string]any{"type": "keyword"}, "label": map[string]any{"type": "text"},
				"search_text": map[string]any{"type": "text"}, "snapshot_hash": map[string]any{"type": "keyword"},
				"revision": map[string]any{"type": "long"}, "projection_source_kind": map[string]any{"type": "keyword"},
				"source_event_id": map[string]any{"type": "keyword"}, "indexed_at": map[string]any{"type": "date"},
				"evidence": map[string]any{"type": "object", "dynamic": "strict", "properties": map[string]any{
					"document_revision_id": map[string]any{"type": "keyword"}, "start": map[string]any{"type": "integer"},
					"end": map[string]any{"type": "integer"}, "text_hash": map[string]any{"type": "keyword"},
				}},
			},
		},
	}
	if alias != "" {
		body["aliases"] = map[string]any{alias: map[string]any{"is_write_index": true}}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	raw, status, err := index.perform(ctx, http.MethodPut, "/"+url.PathEscape(backing), encoded)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return responseError("create Elasticsearch backing index", status, raw)
	}
	return nil
}

func (index *Index) resolveAlias(ctx context.Context, alias string) (string, error) {
	indices, err := index.aliasIndices(ctx, alias)
	if err != nil {
		return "", err
	}
	if len(indices) != 1 {
		return "", errors.New("Elasticsearch search alias must resolve to exactly one backing index")
	}
	return indices[0], nil
}

func (index *Index) aliasIndices(ctx context.Context, alias string) ([]string, error) {
	raw, status, err := index.perform(ctx, http.MethodGet, "/_alias/"+url.PathEscape(alias), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return []string{}, nil
	}
	if status != http.StatusOK {
		return nil, responseError("resolve Elasticsearch alias", status, raw)
	}
	var values map[string]json.RawMessage
	if err = json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode Elasticsearch alias: %w", err)
	}
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func (index *Index) alias(kind search.Kind) string {
	if kind == search.KindScript {
		return index.scriptAlias
	}
	return index.storyGraphAlias
}

func (index *Index) perform(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	if index == nil || index.client == nil {
		return nil, 0, errors.New("Elasticsearch client is not configured")
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		if strings.Contains(path, "_bulk") {
			request.Header.Set("Content-Type", "application/x-ndjson")
		} else {
			request.Header.Set("Content-Type", "application/json")
		}
	}
	response, err := index.client.Perform(request)
	if err != nil {
		return nil, 0, fmt.Errorf("perform Elasticsearch request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("read Elasticsearch response: %w", err)
	}
	if len(raw) > maximumResponseBytes {
		return nil, 0, errors.New("Elasticsearch response exceeds size limit")
	}
	return raw, response.StatusCode, nil
}

func responseError(operation string, status int, raw []byte) error {
	message := strings.TrimSpace(string(raw))
	if len(message) > 500 {
		message = message[:500]
	}
	return fmt.Errorf("%s: status %d: %s", operation, status, message)
}

func markerID(projectID string) string { return "_snapshot:" + projectID }

func firstHighlight(values map[string][]string) string {
	for _, field := range []string{"label", "search_text"} {
		if len(values[field]) > 0 {
			return values[field][0]
		}
	}
	return ""
}

func truncate(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}
