package gormdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) CurrentScriptSnapshot(ctx context.Context, projectID string) (search.Snapshot, error) {
	project, err := store.project(ctx, projectID)
	if err != nil {
		return search.Snapshot{}, err
	}
	var episodes []model.Episode
	if err = store.database.WithContext(ctx).Where("project_id = ? AND status = ?", project.ID, "active").Order("position").Order("id").Find(&episodes).Error; err != nil {
		return search.Snapshot{}, err
	}
	documents := make([]search.Document, 0, len(episodes))
	revision := int64(0)
	for _, episode := range episodes {
		if episode.CurrentScriptVersionID == nil {
			continue
		}
		var version model.EpisodeScriptVersion
		if err = store.database.WithContext(ctx).Where(
			"id = ? AND workspace_id = ? AND project_id = ? AND episode_id = ? AND status = ?",
			*episode.CurrentScriptVersionID, project.WorkspaceID, project.ID, episode.ID, "published",
		).First(&version).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return search.Snapshot{}, errors.New("episode current script pointer does not resolve to a published Owner version")
			}
			return search.Snapshot{}, err
		}
		if int64(version.VersionNo) > revision {
			revision = int64(version.VersionNo)
		}
		documents = append(documents, search.Document{
			ID: "script:" + episode.ID.String(), Kind: search.KindScript,
			WorkspaceID: project.WorkspaceID.String(), ProjectID: project.ID.String(),
			OwnerKind: "production/script", OwnerLogicalID: episode.ID.String(), OwnerVersionID: version.ID.String(),
			OwnerRevision: int64(version.VersionNo), OwnerContentHash: version.ContentHash, ProjectionVersionID: version.ID.String(),
			Label: episode.Name, SearchText: strings.TrimSpace(episode.Name + "\n" + version.Content),
			Evidence: []search.Evidence{{
				DocumentRevisionID: version.DocumentRevisionID.String(), Start: version.SourceStart,
				End: version.SourceEnd, TextHash: version.ContentHash,
			}},
		})
	}
	hash, err := hashScriptDocuments(documents)
	if err != nil {
		return search.Snapshot{}, err
	}
	value := search.Snapshot{
		Kind: search.KindScript, WorkspaceID: project.WorkspaceID.String(), ProjectID: project.ID.String(),
		VersionID: project.ID.String(), Revision: revision, ContentHash: hash, Documents: documents,
	}
	if err = value.Validate(); err != nil {
		return search.Snapshot{}, err
	}
	return value, nil
}

func (store *Store) CurrentStoryGraphSnapshot(ctx context.Context, projectID string) (search.Snapshot, error) {
	project, err := store.project(ctx, projectID)
	if err != nil {
		return search.Snapshot{}, err
	}
	var head model.StoryGraphHead
	if err = store.database.WithContext(ctx).Where("project_id = ? AND workspace_id = ?", project.ID, project.WorkspaceID).First(&head).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return search.Snapshot{}, search.ErrSnapshotNotFound
		}
		return search.Snapshot{}, err
	}
	var version model.StoryGraphVersion
	if err = store.database.WithContext(ctx).Where(
		"id = ? AND workspace_id = ? AND project_id = ? AND status = ?",
		head.CurrentVersionID, project.WorkspaceID, project.ID, "published",
	).First(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return search.Snapshot{}, errors.New("StoryGraph head does not resolve to a published Owner version")
		}
		return search.Snapshot{}, err
	}
	if version.VersionNo != head.Revision || version.ContentHash != head.CurrentContentHash {
		return search.Snapshot{}, errors.New("StoryGraph head and immutable version have drifted")
	}
	var nodes []storygraph.Node
	if err = json.Unmarshal(version.Nodes, &nodes); err != nil {
		return search.Snapshot{}, fmt.Errorf("decode StoryGraph nodes for search: %w", err)
	}
	documents := make([]search.Document, len(nodes))
	for position, node := range nodes {
		evidence := make([]search.Evidence, len(node.EvidenceRefs))
		for evidencePosition, item := range node.EvidenceRefs {
			evidence[evidencePosition] = search.Evidence{
				DocumentRevisionID: item.DocumentRevisionID, Start: item.AbsoluteStart,
				End: item.AbsoluteEnd, TextHash: item.TextHash,
			}
		}
		documents[position] = search.Document{
			ID: "storygraph:" + project.ID.String() + ":" + node.StoryNodeKey, Kind: search.KindStoryGraph,
			WorkspaceID: project.WorkspaceID.String(), ProjectID: project.ID.String(),
			OwnerKind: node.OwnerRef.OwnerKind, OwnerLogicalID: node.OwnerRef.OwnerLogicalID,
			OwnerVersionID: node.OwnerRef.OwnerVersionID, OwnerRevision: node.OwnerRef.OwnerRevision,
			OwnerContentHash: node.OwnerRef.ContentHash, ProjectionVersionID: version.ID.String(),
			StoryNodeKey: node.StoryNodeKey, NodeType: string(node.NodeType), Label: node.Label,
			SearchText: searchableNodeText(node), Evidence: evidence,
		}
	}
	value := search.Snapshot{
		Kind: search.KindStoryGraph, WorkspaceID: project.WorkspaceID.String(), ProjectID: project.ID.String(),
		VersionID: version.ID.String(), Revision: version.VersionNo, ContentHash: version.ContentHash, Documents: documents,
	}
	if err = value.Validate(); err != nil {
		return search.Snapshot{}, err
	}
	return value, nil
}

func (store *Store) AllScriptSnapshots(ctx context.Context) ([]search.Snapshot, error) {
	projects, err := store.projects(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]search.Snapshot, 0, len(projects))
	for _, project := range projects {
		snapshot, snapshotErr := store.CurrentScriptSnapshot(ctx, project.ID.String())
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (store *Store) AllStoryGraphSnapshots(ctx context.Context) ([]search.Snapshot, error) {
	projects, err := store.projects(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]search.Snapshot, 0, len(projects))
	for _, project := range projects {
		snapshot, snapshotErr := store.CurrentStoryGraphSnapshot(ctx, project.ID.String())
		if errors.Is(snapshotErr, search.ErrSnapshotNotFound) {
			continue
		}
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (store *Store) project(ctx context.Context, rawID string) (model.Project, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return model.Project{}, search.ErrSnapshotNotFound
	}
	var value model.Project
	if err = store.database.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Project{}, search.ErrSnapshotNotFound
		}
		return model.Project{}, err
	}
	return value, nil
}

func (store *Store) projects(ctx context.Context) ([]model.Project, error) {
	var values []model.Project
	err := store.database.WithContext(ctx).Where("status = ?", "active").Order("workspace_id").Order("id").Find(&values).Error
	return values, err
}

func hashScriptDocuments(documents []search.Document) (string, error) {
	type ownerHead struct {
		LogicalID, VersionID string
		Revision             int64
		ContentHash          string
	}
	values := make([]ownerHead, len(documents))
	for index, document := range documents {
		values[index] = ownerHead{document.OwnerLogicalID, document.OwnerVersionID, document.OwnerRevision, document.OwnerContentHash}
	}
	sort.Slice(values, func(left, right int) bool { return values[left].LogicalID < values[right].LogicalID })
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func searchableNodeText(node storygraph.Node) string {
	parts := []string{node.Label, string(node.NodeType), node.OwnerRef.OwnerKind, node.OwnerRef.OwnerLogicalID, node.OwnerRef.FragmentKey}
	var payload any
	if len(node.Payload) > 0 && json.Unmarshal(node.Payload, &payload) == nil {
		appendScalars(&parts, payload, 0)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func appendScalars(parts *[]string, value any, depth int) {
	if depth > 12 || len(*parts) > 5000 {
		return
	}
	switch typed := value.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			*parts = append(*parts, trimmed)
		}
	case json.Number:
		*parts = append(*parts, typed.String())
	case float64, bool:
		*parts = append(*parts, fmt.Sprint(typed))
	case []any:
		for _, item := range typed {
			appendScalars(parts, item, depth+1)
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			appendScalars(parts, typed[key], depth+1)
		}
	}
}
