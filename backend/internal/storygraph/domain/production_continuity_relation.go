package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type productionContinuityPayload struct {
	productionProjectionPayload
	ClaimType          string                 `json:"claim_type"`
	ClaimSeriesKey     string                 `json:"claim_series_key"`
	ClaimRevision      int64                  `json:"claim_revision"`
	Predicate          string                 `json:"predicate"`
	Subject            productionOwnerNodeRef `json:"subject"`
	StateBefore        productionOwnerNodeRef `json:"state_before"`
	StateAfter         productionOwnerNodeRef `json:"state_after"`
	AnchorStart        productionOwnerNodeRef `json:"anchor_start"`
	AnchorEnd          productionOwnerNodeRef `json:"anchor_end"`
	ValidScope         ClaimScope             `json:"valid_scope"`
	StoryTimeStart     string                 `json:"story_time_start"`
	StoryTimeEnd       string                 `json:"story_time_end"`
	Status             string                 `json:"status"`
	CreatorDecisionRef json.RawMessage        `json:"creator_decision_ref"`
	SupersedesClaimRef json.RawMessage        `json:"supersedes_claim_ref"`
}

type productionContinuityResolution struct {
	node    Node
	payload productionContinuityPayload
}

func validateProductionContinuityRelations(nodes []Node, edges []Edge) error {
	refIndex := make(map[string][]Node, len(nodes))
	nodeByKey := make(map[string]Node, len(nodes))
	stateOwner := make(map[string]string)
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
	for _, edge := range edges {
		if edge.EdgeType == EdgeTypeHasState {
			stateOwner[edge.ToNodeKey] = edge.FromNodeKey
		}
	}

	claims := make(map[string]productionContinuityResolution)
	for _, node := range nodes {
		if node.NodeType != NodeTypeContinuityClaim {
			continue
		}
		var discriminant struct {
			ClaimType string `json:"claim_type"`
		}
		if json.Unmarshal(node.Payload, &discriminant) != nil || discriminant.ClaimType != "continuity" {
			continue
		}
		var payload productionContinuityPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil ||
			payload.ClaimType != "continuity" || !productionStableKey(payload.ClaimSeriesKey) ||
			payload.ClaimRevision < 1 || payload.ClaimRevision > productionMaximumSafeInteger ||
			(payload.Predicate != "state_persists" && payload.Predicate != "state_changes") ||
			(payload.Status != "asserted" && payload.Status != "negated") ||
			!productionStableKey(payload.StoryTimeStart) || !productionStableKey(payload.StoryTimeEnd) ||
			payload.StoryTimeStart >= payload.StoryTimeEnd ||
			validateProductionAuditRef(node, payload.CreatorDecisionRef) != nil ||
			!validProductionClaimScope(payload.ValidScope, node, nodes) {
			return fmt.Errorf("Production StoryGraph Continuity Claim %s has an invalid payload", node.StoryNodeKey)
		}
		claims[node.StoryNodeKey] = productionContinuityResolution{node: node, payload: payload}
	}

	expected := make(map[productionRelationEdge]struct{})
	for key, claim := range claims {
		subject, subjectErr := resolveProductionNodeRef(refIndex, claim.payload.Subject, NodeTypeAssetIdentity)
		before, beforeErr := resolveProductionNodeRef(refIndex, claim.payload.StateBefore, NodeTypeAssetState)
		after, afterErr := resolveProductionNodeRef(refIndex, claim.payload.StateAfter, NodeTypeAssetState)
		start, startErr := resolveProductionNodeRef(refIndex, claim.payload.AnchorStart, NodeTypeEpisode, NodeTypeScene, NodeTypeNarrativeBeat, NodeTypeOccurrence)
		end, endErr := resolveProductionNodeRef(refIndex, claim.payload.AnchorEnd, NodeTypeEpisode, NodeTypeScene, NodeTypeNarrativeBeat, NodeTypeOccurrence)
		if subjectErr != nil || beforeErr != nil || afterErr != nil || startErr != nil || endErr != nil ||
			stateOwner[before.StoryNodeKey] != subject.StoryNodeKey || stateOwner[after.StoryNodeKey] != subject.StoryNodeKey {
			return fmt.Errorf("Production StoryGraph Continuity Claim %s does not resolve one subject timeline", key)
		}
		if claim.payload.Predicate == "state_persists" && before.StoryNodeKey != after.StoryNodeKey ||
			claim.payload.Predicate == "state_changes" && before.StoryNodeKey == after.StoryNodeKey {
			return fmt.Errorf("Production StoryGraph Continuity Claim %s violates its state transition", key)
		}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: subject.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "subject"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimState, from: before.StoryNodeKey, to: key, qualifier: EdgeQualifier{StateRole: "before"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimState, from: after.StoryNodeKey, to: key, qualifier: EdgeQualifier{StateRole: "after"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: start.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "scope_start"}}] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: end.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: "scope_end"}}] = struct{}{}

		if !bytes.Equal(bytes.TrimSpace(claim.payload.SupersedesClaimRef), []byte("null")) {
			var ref productionOwnerNodeRef
			if decodeStrictObject(claim.payload.SupersedesClaimRef, &ref) != nil {
				return fmt.Errorf("Production StoryGraph Continuity Claim %s has an invalid superseded claim ref", key)
			}
			previous, err := resolveProductionNodeRef(refIndex, ref, NodeTypeContinuityClaim)
			previousClaim, ok := claims[previous.StoryNodeKey]
			if err != nil || !ok || previousClaim.payload.ClaimSeriesKey != claim.payload.ClaimSeriesKey || previousClaim.payload.ClaimRevision+1 != claim.payload.ClaimRevision {
				return fmt.Errorf("Production StoryGraph Continuity Claim %s has an invalid supersession", key)
			}
			expected[productionRelationEdge{edgeType: EdgeTypeSupersedes, from: previous.StoryNodeKey, to: key}] = struct{}{}
		} else if claim.payload.ClaimRevision != 1 {
			return fmt.Errorf("Production StoryGraph Continuity Claim %s starts at an invalid revision", key)
		}
	}

	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		if _, ok := claims[edge.ToNodeKey]; !ok || !oneOfNodeTypeEdge(edge.EdgeType, EdgeTypeClaimParticipant, EdgeTypeClaimState, EdgeTypeClaimAnchor, EdgeTypeSupersedes) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph Continuity Claim has an unexpected %s edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Continuity Claim edges do not match its payload")
	}
	return nil
}

func validProductionClaimScope(scope ClaimScope, node Node, nodes []Node) bool {
	if !productionStableKey(scope.OwnerLogicalID) {
		return false
	}
	switch scope.Kind {
	case "project":
		return scope.OwnerLogicalID == node.OwnerRef.ProjectID
	case "episode", "scene", "beat":
		expectedType := map[string]NodeType{"episode": NodeTypeEpisode, "scene": NodeTypeScene, "beat": NodeTypeNarrativeBeat}[scope.Kind]
		for _, candidate := range nodes {
			if candidate.NodeType == expectedType && candidate.OwnerRef.OwnerLogicalID == scope.OwnerLogicalID {
				return true
			}
		}
	}
	return false
}

func oneOfNodeTypeEdge(value EdgeType, candidates ...EdgeType) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}
