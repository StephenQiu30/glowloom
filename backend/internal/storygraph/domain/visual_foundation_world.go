package domain

import (
	"errors"
	"slices"
	"strings"
)

// VisualFoundationWorldRoot is the exact confirmed P0 collection identity that
// a visual stage may consume. It deliberately omits StoryGraph nodes and layout.
type VisualFoundationWorldRoot struct {
	OwnerFamily        string `json:"owner_family"`
	ScopeKey           string `json:"scope_key"`
	CollectionRootHash string `json:"collection_root_hash"`
}

// VisualFoundationWorldReadSet is an internal Backend proof. The StoryGraph
// identity is retained for stale-read validation and is not an Agent input.
type VisualFoundationWorldReadSet struct {
	WorkspaceID                 string                      `json:"workspace_id"`
	ProjectID                   string                      `json:"project_id"`
	StoryGraphVersionID         string                      `json:"storygraph_version_id"`
	StoryGraphContentHash       string                      `json:"storygraph_content_hash"`
	ProductionWorldOwnerSetHash string                      `json:"production_world_owner_set_hash"`
	ConfirmedWorldRoots         []VisualFoundationWorldRoot `json:"confirmed_world_roots"`
}

func BuildVisualFoundationWorldReadSet(version Version) (VisualFoundationWorldReadSet, error) {
	if ValidateProductionVersion(version) != nil || version.ProductionInput == nil {
		return VisualFoundationWorldReadSet{}, errors.New("Visual Foundation requires a valid Production StoryGraph")
	}
	roots := make([]VisualFoundationWorldRoot, 0)
	assetRoots, bibleRoots, planningRoots := 0, 0, 0
	projectScope := "project:" + version.ProjectID
	for _, collection := range version.ProductionInput.OwnerCollections {
		switch collection.VersionFamily {
		case "asset_identity_state_set":
			if collection.OwnerKind != "asset" || collection.ScopeKind != "project" || collection.ScopeKey != projectScope {
				return VisualFoundationWorldReadSet{}, errors.New("Visual Foundation Asset world root is invalid")
			}
			assetRoots++
		case "bible_production_world_set":
			if collection.OwnerKind != "production/bible" || collection.ScopeKind != "project" || collection.ScopeKey != projectScope {
				return VisualFoundationWorldReadSet{}, errors.New("Visual Foundation Bible world root is invalid")
			}
			bibleRoots++
		case "planning_scene_set":
			if collection.OwnerKind != "production/planning" || collection.ScopeKind != "episode" || !strings.HasPrefix(collection.ScopeKey, "episode:") {
				return VisualFoundationWorldReadSet{}, errors.New("Visual Foundation Planning world root is invalid")
			}
			planningRoots++
		default:
			continue
		}
		roots = append(roots, VisualFoundationWorldRoot{
			OwnerFamily: collection.VersionFamily, ScopeKey: collection.ScopeKey,
			CollectionRootHash: collection.CollectionRootHash,
		})
	}
	if assetRoots != 1 || bibleRoots != 1 || planningRoots < 1 {
		return VisualFoundationWorldReadSet{}, errors.New("Visual Foundation confirmed world roots are incomplete")
	}
	slices.SortFunc(roots, func(left, right VisualFoundationWorldRoot) int {
		return strings.Compare(
			left.OwnerFamily+"\x00"+left.ScopeKey+"\x00"+left.CollectionRootHash,
			right.OwnerFamily+"\x00"+right.ScopeKey+"\x00"+right.CollectionRootHash,
		)
	})
	return VisualFoundationWorldReadSet{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		StoryGraphVersionID: version.ID, StoryGraphContentHash: version.ContentHash,
		ProductionWorldOwnerSetHash: version.OwnerSetHash, ConfirmedWorldRoots: roots,
	}, nil
}
