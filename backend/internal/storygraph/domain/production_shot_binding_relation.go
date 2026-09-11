package domain

import (
	"errors"
	"fmt"
)

type productionShotBindingPayload struct {
	productionProjectionPayload
	ShotRef                         productionOwnerNodeRef   `json:"shot_ref"`
	OccurrenceRefs                  []productionOwnerNodeRef `json:"occurrence_refs"`
	AssetVersionRefs                []productionOwnerNodeRef `json:"asset_version_refs"`
	SceneReferenceBindingRefs       []productionOwnerNodeRef `json:"scene_reference_binding_refs"`
	InteractionReferenceBindingRefs []productionOwnerNodeRef `json:"interaction_reference_binding_refs"`
	EffectiveStyleSnapshotRef       productionOwnerNodeRef   `json:"effective_style_snapshot_ref"`
	Constraints                     productionConstraintRefs `json:"constraints"`
}

type productionShotBindingResolution struct {
	node                Node
	payload             productionShotBindingPayload
	shot                Node
	scene               Node
	occurrences         []Node
	assetVersions       []Node
	sceneBindings       []Node
	interactionBindings []Node
	style               Node
	constraints         resolvedProductionConstraints
}

func validateProductionShotBindingRelations(nodes []Node, edges []Edge) error {
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
	beatScenes := make(map[string][]string)
	for _, edge := range edges {
		from, fromOK := nodeByKey[edge.FromNodeKey]
		to, toOK := nodeByKey[edge.ToNodeKey]
		if edge.EdgeType == EdgeTypeContains && fromOK && toOK && from.NodeType == NodeTypeScene && to.NodeType == NodeTypeNarrativeBeat {
			beatScenes[to.StoryNodeKey] = append(beatScenes[to.StoryNodeKey], from.StoryNodeKey)
		}
	}

	expected := make(map[productionRelationEdge]struct{})
	bindingCounts := make(map[string]int)
	for _, node := range nodes {
		if node.NodeType != NodeTypeShotProductionBindingVersion {
			continue
		}
		resolution, err := resolveProductionShotBinding(node, refIndex, nodeByKey, beatScenes)
		if err != nil {
			return err
		}
		bindingCounts[resolution.shot.StoryNodeKey]++
		addProductionShotBindingEdges(expected, resolution)
	}
	for _, node := range nodes {
		if node.NodeType == NodeTypeShot && bindingCounts[node.StoryNodeKey] != 1 {
			return fmt.Errorf("Production StoryGraph Shot %s does not have exactly one production binding", node.StoryNodeKey)
		}
	}
	return validateProductionShotBindingEdges(edges, nodeByKey, expected)
}

func resolveProductionShotBinding(
	node Node,
	refIndex map[string][]Node,
	nodeByKey map[string]Node,
	beatScenes map[string][]string,
) (productionShotBindingResolution, error) {
	var payload productionShotBindingPayload
	if err := decodeStrictObject(node.Payload, &payload); err != nil ||
		payload.OccurrenceRefs == nil || payload.AssetVersionRefs == nil || payload.SceneReferenceBindingRefs == nil || payload.InteractionReferenceBindingRefs == nil ||
		validateSortedProductionRefs(payload.OccurrenceRefs) != nil || validateSortedProductionRefs(payload.AssetVersionRefs) != nil ||
		validateSortedProductionRefs(payload.SceneReferenceBindingRefs) != nil || validateSortedProductionRefs(payload.InteractionReferenceBindingRefs) != nil {
		return productionShotBindingResolution{}, fmt.Errorf("Production StoryGraph Shot Binding %s has an invalid payload", node.StoryNodeKey)
	}
	result := productionShotBindingResolution{node: node, payload: payload}
	var err error
	result.shot, err = resolveProductionNodeRef(refIndex, payload.ShotRef, NodeTypeShot)
	if err != nil || !sameProductionOwnerVersion(node.OwnerRef, result.shot.OwnerRef) {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	shot, err := resolveProductionShot(result.shot, refIndex, beatScenes)
	if err != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	result.scene = nodeByKey[shot.sceneKey]
	if result.scene.NodeType != NodeTypeScene {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	if result.occurrences, err = resolveProductionNodeRefs(refIndex, payload.OccurrenceRefs, NodeTypeOccurrence); err != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	if result.assetVersions, err = resolveProductionNodeRefs(refIndex, payload.AssetVersionRefs, NodeTypeAssetVersion); err != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	if result.sceneBindings, err = resolveProductionNodeRefs(refIndex, payload.SceneReferenceBindingRefs, NodeTypeSceneReferenceBindingVersion); err != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	if result.interactionBindings, err = resolveProductionNodeRefs(refIndex, payload.InteractionReferenceBindingRefs, NodeTypeInteractionReferenceBindingVersion); err != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	result.style, err = resolveProductionNodeRef(refIndex, payload.EffectiveStyleSnapshotRef, NodeTypeEffectiveStyleSnapshot)
	if err != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	result.constraints, err = resolveProductionConstraints(payload.Constraints, refIndex)
	if err != nil || result.style.StoryNodeKey != result.constraints.style.StoryNodeKey || validateProductionShotBindingInputs(result, refIndex) != nil {
		return productionShotBindingResolution{}, errors.New("shot_binding_input_mismatch")
	}
	return result, nil
}

func validateProductionShotBindingInputs(value productionShotBindingResolution, refIndex map[string][]Node) error {
	sceneRef, err := productionOwnerNodeRefFromOwner(value.scene.OwnerRef)
	if err != nil {
		return errors.New("shot_binding_input_mismatch")
	}
	assetPairCounts := make(map[string]int, len(value.assetVersions))
	assetRefKeys := make(map[string]struct{}, len(value.assetVersions))
	for _, assetVersion := range value.assetVersions {
		var payload productionAssetVersionPayload
		if decodeStrictObject(assetVersion.Payload, &payload) != nil || !productionConstraintsEqual(payload.Constraints, value.payload.Constraints) {
			return errors.New("shot_binding_input_mismatch")
		}
		assetKey, assetErr := payload.AssetIdentityRef.identityKey()
		stateKey, stateErr := payload.AssetStateRef.identityKey()
		ref, refErr := productionOwnerNodeRefFromOwner(assetVersion.OwnerRef)
		refKey, refKeyErr := ref.identityKey()
		target, targetErr := resolveProductionNodeRef(refIndex, payload.FulfilledReferenceTargetRef, NodeTypeReferencePlanTarget)
		var targetPayload productionReferenceTargetPayload
		if assetErr != nil || stateErr != nil || refErr != nil || refKeyErr != nil || targetErr != nil || decodeStrictObject(target.Payload, &targetPayload) != nil ||
			!productionStringSuperset(targetPayload.CoverageScopeKeys, []string{value.scene.OwnerRef.OwnerLogicalID}) {
			return errors.New("shot_binding_input_mismatch")
		}
		assetPairCounts[assetKey+"\x00"+stateKey]++
		assetRefKeys[refKey] = struct{}{}
	}
	occurrenceRefKeys := make(map[string]struct{}, len(value.occurrences))
	for _, occurrence := range value.occurrences {
		var payload productionOccurrencePayload
		if decodeStrictObject(occurrence.Payload, &payload) != nil || !productionRefsEqual(payload.SceneRef, sceneRef) {
			return errors.New("shot_binding_input_mismatch")
		}
		assetKey, assetErr := payload.AssetIdentityRef.identityKey()
		stateKey, stateErr := payload.AssetStateRef.identityKey()
		ref, refErr := productionOwnerNodeRefFromOwner(occurrence.OwnerRef)
		refKey, refKeyErr := ref.identityKey()
		if assetErr != nil || stateErr != nil || refErr != nil || refKeyErr != nil || assetPairCounts[assetKey+"\x00"+stateKey] != 1 {
			return errors.New("shot_binding_input_mismatch")
		}
		occurrenceRefKeys[refKey] = struct{}{}
	}
	for _, sceneBinding := range value.sceneBindings {
		var payload productionSceneReferenceBindingPayload
		if decodeStrictObject(sceneBinding.Payload, &payload) != nil || !productionRefsEqual(payload.SceneRef, sceneRef) ||
			!productionConstraintsEqual(payload.Constraints, value.payload.Constraints) {
			return errors.New("shot_binding_input_mismatch")
		}
		locationKey, locationErr := payload.LocationAssetVersionRef.identityKey()
		if locationErr != nil {
			return errors.New("shot_binding_input_mismatch")
		}
		if _, exists := assetRefKeys[locationKey]; !exists {
			return errors.New("shot_binding_input_mismatch")
		}
	}
	for _, interactionBinding := range value.interactionBindings {
		var payload productionInteractionReferenceBindingPayload
		if decodeStrictObject(interactionBinding.Payload, &payload) != nil || !productionConstraintsEqual(payload.Constraints, value.payload.Constraints) {
			return errors.New("shot_binding_input_mismatch")
		}
		claim, err := resolveProductionNodeRef(refIndex, payload.InteractionClaimRef, NodeTypeContinuityClaim)
		var claimPayload productionInteractionPayload
		if err != nil || decodeStrictObject(claim.Payload, &claimPayload) != nil || claimPayload.ClaimType != "interaction" ||
			!productionRefsEqual(claimPayload.SceneRef, sceneRef) {
			return errors.New("shot_binding_input_mismatch")
		}
		participantRefs := append([]productionOwnerNodeRef(nil), claimPayload.ActorOccurrenceRefs...)
		participantRefs = append(participantRefs, claimPayload.PropOccurrenceRef)
		if claimPayload.CounterpartyOccurrenceRef != nil {
			participantRefs = append(participantRefs, *claimPayload.CounterpartyOccurrenceRef)
		}
		if !productionRefsContained(participantRefs, occurrenceRefKeys) {
			return errors.New("shot_binding_input_mismatch")
		}
		assetRefs := append([]productionOwnerNodeRef(nil), payload.CharacterAssetVersionRefs...)
		assetRefs = append(assetRefs, payload.PropAssetVersionRef)
		if !productionRefsContained(assetRefs, assetRefKeys) {
			return errors.New("shot_binding_input_mismatch")
		}
	}
	return nil
}

func productionRefsContained(refs []productionOwnerNodeRef, keys map[string]struct{}) bool {
	for _, ref := range refs {
		key, err := ref.identityKey()
		if err != nil {
			return false
		}
		if _, exists := keys[key]; !exists {
			return false
		}
	}
	return true
}

func sameProductionOwnerVersion(left, right OwnerRef) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID && left.OwnerKind == right.OwnerKind &&
		left.VersionFamily == right.VersionFamily && left.OwnerLogicalID == right.OwnerLogicalID && left.OwnerVersionID == right.OwnerVersionID &&
		left.OwnerRevision == right.OwnerRevision && left.OwnerContentHash == right.OwnerContentHash
}

func addProductionShotBindingEdges(expected map[productionRelationEdge]struct{}, value productionShotBindingResolution) {
	to := value.node.StoryNodeKey
	expected[productionRelationEdge{edgeType: EdgeTypeBindsInput, from: value.shot.StoryNodeKey, to: to, qualifier: EdgeQualifier{BindingRole: "shot"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsInput, from: value.style.StoryNodeKey, to: to, qualifier: EdgeQualifier{BindingRole: "style"}}] = struct{}{}
	for _, occurrence := range value.occurrences {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsInput, from: occurrence.StoryNodeKey, to: to, qualifier: EdgeQualifier{BindingRole: "occurrence"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeInforms, from: occurrence.StoryNodeKey, to: to, qualifier: EdgeQualifier{InformsRole: "occurrence"}}] = struct{}{}
	}
	for _, assetVersion := range value.assetVersions {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsInput, from: assetVersion.StoryNodeKey, to: to, qualifier: EdgeQualifier{BindingRole: "asset_version"}}] = struct{}{}
	}
	for _, sceneBinding := range value.sceneBindings {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsInput, from: sceneBinding.StoryNodeKey, to: to, qualifier: EdgeQualifier{BindingRole: "scene_reference"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeInforms, from: sceneBinding.StoryNodeKey, to: to, qualifier: EdgeQualifier{InformsRole: "scene_reference"}}] = struct{}{}
	}
	for _, interactionBinding := range value.interactionBindings {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsInput, from: interactionBinding.StoryNodeKey, to: to, qualifier: EdgeQualifier{BindingRole: "interaction_reference"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeInforms, from: interactionBinding.StoryNodeKey, to: to, qualifier: EdgeQualifier{InformsRole: "interaction_reference"}}] = struct{}{}
	}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraints.policy.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "policy"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraints.style.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "style"}}] = struct{}{}
	for _, world := range value.constraints.world {
		expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: world.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "world"}}] = struct{}{}
	}
}

func validateProductionShotBindingEdges(edges []Edge, nodeByKey map[string]Node, expected map[productionRelationEdge]struct{}) error {
	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		to, ok := nodeByKey[edge.ToNodeKey]
		if !ok || to.NodeType != NodeTypeShotProductionBindingVersion || !oneOfEdge(edge.EdgeType, EdgeTypeInforms, EdgeTypeBindsInput, EdgeTypeConstrains) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s Shot Binding edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Shot Binding edges do not match payloads")
	}
	return nil
}
