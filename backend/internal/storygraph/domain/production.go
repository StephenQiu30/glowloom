package domain

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/google/uuid"
)

const (
	ProductionCoverageP0 = "p0"
	ProductionSchemaRank = 2
)

type OwnerVersionIdentity struct {
	WorkspaceID   string `json:"workspace_id"`
	ProjectID     string `json:"project_id"`
	OwnerKind     string `json:"owner_kind"`
	VersionFamily string `json:"version_family"`
	LogicalID     string `json:"owner_logical_id"`
	VersionID     string `json:"owner_version_id"`
	Revision      int64  `json:"owner_revision"`
	ContentHash   string `json:"owner_content_hash"`
}

type OwnerCollectionRef struct {
	WorkspaceID        string                 `json:"workspace_id"`
	ProjectID          string                 `json:"project_id"`
	OwnerKind          string                 `json:"owner_kind"`
	VersionFamily      string                 `json:"version_family"`
	ScopeKind          string                 `json:"scope_kind"`
	ScopeKey           string                 `json:"scope_key"`
	ScopeRevision      int64                  `json:"scope_revision"`
	ScopeContentHash   string                 `json:"scope_content_hash"`
	Members            []OwnerVersionIdentity `json:"members"`
	MemberCount        int                    `json:"member_count"`
	MembersHash        string                 `json:"members_hash"`
	CollectionRootHash string                 `json:"collection_root_hash"`
}

type ProductionOwnerSnapshot struct {
	Origin                          string                  `json:"origin"`
	WorkspaceID                     string                  `json:"workspace_id"`
	ProjectID                       string                  `json:"project_id"`
	SourceRevisionID                string                  `json:"source_revision_id"`
	SourceRevisionHash              string                  `json:"source_revision_hash"`
	ProductionWorldConfirmationID   string                  `json:"-"`
	ProductionWorldConfirmationHash string                  `json:"-"`
	Coverage                        ProductionCoverageProof `json:"coverage"`
	OwnerCollections                []OwnerCollectionRef    `json:"owner_collections"`
	Graph                           Snapshot                `json:"graph"`
}

type CompiledProductionOwnerSnapshot struct {
	WorkspaceID, ProjectID, SourceRevisionID, SourceRevisionHash string
	SchemaID                                                     string
	SchemaRank                                                   int64
	SchemaManifestHash                                           string
	NodeKeyDerivationID                                          string
	EdgeKeyDerivationID                                          string
	Coverage                                                     ProductionCoverageProof
	OwnerCollections                                             []OwnerCollectionRef
	OwnerHeads                                                   []OwnerHeadRef
	OwnerSetHash                                                 string
	Graph                                                        CanonicalSnapshot
}

type ProductionCompilationInput struct {
	SchemaID                  string                  `json:"schema_id"`
	SchemaRank                int64                   `json:"schema_rank"`
	SchemaManifestHash        string                  `json:"schema_manifest_hash"`
	Coverage                  ProductionCoverageProof `json:"verified_coverage_proof"`
	CoveragePhase             string                  `json:"coverage_phase"`
	CoverageScopeManifestHash string                  `json:"coverage_scope_manifest_hash"`
	NodeKeyDerivationID       string                  `json:"node_key_derivation_id"`
	EdgeKeyDerivationID       string                  `json:"edge_key_derivation_id"`
	OwnerCollections          []OwnerCollectionRef    `json:"exact_owner_collections"`
}

type productionCollectionDefinition struct {
	OwnerKind, ScopeKind string
	AllowEmpty           bool
}

var productionP0Collections = map[string]productionCollectionDefinition{
	"script_source_set":             {OwnerKind: "production/script", ScopeKind: "project"},
	"project_episode_set":           {OwnerKind: "production/project", ScopeKind: "project"},
	"bible_structure_identity_set":  {OwnerKind: "production/bible", ScopeKind: "project"},
	"bible_production_world_set":    {OwnerKind: "production/bible", ScopeKind: "project"},
	"planning_scene_set":            {OwnerKind: "production/planning", ScopeKind: "episode"},
	"planning_structure_rebase_set": {OwnerKind: "production/planning", ScopeKind: "project", AllowEmpty: true},
	"asset_identity_state_set":      {OwnerKind: "asset", ScopeKind: "project"},
}

func BuildOwnerCollectionRef(value OwnerCollectionRef) (OwnerCollectionRef, error) {
	definition, ok := productionP0Collections[value.VersionFamily]
	if !ok || value.OwnerKind != definition.OwnerKind || value.ScopeKind != definition.ScopeKind ||
		strings.TrimSpace(value.ScopeKey) == "" || value.ScopeRevision < 1 ||
		!validProductionScope(value.WorkspaceID, value.ProjectID) || (!definition.AllowEmpty && len(value.Members) == 0) {
		return OwnerCollectionRef{}, errors.New("invalid StoryGraph Owner Collection")
	}
	members := make([]OwnerVersionIdentity, len(value.Members))
	copy(members, value.Members)
	slices.SortFunc(members, func(left, right OwnerVersionIdentity) int {
		return strings.Compare(productionOwnerKey(left), productionOwnerKey(right))
	})
	contractMembers := make([]ownercollection.VersionRef, len(members))
	for index := range members {
		member := members[index]
		if err := validateProductionOwnerIdentity(member, value); err != nil ||
			(index > 0 && productionOwnerKey(members[index-1]) == productionOwnerKey(member)) {
			return OwnerCollectionRef{}, errors.New("invalid StoryGraph Owner Collection member set")
		}
		contractMembers[index] = ownercollection.VersionRef{
			WorkspaceID: member.WorkspaceID, ProjectID: member.ProjectID,
			OwnerKind: member.OwnerKind, VersionFamily: member.VersionFamily,
			OwnerLogicalID: member.LogicalID, OwnerVersionID: member.VersionID,
			OwnerRevision: member.Revision, OwnerContentHash: member.ContentHash,
		}
	}
	contract, err := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily,
		ScopeKind: value.ScopeKind, ScopeKey: value.ScopeKey, ScopeRevision: value.ScopeRevision,
	}, contractMembers)
	if err != nil {
		return OwnerCollectionRef{}, fmt.Errorf("build StoryGraph Owner Collection: %w", err)
	}
	value.Members = members
	value.MemberCount = int(contract.MemberCount)
	value.MembersHash = contract.MembersHash
	value.ScopeContentHash = contract.ScopeContentHash
	value.CollectionRootHash = contract.CollectionRootHash
	return value, nil
}

func CompileProductionOwnerSnapshot(snapshot ProductionOwnerSnapshot) (CompiledProductionOwnerSnapshot, error) {
	if snapshot.Origin != OwnerSnapshotOriginConfirmed || snapshot.Graph.SchemaVersion != ProductionSchemaID ||
		!validProductionScope(snapshot.WorkspaceID, snapshot.ProjectID) ||
		!hashPattern.MatchString(snapshot.ProductionWorldConfirmationHash) ||
		!hashPattern.MatchString(snapshot.SourceRevisionHash) {
		return CompiledProductionOwnerSnapshot{}, errors.New("invalid Production StoryGraph compilation input")
	}
	if _, err := uuid.Parse(snapshot.SourceRevisionID); err != nil {
		return CompiledProductionOwnerSnapshot{}, errors.New("invalid Production StoryGraph source revision")
	}
	if _, err := uuid.Parse(snapshot.ProductionWorldConfirmationID); err != nil {
		return CompiledProductionOwnerSnapshot{}, errors.New("invalid Production World confirmation identity")
	}
	registry, err := currentProductionSchemaIdentity()
	if err != nil {
		return CompiledProductionOwnerSnapshot{}, err
	}

	collections := append([]OwnerCollectionRef(nil), snapshot.OwnerCollections...)
	slices.SortFunc(collections, func(left, right OwnerCollectionRef) int {
		return strings.Compare(productionCollectionKey(left), productionCollectionKey(right))
	})
	families := make(map[string]int, len(productionP0Collections))
	members := make(map[string]OwnerVersionIdentity)
	ownerHeads := make([]OwnerHeadRef, 0)
	sourceFound := false
	for index := range collections {
		collection := collections[index]
		if collection.WorkspaceID != snapshot.WorkspaceID || collection.ProjectID != snapshot.ProjectID ||
			(index > 0 && productionCollectionKey(collections[index-1]) == productionCollectionKey(collection)) {
			return CompiledProductionOwnerSnapshot{}, errors.New("duplicate or cross-project StoryGraph Owner Collection")
		}
		rebuilt, err := BuildOwnerCollectionRef(OwnerCollectionRef{
			WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID, OwnerKind: collection.OwnerKind,
			VersionFamily: collection.VersionFamily, ScopeKind: collection.ScopeKind, ScopeKey: collection.ScopeKey,
			ScopeRevision: collection.ScopeRevision, Members: collection.Members,
		})
		if err != nil || !reflect.DeepEqual(rebuilt, collection) {
			return CompiledProductionOwnerSnapshot{}, errors.New("StoryGraph Owner Collection has drifted")
		}
		families[collection.VersionFamily]++
		for _, member := range collection.Members {
			key := productionOwnerVersionKey(member.OwnerKind, member.LogicalID, member.VersionID)
			if _, exists := members[key]; exists {
				return CompiledProductionOwnerSnapshot{}, errors.New("duplicate StoryGraph Owner Version")
			}
			members[key] = member
			ownerHeads = append(ownerHeads, OwnerHeadRef{
				OwnerKind: member.OwnerKind, VersionFamily: member.VersionFamily,
				OwnerLogicalID: member.LogicalID, OwnerVersionID: member.VersionID,
				OwnerRevision: member.Revision, ContentHash: member.ContentHash,
			})
			if member.VersionID == snapshot.SourceRevisionID && member.ContentHash == snapshot.SourceRevisionHash &&
				member.VersionFamily == "script_source_set" {
				sourceFound = true
			}
		}
	}
	for family := range productionP0Collections {
		count := families[family]
		if count == 0 || (family != "planning_scene_set" && count != 1) {
			return CompiledProductionOwnerSnapshot{}, fmt.Errorf("Production StoryGraph Owner Collection %s is incomplete", family)
		}
	}
	if !sourceFound {
		return CompiledProductionOwnerSnapshot{}, errors.New("source revision is outside Production StoryGraph Owner Collections")
	}
	for _, node := range snapshot.Graph.Nodes {
		member, exists := members[productionOwnerVersionKey(node.OwnerRef.OwnerKind, node.OwnerRef.OwnerLogicalID, node.OwnerRef.OwnerVersionID)]
		if !exists || member.Revision != node.OwnerRef.OwnerRevision || member.ContentHash != node.OwnerRef.ownerContentHash() ||
			node.OwnerRef.WorkspaceID != snapshot.WorkspaceID || node.OwnerRef.ProjectID != snapshot.ProjectID ||
			node.OwnerRef.VersionFamily != member.VersionFamily {
			return CompiledProductionOwnerSnapshot{}, fmt.Errorf("Production StoryGraph node %s is outside the frozen Owner Collections", node.StoryNodeKey)
		}
	}
	if err = ValidateProductionCoverageProof(snapshot.WorkspaceID, snapshot.ProjectID, snapshot.Coverage, collections); err != nil {
		return CompiledProductionOwnerSnapshot{}, err
	}
	heads, _, err := CanonicalOwnerHeadRefs(ownerHeads)
	if err != nil {
		return CompiledProductionOwnerSnapshot{}, err
	}
	graph, err := Canonicalize(snapshot.Graph)
	if err != nil {
		return CompiledProductionOwnerSnapshot{}, err
	}
	ownerSetHash, err := canonicalValueHash(struct {
		SchemaID           string                  `json:"schema_id"`
		SchemaManifestHash string                  `json:"schema_manifest_hash"`
		Coverage           ProductionCoverageProof `json:"verified_coverage_proof"`
		OwnerCollections   []OwnerCollectionRef    `json:"exact_owner_collections"`
	}{ProductionSchemaID, registry.SchemaHash, snapshot.Coverage, collections})
	if err != nil {
		return CompiledProductionOwnerSnapshot{}, err
	}
	graph.TopologyHash, graph.ContentHash, err = productionGraphHashes(graph, registry, snapshot.Coverage, ownerSetHash)
	if err != nil {
		return CompiledProductionOwnerSnapshot{}, err
	}
	return CompiledProductionOwnerSnapshot{
		WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
		SourceRevisionID: snapshot.SourceRevisionID, SourceRevisionHash: snapshot.SourceRevisionHash,
		SchemaID: registry.SchemaID, SchemaRank: ProductionSchemaRank,
		SchemaManifestHash: registry.SchemaHash, NodeKeyDerivationID: registry.NodeKeyDerivationID,
		EdgeKeyDerivationID: registry.EdgeKeyDerivationID,
		Coverage:            snapshot.Coverage, OwnerCollections: collections, OwnerHeads: heads,
		OwnerSetHash: ownerSetHash, Graph: graph,
	}, nil
}

func validateProductionOwnerIdentity(value OwnerVersionIdentity, collection OwnerCollectionRef) error {
	if value.WorkspaceID != collection.WorkspaceID || value.ProjectID != collection.ProjectID ||
		value.VersionFamily != collection.VersionFamily || strings.TrimSpace(value.OwnerKind) == "" ||
		strings.TrimSpace(value.LogicalID) == "" || value.Revision < 1 || !hashPattern.MatchString(value.ContentHash) {
		return errors.New("invalid StoryGraph Owner Version identity")
	}
	if _, err := uuid.Parse(value.VersionID); err != nil {
		return errors.New("invalid StoryGraph Owner Version identity")
	}
	definition := productionP0Collections[collection.VersionFamily]
	if value.OwnerKind != definition.OwnerKind {
		return errors.New("invalid StoryGraph Owner Version kind")
	}
	return nil
}

func validProductionScope(workspaceID, projectID string) bool {
	if _, err := uuid.Parse(workspaceID); err != nil {
		return false
	}
	_, err := uuid.Parse(projectID)
	return err == nil
}

func productionOwnerKey(value OwnerVersionIdentity) string {
	return value.OwnerKind + "\x00" + value.VersionFamily + "\x00" + value.LogicalID + "\x00" + value.VersionID
}

func productionOwnerVersionKey(ownerKind, logicalID, versionID string) string {
	return ownerKind + "\x00" + logicalID + "\x00" + versionID
}

func productionCollectionKey(value OwnerCollectionRef) string {
	return value.OwnerKind + "\x00" + value.VersionFamily + "\x00" + value.ScopeKind + "\x00" + value.ScopeKey
}
