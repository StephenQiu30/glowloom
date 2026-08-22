package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
)

type Document struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	ProjectID     string `json:"project_id"`
	RevisionID    string `json:"revision_id"`
	Visibility    string `json:"visibility"`
	Publication   string `json:"publication"`
	RightBasis    string `json:"right_basis"`
	NodeType      string `json:"node_type"`
	ContentUnitID string `json:"content_unit_id"`
	Text          string `json:"text"`
	Anchor        any    `json:"anchor"`
}

type SearchResult struct {
	Documents []Document
	Total     int64
	Stale     bool
}

// SearchIndex is a derived data-plane adapter. It never writes PostgreSQL facts and
// every query requires workspace/project and approved/private filters.
type SearchIndex struct {
	client *elasticsearch.Client
	name   string
}

func NewSearchIndex() (*SearchIndex, error) {
	endpoint := os.Getenv("ELASTICSEARCH_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:9200"
	}
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{endpoint}, Username: os.Getenv("ELASTICSEARCH_USERNAME"), Password: os.Getenv("ELASTICSEARCH_PASSWORD")})
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch client: %w", err)
	}
	name := os.Getenv("ELASTICSEARCH_NARRATIVE_INDEX")
	if name == "" {
		name = "lanverse-narrative-approved"
	}
	return &SearchIndex{client: client, name: name}, nil
}

func (i *SearchIndex) Index(ctx context.Context, document Document) error {
	payload, err := json.Marshal(document)
	if err != nil {
		return err
	}
	response, err := i.client.Index(i.name, bytes.NewReader(payload), i.client.Index.WithContext(ctx), i.client.Index.WithDocumentID(document.ID), i.client.Index.WithRefresh("false"))
	if err != nil {
		return fmt.Errorf("index narrative document: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		return fmt.Errorf("index narrative document: %s", response.String())
	}
	return nil
}

func (i *SearchIndex) Search(ctx context.Context, workspaceID, projectID, query string, limit int) (SearchResult, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(projectID) == "" {
		return SearchResult{}, fmt.Errorf("workspace and project filters are required")
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	body := map[string]any{"size": limit, "query": map[string]any{"bool": map[string]any{"filter": []any{
		map[string]any{"term": map[string]any{"workspace_id": workspaceID}},
		map[string]any{"term": map[string]any{"project_id": projectID}},
		map[string]any{"term": map[string]any{"visibility": "private"}},
		map[string]any{"term": map[string]any{"publication": "approved"}},
	}, "must": []any{map[string]any{"match": map[string]any{"text": query}}}}}}
	payload, _ := json.Marshal(body)
	response, err := i.client.Search(i.client.Search.WithContext(ctx), i.client.Search.WithIndex(i.name), i.client.Search.WithBody(bytes.NewReader(payload)))
	if err != nil {
		return SearchResult{}, fmt.Errorf("search narrative: %w", err)
	}
	defer response.Body.Close()
	if response.IsError() {
		return SearchResult{}, fmt.Errorf("search narrative: %s", response.String())
	}
	var decoded struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source Document `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return SearchResult{}, fmt.Errorf("decode narrative search: %w", err)
	}
	result := SearchResult{Total: decoded.Hits.Total.Value}
	for _, hit := range decoded.Hits.Hits {
		result.Documents = append(result.Documents, hit.Source)
	}
	return result, nil
}
