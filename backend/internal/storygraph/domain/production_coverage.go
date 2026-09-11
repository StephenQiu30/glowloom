package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

func DecodeProductionCompilationInput(raw []byte) (ProductionCompilationInput, error) {
	var value ProductionCompilationInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ProductionCompilationInput{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ProductionCompilationInput{}, errors.New("multiple Production compilation input values are not allowed")
	}
	return value, nil
}

func ValidateProductionVersion(version Version) error {
	if version.SchemaVersion != ProductionSchemaID || version.ProductionInput == nil ||
		!validProductionScope(version.WorkspaceID, version.ProjectID) ||
		!hashPattern.MatchString(version.SourceRevisionHash) {
		return errors.New("invalid persisted Production StoryGraph Version")
	}
	input := *version.ProductionInput
	registry, err := BuildProductionSchemaRegistry()
	if err != nil {
		return err
	}
	if input.SchemaID != registry.Manifest.SchemaID || input.SchemaRank != ProductionSchemaRank ||
		input.SchemaManifestHash != registry.SchemaHash || input.CoveragePhase != input.Coverage.CoveragePhase ||
		input.CoverageScopeManifestHash != input.Coverage.CoverageScopeManifestHash ||
		input.NodeKeyDerivationID != registry.Manifest.NodeKeyDerivationID ||
		input.EdgeKeyDerivationID != registry.Manifest.EdgeKeyDerivationID {
		return errors.New("persisted Production Schema or coverage identity has drifted")
	}
	collections := append([]OwnerCollectionRef(nil), input.OwnerCollections...)
	slices.SortFunc(collections, func(left, right OwnerCollectionRef) int {
		return strings.Compare(productionCollectionKey(left), productionCollectionKey(right))
	})
	if !reflect.DeepEqual(collections, input.OwnerCollections) ||
		ValidateProductionCoverageProof(version.WorkspaceID, version.ProjectID, input.Coverage, collections) != nil {
		return errors.New("persisted Production coverage preimage has drifted")
	}
	ownerSetHash, err := canonicalValueHash(struct {
		SchemaID           string                  `json:"schema_id"`
		SchemaManifestHash string                  `json:"schema_manifest_hash"`
		Coverage           ProductionCoverageProof `json:"verified_coverage_proof"`
		OwnerCollections   []OwnerCollectionRef    `json:"exact_owner_collections"`
	}{input.SchemaID, input.SchemaManifestHash, input.Coverage, collections})
	if err != nil || ownerSetHash != version.OwnerSetHash {
		return errors.New("persisted Production Owner Set hash has drifted")
	}
	ownerHeads := make([]OwnerHeadRef, 0)
	sourceFound := false
	seenOwners := make(map[string]struct{})
	for _, collection := range collections {
		for _, member := range collection.Members {
			key := productionOwnerVersionKey(member.OwnerKind, member.LogicalID, member.VersionID)
			if _, exists := seenOwners[key]; exists {
				return errors.New("duplicate persisted Production Owner Version")
			}
			seenOwners[key] = struct{}{}
			ownerHeads = append(ownerHeads, OwnerHeadRef{
				OwnerKind: member.OwnerKind, VersionFamily: member.VersionFamily,
				OwnerLogicalID: member.LogicalID, OwnerVersionID: member.VersionID,
				OwnerRevision: member.Revision, ContentHash: member.ContentHash,
			})
			if member.VersionFamily == "script_source_set" && member.VersionID == version.SourceRevisionID &&
				member.ContentHash == version.SourceRevisionHash {
				sourceFound = true
			}
		}
	}
	canonicalHeads, _, err := CanonicalOwnerHeadRefs(ownerHeads)
	if err != nil || !sourceFound || !reflect.DeepEqual(canonicalHeads, version.OwnerHeads) {
		return errors.New("persisted Production Owner Head refs have drifted")
	}
	graph, err := Canonicalize(Snapshot{SchemaVersion: version.SchemaVersion, Nodes: version.Nodes, Edges: version.Edges})
	if err != nil {
		return errors.New("persisted Production Graph is invalid")
	}
	canonicalNodeHash, canonicalNodeErr := canonicalValueHash(graph.Nodes)
	persistedNodeHash, persistedNodeErr := canonicalValueHash(version.Nodes)
	if canonicalNodeErr != nil || persistedNodeErr != nil || canonicalNodeHash != persistedNodeHash {
		return errors.New("persisted Production Graph nodes are not canonical")
	}
	canonicalEdgeHash, canonicalEdgeErr := canonicalValueHash(graph.Edges)
	persistedEdgeHash, persistedEdgeErr := canonicalValueHash(version.Edges)
	if canonicalEdgeErr != nil || persistedEdgeErr != nil || canonicalEdgeHash != persistedEdgeHash {
		return errors.New("persisted Production Graph edges are not canonical")
	}
	topologyHash, contentHash, err := productionGraphHashes(graph, registry, input.Coverage, ownerSetHash)
	if err != nil || topologyHash != version.TopologyHash || contentHash != version.ContentHash {
		return errors.New("persisted Production Graph hash has drifted")
	}
	return nil
}

type ProductionCoverageScopeSets struct {
	P0ScopeKeys []string `json:"p0_scope_keys"`
	P1ScopeKeys []string `json:"p1_scope_keys"`
	P2ScopeKeys []string `json:"p2_scope_keys"`
	P3ScopeKeys []string `json:"p3_scope_keys"`
}

type ProductionOwnerApplyReceiptRef struct {
	DecisionCheckpointID        string                      `json:"decision_checkpoint_id"`
	ReceiptScopeKey             string                      `json:"receipt_scope_key"`
	CoveredScopeKeys            []string                    `json:"covered_scope_keys"`
	OwnerKind                   string                      `json:"owner_kind"`
	VersionFamily               string                      `json:"version_family"`
	CommittedCollectionRootHash string                      `json:"committed_collection_root_hash"`
	CommittedOwnerVersionRef    *ownercollection.VersionRef `json:"committed_owner_version_ref"`
	ReceiptContentHash          string                      `json:"receipt_content_hash"`
}

type ProductionCoverageProof struct {
	CoveragePhase             string                           `json:"coverage_phase"`
	CoverageScopeSets         ProductionCoverageScopeSets      `json:"coverage_scope_sets"`
	OwnerApplyReceiptRefs     []ProductionOwnerApplyReceiptRef `json:"owner_apply_receipt_refs"`
	CollectionRootHashes      []string                         `json:"collection_root_hashes"`
	ScopeRebase               any                              `json:"scope_rebase"`
	CoverageScopeManifestHash string                           `json:"coverage_scope_manifest_hash"`
}

type ProductionCoverageProofInput struct {
	WorkspaceID           string
	ProjectID             string
	CoverageScopeSets     ProductionCoverageScopeSets
	OwnerApplyReceiptRefs []ProductionOwnerApplyReceiptRef
	OwnerCollections      []OwnerCollectionRef
}

func BuildProductionCoverageProof(input ProductionCoverageProofInput) (ProductionCoverageProof, error) {
	if !validProductionScope(input.WorkspaceID, input.ProjectID) {
		return ProductionCoverageProof{}, errors.New("invalid Production coverage scope")
	}
	scopeSets := ProductionCoverageScopeSets{
		P0ScopeKeys: canonicalProductionSceneScopes(input.CoverageScopeSets.P0ScopeKeys),
		P1ScopeKeys: canonicalProductionSceneScopes(input.CoverageScopeSets.P1ScopeKeys),
		P2ScopeKeys: canonicalProductionSceneScopes(input.CoverageScopeSets.P2ScopeKeys),
		P3ScopeKeys: canonicalProductionSceneScopes(input.CoverageScopeSets.P3ScopeKeys),
	}
	if len(scopeSets.P0ScopeKeys) == 0 || len(scopeSets.P1ScopeKeys) != 0 ||
		len(scopeSets.P2ScopeKeys) != 0 || len(scopeSets.P3ScopeKeys) != 0 ||
		!validProductionSceneScopes(scopeSets.P0ScopeKeys) {
		return ProductionCoverageProof{}, errors.New("invalid P0 Production coverage scope set")
	}

	collections, collectionByReceiptKey, roots, err := validateProductionCoverageCollections(
		input.WorkspaceID,
		input.ProjectID,
		input.OwnerCollections,
	)
	if err != nil {
		return ProductionCoverageProof{}, err
	}
	receipts, err := validateProductionCoverageReceipts(
		input.ProjectID,
		scopeSets.P0ScopeKeys,
		input.OwnerApplyReceiptRefs,
		collections,
		collectionByReceiptKey,
	)
	if err != nil {
		return ProductionCoverageProof{}, err
	}
	proof := ProductionCoverageProof{
		CoveragePhase: ProductionCoverageP0, CoverageScopeSets: scopeSets,
		OwnerApplyReceiptRefs: receipts, CollectionRootHashes: roots, ScopeRebase: nil,
	}
	proof.CoverageScopeManifestHash, err = canonicalValueHash(struct {
		WorkspaceID           string                           `json:"workspace_id"`
		ProjectID             string                           `json:"project_id"`
		CoveragePhase         string                           `json:"coverage_phase"`
		CoverageScopeSets     ProductionCoverageScopeSets      `json:"coverage_scope_sets"`
		OwnerApplyReceiptRefs []ProductionOwnerApplyReceiptRef `json:"owner_apply_receipt_refs"`
		CollectionRootHashes  []string                         `json:"collection_root_hashes"`
		ScopeRebase           any                              `json:"scope_rebase"`
	}{
		input.WorkspaceID, input.ProjectID, proof.CoveragePhase, proof.CoverageScopeSets,
		proof.OwnerApplyReceiptRefs, proof.CollectionRootHashes, proof.ScopeRebase,
	})
	return proof, err
}

func ValidateProductionCoverageProof(
	workspaceID string,
	projectID string,
	proof ProductionCoverageProof,
	collections []OwnerCollectionRef,
) error {
	if proof.CoveragePhase != ProductionCoverageP0 || proof.ScopeRebase != nil ||
		!hashPattern.MatchString(proof.CoverageScopeManifestHash) {
		return errors.New("invalid Production coverage proof")
	}
	rebuilt, err := BuildProductionCoverageProof(ProductionCoverageProofInput{
		WorkspaceID: workspaceID, ProjectID: projectID,
		CoverageScopeSets: proof.CoverageScopeSets, OwnerApplyReceiptRefs: proof.OwnerApplyReceiptRefs,
		OwnerCollections: collections,
	})
	if err != nil || !reflect.DeepEqual(rebuilt, proof) {
		return errors.New("Production coverage proof has drifted")
	}
	return nil
}

func validateProductionCoverageCollections(
	workspaceID string,
	projectID string,
	values []OwnerCollectionRef,
) ([]OwnerCollectionRef, map[string]OwnerCollectionRef, []string, error) {
	collections := append([]OwnerCollectionRef(nil), values...)
	slices.SortFunc(collections, func(left, right OwnerCollectionRef) int {
		return strings.Compare(productionCollectionKey(left), productionCollectionKey(right))
	})
	families := make(map[string]int, len(productionP0Collections))
	byReceiptKey := make(map[string]OwnerCollectionRef, len(collections))
	roots := make([]string, 0, len(collections))
	for index, collection := range collections {
		if collection.WorkspaceID != workspaceID || collection.ProjectID != projectID ||
			(index > 0 && productionCollectionKey(collections[index-1]) == productionCollectionKey(collection)) {
			return nil, nil, nil, errors.New("invalid Production coverage Owner Collection set")
		}
		rebuilt, err := BuildOwnerCollectionRef(OwnerCollectionRef{
			WorkspaceID: collection.WorkspaceID, ProjectID: collection.ProjectID,
			OwnerKind: collection.OwnerKind, VersionFamily: collection.VersionFamily,
			ScopeKind: collection.ScopeKind, ScopeKey: collection.ScopeKey,
			ScopeRevision: collection.ScopeRevision, Members: collection.Members,
		})
		if err != nil || !reflect.DeepEqual(rebuilt, collection) {
			return nil, nil, nil, errors.New("Production coverage Owner Collection has drifted")
		}
		families[collection.VersionFamily]++
		key := productionCoverageCollectionKey(collection.OwnerKind, collection.VersionFamily, collection.CollectionRootHash)
		if _, exists := byReceiptKey[key]; exists {
			return nil, nil, nil, errors.New("duplicate Production coverage Collection Root")
		}
		byReceiptKey[key] = collection
		roots = append(roots, collection.CollectionRootHash)
	}
	for family := range productionP0Collections {
		count := families[family]
		if count == 0 || (family != "planning_scene_set" && count != 1) {
			return nil, nil, nil, errors.New("incomplete Production coverage Owner Collection set")
		}
	}
	slices.Sort(roots)
	if len(slices.Compact(roots)) != len(collections) {
		return nil, nil, nil, errors.New("duplicate Production coverage Collection Root")
	}
	return collections, byReceiptKey, roots, nil
}

func validateProductionCoverageReceipts(
	projectID string,
	p0Scopes []string,
	values []ProductionOwnerApplyReceiptRef,
	collections []OwnerCollectionRef,
	collectionByReceiptKey map[string]OwnerCollectionRef,
) ([]ProductionOwnerApplyReceiptRef, error) {
	receipts := append([]ProductionOwnerApplyReceiptRef(nil), values...)
	for index := range receipts {
		receipts[index].CoveredScopeKeys = canonicalProductionSceneScopes(receipts[index].CoveredScopeKeys)
	}
	slices.SortFunc(receipts, func(left, right ProductionOwnerApplyReceiptRef) int {
		return strings.Compare(productionCoverageReceiptKey(left, true), productionCoverageReceiptKey(right, true))
	})
	receiptCollections := make(map[string]struct{}, len(receipts))
	gateOneScopes := make(map[string]struct{}, len(p0Scopes))
	gateTwoScopes := make(map[string]struct{}, len(p0Scopes))
	allowed := make(map[string]struct{})
	for _, definition := range productionCheckpointDefinitions() {
		if definition.DecisionCheckpointID == "source_revision_accepted" ||
			definition.DecisionCheckpointID == "project_episode_lifecycle_confirmed" ||
			definition.DecisionCheckpointID == "gate_1_structure_identity" ||
			definition.DecisionCheckpointID == "gate_2_bible_continuity" {
			allowed[definition.DecisionCheckpointID+"\x00"+definition.OwnerKind+"\x00"+definition.VersionFamily] = struct{}{}
		}
	}
	projectScope := "project:" + projectID
	seenCoverageKeys := make(map[string]struct{}, len(receipts))
	for index, receipt := range receipts {
		if index > 0 && productionCoverageReceiptKey(receipts[index-1], true) == productionCoverageReceiptKey(receipt, true) {
			return nil, errors.New("duplicate Production Owner Apply Receipt ref")
		}
		if receipt.ReceiptScopeKey != projectScope || !hashPattern.MatchString(receipt.CommittedCollectionRootHash) ||
			!hashPattern.MatchString(receipt.ReceiptContentHash) || !validProductionSceneScopes(receipt.CoveredScopeKeys) {
			return nil, errors.New("invalid Production Owner Apply Receipt ref")
		}
		if _, exists := allowed[receipt.DecisionCheckpointID+"\x00"+receipt.OwnerKind+"\x00"+receipt.VersionFamily]; !exists {
			return nil, errors.New("Production Owner Apply Receipt checkpoint is not authorized")
		}
		coverageKey := productionCoverageReceiptKey(receipt, false)
		if _, exists := seenCoverageKeys[coverageKey]; exists {
			return nil, errors.New("duplicate Production Owner Apply coverage")
		}
		seenCoverageKeys[coverageKey] = struct{}{}
		collectionKey := productionCoverageCollectionKey(receipt.OwnerKind, receipt.VersionFamily, receipt.CommittedCollectionRootHash)
		collection, exists := collectionByReceiptKey[collectionKey]
		if !exists || receipt.VersionFamily == "planning_structure_rebase_set" || len(collection.Members) == 0 ||
			receipt.CommittedOwnerVersionRef == nil || !productionCollectionContains(collection, *receipt.CommittedOwnerVersionRef) {
			return nil, errors.New("Production Owner Apply Receipt does not prove its Collection")
		}
		receiptCollections[collectionKey] = struct{}{}
		switch receipt.DecisionCheckpointID {
		case "source_revision_accepted", "project_episode_lifecycle_confirmed":
			if len(receipt.CoveredScopeKeys) != 0 {
				return nil, errors.New("source or Episode receipt exceeds its Production coverage authority")
			}
		case "gate_1_structure_identity":
			if len(receipt.CoveredScopeKeys) == 0 {
				return nil, errors.New("Gate 1 Production coverage is empty")
			}
			addProductionScopes(gateOneScopes, receipt.CoveredScopeKeys)
		case "gate_2_bible_continuity":
			if len(receipt.CoveredScopeKeys) == 0 {
				return nil, errors.New("Gate 2 Production coverage is empty")
			}
			addProductionScopes(gateTwoScopes, receipt.CoveredScopeKeys)
		}
	}
	for _, collection := range collections {
		key := productionCoverageCollectionKey(collection.OwnerKind, collection.VersionFamily, collection.CollectionRootHash)
		if collection.VersionFamily == "planning_structure_rebase_set" {
			if len(collection.Members) != 0 {
				return nil, errors.New("P0 Production Structure Rebase Collection is not empty")
			}
			if _, exists := receiptCollections[key]; exists {
				return nil, errors.New("empty P0 Production Structure Rebase Collection has a receipt")
			}
			continue
		}
		if _, exists := receiptCollections[key]; !exists {
			return nil, errors.New("Production Owner Collection is missing its Apply Receipt")
		}
	}
	if !productionScopeUnionEquals(gateOneScopes, p0Scopes) || !productionScopeUnionEquals(gateTwoScopes, p0Scopes) {
		return nil, errors.New("Production Gate coverage does not equal the P0 scope set")
	}
	return receipts, nil
}

func canonicalProductionSceneScopes(values []string) []string {
	result := append([]string(nil), values...)
	slices.Sort(result)
	return slices.Compact(result)
}

func validProductionSceneScopes(values []string) bool {
	if !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if !strings.HasPrefix(value, "scene:") || len(value) == len("scene:") ||
			(index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func productionCoverageCollectionKey(ownerKind, family, root string) string {
	return ownerKind + "\x00" + family + "\x00" + root
}

func productionCoverageReceiptKey(value ProductionOwnerApplyReceiptRef, includeReceiptHash bool) string {
	key := value.DecisionCheckpointID + "\x00" + value.ReceiptScopeKey + "\x00" + value.OwnerKind + "\x00" +
		value.VersionFamily + "\x00" + value.CommittedCollectionRootHash
	if includeReceiptHash {
		key += "\x00" + value.ReceiptContentHash
	}
	return key
}

func productionCollectionContains(collection OwnerCollectionRef, value ownercollection.VersionRef) bool {
	for _, member := range collection.Members {
		if value.WorkspaceID == member.WorkspaceID && value.ProjectID == member.ProjectID &&
			value.OwnerKind == member.OwnerKind && value.VersionFamily == member.VersionFamily &&
			value.OwnerLogicalID == member.LogicalID && value.OwnerVersionID == member.VersionID &&
			value.OwnerRevision == member.Revision && value.OwnerContentHash == member.ContentHash {
			return true
		}
	}
	return false
}

func addProductionScopes(target map[string]struct{}, values []string) {
	for _, value := range values {
		target[value] = struct{}{}
	}
}

func productionScopeUnionEquals(actual map[string]struct{}, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, value := range expected {
		if _, exists := actual[value]; !exists {
			return false
		}
	}
	return true
}

func productionGraphHashes(
	graph CanonicalSnapshot,
	registry ProductionSchemaRegistry,
	coverage ProductionCoverageProof,
	ownerSetHash string,
) (string, string, error) {
	topologyNodes := make([]struct {
		StoryNodeKey string   `json:"story_node_key"`
		NodeType     NodeType `json:"node_type"`
	}, len(graph.Nodes))
	for index, node := range graph.Nodes {
		topologyNodes[index].StoryNodeKey, topologyNodes[index].NodeType = node.StoryNodeKey, node.NodeType
	}
	topologyEdges := make([]struct {
		EdgeKey     string        `json:"edge_key"`
		EdgeType    EdgeType      `json:"edge_type"`
		FromNodeKey string        `json:"from_node_key"`
		ToNodeKey   string        `json:"to_node_key"`
		Qualifier   EdgeQualifier `json:"qualifier"`
	}, len(graph.Edges))
	for index, edge := range graph.Edges {
		topologyEdges[index].EdgeKey, topologyEdges[index].EdgeType = edge.EdgeKey, edge.EdgeType
		topologyEdges[index].FromNodeKey, topologyEdges[index].ToNodeKey = edge.FromNodeKey, edge.ToNodeKey
		topologyEdges[index].Qualifier = edge.Qualifier
	}
	topologyHash, err := canonicalValueHash(struct {
		SchemaID                  string `json:"schema_id"`
		SchemaManifestHash        string `json:"schema_manifest_hash"`
		CoveragePhase             string `json:"coverage_phase"`
		CoverageScopeManifestHash string `json:"coverage_scope_manifest_hash"`
		NodeKeyDerivationID       string `json:"node_key_derivation_id"`
		EdgeKeyDerivationID       string `json:"edge_key_derivation_id"`
		Nodes                     any    `json:"nodes"`
		Edges                     any    `json:"edges"`
	}{
		registry.Manifest.SchemaID, registry.SchemaHash, coverage.CoveragePhase,
		coverage.CoverageScopeManifestHash, registry.Manifest.NodeKeyDerivationID,
		registry.Manifest.EdgeKeyDerivationID, topologyNodes, topologyEdges,
	})
	if err != nil {
		return "", "", err
	}
	contentHash, err := canonicalValueHash(struct {
		SchemaID                  string `json:"schema_id"`
		SchemaManifestHash        string `json:"schema_manifest_hash"`
		CoveragePhase             string `json:"coverage_phase"`
		CoverageScopeManifestHash string `json:"coverage_scope_manifest_hash"`
		NodeKeyDerivationID       string `json:"node_key_derivation_id"`
		EdgeKeyDerivationID       string `json:"edge_key_derivation_id"`
		OwnerSetHash              string `json:"owner_set_hash"`
		Nodes                     []Node `json:"nodes"`
		Edges                     []Edge `json:"edges"`
	}{
		registry.Manifest.SchemaID, registry.SchemaHash, coverage.CoveragePhase,
		coverage.CoverageScopeManifestHash, registry.Manifest.NodeKeyDerivationID,
		registry.Manifest.EdgeKeyDerivationID, ownerSetHash, graph.Nodes, graph.Edges,
	})
	return topologyHash, contentHash, err
}
