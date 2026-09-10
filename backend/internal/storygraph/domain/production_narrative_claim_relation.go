package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var productionNarrativePredicatePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type productionNarrativeClaimPayload struct {
	productionProjectionPayload
	ClaimSeriesKey     string                   `json:"claim_series_key"`
	ClaimRevision      int64                    `json:"claim_revision"`
	Predicate          string                   `json:"predicate"`
	SubjectRef         productionOwnerNodeRef   `json:"subject_ref"`
	ObjectRef          json.RawMessage          `json:"object_ref"`
	ParticipantRefs    []productionOwnerNodeRef `json:"participant_refs"`
	AnchorRefs         []productionOwnerNodeRef `json:"anchor_refs"`
	ValidScope         ClaimScope               `json:"valid_scope"`
	StoryTimeRange     json.RawMessage          `json:"story_time_range"`
	Polarity           string                   `json:"polarity"`
	Status             string                   `json:"status"`
	CreatorDecisionRef json.RawMessage          `json:"creator_decision_ref"`
	SupersedesClaimRef json.RawMessage          `json:"supersedes_claim_ref"`
}

type productionNarrativeClaimResolution struct {
	node    Node
	payload productionNarrativeClaimPayload
}

func validateProductionNarrativeClaimRelations(nodes []Node, edges []Edge) error {
	refIndex := make(map[string][]Node, len(nodes))
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
	}

	claims := make(map[string]productionNarrativeClaimResolution)
	for _, node := range nodes {
		if !oneOfNode(node.NodeType, NodeTypeRelationshipClaim, NodeTypeForeshadowingClaim, NodeTypePayoffClaim, NodeTypeCausalClaim) {
			continue
		}
		var payload productionNarrativeClaimPayload
		if err := decodeStrictObject(node.Payload, &payload); err != nil ||
			!productionStableKey(payload.ClaimSeriesKey) || payload.ClaimRevision < 1 || payload.ClaimRevision > productionMaximumSafeInteger ||
			!productionNarrativePredicatePattern.MatchString(payload.Predicate) || payload.ParticipantRefs == nil || len(payload.AnchorRefs) == 0 ||
			validateSortedProductionRefs(payload.ParticipantRefs) != nil || validateSortedProductionRefs(payload.AnchorRefs) != nil ||
			!oneOf(payload.Polarity, "positive", "negative", "neutral") || !oneOf(payload.Status, "asserted", "negated") ||
			validateProductionStoryTimeRange(payload.StoryTimeRange) != nil || validateProductionAuditRef(node, payload.CreatorDecisionRef) != nil ||
			!validProductionClaimScope(payload.ValidScope, node, nodes) {
			return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid payload", node.StoryNodeKey)
		}
		claims[node.StoryNodeKey] = productionNarrativeClaimResolution{node: node, payload: payload}
	}

	expected := make(map[productionRelationEdge]struct{})
	for key, claim := range claims {
		seenParticipants := make(map[string]struct{})
		subject, err := resolveProductionNodeRef(refIndex, claim.payload.SubjectRef, NodeTypeAssetIdentity, NodeTypeWorldRule)
		if err != nil {
			return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid subject", key)
		}
		seenParticipants[subject.StoryNodeKey] = struct{}{}
		expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: subject.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "subject"}}] = struct{}{}
		if !bytes.Equal(bytes.TrimSpace(claim.payload.ObjectRef), []byte("null")) {
			var ref productionOwnerNodeRef
			if len(claim.payload.ObjectRef) == 0 || decodeStrictObject(claim.payload.ObjectRef, &ref) != nil {
				return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid object", key)
			}
			object, resolveErr := resolveProductionNodeRef(refIndex, ref, NodeTypeAssetIdentity, NodeTypeWorldRule)
			if resolveErr != nil || object.StoryNodeKey == subject.StoryNodeKey {
				return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid object", key)
			}
			seenParticipants[object.StoryNodeKey] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: object.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "object"}}] = struct{}{}
		}
		for _, ref := range claim.payload.ParticipantRefs {
			participant, resolveErr := resolveProductionNodeRef(refIndex, ref, NodeTypeAssetIdentity, NodeTypeWorldRule)
			if resolveErr != nil {
				return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid participant", key)
			}
			if _, duplicate := seenParticipants[participant.StoryNodeKey]; duplicate {
				return fmt.Errorf("Production StoryGraph Narrative Claim %s duplicates a participant", key)
			}
			seenParticipants[participant.StoryNodeKey] = struct{}{}
			expected[productionRelationEdge{edgeType: EdgeTypeClaimParticipant, from: participant.StoryNodeKey, to: key, qualifier: EdgeQualifier{ParticipantRole: "participant"}}] = struct{}{}
		}
		for _, ref := range claim.payload.AnchorRefs {
			anchor, resolveErr := resolveProductionNodeRef(refIndex, ref, NodeTypeEpisode, NodeTypeScene, NodeTypeNarrativeBeat)
			if resolveErr != nil {
				return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid anchor", key)
			}
			role := map[NodeType]string{NodeTypeEpisode: "episode", NodeTypeScene: "scene", NodeTypeNarrativeBeat: "beat"}[anchor.NodeType]
			expected[productionRelationEdge{edgeType: EdgeTypeClaimAnchor, from: anchor.StoryNodeKey, to: key, qualifier: EdgeQualifier{AnchorRole: role}}] = struct{}{}
		}
		if err := addExpectedNarrativeSupersession(expected, refIndex, claims, claim); err != nil {
			return err
		}
	}

	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		if _, ok := claims[edge.ToNodeKey]; !ok || !oneOfNodeTypeEdge(edge.EdgeType, EdgeTypeClaimParticipant, EdgeTypeClaimAnchor, EdgeTypeSupersedes) {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return fmt.Errorf("Production StoryGraph Narrative Claim has an unexpected %s edge", edge.EdgeType)
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Narrative Claim edges do not match its payload")
	}
	return nil
}

func addExpectedNarrativeSupersession(
	expected map[productionRelationEdge]struct{},
	refIndex map[string][]Node,
	claims map[string]productionNarrativeClaimResolution,
	claim productionNarrativeClaimResolution,
) error {
	if bytes.Equal(bytes.TrimSpace(claim.payload.SupersedesClaimRef), []byte("null")) {
		if claim.payload.ClaimRevision != 1 {
			return fmt.Errorf("Production StoryGraph Narrative Claim %s starts at an invalid revision", claim.node.StoryNodeKey)
		}
		return nil
	}
	var ref productionOwnerNodeRef
	if len(claim.payload.SupersedesClaimRef) == 0 || decodeStrictObject(claim.payload.SupersedesClaimRef, &ref) != nil {
		return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid superseded claim ref", claim.node.StoryNodeKey)
	}
	previous, err := resolveProductionNodeRef(refIndex, ref, claim.node.NodeType)
	previousClaim, ok := claims[previous.StoryNodeKey]
	if err != nil || !ok || previousClaim.node.NodeType != claim.node.NodeType || previousClaim.payload.ClaimSeriesKey != claim.payload.ClaimSeriesKey ||
		previousClaim.payload.ClaimRevision+1 != claim.payload.ClaimRevision || previousClaim.payload.ValidScope != claim.payload.ValidScope {
		return fmt.Errorf("Production StoryGraph Narrative Claim %s has an invalid supersession", claim.node.StoryNodeKey)
	}
	expected[productionRelationEdge{edgeType: EdgeTypeSupersedes, from: previous.StoryNodeKey, to: claim.node.StoryNodeKey}] = struct{}{}
	return nil
}
