package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const (
	productionEvidenceNone              = "none"
	productionEvidenceRoot              = "source_evidence_root"
	productionEvidenceRequired          = "evidence_required"
	productionEvidenceOrCreatorDecision = "evidence_or_creator_decision"
)

type productionNodeDefinition struct {
	ownerKind        string
	versionFamilies  []string
	payloadContracts map[string]string
	discriminant     string
	evidencePolicy   string
}

type productionAuditableBibleFactPayload struct {
	productionProjectionPayload
	CreatorDecisionRef json.RawMessage `json:"creator_decision_ref"`
}

var productionNodeDefinitions = map[NodeType]productionNodeDefinition{
	NodeTypeSourceRevision:                     productionNode("production/script", productionEvidenceNone, []string{"script_source_set"}, "storygraph-production/source_revision-ref-payload-contract"),
	NodeTypeSourceEvidence:                     productionNode("production/bible", productionEvidenceRoot, []string{"bible_production_world_set"}, "storygraph-production/source_evidence-ref-payload-contract"),
	NodeTypePolicySnapshot:                     productionNode("preset", productionEvidenceNone, []string{"preset_effective_set"}, "storygraph-production/policy_snapshot-ref-payload-contract"),
	NodeTypeEffectiveStyleSnapshot:             productionNode("preset", productionEvidenceNone, []string{"preset_effective_set"}, "storygraph-production/effective_style_snapshot-ref-payload-contract"),
	NodeTypeAssetIdentity:                      productionNode("asset", productionEvidenceOrCreatorDecision, []string{"asset_identity_state_set"}, "storygraph-production/asset-identity-payload-contract"),
	NodeTypeCharacterSpecification:             productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/specification-payload-contract"),
	NodeTypeLocationSpecification:              productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/specification-payload-contract"),
	NodeTypePropSpecification:                  productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/specification-payload-contract"),
	NodeTypeAssetState:                         productionNode("asset", productionEvidenceOrCreatorDecision, []string{"asset_identity_state_set"}, "storygraph-production/asset-state-payload-contract"),
	NodeTypeProductionBinding:                  productionNode("production/bible", productionEvidenceNone, []string{"bible_production_world_set"}, "storygraph-production/production-binding-payload-contract"),
	NodeTypeWorldRule:                          productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/auditable-bible-fact-payload-contract"),
	NodeTypeStoryArc:                           productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/auditable-bible-fact-payload-contract"),
	NodeTypePlotThread:                         productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/auditable-bible-fact-payload-contract"),
	NodeTypeRelationshipClaim:                  productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/narrative-claim-payload-contract"),
	NodeTypeForeshadowingClaim:                 productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/narrative-claim-payload-contract"),
	NodeTypePayoffClaim:                        productionNode("production/bible", productionEvidenceOrCreatorDecision, []string{"bible_production_world_set"}, "storygraph-production/narrative-claim-payload-contract"),
	NodeTypeEpisode:                            productionNode("production/project", productionEvidenceRequired, []string{"project_episode_set"}, "storygraph-production/episode-ref-payload-contract"),
	NodeTypeScene:                              productionNode("production/planning", productionEvidenceRequired, []string{"planning_scene_set"}, "storygraph-production/scene-ref-payload-contract"),
	NodeTypeDialogue:                           productionNode("production/planning", productionEvidenceRequired, []string{"planning_scene_set"}, "storygraph-production/dialogue-ref-payload-contract"),
	NodeTypeNarrativeBeat:                      productionNode("production/planning", productionEvidenceRequired, []string{"planning_scene_set"}, "storygraph-production/narrative_beat-ref-payload-contract"),
	NodeTypeOccurrence:                         productionNode("production/planning", productionEvidenceOrCreatorDecision, []string{"planning_scene_set"}, "storygraph-production/occurrence-payload-contract"),
	NodeTypeCausalClaim:                        productionNode("production/planning", productionEvidenceOrCreatorDecision, []string{"planning_scene_set"}, "storygraph-production/narrative-claim-payload-contract"),
	NodeTypeArtifact:                           productionNode("asset", productionEvidenceNone, []string{"asset_base_reference_set", "asset_composition_artifact_set"}, "storygraph-production/artifact-ref-payload-contract"),
	NodeTypeAssetVersion:                       productionNode("asset", productionEvidenceNone, []string{"asset_base_reference_set"}, "storygraph-production/asset-version-payload-contract"),
	NodeTypeApprovedReferencePlanVersion:       productionNode("production/reference", productionEvidenceNone, []string{"reference_plan_set"}, "storygraph-production/approved_reference_plan_version-ref-payload-contract"),
	NodeTypeReferencePlanTarget:                productionNode("production/reference", productionEvidenceNone, []string{"reference_plan_set"}, "storygraph-production/reference-plan-target-payload-contract"),
	NodeTypeSceneReferenceBindingVersion:       productionNode("production/reference", productionEvidenceNone, []string{"reference_binding_set"}, "storygraph-production/scene-reference-binding-payload-contract"),
	NodeTypeInteractionReferenceBindingVersion: productionNode("production/reference", productionEvidenceNone, []string{"reference_binding_set"}, "storygraph-production/interaction-reference-binding-payload-contract"),
	NodeTypeShot:                               productionNode("production/storyboard", productionEvidenceNone, []string{"storyboard_formal_set"}, "storygraph-production/shot-payload-contract"),
	NodeTypeShotProductionBindingVersion:       productionNode("production/storyboard", productionEvidenceNone, []string{"storyboard_formal_set"}, "storygraph-production/shot-production-binding-payload-contract"),
	NodeTypeContinuityClaim: {
		ownerKind: "production/planning", versionFamilies: []string{"planning_scene_set"},
		discriminant: "claim_type", evidencePolicy: productionEvidenceOrCreatorDecision,
		payloadContracts: map[string]string{
			"continuity":  "storygraph-production/continuity-state-payload-contract",
			"interaction": "storygraph-production/continuity-interaction-payload-contract",
		},
	},
}

func productionNode(ownerKind, evidencePolicy string, families []string, payloadContract string) productionNodeDefinition {
	return productionNodeDefinition{
		ownerKind: ownerKind, versionFamilies: families, evidencePolicy: evidencePolicy,
		payloadContracts: map[string]string{"": payloadContract},
	}
}

// ProductionPayloadContract resolves the only payload contract declared by the
// production schema for a node and its optional union discriminant.
func ProductionPayloadContract(nodeType NodeType, discriminant string) (string, error) {
	definition, ok := productionNodeDefinitions[nodeType]
	if !ok {
		return "", errors.New("node type is excluded from the Production StoryGraph schema")
	}
	contract, ok := definition.payloadContracts[discriminant]
	if !ok || (definition.discriminant == "" && discriminant != "") {
		return "", errors.New("payload discriminant is not declared by the Production StoryGraph schema")
	}
	return contract, nil
}

func validateProductionNode(node Node) error {
	definition, ok := productionNodeDefinitions[node.NodeType]
	if !ok {
		return fmt.Errorf("Production StoryGraph node type %s is not allowed", node.NodeType)
	}
	if node.Label != "" || len(node.BusinessPosition) != 0 {
		return fmt.Errorf("Production StoryGraph node %s copies mutable Owner presentation", node.StoryNodeKey)
	}
	if node.OwnerRef.OwnerKind != definition.ownerKind || !slices.Contains(definition.versionFamilies, node.OwnerRef.VersionFamily) {
		return fmt.Errorf("Production StoryGraph node %s has an invalid Owner family", node.StoryNodeKey)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(node.Payload, &payload); err != nil || payload == nil {
		return errors.New("Production StoryGraph node payload is invalid")
	}
	var discriminant string
	if definition.discriminant != "" {
		if err := json.Unmarshal(payload[definition.discriminant], &discriminant); err != nil {
			return fmt.Errorf("Production StoryGraph node %s has an invalid payload discriminant", node.StoryNodeKey)
		}
	}
	expectedContract, err := ProductionPayloadContract(node.NodeType, discriminant)
	if err != nil {
		return err
	}
	var contractID, projectionHash string
	if json.Unmarshal(payload["payload_contract_id"], &contractID) != nil || contractID != expectedContract ||
		json.Unmarshal(payload["projection_hash"], &projectionHash) != nil {
		return fmt.Errorf("Production StoryGraph node %s does not match its payload contract", node.StoryNodeKey)
	}
	expectedProjectionHash := node.OwnerRef.OwnerContentHash
	if node.OwnerRef.FragmentKey != "" {
		expectedProjectionHash = node.OwnerRef.FragmentContentHash
	}
	if projectionHash != expectedProjectionHash {
		return fmt.Errorf("Production StoryGraph node %s projection hash differs from its Owner ref", node.StoryNodeKey)
	}
	if productionRefOnlyNode(node.NodeType) {
		var strictPayload productionProjectionPayload
		if err = decodeStrictObject(node.Payload, &strictPayload); err != nil {
			return fmt.Errorf("Production StoryGraph node %s copies Owner business content", node.StoryNodeKey)
		}
	}
	if oneOfNode(node.NodeType, NodeTypeWorldRule, NodeTypeStoryArc, NodeTypePlotThread) {
		var strictPayload productionAuditableBibleFactPayload
		if err = decodeStrictObject(node.Payload, &strictPayload); err != nil ||
			validateProductionAuditRef(node, strictPayload.CreatorDecisionRef) != nil {
			return fmt.Errorf("Production StoryGraph node %s has an invalid auditable Bible fact payload", node.StoryNodeKey)
		}
	}
	creatorDecision := payload["creator_decision_ref"]
	hasCreatorDecision := len(creatorDecision) > 0 && !bytes.Equal(bytes.TrimSpace(creatorDecision), []byte("null"))
	switch definition.evidencePolicy {
	case productionEvidenceNone, productionEvidenceRoot:
		if len(node.EvidenceRefs) != 0 || hasCreatorDecision {
			return fmt.Errorf("Production StoryGraph node %s forbids evidence", node.StoryNodeKey)
		}
	case productionEvidenceRequired:
		if len(node.EvidenceRefs) == 0 || hasCreatorDecision {
			return fmt.Errorf("Production StoryGraph node %s requires source evidence", node.StoryNodeKey)
		}
	case productionEvidenceOrCreatorDecision:
		if len(creatorDecision) == 0 || (len(node.EvidenceRefs) > 0) == hasCreatorDecision {
			return fmt.Errorf("Production StoryGraph node %s must select evidence or creator decision", node.StoryNodeKey)
		}
	default:
		return errors.New("Production StoryGraph evidence policy is invalid")
	}
	return nil
}

func productionRefOnlyNode(nodeType NodeType) bool {
	return slices.Contains([]NodeType{
		NodeTypeSourceRevision, NodeTypeSourceEvidence, NodeTypePolicySnapshot, NodeTypeEffectiveStyleSnapshot,
		NodeTypeEpisode, NodeTypeScene, NodeTypeDialogue, NodeTypeNarrativeBeat, NodeTypeArtifact,
		NodeTypeApprovedReferencePlanVersion,
	}, nodeType)
}
