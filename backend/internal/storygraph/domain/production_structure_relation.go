package domain

import (
	"errors"
	"fmt"
	"slices"
)

type productionOrderedNode struct {
	key      string
	sequence string
}

func validateProductionStructureRelations(nodes []Node, edges []Edge) error {
	nodeByKey := make(map[string]Node, len(nodes))
	parentCounts := make(map[string]int)
	groups := make(map[string][]productionOrderedNode)
	for _, node := range nodes {
		nodeByKey[node.StoryNodeKey] = node
	}
	for _, edge := range edges {
		if edge.EdgeType != EdgeTypeContains {
			continue
		}
		parent, parentOK := nodeByKey[edge.FromNodeKey]
		child, childOK := nodeByKey[edge.ToNodeKey]
		if !parentOK || !childOK || !productionStableKey(edge.Qualifier.SequenceKey) {
			return errors.New("Production StoryGraph contains edge has an invalid sequence")
		}
		parentCounts[child.StoryNodeKey]++
		groupKey := parent.StoryNodeKey + "\x00" + string(child.NodeType)
		groups[groupKey] = append(groups[groupKey], productionOrderedNode{key: child.StoryNodeKey, sequence: edge.Qualifier.SequenceKey})
	}
	for _, node := range nodes {
		if oneOfNode(node.NodeType, NodeTypeScene, NodeTypeDialogue, NodeTypeNarrativeBeat) && parentCounts[node.StoryNodeKey] != 1 {
			return fmt.Errorf("Production StoryGraph node %s does not have exactly one structural parent", node.StoryNodeKey)
		}
	}

	expected := make(map[productionRelationEdge]struct{})
	for _, group := range groups {
		slices.SortFunc(group, compareProductionOrderedNodes)
		for index := 1; index < len(group); index++ {
			if group[index-1].sequence == group[index].sequence {
				return errors.New("Production StoryGraph siblings have duplicate sequence keys")
			}
			childType := nodeByKey[group[index].key].NodeType
			if childType != NodeTypeScene && childType != NodeTypeNarrativeBeat {
				continue
			}
			expected[productionRelationEdge{
				edgeType: EdgeTypePrecedes, from: group[index-1].key, to: group[index].key,
				qualifier: EdgeQualifier{SequenceKey: group[index].sequence},
			}] = struct{}{}
		}
	}

	observed := make(map[productionRelationEdge]struct{})
	var episodeEdges []Edge
	for _, edge := range edges {
		if edge.EdgeType != EdgeTypePrecedes {
			continue
		}
		from, to := nodeByKey[edge.FromNodeKey], nodeByKey[edge.ToNodeKey]
		if from.NodeType == NodeTypeEpisode && to.NodeType == NodeTypeEpisode {
			episodeEdges = append(episodeEdges, edge)
			continue
		}
		if from.NodeType == NodeTypeShot && to.NodeType == NodeTypeShot {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return errors.New("Production StoryGraph has an unexpected structural precedes edge")
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph structural precedes edges do not match sibling order")
	}
	return validateProductionEpisodeChain(nodes, episodeEdges)
}

func compareProductionOrderedNodes(left, right productionOrderedNode) int {
	if left.sequence < right.sequence {
		return -1
	}
	if left.sequence > right.sequence {
		return 1
	}
	return 0
}

func validateProductionEpisodeChain(nodes []Node, edges []Edge) error {
	episodes := make(map[string]struct{})
	for _, node := range nodes {
		if node.NodeType == NodeTypeEpisode {
			episodes[node.StoryNodeKey] = struct{}{}
		}
	}
	if len(episodes) <= 1 {
		if len(edges) != 0 {
			return errors.New("Production StoryGraph single Episode has a precedes edge")
		}
		return nil
	}
	if len(edges) != len(episodes)-1 {
		return errors.New("Production StoryGraph Episode order is not one complete chain")
	}
	incoming := make(map[string]string)
	outgoing := make(map[string]Edge)
	sequenceKeys := make(map[string]struct{})
	for _, edge := range edges {
		if !productionStableKey(edge.Qualifier.SequenceKey) || incoming[edge.ToNodeKey] != "" || outgoing[edge.FromNodeKey].ToNodeKey != "" {
			return errors.New("Production StoryGraph Episode order branches")
		}
		if _, duplicate := sequenceKeys[edge.Qualifier.SequenceKey]; duplicate {
			return errors.New("Production StoryGraph Episode order has duplicate sequence keys")
		}
		incoming[edge.ToNodeKey] = edge.FromNodeKey
		outgoing[edge.FromNodeKey] = edge
		sequenceKeys[edge.Qualifier.SequenceKey] = struct{}{}
	}
	start := ""
	for key := range episodes {
		if incoming[key] == "" {
			if start != "" {
				return errors.New("Production StoryGraph Episode order has multiple starts")
			}
			start = key
		}
	}
	visited := make(map[string]struct{}, len(episodes))
	previousSequence := ""
	for current := start; current != ""; {
		visited[current] = struct{}{}
		edge := outgoing[current]
		if edge.ToNodeKey == "" {
			break
		}
		if previousSequence != "" && previousSequence >= edge.Qualifier.SequenceKey {
			return errors.New("Production StoryGraph Episode order is not strictly increasing")
		}
		previousSequence = edge.Qualifier.SequenceKey
		current = edge.ToNodeKey
	}
	if len(visited) != len(episodes) {
		return errors.New("Production StoryGraph Episode order does not cover every Episode")
	}
	return nil
}
