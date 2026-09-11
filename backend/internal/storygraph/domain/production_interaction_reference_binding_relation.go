package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type productionInteractionReferenceBindingPayload struct {
	productionProjectionPayload
	InteractionClaimRef            productionOwnerNodeRef   `json:"interaction_claim_ref"`
	CharacterAssetVersionRefs      []productionOwnerNodeRef `json:"character_asset_version_refs"`
	PropAssetVersionRef            productionOwnerNodeRef   `json:"prop_asset_version_ref"`
	SelectedCompositionArtifactRef productionOwnerNodeRef   `json:"selected_composition_artifact_ref"`
	FulfilledReferenceTargetRef    productionOwnerNodeRef   `json:"fulfilled_reference_target_ref"`
	Constraints                    productionConstraintRefs `json:"constraints"`
}

type productionInteractionReferenceBindingResolution struct {
	node                 Node
	payload              productionInteractionReferenceBindingPayload
	interaction          Node
	interactionPayload   productionInteractionPayload
	characterOccurrences []Node
	propOccurrence       Node
	characters           []Node
	prop                 Node
	artifact             Node
	target               Node
	constraints          resolvedProductionConstraints
}

func validateProductionInteractionReferenceBindingRelations(nodes []Node, edges []Edge) error {
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
		if node.NodeType != NodeTypeInteractionReferenceBindingVersion {
			continue
		}
		resolution, err := resolveProductionInteractionReferenceBinding(node, refIndex)
		if err != nil {
			return err
		}
		addProductionInteractionReferenceBindingEdges(expected, resolution)
	}
	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		to, ok := nodeByKey[edge.ToNodeKey]
		if !ok || to.NodeType != NodeTypeInteractionReferenceBindingVersion ||
			!oneOfEdge(edge.EdgeType, EdgeTypeBindsReferenceInput, EdgeTypeBindsReferenceOutput, EdgeTypeFulfillsReferenceTarget, EdgeTypeConstrains) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s Interaction Reference Binding edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Interaction Reference Binding edges do not match payloads")
	}
	return nil
}

func resolveProductionInteractionReferenceBinding(node Node, refIndex map[string][]Node) (productionInteractionReferenceBindingResolution, error) {
	var payload productionInteractionReferenceBindingPayload
	if err := decodeStrictObject(node.Payload, &payload); err != nil || len(payload.CharacterAssetVersionRefs) == 0 ||
		validateSortedProductionRefs(payload.CharacterAssetVersionRefs) != nil {
		return productionInteractionReferenceBindingResolution{}, fmt.Errorf("Production StoryGraph Interaction Reference Binding %s has an invalid payload", node.StoryNodeKey)
	}
	result := productionInteractionReferenceBindingResolution{node: node, payload: payload}
	var err error
	result.interaction, err = resolveProductionNodeRef(refIndex, payload.InteractionClaimRef, NodeTypeContinuityClaim)
	if err != nil || decodeStrictObject(result.interaction.Payload, &result.interactionPayload) != nil || result.interactionPayload.ClaimType != "interaction" || len(result.interactionPayload.ActorOccurrenceRefs) == 0 {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.characterOccurrences, err = resolveProductionNodeRefs(refIndex, result.interactionPayload.ActorOccurrenceRefs, NodeTypeOccurrence); err != nil {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.interactionPayload.CounterpartyOccurrenceRef != nil {
		counterparty, counterpartyErr := resolveProductionNodeRef(refIndex, *result.interactionPayload.CounterpartyOccurrenceRef, NodeTypeOccurrence)
		if counterpartyErr != nil {
			return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
		}
		result.characterOccurrences = append(result.characterOccurrences, counterparty)
	}
	result.propOccurrence, err = resolveProductionNodeRef(refIndex, result.interactionPayload.PropOccurrenceRef, NodeTypeOccurrence)
	if err != nil {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	if result.characters, err = resolveProductionNodeRefs(refIndex, payload.CharacterAssetVersionRefs, NodeTypeAssetVersion); err != nil {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.prop, err = resolveProductionNodeRef(refIndex, payload.PropAssetVersionRef, NodeTypeAssetVersion)
	if err != nil {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.artifact, err = resolveProductionNodeRef(refIndex, payload.SelectedCompositionArtifactRef, NodeTypeArtifact)
	if err != nil || result.artifact.OwnerRef.VersionFamily != "asset_composition_artifact_set" {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.target, err = resolveProductionNodeRef(refIndex, payload.FulfilledReferenceTargetRef, NodeTypeReferencePlanTarget)
	if err != nil {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	result.constraints, err = resolveProductionConstraints(payload.Constraints, refIndex)
	if err != nil || validateProductionInteractionReferenceBindingTarget(result) != nil {
		return productionInteractionReferenceBindingResolution{}, errors.New("reference_target_result_mismatch")
	}
	return result, nil
}

func validateProductionInteractionReferenceBindingTarget(value productionInteractionReferenceBindingResolution) error {
	var target productionReferenceTargetPayload
	if err := decodeStrictObject(value.target.Payload, &target); err != nil || target.TargetKind != "interaction_composition" || target.Fulfillment == "not_generated" ||
		!productionSingleRefEqual(target.TargetOwnerRefs.Interaction, value.payload.InteractionClaimRef) ||
		!productionSingleRefEqual(target.TargetOwnerRefs.Scene, value.interactionPayload.SceneRef) ||
		!productionConstraintsEqual(target.Constraints, value.payload.Constraints) {
		return errors.New("reference_target_result_mismatch")
	}
	expectedOccurrenceRefs := append([]productionOwnerNodeRef(nil), value.interactionPayload.ActorOccurrenceRefs...)
	expectedOccurrenceRefs = append(expectedOccurrenceRefs, value.interactionPayload.PropOccurrenceRef)
	if value.interactionPayload.CounterpartyOccurrenceRef != nil {
		expectedOccurrenceRefs = append(expectedOccurrenceRefs, *value.interactionPayload.CounterpartyOccurrenceRef)
	}
	slices.SortFunc(expectedOccurrenceRefs, func(left, right productionOwnerNodeRef) int {
		return strings.Compare(left.sortKey(), right.sortKey())
	})
	if !productionRefArraysEqual(target.TargetOwnerRefs.Occurrence, expectedOccurrenceRefs) {
		return errors.New("reference_target_result_mismatch")
	}

	assetVersions := append(append([]Node(nil), value.characters...), value.prop)
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
	if !productionRefSetEqual(identityRefs, target.TargetOwnerRefs.Identity) ||
		!productionRefSetEqual(specificationRefs, target.TargetOwnerRefs.Specification) ||
		!productionRefSetEqual(stateRefs, target.TargetOwnerRefs.State) ||
		!productionRefSetEqual(fulfilledTargetRefs, target.DependsOnTargetRefs) {
		return errors.New("reference_target_result_mismatch")
	}
	if !productionAssetVersionsMatchOccurrences(value.characters, value.characterOccurrences, "character_identity_anchor", "character_appearance") ||
		!productionAssetVersionsMatchOccurrences([]Node{value.prop}, []Node{value.propOccurrence}, "prop_sheet") {
		return errors.New("reference_target_result_mismatch")
	}
	return nil
}

func productionAssetVersionsMatchOccurrences(assetVersions, occurrences []Node, purposes ...string) bool {
	assetKeys := make([]string, 0, len(assetVersions))
	for _, assetVersion := range assetVersions {
		var payload productionAssetVersionPayload
		if decodeStrictObject(assetVersion.Payload, &payload) != nil || !oneOf(payload.Purpose, purposes...) {
			return false
		}
		assetKey, assetErr := payload.AssetIdentityRef.identityKey()
		stateKey, stateErr := payload.AssetStateRef.identityKey()
		if assetErr != nil || stateErr != nil {
			return false
		}
		assetKeys = append(assetKeys, assetKey+"\x00"+stateKey)
	}
	occurrenceKeys := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		var payload productionOccurrencePayload
		if decodeStrictObject(occurrence.Payload, &payload) != nil {
			return false
		}
		assetKey, assetErr := payload.AssetIdentityRef.identityKey()
		stateKey, stateErr := payload.AssetStateRef.identityKey()
		if assetErr != nil || stateErr != nil {
			return false
		}
		occurrenceKeys = append(occurrenceKeys, assetKey+"\x00"+stateKey)
	}
	slices.Sort(assetKeys)
	slices.Sort(occurrenceKeys)
	return slices.Equal(assetKeys, occurrenceKeys)
}

func addProductionInteractionReferenceBindingEdges(expected map[productionRelationEdge]struct{}, value productionInteractionReferenceBindingResolution) {
	to := value.node.StoryNodeKey
	expected[productionRelationEdge{edgeType: EdgeTypeFulfillsReferenceTarget, from: value.target.StoryNodeKey, to: to}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: value.interaction.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "interaction"}}] = struct{}{}
	for _, character := range value.characters {
		expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: character.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "character_asset"}}] = struct{}{}
	}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceInput, from: value.prop.StoryNodeKey, to: to, qualifier: EdgeQualifier{ReferenceRole: "prop_asset"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeBindsReferenceOutput, from: value.artifact.StoryNodeKey, to: to}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraints.policy.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "policy"}}] = struct{}{}
	expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: value.constraints.style.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "style"}}] = struct{}{}
	for _, world := range value.constraints.world {
		expected[productionRelationEdge{edgeType: EdgeTypeConstrains, from: world.StoryNodeKey, to: to, qualifier: EdgeQualifier{ConstraintRole: "world"}}] = struct{}{}
	}
}
