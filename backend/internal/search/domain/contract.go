package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindScript     Kind = "script"
	KindStoryGraph Kind = "storygraph"

	SourceEvent   = "event"
	SourceReindex = "reindex"

	StatusFresh    = "fresh"
	StatusStale    = "stale"
	StatusDegraded = "degraded"
)

var (
	ErrSnapshotNotFound = errors.New("search Owner snapshot not found")
	hashPattern         = regexp.MustCompile(`^[0-9a-f]{64}$`)
	storyNodePattern    = regexp.MustCompile(`^sgn_[0-9a-f]{64}$`)
)

type Evidence struct {
	DocumentRevisionID string `json:"document_revision_id"`
	Start              int    `json:"start"`
	End                int    `json:"end"`
	TextHash           string `json:"text_hash"`
}

type Document struct {
	ID                  string     `json:"-"`
	Kind                Kind       `json:"document_kind"`
	WorkspaceID         string     `json:"workspace_id"`
	ProjectID           string     `json:"project_id"`
	OwnerKind           string     `json:"owner_kind"`
	OwnerLogicalID      string     `json:"owner_logical_id"`
	OwnerVersionID      string     `json:"owner_version_id"`
	OwnerRevision       int64      `json:"owner_revision"`
	OwnerContentHash    string     `json:"owner_content_hash"`
	ProjectionVersionID string     `json:"projection_version_id"`
	StoryNodeKey        string     `json:"story_node_key,omitempty"`
	NodeType            string     `json:"node_type,omitempty"`
	Label               string     `json:"label"`
	SearchText          string     `json:"search_text"`
	Evidence            []Evidence `json:"evidence"`
}

type Snapshot struct {
	Kind        Kind
	WorkspaceID string
	ProjectID   string
	VersionID   string
	Revision    int64
	ContentHash string
	Documents   []Document
}

func (value Snapshot) Validate() error {
	if !knownKind(value.Kind) || !canonicalUUID(value.WorkspaceID) || !canonicalUUID(value.ProjectID) ||
		!canonicalUUID(value.VersionID) || value.Revision < 0 || !hashPattern.MatchString(value.ContentHash) || value.Documents == nil {
		return errors.New("search snapshot metadata is invalid")
	}
	if value.Kind == KindStoryGraph && value.Revision < 1 {
		return errors.New("StoryGraph search snapshot revision is invalid")
	}
	seen := make(map[string]struct{}, len(value.Documents))
	for _, document := range value.Documents {
		if err := document.validate(value); err != nil {
			return err
		}
		if _, exists := seen[document.ID]; exists {
			return errors.New("search snapshot contains duplicate document id")
		}
		seen[document.ID] = struct{}{}
	}
	return nil
}

func (value Document) validate(snapshot Snapshot) error {
	if strings.TrimSpace(value.ID) == "" || value.Kind != snapshot.Kind || value.WorkspaceID != snapshot.WorkspaceID ||
		value.ProjectID != snapshot.ProjectID || strings.TrimSpace(value.OwnerKind) == "" || strings.TrimSpace(value.OwnerLogicalID) == "" ||
		!canonicalUUID(value.OwnerVersionID) || value.OwnerRevision < 1 || !hashPattern.MatchString(value.OwnerContentHash) ||
		!canonicalUUID(value.ProjectionVersionID) || strings.TrimSpace(value.SearchText) == "" || value.Evidence == nil {
		return errors.New("search document metadata is invalid")
	}
	if value.Kind == KindStoryGraph {
		if !storyNodePattern.MatchString(value.StoryNodeKey) || strings.TrimSpace(value.NodeType) == "" {
			return errors.New("StoryGraph search document trace is invalid")
		}
	} else if value.StoryNodeKey != "" || value.NodeType != "" || len(value.Evidence) == 0 {
		return errors.New("script search document trace is invalid")
	}
	for _, evidence := range value.Evidence {
		if !canonicalUUID(evidence.DocumentRevisionID) || evidence.Start < 0 || evidence.End <= evidence.Start || !hashPattern.MatchString(evidence.TextHash) {
			return errors.New("search document evidence is invalid")
		}
	}
	return nil
}

type ProjectionSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (value ProjectionSource) Validate() error {
	if (value.Kind != SourceEvent && value.Kind != SourceReindex) || !canonicalUUID(value.ID) {
		return errors.New("search projection source is invalid")
	}
	return nil
}

type IndexQuery struct {
	Kind        Kind
	WorkspaceID string
	ProjectID   string
	Text        string
	Limit       int
}

type EvidenceHit struct {
	Evidence
	Href string `json:"href"`
}

type Hit struct {
	Score            float64       `json:"score"`
	Snippet          string        `json:"snippet"`
	OwnerKind        string        `json:"owner_kind"`
	OwnerLogicalID   string        `json:"owner_logical_id"`
	OwnerVersionID   string        `json:"owner_version_id"`
	OwnerRevision    int64         `json:"owner_revision"`
	OwnerContentHash string        `json:"owner_content_hash"`
	StoryNodeKey     string        `json:"story_node_key,omitempty"`
	NodeType         string        `json:"node_type,omitempty"`
	OwnerHref        string        `json:"owner_href"`
	VersionHref      string        `json:"version_href"`
	Evidence         []EvidenceHit `json:"evidence"`
	Document         Document      `json:"-"`
}

type IndexResult struct {
	IndexVersion string
	SnapshotHash string
	Source       ProjectionSource
	IndexedAt    time.Time
	Hits         []Hit
}

type Result struct {
	Kind                 Kind              `json:"kind"`
	Status               string            `json:"status"`
	Stale                bool              `json:"stale"`
	ErrorCode            string            `json:"error_code,omitempty"`
	ExpectedSnapshotHash string            `json:"expected_snapshot_hash"`
	IndexedSnapshotHash  string            `json:"indexed_snapshot_hash"`
	IndexVersion         string            `json:"index_version"`
	Source               *ProjectionSource `json:"source,omitempty"`
	IndexedAt            *time.Time        `json:"indexed_at,omitempty"`
	Hits                 []Hit             `json:"hits"`
}

type ReindexResult struct {
	Kind         Kind   `json:"kind"`
	IndexVersion string `json:"index_version"`
	Alias        string `json:"alias"`
	Documents    int    `json:"documents"`
}

func knownKind(value Kind) bool { return value == KindScript || value == KindStoryGraph }

func canonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}
