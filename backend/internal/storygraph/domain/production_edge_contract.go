package domain

import (
	"fmt"
)

func validateProductionEdgeEndpoints(nodes []Node, edges []Edge) error {
	types := make(map[string]NodeType, len(nodes))
	for _, node := range nodes {
		types[node.StoryNodeKey] = node.NodeType
	}
	for _, edge := range edges {
		if err := ValidateProductionEdgeEndpoint(edge.EdgeType, types[edge.FromNodeKey], types[edge.ToNodeKey], edge.Qualifier); err != nil {
			return err
		}
	}
	return nil
}

// ValidateProductionEdgeEndpoint verifies the exact edge vocabulary, endpoint
// direction and qualifier role frozen by the production schema manifest.
func ValidateProductionEdgeEndpoint(edgeType EdgeType, from, to NodeType, qualifier EdgeQualifier) error {
	if err := qualifier.validate(edgeType); err != nil {
		return err
	}
	if !productionEdgeEndpointAllowed(edgeType, from, to, qualifier) {
		return fmt.Errorf("Production StoryGraph edge type %s does not allow %s -> %s with this qualifier", edgeType, from, to)
	}
	return nil
}

func productionEdgeEndpointAllowed(edgeType EdgeType, from, to NodeType, qualifier EdgeQualifier) bool {
	switch edgeType {
	case EdgeTypeContains:
		return productionStableKey(qualifier.SequenceKey) && (from == NodeTypeEpisode && to == NodeTypeScene ||
			from == NodeTypeScene && oneOfNode(to, NodeTypeDialogue, NodeTypeNarrativeBeat))
	case EdgeTypeDerivedFrom:
		return from == NodeTypeSourceRevision && to == NodeTypeSourceEvidence ||
			from == NodeTypeSourceEvidence && oneOfNode(to,
				NodeTypeAssetIdentity, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification,
				NodeTypeAssetState, NodeTypeWorldRule, NodeTypeStoryArc, NodeTypePlotThread, NodeTypeEpisode, NodeTypeScene,
				NodeTypeDialogue, NodeTypeNarrativeBeat, NodeTypeOccurrence)
	case EdgeTypeDescribesIdentity:
		return from == NodeTypeAssetIdentity && oneOfNode(to, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification)
	case EdgeTypeHasState:
		return from == NodeTypeAssetIdentity && to == NodeTypeAssetState
	case EdgeTypePrecedes:
		return productionStableKey(qualifier.SequenceKey) && from == to && oneOfNode(from, NodeTypeEpisode, NodeTypeScene, NodeTypeNarrativeBeat, NodeTypeShot)
	case EdgeTypeAnchorsOccurrence:
		return to == NodeTypeOccurrence && (from == NodeTypeScene && qualifier.AnchorRole == "scene" || from == NodeTypeNarrativeBeat && qualifier.AnchorRole == "beat")
	case EdgeTypeInstantiatesOccurrence:
		return from == NodeTypeAssetState && to == NodeTypeOccurrence
	case EdgeTypeSupports:
		return from == NodeTypeSourceEvidence && isClaimNode(to)
	case EdgeTypeClaimParticipant:
		return oneOfNode(from, NodeTypeAssetIdentity, NodeTypeOccurrence) && isClaimNode(to)
	case EdgeTypeClaimAnchor:
		return productionClaimAnchorEndpoint(from, qualifier.AnchorRole) && isClaimNode(to)
	case EdgeTypeClaimState:
		return from == NodeTypeAssetState && to == NodeTypeContinuityClaim
	case EdgeTypeSupersedes:
		return from == to && isClaimNode(from)
	case EdgeTypeConstrains:
		return productionConstraintSourceRole(from, qualifier.ConstraintRole) && oneOfNode(to,
			NodeTypeReferencePlanTarget, NodeTypeAssetVersion, NodeTypeSceneReferenceBindingVersion,
			NodeTypeInteractionReferenceBindingVersion, NodeTypeShotProductionBindingVersion)
	case EdgeTypeMaterializes:
		return productionMaterializationEndpoint(from, to, qualifier.BindingRole)
	case EdgeTypeContainsReferenceTarget:
		return productionStableKey(qualifier.SequenceKey) && from == NodeTypeApprovedReferencePlanVersion && to == NodeTypeReferencePlanTarget
	case EdgeTypeDependsOnReferenceTarget:
		return from == NodeTypeReferencePlanTarget && to == NodeTypeReferencePlanTarget
	case EdgeTypePlansReference:
		return to == NodeTypeReferencePlanTarget && productionReferenceSourceRole(from, qualifier.ReferenceRole)
	case EdgeTypeFulfillsReferenceTarget:
		return from == NodeTypeReferencePlanTarget && oneOfNode(to, NodeTypeAssetVersion, NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion)
	case EdgeTypeBindsReferenceInput:
		return productionReferenceBindingEndpoint(from, to, qualifier.ReferenceRole)
	case EdgeTypeBindsReferenceOutput:
		return from == NodeTypeArtifact && oneOfNode(to, NodeTypeSceneReferenceBindingVersion, NodeTypeInteractionReferenceBindingVersion)
	case EdgeTypeRealizes:
		return from == NodeTypeNarrativeBeat && to == NodeTypeShot
	case EdgeTypeInforms:
		return to == NodeTypeShotProductionBindingVersion && (from == NodeTypeOccurrence && qualifier.InformsRole == "occurrence" ||
			from == NodeTypeSceneReferenceBindingVersion && qualifier.InformsRole == "scene_reference" ||
			from == NodeTypeInteractionReferenceBindingVersion && qualifier.InformsRole == "interaction_reference")
	case EdgeTypeBindsInput:
		return to == NodeTypeShotProductionBindingVersion && productionShotBindingSourceRole(from, qualifier.BindingRole)
	default:
		return false
	}
}

func productionClaimAnchorEndpoint(from NodeType, role string) bool {
	switch from {
	case NodeTypeEpisode:
		return role == "episode" || role == "scope_start" || role == "scope_end"
	case NodeTypeScene:
		return role == "scene" || role == "scope_start" || role == "scope_end"
	case NodeTypeNarrativeBeat:
		return role == "beat" || role == "scope_start" || role == "scope_end"
	case NodeTypeOccurrence:
		return oneOf(role, "character_occurrence", "prop_occurrence", "scope_start", "scope_end")
	default:
		return false
	}
}

func productionConstraintSourceRole(from NodeType, role string) bool {
	return from == NodeTypeWorldRule && role == "world" || from == NodeTypePolicySnapshot && role == "policy" ||
		from == NodeTypeEffectiveStyleSnapshot && role == "style"
}

func productionMaterializationEndpoint(from, to NodeType, role string) bool {
	if to == NodeTypeProductionBinding {
		return from == NodeTypeAssetIdentity && role == "asset" || from == NodeTypeAssetState && role == "state" ||
			oneOfNode(from, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification) && role == "specification"
	}
	if to != NodeTypeAssetVersion {
		return false
	}
	return from == NodeTypeAssetIdentity && role == "asset" || from == NodeTypeAssetState && role == "state" ||
		oneOfNode(from, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification) && role == "specification" ||
		from == NodeTypeAssetVersion && role == "identity_anchor" || from == NodeTypeArtifact && role == "artifact"
}

func productionReferenceSourceRole(from NodeType, role string) bool {
	switch role {
	case "identity":
		return from == NodeTypeAssetIdentity
	case "specification":
		return oneOfNode(from, NodeTypeCharacterSpecification, NodeTypeLocationSpecification, NodeTypePropSpecification)
	case "state":
		return from == NodeTypeAssetState
	case "style":
		return from == NodeTypeEffectiveStyleSnapshot
	case "scene":
		return from == NodeTypeScene
	case "occurrence":
		return from == NodeTypeOccurrence
	case "interaction":
		return from == NodeTypeContinuityClaim
	default:
		return false
	}
}

func productionReferenceBindingEndpoint(from, to NodeType, role string) bool {
	switch to {
	case NodeTypeSceneReferenceBindingVersion:
		return from == NodeTypeScene && role == "scene" || from == NodeTypeOccurrence && role == "occurrence" ||
			from == NodeTypeContinuityClaim && role == "interaction" || from == NodeTypeAssetVersion && oneOf(role, "character_asset", "location_asset", "prop_asset")
	case NodeTypeInteractionReferenceBindingVersion:
		return from == NodeTypeContinuityClaim && role == "interaction" ||
			from == NodeTypeAssetVersion && oneOf(role, "character_asset", "prop_asset")
	default:
		return false
	}
}

func productionShotBindingSourceRole(from NodeType, role string) bool {
	switch role {
	case "shot":
		return from == NodeTypeShot
	case "occurrence":
		return from == NodeTypeOccurrence
	case "asset_version":
		return from == NodeTypeAssetVersion
	case "scene_reference":
		return from == NodeTypeSceneReferenceBindingVersion
	case "interaction_reference":
		return from == NodeTypeInteractionReferenceBindingVersion
	case "style":
		return from == NodeTypeEffectiveStyleSnapshot
	default:
		return false
	}
}
