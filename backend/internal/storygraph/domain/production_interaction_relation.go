package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type productionPositiveRational struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

type productionInteractionPayload struct {
	productionProjectionPayload
	ClaimType                 string                      `json:"claim_type"`
	ClaimSeriesKey            string                      `json:"claim_series_key"`
	ClaimRevision             int64                       `json:"claim_revision"`
	Predicate                 string                      `json:"predicate"`
	ActorOccurrenceRefs       []productionOwnerNodeRef    `json:"actor_occurrence_refs"`
	PropOccurrenceRef         productionOwnerNodeRef      `json:"prop_occurrence_ref"`
	CounterpartyOccurrenceRef *productionOwnerNodeRef     `json:"counterparty_occurrence_ref,omitempty"`
	SceneRef                  productionOwnerNodeRef      `json:"scene_ref"`
	BeatRef                   *productionOwnerNodeRef     `json:"beat_ref,omitempty"`
	ValidScope                ClaimScope                  `json:"valid_scope"`
	StoryTime                 string                      `json:"story_time"`
	Status                    string                      `json:"status"`
	Hand                      string                      `json:"hand"`
	GripType                  *string                     `json:"grip_type,omitempty"`
	ContactPoint              *string                     `json:"contact_point,omitempty"`
	Direction                 *string                     `json:"direction,omitempty"`
	RelativeScale             *productionPositiveRational `json:"relative_scale,omitempty"`
	HolderBefore              json.RawMessage             `json:"holder_before"`
	HolderAfter               json.RawMessage             `json:"holder_after"`
	PropStateBefore           productionOwnerNodeRef      `json:"prop_state_before"`
	PropStateAfter            productionOwnerNodeRef      `json:"prop_state_after"`
	CreatorDecisionRef        json.RawMessage             `json:"creator_decision_ref"`
	SupersedesClaimRef        json.RawMessage             `json:"supersedes_claim_ref"`
}

type productionInteractionResolution struct {
	node    Node
	payload productionInteractionPayload
}

type productionOccurrenceResolution struct {
	node  Node
	asset Node
	state Node
	scene Node
}

func validateProductionInteractionRelations(nodes []Node, edges []Edge) error {
	refIndex := make(map[string][]Node, len(nodes))
	assetKinds := make(map[string]string)
	stateOwner := make(map[string]string)
	structuralParent := make(map[string]string)
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
		if node.NodeType == NodeTypeAssetIdentity {
			var payload productionAuditableAssetPayload
			if decodeStrictObject(node.Payload, &payload) == nil {
				assetKinds[node.StoryNodeKey] = payload.AssetKind
			}
		}
	}
	for _, edge := range edges {
		if edge.EdgeType == EdgeTypeHasState {
			stateOwner[edge.ToNodeKey] = edge.FromNodeKey
		}
		if edge.EdgeType == EdgeTypeContains {
			structuralParent[edge.ToNodeKey] = edge.FromNodeKey
		}
	}

	occurrences := make(map[string]productionOccurrenceResolution)
	for _, node := range nodes {
		if node.NodeType != NodeTypeOccurrence {
			continue
		}
		var payload productionOccurrencePayload
		if decodeStrictObject(node.Payload, &payload) != nil {
			return fmt.Errorf("Production StoryGraph Occurrence %s has an invalid payload", node.StoryNodeKey)
		}
		asset, assetErr := resolveProductionNodeRef(refIndex, payload.AssetIdentityRef, NodeTypeAssetIdentity)
		state, stateErr := resolveProductionNodeRef(refIndex, payload.AssetStateRef, NodeTypeAssetState)
		scene, sceneErr := resolveProductionNodeRef(refIndex, payload.SceneRef, NodeTypeScene)
		if assetErr != nil || stateErr != nil || sceneErr != nil {
			return fmt.Errorf("Production StoryGraph Occurrence %s cannot resolve its interaction identity", node.StoryNodeKey)
		}
		occurrences[node.StoryNodeKey] = productionOccurrenceResolution{node: node, asset: asset, state: state, scene: scene}
	}

	claims := make(map[string]productionInteractionResolution)
	for _, node := range nodes {
		if node.NodeType != NodeTypeContinuityClaim {
			continue
		}
		var discriminant struct {
			ClaimType string `json:"claim_type"`
		}
		if json.Unmarshal(node.Payload, &discriminant) != nil || discriminant.ClaimType != "interaction" {
			continue
		}
		var payload productionInteractionPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil ||
			payload.ClaimType != "interaction" || !productionStableKey(payload.ClaimSeriesKey) ||
			payload.ClaimRevision < 1 || payload.ClaimRevision > productionMaximumSafeInteger ||
			!productionInteractionPredicate(payload.Predicate) || payload.ActorOccurrenceRefs == nil || len(payload.ActorOccurrenceRefs) == 0 ||
			!productionStableKey(payload.StoryTime) || payload.Status != "asserted" ||
			!oneOf(payload.Hand, "left", "right", "both", "unspecified") ||
			!validProductionInteractionDescriptors(payload) || validateSortedProductionRefs(payload.ActorOccurrenceRefs) != nil ||
			validateProductionAuditRef(node, payload.CreatorDecisionRef) != nil || !validProductionClaimScope(payload.ValidScope, node, nodes) {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid payload", node.StoryNodeKey)
		}
		claims[node.StoryNodeKey] = productionInteractionResolution{node: node, payload: payload}
	}

	expected := make(map[productionRelationEdge]struct{})
	for key, claim := range claims {
		scene, sceneErr := resolveProductionNodeRef(refIndex, claim.payload.SceneRef, NodeTypeScene)
		propOccurrenceNode, propErr := resolveProductionNodeRef(refIndex, claim.payload.PropOccurrenceRef, NodeTypeOccurrence)
		propOccurrence, propExists := occurrences[propOccurrenceNode.StoryNodeKey]
		beforeState, beforeErr := resolveProductionNodeRef(refIndex, claim.payload.PropStateBefore, NodeTypeAssetState)
		afterState, afterErr := resolveProductionNodeRef(refIndex, claim.payload.PropStateAfter, NodeTypeAssetState)
		if sceneErr != nil || propErr != nil || !propExists || beforeErr != nil || afterErr != nil ||
			assetKinds[propOccurrence.asset.StoryNodeKey] != "prop" || propOccurrence.scene.StoryNodeKey != scene.StoryNodeKey ||
			stateOwner[beforeState.StoryNodeKey] != propOccurrence.asset.StoryNodeKey || stateOwner[afterState.StoryNodeKey] != propOccurrence.asset.StoryNodeKey ||
			claim.payload.ValidScope.Kind != "scene" || claim.payload.ValidScope.OwnerLogicalID != scene.OwnerRef.OwnerLogicalID {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s does not resolve one Prop timeline", key)
		}

		actorAssets := make(map[string]struct{}, len(claim.payload.ActorOccurrenceRefs))
		for _, ref := range claim.payload.ActorOccurrenceRefs {
			actorNode, err := resolveProductionNodeRef(refIndex, ref, NodeTypeOccurrence)
			actor, ok := occurrences[actorNode.StoryNodeKey]
			if err != nil || !ok || assetKinds[actor.asset.StoryNodeKey] != "character" || actor.scene.StoryNodeKey != scene.StoryNodeKey {
				return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid actor", key)
			}
			actorAssets[actor.asset.StoryNodeKey] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: actor.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "actor"}}] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: actor.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "character_occurrence"}}] = struct{}{}
		}

		participantAssets := make(map[string]struct{}, len(actorAssets)+1)
		for asset := range actorAssets {
			participantAssets[asset] = struct{}{}
		}
		var counterparty *productionOccurrenceResolution
		if claim.payload.CounterpartyOccurrenceRef != nil {
			counterpartyNode, err := resolveProductionNodeRef(refIndex, *claim.payload.CounterpartyOccurrenceRef, NodeTypeOccurrence)
			value, ok := occurrences[counterpartyNode.StoryNodeKey]
			if err != nil || !ok || assetKinds[value.asset.StoryNodeKey] != "character" || value.scene.StoryNodeKey != scene.StoryNodeKey {
				return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid counterparty", key)
			}
			if _, duplicateActor := actorAssets[value.asset.StoryNodeKey]; duplicateActor {
				return fmt.Errorf("Production StoryGraph Interaction Claim %s reuses its actor as counterparty", key)
			}
			counterparty = &value
			participantAssets[value.asset.StoryNodeKey] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: value.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "counterparty"}}] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: value.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "character_occurrence"}}] = struct{}{}
		}
		transfer := claim.payload.Predicate == "give" || claim.payload.Predicate == "receive"
		if transfer != (counterparty != nil) {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid transfer participant set", key)
		}

		holderBefore, hasHolderBefore, err := resolveNullableProductionHolder(refIndex, claim.payload.HolderBefore)
		if err != nil {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid before holder", key)
		}
		holderAfter, hasHolderAfter, err := resolveNullableProductionHolder(refIndex, claim.payload.HolderAfter)
		if err != nil {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid after holder", key)
		}
		for _, holder := range []struct {
			node Node
			has  bool
			role string
		}{{holderBefore, hasHolderBefore, "holder_before"}, {holderAfter, hasHolderAfter, "holder_after"}} {
			if !holder.has {
				continue
			}
			if assetKinds[holder.node.StoryNodeKey] != "character" {
				return fmt.Errorf("Production StoryGraph Interaction Claim %s holder is not a Character", key)
			}
			if _, participant := participantAssets[holder.node.StoryNodeKey]; !participant {
				return fmt.Errorf("Production StoryGraph Interaction Claim %s holder is outside participants", key)
			}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: holder.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: holder.role}}] = struct{}{}
		}
		if !validProductionInteractionTransition(claim.payload.Predicate, actorAssets, counterparty, holderBefore, hasHolderBefore, holderAfter, hasHolderAfter, beforeState.StoryNodeKey != afterState.StoryNodeKey) {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s violates its holder or state transition", key)
		}

		expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: propOccurrence.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "prop"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: scene.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "scene"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: propOccurrence.node.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "prop_occurrence"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimState, from: beforeState.StoryNodeKey, to: key, qualifier: EdgeQualifier{StateRole: "prop_before"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimState, from: afterState.StoryNodeKey, to: key, qualifier: EdgeQualifier{StateRole: "prop_after"}}] = struct{}{}
		if claim.payload.BeatRef != nil {
			beat, err := resolveProductionNodeRef(refIndex, *claim.payload.BeatRef, NodeTypeNarrativeBeat)
			if err != nil || structuralParent[beat.StoryNodeKey] != scene.StoryNodeKey {
				return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid Beat", key)
			}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: beat.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "beat"}}] = struct{}{}
		}
		if err := addExpectedInteractionSupersession(expected, refIndex, claims, claim); err != nil {
			return err
		}
	}

	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		if _, ok := claims[edge.ToNodeKey]; !ok || !oneOfNodeTypeEdge(edge.EdgeType, EdgeTypeClaimParticipant, EdgeTypeClaimState, EdgeTypeClaimAnchor, EdgeTypeSupersedes) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph Interaction Claim has an unexpected %s edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Interaction Claim edges do not match its payload")
	}
	return nil
}

func resolveNullableProductionHolder(index map[string][]Node, raw json.RawMessage) (Node, bool, error) {
	if len(raw) == 0 {
		return Node{}, false, errors.New("Production StoryGraph Interaction holder is required")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Node{}, false, nil
	}
	var ref productionOwnerNodeRef
	if decodeStrictObject(raw, &ref) != nil {
		return Node{}, false, errors.New("Production StoryGraph Interaction holder ref is invalid")
	}
	node, err := resolveProductionNodeRef(index, ref, NodeTypeAssetIdentity)
	return node, err == nil, err
}

func validProductionInteractionDescriptors(value productionInteractionPayload) bool {
	for _, descriptor := range []*string{value.GripType, value.ContactPoint, value.Direction} {
		if descriptor != nil && (!productionStableKey(*descriptor) || strings.TrimSpace(*descriptor) != *descriptor) {
			return false
		}
	}
	if value.RelativeScale == nil {
		return true
	}
	return value.RelativeScale.Numerator > 0 && value.RelativeScale.Denominator > 0 &&
		value.RelativeScale.Numerator <= productionMaximumSafeInteger && value.RelativeScale.Denominator <= productionMaximumSafeInteger &&
		productionGreatestCommonDivisor(value.RelativeScale.Numerator, value.RelativeScale.Denominator) == 1
}

func productionGreatestCommonDivisor(left, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	return left
}

func productionInteractionPredicate(value string) bool {
	return oneOf(value, "hold", "carry", "wear", "use", "give", "receive", "place", "drop", "open", "break")
}

func validProductionInteractionTransition(
	predicate string,
	actorAssets map[string]struct{},
	counterparty *productionOccurrenceResolution,
	holderBefore Node,
	hasHolderBefore bool,
	holderAfter Node,
	hasHolderAfter bool,
	stateChanged bool,
) bool {
	isActor := func(holder Node, exists bool) bool {
		if !exists {
			return false
		}
		_, ok := actorAssets[holder.StoryNodeKey]
		return ok
	}
	sameHolder := hasHolderBefore == hasHolderAfter && (!hasHolderBefore || holderBefore.StoryNodeKey == holderAfter.StoryNodeKey)
	switch predicate {
	case "hold":
		return (!hasHolderBefore || isActor(holderBefore, true)) && isActor(holderAfter, hasHolderAfter) &&
			(!hasHolderBefore || holderBefore.StoryNodeKey == holderAfter.StoryNodeKey)
	case "carry":
		return isActor(holderBefore, hasHolderBefore) && isActor(holderAfter, hasHolderAfter) && sameHolder
	case "wear":
		return (!hasHolderBefore || isActor(holderBefore, true)) && isActor(holderAfter, hasHolderAfter) &&
			(!hasHolderBefore || holderBefore.StoryNodeKey == holderAfter.StoryNodeKey) && (hasHolderBefore || stateChanged)
	case "use":
		return sameHolder
	case "give":
		return counterparty != nil && isActor(holderBefore, hasHolderBefore) && hasHolderAfter && holderAfter.StoryNodeKey == counterparty.asset.StoryNodeKey
	case "receive":
		return counterparty != nil && hasHolderBefore && holderBefore.StoryNodeKey == counterparty.asset.StoryNodeKey && isActor(holderAfter, hasHolderAfter)
	case "place", "drop":
		return isActor(holderBefore, hasHolderBefore) && !hasHolderAfter && stateChanged
	case "open", "break":
		return sameHolder && stateChanged
	default:
		return false
	}
}

func addExpectedInteractionSupersession(
	expected map[productionRelationEdge]struct{},
	refIndex map[string][]Node,
	claims map[string]productionInteractionResolution,
	claim productionInteractionResolution,
) error {
	if bytes.Equal(bytes.TrimSpace(claim.payload.SupersedesClaimRef), []byte("null")) {
		if claim.payload.ClaimRevision != 1 {
			return fmt.Errorf("Production StoryGraph Interaction Claim %s starts at an invalid revision", claim.node.StoryNodeKey)
		}
		return nil
	}
	var ref productionOwnerNodeRef
	if decodeStrictObject(claim.payload.SupersedesClaimRef, &ref) != nil {
		return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid superseded claim ref", claim.node.StoryNodeKey)
	}
	previous, err := resolveProductionNodeRef(refIndex, ref, NodeTypeContinuityClaim)
	previousClaim, ok := claims[previous.StoryNodeKey]
	if err != nil || !ok || previousClaim.payload.ClaimSeriesKey != claim.payload.ClaimSeriesKey || previousClaim.payload.ClaimRevision+1 != claim.payload.ClaimRevision ||
		previousClaim.payload.ValidScope != claim.payload.ValidScope {
		return fmt.Errorf("Production StoryGraph Interaction Claim %s has an invalid supersession", claim.node.StoryNodeKey)
	}
	expected[productionRelationEdge{edgeType: EdgeTypeSupersedes, from: previous.StoryNodeKey, to: claim.node.StoryNodeKey}] = struct{}{}
	return nil
}
