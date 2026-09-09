package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type productionEvidenceEdge struct {
	edgeType EdgeType
	from     string
	to       string
}

func validateProductionEvidenceRelations(nodes []Node, edges []Edge) error {
	sources := make(map[string]string)
	evidenceNodes := make(map[EvidenceRef]string)
	for _, node := range nodes {
		switch node.NodeType {
		case NodeTypeSourceRevision:
			if _, exists := sources[node.OwnerRef.OwnerVersionID]; exists {
				return errors.New("Production StoryGraph has duplicate Source Revision identity")
			}
			sources[node.OwnerRef.OwnerVersionID] = node.StoryNodeKey
		case NodeTypeSourceEvidence:
			identity, err := productionEvidenceIdentity(node.OwnerRef.FragmentKey, node.OwnerRef.FragmentContentHash)
			if err != nil {
				return err
			}
			if _, exists := evidenceNodes[identity]; exists {
				return errors.New("Production StoryGraph has duplicate Evidence identity")
			}
			evidenceNodes[identity] = node.StoryNodeKey
		}
	}

	expected := make(map[productionEvidenceEdge]struct{})
	for identity, evidenceNodeKey := range evidenceNodes {
		sourceNodeKey := sources[identity.DocumentRevisionID]
		if sourceNodeKey == "" {
			return errors.New("Production StoryGraph Evidence is outside the Source Revision set")
		}
		expected[productionEvidenceEdge{edgeType: EdgeTypeDerivedFrom, from: sourceNodeKey, to: evidenceNodeKey}] = struct{}{}
	}
	for _, node := range nodes {
		if node.NodeType == NodeTypeSourceEvidence {
			continue
		}
		edgeType := EdgeTypeDerivedFrom
		if productionClaimNode(node.NodeType) {
			edgeType = EdgeTypeSupports
		}
		for _, evidence := range node.EvidenceRefs {
			evidenceNodeKey := evidenceNodes[evidence]
			if evidenceNodeKey == "" {
				return fmt.Errorf("Production StoryGraph node %s references missing Evidence", node.StoryNodeKey)
			}
			expected[productionEvidenceEdge{edgeType: edgeType, from: evidenceNodeKey, to: node.StoryNodeKey}] = struct{}{}
		}
	}

	observed := make(map[productionEvidenceEdge]struct{})
	for _, edge := range edges {
		if edge.EdgeType != EdgeTypeDerivedFrom && edge.EdgeType != EdgeTypeSupports {
			continue
		}
		identity := productionEvidenceEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey}
		if _, ok := expected[identity]; !ok {
			return fmt.Errorf("Production StoryGraph has an unexpected %s Evidence edge", edge.EdgeType)
		}
		observed[identity] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Evidence edges do not match Node evidence refs")
	}
	return nil
}

func productionEvidenceIdentity(fragmentKey, fragmentHash string) (EvidenceRef, error) {
	parts := strings.Split(fragmentKey, ":")
	if len(parts) != 5 || parts[0] != "source-evidence" || len(parts[2]) != 12 || len(parts[3]) != 12 || !hashPattern.MatchString(fragmentHash) {
		return EvidenceRef{}, errors.New("Production StoryGraph Evidence fragment identity is invalid")
	}
	documentRevisionID, err := uuid.Parse(parts[1])
	if err != nil || documentRevisionID.String() != parts[1] {
		return EvidenceRef{}, errors.New("Production StoryGraph Evidence fragment identity is invalid")
	}
	start, startErr := strconv.Atoi(parts[2])
	end, endErr := strconv.Atoi(parts[3])
	identity := EvidenceRef{DocumentRevisionID: parts[1], AbsoluteStart: start, AbsoluteEnd: end, TextHash: parts[4]}
	if startErr != nil || endErr != nil || parts[4] != fragmentHash || identity.Validate() != nil {
		return EvidenceRef{}, errors.New("Production StoryGraph Evidence fragment identity is invalid")
	}
	return identity, nil
}

func productionClaimNode(nodeType NodeType) bool {
	return nodeType == NodeTypeRelationshipClaim || nodeType == NodeTypeForeshadowingClaim ||
		nodeType == NodeTypePayoffClaim || nodeType == NodeTypeContinuityClaim || nodeType == NodeTypeCausalClaim
}
