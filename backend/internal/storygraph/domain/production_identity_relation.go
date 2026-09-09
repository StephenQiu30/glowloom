package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

const productionMaximumSafeInteger int64 = 9007199254740991

type productionOwnerNodeRef struct {
	WorkspaceID         string  `json:"workspace_id"`
	ProjectID           string  `json:"project_id"`
	OwnerKind           string  `json:"owner_kind"`
	VersionFamily       string  `json:"version_family"`
	OwnerLogicalID      string  `json:"owner_logical_id"`
	OwnerVersionID      string  `json:"owner_version_id"`
	OwnerRevision       int64   `json:"owner_revision"`
	OwnerContentHash    string  `json:"owner_content_hash"`
	FragmentKey         *string `json:"fragment_key,omitempty"`
	FragmentContentHash *string `json:"fragment_content_hash,omitempty"`
}

type productionProjectionPayload struct {
	PayloadContractID string `json:"payload_contract_id"`
	ProjectionHash    string `json:"projection_hash"`
}

type productionAuditableAssetPayload struct {
	productionProjectionPayload
	AssetKind          string          `json:"asset_kind"`
	CreatorDecisionRef json.RawMessage `json:"creator_decision_ref"`
}

type productionAssetStatePayload struct {
	productionProjectionPayload
	AssetKind          string          `json:"asset_kind"`
	StateKey           string          `json:"state_key"`
	StoryTimeRange     json.RawMessage `json:"story_time_range"`
	CreatorDecisionRef json.RawMessage `json:"creator_decision_ref"`
}

type productionBindingPayload struct {
	productionProjectionPayload
	AssetIdentityRef productionOwnerNodeRef   `json:"asset_identity_ref"`
	SpecificationRef productionOwnerNodeRef   `json:"specification_ref"`
	StateRefs        []productionOwnerNodeRef `json:"state_refs"`
}

type productionOccurrencePayload struct {
	productionProjectionPayload
	AssetIdentityRef   productionOwnerNodeRef `json:"asset_identity_ref"`
	AssetStateRef      productionOwnerNodeRef `json:"asset_state_ref"`
	SceneRef           productionOwnerNodeRef `json:"scene_ref"`
	BeatRef            json.RawMessage        `json:"beat_ref"`
	CreatorDecisionRef json.RawMessage        `json:"creator_decision_ref"`
}

type productionContentAddressedAuditRef struct {
	WorkspaceID      string `json:"workspace_id"`
	ProjectID        string `json:"project_id"`
	AuditOwnerKind   string `json:"audit_owner_kind"`
	AuditID          string `json:"audit_id"`
	AuditRevision    int64  `json:"audit_revision"`
	AuditContentHash string `json:"audit_content_hash"`
}

type productionStoryTimeRange struct {
	StartKey string `json:"start_key"`
	EndKey   string `json:"end_key"`
}

type productionRelationEdge struct {
	edgeType  EdgeType
	from      string
	to        string
	qualifier EdgeQualifier
}

type productionBindingResolution struct {
	assetKey  string
	stateKeys []string
}

func validateProductionIdentityRelations(nodes []Node, edges []Edge) error {
	refIndex := make(map[string][]Node, len(nodes))
	nodeByKey := make(map[string]Node, len(nodes))
	assetKinds := make(map[string]string)
	specificationKinds := make(map[string]string)
	stateKinds := make(map[string]string)
	for _, node := range nodes {
		ref, err := productionOwnerNodeRefFromOwner(node.OwnerRef)
		if err != nil {
			return err
		}
		key, err := ref.identityKey()
		if err != nil {
			return err
		}
		refIndex[key] = append(refIndex[key], node)
		nodeByKey[node.StoryNodeKey] = node
		switch node.NodeType {
		case NodeTypeAssetIdentity, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification:
			var payload productionAuditableAssetPayload
			if err = decodeStrictObject(node.Payload, &payload); err != nil || !productionAssetKind(payload.AssetKind) || validateProductionAuditRef(node, payload.CreatorDecisionRef) != nil {
				return fmt.Errorf("Production StoryGraph node %s has an invalid identity payload", node.StoryNodeKey)
			}
			if node.NodeType == NodeTypeAssetIdentity {
				assetKinds[node.StoryNodeKey] = payload.AssetKind
			} else {
				expectedKind := map[NodeType]string{NodeTypeCharacterSpecification: "character", NodeTypeLocationSpecification: "location", NodeTypePropSpecification: "prop"}[node.NodeType]
				if payload.AssetKind != expectedKind {
					return fmt.Errorf("Production StoryGraph specification %s has an invalid asset kind", node.StoryNodeKey)
				}
				specificationKinds[node.StoryNodeKey] = payload.AssetKind
			}
		case NodeTypeAssetState:
			var payload productionAssetStatePayload
			if err = decodeStrictObject(node.Payload, &payload); err != nil || !productionAssetKind(payload.AssetKind) || !productionStableKey(payload.StateKey) ||
				validateProductionStoryTimeRange(payload.StoryTimeRange) != nil || validateProductionAuditRef(node, payload.CreatorDecisionRef) != nil {
				return fmt.Errorf("Production StoryGraph Asset State %s has an invalid payload", node.StoryNodeKey)
			}
			stateKinds[node.StoryNodeKey] = payload.AssetKind
		}
	}

	expected := make(map[productionRelationEdge]struct{})
	assetBindingCount := make(map[string]int)
	specificationBindingCount := make(map[string]int)
	stateBindingCount := make(map[string]int)
	bindingByState := make(map[string]productionBindingResolution)
	for _, node := range nodes {
		if node.NodeType != NodeTypeProductionBinding {
			continue
		}
		var payload productionBindingPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil || len(payload.StateRefs) == 0 {
			return fmt.Errorf("Production StoryGraph binding %s has an invalid payload", node.StoryNodeKey)
		}
		if err := validateSortedProductionRefs(payload.StateRefs); err != nil {
			return err
		}
		asset, err := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
		if err != nil {
			return fmt.Errorf("Production StoryGraph binding %s asset ref is invalid", node.StoryNodeKey)
		}
		specification, err := resolveProductionNodeRef(refIndex, payload.SpecificationRef, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification)
		if err != nil || assetKinds[asset.StoryNodeKey] != specificationKinds[specification.StoryNodeKey] {
			return fmt.Errorf("Production StoryGraph binding %s specification ref is invalid", node.StoryNodeKey)
		}
		resolution := productionBindingResolution{assetKey: asset.StoryNodeKey}
		expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: asset.StoryNodeKey, to: node.StoryNodeKey, qualifier: EdgeQualifier{BindingRole: "asset"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: specification.StoryNodeKey, to: node.StoryNodeKey, qualifier: EdgeQualifier{BindingRole: "specification"}}] = struct{}{}
		assetBindingCount[asset.StoryNodeKey]++
		specificationBindingCount[specification.StoryNodeKey]++
		expected[productionRelationEdge{edgeType: EdgeTypeDescribesIdentity, from: asset.StoryNodeKey, to: specification.StoryNodeKey}] = struct{}{}
		for _, ref := range payload.StateRefs {
			state, resolveErr := resolveProductionNodeRef(refIndex, ref, NodeTypeAssetState)
			if resolveErr != nil || assetKinds[asset.StoryNodeKey] != stateKinds[state.StoryNodeKey] {
				return fmt.Errorf("Production StoryGraph binding %s state ref is invalid", node.StoryNodeKey)
			}
			resolution.stateKeys = append(resolution.stateKeys, state.StoryNodeKey)
			stateBindingCount[state.StoryNodeKey]++
			if _, exists := bindingByState[state.StoryNodeKey]; exists {
				return errors.New("Production StoryGraph Asset State has multiple Production Bindings")
			}
			bindingByState[state.StoryNodeKey] = resolution
			expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: state.StoryNodeKey, to: node.StoryNodeKey, qualifier: EdgeQualifier{BindingRole: "state"}}] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeHasState, from: asset.StoryNodeKey, to: state.StoryNodeKey}] = struct{}{}
		}
		for _, stateKey := range resolution.stateKeys {
			bindingByState[stateKey] = resolution
		}
	}
	if err := validateProductionBindingCoverage(assetKinds, specificationKinds, stateKinds, assetBindingCount, specificationBindingCount, stateBindingCount); err != nil {
		return err
	}

	for _, node := range nodes {
		if node.NodeType != NodeTypeOccurrence {
			continue
		}
		var payload productionOccurrencePayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil || len(payload.BeatRef) == 0 || validateProductionAuditRef(node, payload.CreatorDecisionRef) != nil {
			return fmt.Errorf("Production StoryGraph Occurrence %s has an invalid payload", node.StoryNodeKey)
		}
		asset, assetErr := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
		state, stateErr := resolveProductionNodeRef(refIndex, payload.AssetStateRef, NodeTypeAssetState)
		scene, sceneErr := resolveProductionNodeRef(refIndex, payload.SceneRef, NodeTypeScene)
		binding, bound := bindingByState[state.StoryNodeKey]
		if assetErr != nil || stateErr != nil || sceneErr != nil || !bound || binding.assetKey != asset.StoryNodeKey {
			return fmt.Errorf("Production StoryGraph Occurrence %s does not resolve one identity binding", node.StoryNodeKey)
		}
		expected[productionRelationEdge{edgeType: EdgeTypeAnchorsOccurrence, from: scene.StoryNodeKey, to: node.StoryNodeKey, qualifier: EdgeQualifier{AnchorRole: "scene"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeInstantiatesOccurrence, from: state.StoryNodeKey, to: node.StoryNodeKey}] = struct{}{}
		if !bytes.Equal(bytes.TrimSpace(payload.BeatRef), []byte("null")) {
			var beatRef productionOwnerNodeRef
			if err := decodeStrictObject(payload.BeatRef, &beatRef); err != nil {
				return fmt.Errorf("Production StoryGraph Occurrence %s beat ref is invalid", node.StoryNodeKey)
			}
			beat, resolveErr := resolveProductionNodeRef(refIndex, beatRef, NodeTypeNarrativeBeat)
			if resolveErr != nil {
				return fmt.Errorf("Production StoryGraph Occurrence %s beat ref is invalid", node.StoryNodeKey)
			}
			expected[productionRelationEdge{edgeType: EdgeTypeAnchorsOccurrence, from: beat.StoryNodeKey, to: node.StoryNodeKey, qualifier: EdgeQualifier{AnchorRole: "beat"}}] = struct{}{}
		}
	}

	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		from, fromOK := nodeByKey[edge.FromNodeKey]
		to, toOK := nodeByKey[edge.ToNodeKey]
		if !fromOK || !toOK || !productionIdentityRelationEdge(edge, from, to) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s identity edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph identity edges do not match binding and occurrence payloads")
	}
	return nil
}

func productionOwnerNodeRefFromOwner(value OwnerRef) (productionOwnerNodeRef, error) {
	ref := productionOwnerNodeRef{
		WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: value.OwnerKind,
		VersionFamily: value.VersionFamily, OwnerLogicalID: value.OwnerLogicalID,
		OwnerVersionID: value.OwnerVersionID, OwnerRevision: value.OwnerRevision, OwnerContentHash: value.OwnerContentHash,
	}
	if value.FragmentKey != "" {
		fragmentKey, fragmentHash := value.FragmentKey, value.FragmentContentHash
		ref.FragmentKey, ref.FragmentContentHash = &fragmentKey, &fragmentHash
	}
	if _, err := ref.identityKey(); err != nil {
		return productionOwnerNodeRef{}, err
	}
	return ref, nil
}

func (value productionOwnerNodeRef) identityKey() (string, error) {
	workspaceID, workspaceErr := uuid.Parse(value.WorkspaceID)
	projectID, projectErr := uuid.Parse(value.ProjectID)
	versionID, versionErr := uuid.Parse(value.OwnerVersionID)
	if workspaceErr != nil || projectErr != nil || versionErr != nil || workspaceID.String() != value.WorkspaceID || projectID.String() != value.ProjectID || versionID.String() != value.OwnerVersionID ||
		!productionStableKey(value.OwnerKind) || !productionStableKey(value.VersionFamily) || !productionStableKey(value.OwnerLogicalID) || value.OwnerRevision < 1 || value.OwnerRevision > productionMaximumSafeInteger ||
		!hashPattern.MatchString(value.OwnerContentHash) || (value.FragmentKey == nil) != (value.FragmentContentHash == nil) {
		return "", errors.New("Production StoryGraph Owner Node ref is invalid")
	}
	if value.FragmentKey != nil && (!productionStableKey(*value.FragmentKey) || !hashPattern.MatchString(*value.FragmentContentHash)) {
		return "", errors.New("Production StoryGraph Owner Node ref is invalid")
	}
	encoded, _ := json.Marshal(value)
	return string(encoded), nil
}

func resolveProductionNodeRef(index map[string][]Node, ref productionOwnerNodeRef, allowed ...NodeType) (Node, error) {
	key, err := ref.identityKey()
	if err != nil {
		return Node{}, err
	}
	var matched []Node
	for _, node := range index[key] {
		if slices.Contains(allowed, node.NodeType) {
			matched = append(matched, node)
		}
	}
	if len(matched) != 1 {
		return Node{}, errors.New("Production StoryGraph Owner Node ref does not resolve exactly once")
	}
	return matched[0], nil
}

func validateSortedProductionRefs(refs []productionOwnerNodeRef) error {
	previous := ""
	for index, ref := range refs {
		if _, err := ref.identityKey(); err != nil {
			return err
		}
		key := ref.sortKey()
		if index > 0 && previous >= key {
			return errors.New("Production StoryGraph Owner Node refs are not sorted and unique")
		}
		previous = key
	}
	return nil
}

func (value productionOwnerNodeRef) sortKey() string {
	fragment := ""
	if value.FragmentKey != nil {
		fragment = *value.FragmentKey
	}
	return strings.Join([]string{value.OwnerKind, value.VersionFamily, value.OwnerLogicalID, fragment, value.OwnerVersionID}, "\x00")
}

func validateProductionAuditRef(node Node, raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("Production StoryGraph creator decision ref is required")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var ref productionContentAddressedAuditRef
	if err := decodeStrictObject(raw, &ref); err != nil {
		return err
	}
	workspaceID, workspaceErr := uuid.Parse(ref.WorkspaceID)
	projectID, projectErr := uuid.Parse(ref.ProjectID)
	if workspaceErr != nil || projectErr != nil || workspaceID.String() != ref.WorkspaceID || projectID.String() != ref.ProjectID || !productionStableKey(ref.AuditID) ||
		ref.WorkspaceID != node.OwnerRef.WorkspaceID || ref.ProjectID != node.OwnerRef.ProjectID || ref.AuditOwnerKind != node.OwnerRef.OwnerKind ||
		ref.AuditRevision < 1 || ref.AuditRevision > productionMaximumSafeInteger || !hashPattern.MatchString(ref.AuditContentHash) {
		return errors.New("Production StoryGraph creator decision ref is invalid")
	}
	return nil
}

func validateProductionStoryTimeRange(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("Production StoryGraph story time range is required")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var value productionStoryTimeRange
	if err := decodeStrictObject(raw, &value); err != nil || !productionStableKey(value.StartKey) || !productionStableKey(value.EndKey) || value.StartKey > value.EndKey {
		return errors.New("Production StoryGraph story time range is invalid")
	}
	return nil
}

func validateProductionBindingCoverage(
	assets, specifications, states map[string]string,
	assetCounts, specificationCounts, stateCounts map[string]int,
) error {
	for key := range assets {
		if assetCounts[key] != 1 {
			return errors.New("Production StoryGraph Asset Identity does not have exactly one Production Binding")
		}
	}
	for key := range specifications {
		if specificationCounts[key] != 1 {
			return errors.New("Production StoryGraph Specification does not have exactly one Production Binding")
		}
	}
	for key := range states {
		if stateCounts[key] != 1 {
			return errors.New("Production StoryGraph Asset State does not have exactly one Production Binding")
		}
	}
	return nil
}

func productionIdentityRelationEdge(edge Edge, from, to Node) bool {
	switch edge.EdgeType {
	case EdgeTypeMaterializes:
		return to.NodeType == NodeTypeProductionBinding
	case EdgeTypeDescribesIdentity, EdgeTypeHasState:
		return true
	case EdgeTypeAnchorsOccurrence, EdgeTypeInstantiatesOccurrence:
		return to.NodeType == NodeTypeOccurrence
	default:
		return false
	}
}

func productionAssetKind(value string) bool {
	return value == "character" || value == "location" || value == "prop"
}

func productionStableKey(value string) bool {
	if strings.TrimSpace(value) == "" || !norm.NFC.IsNormalString(value) {
		return false
	}
	for _, character := range value {
		if character == unicode.MaxASCII || character < ' ' {
			return false
		}
	}
	return true
}
