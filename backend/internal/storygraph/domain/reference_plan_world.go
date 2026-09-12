package domain

import (
	"errors"
	"slices"
)

// ReferencePlanWorldReadSet freezes the current Production StoryGraph identity
// and the full-project P1 planning inventory. The Candidate may mark targets as
// not_generated, but it cannot remove an actual Scene from the planning scope.
type ReferencePlanWorldReadSet struct {
	WorkspaceID           string                     `json:"workspace_id"`
	ProjectID             string                     `json:"project_id"`
	StoryGraphVersionID   string                     `json:"storygraph_version_id"`
	StoryGraphContentHash string                     `json:"storygraph_content_hash"`
	Inventory             ReferencePlanSeedInventory `json:"inventory"`
}

func BuildReferencePlanWorldReadSet(version Version) (ReferencePlanWorldReadSet, error) {
	if ValidateProductionVersion(version) != nil || version.ProductionInput == nil {
		return ReferencePlanWorldReadSet{}, errors.New("Reference Plan requires a valid Production StoryGraph")
	}
	scopeSet := make(map[string]struct{})
	for _, node := range version.Nodes {
		if node.NodeType == NodeTypeScene {
			scopeSet[node.OwnerRef.OwnerLogicalID] = struct{}{}
		}
	}
	if len(scopeSet) == 0 {
		return ReferencePlanWorldReadSet{}, errors.New("Reference Plan requires at least one actual Scene")
	}
	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	slices.Sort(scopes)
	inventory, err := BuildReferencePlanSeedInventory(ReferencePlanSeedInventoryInput{
		OwnerSetHash: version.OwnerSetHash,
		P1ScopeKeys:  scopes,
		Graph: Snapshot{
			SchemaVersion: version.SchemaVersion,
			Nodes:         version.Nodes,
			Edges:         version.Edges,
		},
	})
	if err != nil {
		return ReferencePlanWorldReadSet{}, err
	}
	return ReferencePlanWorldReadSet{
		WorkspaceID: version.WorkspaceID, ProjectID: version.ProjectID,
		StoryGraphVersionID: version.ID, StoryGraphContentHash: version.ContentHash,
		Inventory: inventory,
	}, nil
}
