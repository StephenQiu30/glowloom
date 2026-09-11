package domain

import (
	"errors"
	"fmt"
)

type productionShotPayload struct {
	productionProjectionPayload
	SourceBeatRefs []productionOwnerNodeRef `json:"source_beat_refs"`
}

type productionShotResolution struct {
	node     Node
	payload  productionShotPayload
	beats    []Node
	sceneKey string
}

func validateProductionShotRelations(nodes []Node, edges []Edge) error {
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

	expectedRealizes := make(map[productionRelationEdge]struct{})
	shotScenes := make(map[string]string)
	shotsByScene := make(map[string][]string)
	for _, node := range nodes {
		if node.NodeType != NodeTypeShot {
			continue
		}
		resolution, err := resolveProductionShot(node, refIndex, beatScenes)
		if err != nil {
			return err
		}
		shotScenes[node.StoryNodeKey] = resolution.sceneKey
		shotsByScene[resolution.sceneKey] = append(shotsByScene[resolution.sceneKey], node.StoryNodeKey)
		for _, beat := range resolution.beats {
			expectedRealizes[productionRelationEdge{edgeType: EdgeTypeRealizes, from: beat.StoryNodeKey, to: node.StoryNodeKey}] = struct{}{}
		}
	}
	if err := validateProductionShotRealizesEdges(edges, nodeByKey, expectedRealizes); err != nil {
		return err
	}
	return validateProductionShotOrder(edges, nodeByKey, shotScenes, shotsByScene)
}

func resolveProductionShot(node Node, refIndex map[string][]Node, beatScenes map[string][]string) (productionShotResolution, error) {
	var payload productionShotPayload
	if err := decodeStrictObject(node.Payload, &payload); err != nil || len(payload.SourceBeatRefs) == 0 || validateSortedProductionRefs(payload.SourceBeatRefs) != nil {
		return productionShotResolution{}, fmt.Errorf("Production StoryGraph Shot %s has an invalid payload", node.StoryNodeKey)
	}
	beats, err := resolveProductionNodeRefs(refIndex, payload.SourceBeatRefs, NodeTypeNarrativeBeat)
	if err != nil {
		return productionShotResolution{}, fmt.Errorf("Production StoryGraph Shot %s has invalid source Beats", node.StoryNodeKey)
	}
	sceneKey := ""
	for _, beat := range beats {
		parents := beatScenes[beat.StoryNodeKey]
		if len(parents) != 1 || (sceneKey != "" && sceneKey != parents[0]) {
			return productionShotResolution{}, fmt.Errorf("Production StoryGraph Shot %s source Beats do not share one Scene", node.StoryNodeKey)
		}
		sceneKey = parents[0]
	}
	return productionShotResolution{node: node, payload: payload, beats: beats, sceneKey: sceneKey}, nil
}

func validateProductionShotRealizesEdges(edges []Edge, nodeByKey map[string]Node, expected map[productionRelationEdge]struct{}) error {
	observed := make(map[productionRelationEdge]struct{})
	for _, edge := range edges {
		to, ok := nodeByKey[edge.ToNodeKey]
		if !ok || to.NodeType != NodeTypeShot || edge.EdgeType != EdgeTypeRealizes {
			continue
		}
		relation := productionRelationEdge{edgeType: edge.EdgeType, from: edge.FromNodeKey, to: edge.ToNodeKey, qualifier: edge.Qualifier}
		if _, ok := expected[relation]; !ok {
			return errors.New("Production StoryGraph Shot has an unexpected realizes edge")
		}
		observed[relation] = struct{}{}
	}
	if len(observed) != len(expected) {
		return errors.New("Production StoryGraph Shot realizes edges do not match source Beats")
	}
	return nil
}

func validateProductionShotOrder(edges []Edge, nodeByKey map[string]Node, shotScenes map[string]string, shotsByScene map[string][]string) error {
	edgesByScene := make(map[string][]Edge)
	for _, edge := range edges {
		if edge.EdgeType != EdgeTypePrecedes {
			continue
		}
		from, fromOK := nodeByKey[edge.FromNodeKey]
		to, toOK := nodeByKey[edge.ToNodeKey]
		if !fromOK || !toOK || from.NodeType != NodeTypeShot || to.NodeType != NodeTypeShot {
			continue
		}
		fromScene, fromExists := shotScenes[edge.FromNodeKey]
		toScene, toExists := shotScenes[edge.ToNodeKey]
		if !fromExists || !toExists || fromScene != toScene || !productionStableKey(edge.Qualifier.SequenceKey) {
			return errors.New("Production StoryGraph Shot precedes edge crosses its Scene or has an invalid sequence")
		}
		edgesByScene[fromScene] = append(edgesByScene[fromScene], edge)
	}
	for sceneKey, shots := range shotsByScene {
		if err := validateProductionShotOrderChain(shots, edgesByScene[sceneKey]); err != nil {
			return err
		}
	}
	return nil
}

func validateProductionShotOrderChain(shots []string, edges []Edge) error {
	if len(edges) != len(shots)-1 {
		return errors.New("Production StoryGraph Shot order is not one complete Scene chain")
	}
	incoming := make(map[string]string, len(shots))
	outgoing := make(map[string]Edge, len(shots))
	sequenceKeys := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		if incoming[edge.ToNodeKey] != "" || outgoing[edge.FromNodeKey].ToNodeKey != "" || edge.FromNodeKey == edge.ToNodeKey {
			return errors.New("Production StoryGraph Shot order branches")
		}
		if _, duplicate := sequenceKeys[edge.Qualifier.SequenceKey]; duplicate {
			return errors.New("Production StoryGraph Shot order has duplicate sequence keys")
		}
		incoming[edge.ToNodeKey] = edge.FromNodeKey
		outgoing[edge.FromNodeKey] = edge
		sequenceKeys[edge.Qualifier.SequenceKey] = struct{}{}
	}
	start := ""
	for _, key := range shots {
		if incoming[key] == "" {
			if start != "" {
				return errors.New("Production StoryGraph Shot order has multiple starts")
			}
			start = key
		}
	}
	visited := make(map[string]struct{}, len(shots))
	previousSequence := ""
	for current := start; current != ""; {
		if _, exists := visited[current]; exists {
			return errors.New("Production StoryGraph Shot order contains a cycle")
		}
		visited[current] = struct{}{}
		edge := outgoing[current]
		if edge.ToNodeKey == "" {
			break
		}
		if previousSequence != "" && previousSequence >= edge.Qualifier.SequenceKey {
			return errors.New("Production StoryGraph Shot order is not strictly increasing")
		}
		previousSequence = edge.Qualifier.SequenceKey
		current = edge.ToNodeKey
	}
	if len(visited) != len(shots) {
		return errors.New("Production StoryGraph Shot order does not cover every Shot")
	}
	return nil
}
