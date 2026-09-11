package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

type productionSceneReferenceBindingPayload struct {
	productionProjectionPayload
	SceneRef                       productionOwnerNodeRef   `json:"scene_ref"`
	OccurrenceRefs                 []productionOwnerNodeRef `json:"occurrence_refs"`
	InteractionClaimRefs           []productionOwnerNodeRef `json:"interaction_claim_refs"`
	CharacterAssetVersionRefs      []productionOwnerNodeRef `json:"character_asset_version_refs"`
	LocationAssetVersionRef        productionOwnerNodeRef   `json:"location_asset_version_ref"`
	PropAssetVersionRefs           []productionOwnerNodeRef `json:"prop_asset_version_refs"`
	SelectedCompositionArtifactRef productionOwnerNodeRef   `json:"selected_composition_artifact_ref"`
	FulfilledReferenceTargetRef    productionOwnerNodeRef   `json:"fulfilled_reference_target_ref"`
	Constraints                    productionConstraintRefs `json:"constraints"`
}

type productionSceneReferenceBindingResolution struct {
	node         Node
	payload      productionSceneReferenceBindingPayload
	scene        Node
	occurrences  []Node
	interactions []Node
	characters   []Node
	location     Node
	props        []Node
	artifact     Node
	target       Node
	constraints  resolvedProductionConstraints
}

func validateProductionSceneReferenceBindingRelations(nodes []Node, edges []Edge) error {
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
	expected := make(map[productionRelationEdge]struct{})
	for _, node := range nodes {
		if node.NodeType != NodeTypeSceneReferenceBindingVersion {
			continue
		}
		resolution, err := resolveProductionSceneReferenceBinding(node, refIndex)
		if err != nil {
			return err
		}
		addProductionSceneReferenceBindingEdges(expected, resolution)
	}
	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		to, ok := nodeByKey[edge.ToNodeKey]
		if !ok || to.NodeType != NodeTypeSceneReferenceBindingVersion ||
			!oneOfEdge(edge.EdgeType, EdgeTypeBindsReferenceInput, EdgeTypeBindsReferenceOutput, EdgeTypeFulfillsReferenceTarget, EdgeTypeConstrains) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s Scene Reference Binding edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Scene Reference Binding edges do not match payloads")
	}
	return nil
}

func resolveProductionSceneReferenceBinding(node Node, refIndex map[string][]Node) (productionSceneReferenceBindingResolution, error) {
	var payload productionSceneReferenceBindingPayload
	if err := decodeStrictObject(node.Payload, &payload); err != nil ||
		payload.OccurrenceRefs == nil || payload.InteractionClaimRefs == nil || payload.CharacterAssetVersionRefs == nil || payload.PropAssetVersionRefs == nil ||
		validateSortedProductionRefs(payload.OccurrenceRefs) != nil || validateSortedProductionRefs(payload.InteractionClaimRefs) != nil ||
		validateSortedProductionRefs(payload.CharacterAssetVersionRefs) != nil || validateSortedProductionRefs(payload.PropAssetVersionRefs) != nil {
		return productionSceneReferenceBindingResolution{}, fmt.Errorf("Production StoryGraph Scene Reference Binding %s has an invalid payload", node.StoryNodeKey)
	}
	result := productionSceneReferenceBindingResolution{node: node, payload: payload}
	var err error
	result.scene, err = resolveProductionNodeRef(refIndex, payload.SceneRef, NodeTypeScene)
	if err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.occurrences, err = resolveProductionNodeRefs(refIndex, payload.OccurrenceRefs, NodeTypeOccurrence); err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.interactions, err = resolveProductionNodeRefs(refIndex, payload.InteractionClaimRefs, NodeTypeContinuityClaim); err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	for _, interaction := range result.interactions {
		var branch struct {
			ClaimType string `json:"claim_type"`
		}
		if err := json.Unmarshal(interaction.Payload, &branch); err != nil || branch.ClaimType != "interaction" {
			return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
		}
	}
	if result.characters, err = resolveProductionNodeRefs(refIndex, payload.CharacterAssetVersionRefs, NodeTypeAssetVersion); err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.location, err = resolveProductionNodeRef(refIndex, payload.LocationAssetVersionRef, NodeTypeAssetVersion); err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.props, err = resolveProductionNodeRefs(refIndex, payload.PropAssetVersionRefs, NodeTypeAssetVersion); err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.artifact, err = resolveProductionNodeRef(refIndex, payload.SelectedCompositionArtifactRef, NodeTypeArtifact)
	if err != nil || result.artifact.OwnerRef.VersionFamily != "asset_composition_artifact_set" {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.target, err = resolveProductionNodeRef(refIndex, payload.FulfilledReferenceTargetRef, NodeTypeReferencePlanTarget)
	if err != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.constraints, err = resolveProductionConstraints(payload.Constraints, refIndex)
	if err != nil || validateProductionSceneReferenceBindingTarget(result) != nil {
		return productionSceneReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	return result, nil
}

func validateProductionSceneReferenceBindingTarget(value productionSceneReferenceBindingResolution) error {
	var target productionReferenceTargetPayload
	if err := decodeStrictObject(value.target.Payload, &target); err != nil || target.TargetKind != "scene_composition" || target.Fulfillment == "not_generated" ||
		!productionRefsEqual(target.TargetOwnerRefs.Scene[0], value.payload.SceneRef) ||
		!productionRefArraysEqual(target.TargetOwnerRefs.Occurrence, value.payload.OccurrenceRefs) ||
		!productionRefArraysEqual(target.TargetOwnerRefs.Interaction, value.payload.InteractionClaimRefs) ||
		!productionConstraintsEqual(target.Constraints, value.payload.Constraints) {
		return errors.New("reference_target_result_mismatch")
	}
	assetVersions := append(append([]Node(nil), value.characters...), value.location)
	assetVersions = append(assetVersions, value.props...)
	identityRefs := make([]productionOwnerNodeRef, 0, len(assetVersions))
	specificationRefs := make([]productionOwnerNodeRef, 0, len(assetVersions))
	stateRefs := make([]productionOwnerNodeRef, 0, len(assetVersions))
	fulfilledTargetRefs := make([]productionOwnerNodeRef, 0, len(assetVersions))
	for _, assetVersion := range assetVersions {
		var payload productionAssetVersionPayload
		if err := decodeStrictObject(assetVersion.Payload, &payload); err != nil {
			return err
		}
		identityRefs = append(identityRefs, payload.AssetIdentityRef)
		specificationRefs = append(specificationRefs, payload.SpecificationRef)
		stateRefs = append(stateRefs, payload.AssetStateRef)
		fulfilledTargetRefs = append(fulfilledTargetRefs, payload.FulfilledReferenceTargetRef)
	}
	for _, character := range value.characters {
		var payload productionAssetVersionPayload
		_ = decodeStrictObject(character.Payload, &payload)
		if !oneOf(payload.Purpose, "character_identity_anchor", "character_appearance") {
			return errors.New("reference_target_result_mismatch")
		}
	}
	var locationPayload productionAssetVersionPayload
	if decodeStrictObject(value.location.Payload, &locationPayload) != nil || locationPayload.Purpose != "location_board" {
		return errors.New("reference_target_result_mismatch")
	}
	for _, prop := range value.props {
		var payload productionAssetVersionPayload
		if decodeStrictObject(prop.Payload, &payload) != nil || payload.Purpose != "prop_sheet" {
			return errors.New("reference_target_result_mismatch")
		}
	}
	if !productionRefSetEqual(identityRefs, target.TargetOwnerRefs.Identity) ||
		!productionRefSetEqual(specificationRefs, target.TargetOwnerRefs.Specification) ||
		!productionRefSetEqual(stateRefs, target.TargetOwnerRefs.State) ||
		!productionRefSetEqual(fulfilledTargetRefs, target.DependsOnTargetRefs) {
		return errors.New("reference_target_result_mismatch")
	}
	return nil
}

func addProductionSceneReferenceBindingEdges(expected map[productionRelationEdge]struct{}, value productionSceneReferenceBindingResolution) {
	to := value.node.StoryNodeKey
	expected[productionRelationEdge{edgeType: EdgeTypeFulfillsReferenceTarget, from: value.target.StoryNodeKey, to: to}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: value.scene.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "scene"}}] = struct{}{}
	for _, occurrence := range value.occurrences {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: occurrence.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "occurrence"}}] = struct{}{}
	}
	for _, interaction := range value.interactions {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: interaction.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "interaction"}}] = struct{}{}
	}
	for _, character := range value.characters {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: character.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "character_asset"}}] = struct{}{}
	}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: value.location.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "location_asset"}}] = struct{}{}
	for _, prop := range value.props {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: prop.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "prop_asset"}}] = struct{}{}
	}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceOutput, from: value.artifact.StoryNodeKey, to: to}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraints.policy.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "policy"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraints.style.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "style"}}] = struct{}{}
	for _, world := range value.constraints.world {
		expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: world.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "world"}}] = struct{}{}
	}
}

func resolveProductionNodeRefs(index map[string][]Node, refs []productionOwnerNodeRef, allowed ...NodeType) ([]Node, error) {
	result := make([]Node, 0, len(refs))
	for _, ref := range refs {
		node, err := resolveProductionNodeRef(index, ref, allowed...)
		if err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	return result, nil
}

func productionRefArraysEqual(left, right []productionOwnerNodeRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !productionRefsEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func productionRefSetEqual(left, right []productionOwnerNodeRef) bool {
	leftKeys, leftOK := productionUniqueRefKeys(left)
	rightKeys, rightOK := productionUniqueRefKeys(right)
	return leftOK && rightOK && slices.Equal(leftKeys, rightKeys)
}

func productionUniqueRefKeys(refs []productionOwnerNodeRef) ([]string, bool) {
	keys := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key, err := ref.identityKey()
		if err != nil {
			return nil, false
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, true
}
