package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type productionConstraintRefs struct {
	WorldRuleRefs             []productionOwnerNodeRef `json:"world_rule_refs"`
	PolicySnapshotRef         productionOwnerNodeRef   `json:"policy_snapshot_ref"`
	EffectiveStyleSnapshotRef productionOwnerNodeRef   `json:"effective_style_snapshot_ref"`
}

type productionAssetVersionPayload struct {
	productionProjectionPayload
	Purpose                       string                   `json:"purpose"`
	AssetIdentityRef              productionOwnerNodeRef   `json:"asset_identity_ref"`
	SpecificationRef              productionOwnerNodeRef   `json:"specification_ref"`
	AssetStateRef                 productionOwnerNodeRef   `json:"asset_state_ref"`
	IdentityAnchorAssetVersionRef json.RawMessage          `json:"identity_anchor_asset_version_ref"`
	SelectedArtifactRef           productionOwnerNodeRef   `json:"selected_artifact_ref"`
	FulfilledReferenceTargetRef   productionOwnerNodeRef   `json:"fulfilled_reference_target_ref"`
	Constraints                   productionConstraintRefs `json:"constraints"`
}

type productionAssetVersionResolution struct {
	node             Node
	payload          productionAssetVersionPayload
	assetKey         string
	specificationKey string
	stateKey         string
	artifactKey      string
	targetKey        string
	policyKey        string
	styleKey         string
	worldKeys        []string
	anchorKey        string
	anchorRef        *productionOwnerNodeRef
	target           productionReferenceTargetPayload
}

func validateProductionAssetVersionRelations(nodes []Node, edges []Edge) error {
	refIndex := make(map[string][]Node, len(nodes))
	nodeByKey := make(map[string]Node, len(nodes))
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
	}

	bindingTriples, err := productionBindingTriples(nodes, refIndex)
	if err != nil {
		return err
	}
	resolutions := make(map[string]productionAssetVersionResolution)
	expected := make(map[productionRelationEdge]struct{})
	for _, node := range nodes {
		if node.NodeType != NodeTypeAssetVersion {
			continue
		}
		resolution, resolveErr := resolveProductionAssetVersion(node, refIndex, bindingTriples)
		if resolveErr != nil {
			return resolveErr
		}
		resolutions[node.StoryNodeKey] = resolution
		addProductionAssetVersionEdges(expected, resolution)
	}
	for _, resolution := range resolutions {
		if err := validateProductionAssetVersionAnchor(resolution, resolutions, refIndex); err != nil {
			return err
		}
	}

	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		to, ok := nodeByKey[edge.ToNodeKey]
		if !ok || to.NodeType != NodeTypeAssetVersion || !oneOfEdge(edge.EdgeType, EdgeTypeMaterializes, EdgeTypeFulfillsReferenceTarget, EdgeTypeConstrains) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s AssetVersion edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph AssetVersion edges do not match payloads")
	}
	return nil
}

func resolveProductionAssetVersion(
	node Node,
	refIndex map[string][]Node,
	bindingTriples map[string]struct{},
) (productionAssetVersionResolution, error) {
	var payload productionAssetVersionPayload
	if err := decodeStrictObject(node.Payload, &payload); err != nil || !productionAssetVersionPurpose(payload.Purpose) {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s has an invalid payload", node.StoryNodeKey)
	}
	asset, assetErr := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
	specification, specificationErr := resolveProductionNodeRef(refIndex, payload.SpecificationRef, productionAssetVersionSpecification(payload.Purpose))
	state, stateErr := resolveProductionNodeRef(refIndex, payload.AssetStateRef, NodeTypeAssetState)
	artifact, artifactErr := resolveProductionNodeRef(refIndex, payload.SelectedArtifactRef, NodeTypeArtifact)
	target, targetErr := resolveProductionNodeRef(refIndex, payload.FulfilledReferenceTargetRef, NodeTypeReferencePlanTarget)
	if assetErr != nil || specificationErr != nil || stateErr != nil || artifactErr != nil || targetErr != nil ||
		artifact.OwnerRef.VersionFamily != "asset_base_reference_set" ||
		!productionBindingTripleExists(bindingTriples, asset.StoryNodeKey, specification.StoryNodeKey, state.StoryNodeKey) {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s does not resolve one production binding, artifact and target", node.StoryNodeKey)
	}

	constraints, constraintErr := resolveProductionConstraints(payload.Constraints, refIndex)
	if constraintErr != nil {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s has invalid constraints", node.StoryNodeKey)
	}
	var targetInput productionReferenceTargetPayload
	if err := decodeStrictObject(target.Payload, &targetInput); err != nil || targetInput.TargetKind != payload.Purpose || targetInput.Fulfillment == "not_generated" ||
		!oneOf(targetInput.Fulfillment, "required", "optional") ||
		!productionSingleRefEqual(targetInput.TargetOwnerRefs.Identity, payload.AssetIdentityRef) ||
		!productionSingleRefEqual(targetInput.TargetOwnerRefs.Specification, payload.SpecificationRef) ||
		!productionSingleRefEqual(targetInput.TargetOwnerRefs.State, payload.AssetStateRef) ||
		!productionConstraintsEqual(targetInput.Constraints, payload.Constraints) {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s does not match its Reference Target", node.StoryNodeKey)
	}

	resolution := productionAssetVersionResolution{
		node: node, payload: payload, assetKey: asset.StoryNodeKey, specificationKey: specification.StoryNodeKey,
		stateKey: state.StoryNodeKey, artifactKey: artifact.StoryNodeKey, targetKey: target.StoryNodeKey,
		policyKey: constraints.policy.StoryNodeKey, styleKey: constraints.style.StoryNodeKey, target: targetInput,
	}
	for _, world := range constraints.world {
		resolution.worldKeys = append(resolution.worldKeys, world.StoryNodeKey)
	}
	if bytes.Equal(bytes.TrimSpace(payload.IdentityAnchorAssetVersionRef), []byte("null")) {
		if payload.Purpose == "character_appearance" {
			return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s requires an identity anchor", node.StoryNodeKey)
		}
		return resolution, nil
	}
	if payload.Purpose != "character_appearance" {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s forbids an identity anchor", node.StoryNodeKey)
	}
	var anchor productionOwnerNodeRef
	if err := decodeStrictObject(payload.IdentityAnchorAssetVersionRef, &anchor); err != nil {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s has an invalid identity anchor ref", node.StoryNodeKey)
	}
	anchorNode, err := resolveProductionNodeRef(refIndex, anchor, NodeTypeAssetVersion)
	if err != nil {
		return productionAssetVersionResolution{}, fmt.Errorf("Production StoryGraph AssetVersion %s has an invalid identity anchor ref", node.StoryNodeKey)
	}
	resolution.anchorKey = anchorNode.StoryNodeKey
	resolution.anchorRef = &anchor
	return resolution, nil
}

type resolvedProductionConstraints struct {
	world  []Node
	policy Node
	style  Node
}

func resolveProductionConstraints(value productionConstraintRefs, refIndex map[string][]Node) (resolvedProductionConstraints, error) {
	if value.WorldRuleRefs == nil || validateSortedProductionRefs(value.WorldRuleRefs) != nil {
		return resolvedProductionConstraints{}, errors.New("invalid world rule refs")
	}
	result := resolvedProductionConstraints{}
	for _, ref := range value.WorldRuleRefs {
		node, err := resolveProductionNodeRef(refIndex, ref, NodeTypeWorldRule)
		if err != nil {
			return resolvedProductionConstraints{}, err
		}
		result.world = append(result.world, node)
	}
	var err error
	result.policy, err = resolveProductionNodeRef(refIndex, value.PolicySnapshotRef, NodeTypePolicySnapshot)
	if err != nil {
		return resolvedProductionConstraints{}, err
	}
	result.style, err = resolveProductionNodeRef(refIndex, value.EffectiveStyleSnapshotRef, NodeTypeEffectiveStyleSnapshot)
	return result, err
}

func addProductionAssetVersionEdges(expected map[productionRelationEdge]struct{}, value productionAssetVersionResolution) {
	to := value.node.StoryNodeKey
	expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: value.assetKey, to: to, qualifier: EdgeQualifier{BindingRole: "asset"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: value.specificationKey, to: to, qualifier: EdgeQualifier{BindingRole: "specification"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: value.stateKey, to: to, qualifier: EdgeQualifier{BindingRole: "state"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: value.artifactKey, to: to, qualifier: EdgeQualifier{BindingRole: "artifact"}}] = struct{}{}
	if value.anchorKey != "" {
		expected[productionRelationEdge{edgeType: EdgeTypeMaterializes, from: value.anchorKey, to: to, qualifier: EdgeQualifier{BindingRole: "identity_anchor"}}] = struct{}{}
	}
	expected[productionRelationEdge{edgeType: EdgeTypeFulfillsReferenceTarget, from: value.targetKey, to: to}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.policyKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "policy"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.styleKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "style"}}] = struct{}{}
	for _, worldKey := range value.worldKeys {
		expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: worldKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "world"}}] = struct{}{}
	}
}

func validateProductionAssetVersionAnchor(
	value productionAssetVersionResolution,
	resolutions map[string]productionAssetVersionResolution,
	refIndex map[string][]Node,
) error {
	if value.anchorRef == nil {
		return nil
	}
	anchorNode, _ := resolveProductionNodeRef(refIndex, *value.anchorRef, NodeTypeAssetVersion)
	anchor, ok := resolutions[anchorNode.StoryNodeKey]
	if !ok || anchor.payload.Purpose != "character_identity_anchor" || anchor.assetKey != value.assetKey ||
		anchor.specificationKey != value.specificationKey || anchor.stateKey == value.stateKey || anchor.styleKey != value.styleKey ||
		len(value.target.DependsOnTargetRefs) != 1 || !productionRefsEqual(value.target.DependsOnTargetRefs[0], anchor.payload.FulfilledReferenceTargetRef) {
		return fmt.Errorf("Production StoryGraph AssetVersion %s identity anchor does not match its target dependency", value.node.StoryNodeKey)
	}
	return nil
}

func productionBindingTriples(nodes []Node, refIndex map[string][]Node) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for _, node := range nodes {
		if node.NodeType != NodeTypeProductionBinding {
			continue
		}
		var payload productionBindingPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil {
			return nil, err
		}
		asset, assetErr := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
		specification, specificationErr := resolveProductionNodeRef(refIndex, payload.SpecificationRef, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification)
		if assetErr != nil || specificationErr != nil {
			return nil, errors.New("Production StoryGraph binding refs are invalid")
		}
		for _, stateRef := range payload.StateRefs {
			state, err := resolveProductionNodeRef(refIndex, stateRef, NodeTypeAssetState)
			if err != nil {
				return nil, errors.New("Production StoryGraph binding state ref is invalid")
			}
			result[productionBindingTripleKey(asset.StoryNodeKey, specification.StoryNodeKey, state.StoryNodeKey)] = struct{}{}
		}
	}
	return result, nil
}

func productionAssetVersionPurpose(value string) bool {
	return oneOf(value, "character_appearance", "character_identity_anchor", "location_board", "prop_sheet")
}

func productionAssetVersionSpecification(purpose string) NodeType {
	switch purpose {
	case "character_appearance", "character_identity_anchor":
		return NodeTypeCharacterSpecification
	case "location_board":
		return NodeTypeLocationSpecification
	case "prop_sheet":
		return NodeTypePropSpecification
	default:
		return ""
	}
}

func productionBindingTripleExists(values map[string]struct{}, asset, specification, state string) bool {
	_, ok := values[productionBindingTripleKey(asset, specification, state)]
	return ok
}

func productionBindingTripleKey(asset, specification, state string) string {
	return asset + "\x00" + specification + "\x00" + state
}

func productionSingleRefEqual(values []productionOwnerNodeRef, expected productionOwnerNodeRef) bool {
	return len(values) == 1 && productionRefsEqual(values[0], expected)
}

func productionConstraintsEqual(left, right productionConstraintRefs) bool {
	if len(left.WorldRuleRefs) != len(right.WorldRuleRefs) || !productionRefsEqual(left.PolicySnapshotRef, right.PolicySnapshotRef) ||
		!productionRefsEqual(left.EffectiveStyleSnapshotRef, right.EffectiveStyleSnapshotRef) {
		return false
	}
	for index := range left.WorldRuleRefs {
		if !productionRefsEqual(left.WorldRuleRefs[index], right.WorldRuleRefs[index]) {
			return false
		}
	}
	return true
}

func productionRefsEqual(left, right productionOwnerNodeRef) bool {
	leftKey, leftErr := left.identityKey()
	rightKey, rightErr := right.identityKey()
	return leftErr == nil && rightErr == nil && leftKey == rightKey
}

func oneOfEdge(value EdgeType, allowed ...EdgeType) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
