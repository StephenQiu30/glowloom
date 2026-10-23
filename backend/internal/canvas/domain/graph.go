package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// ErrGraphReadOnly protects legacy or damaged graphs from lossy mutation.
var ErrGraphReadOnly = fmt.Errorf("%w: canvas graph requires review", ErrUnsupportedCommand)

// ValidateGraph checks the current graph without changing it. Visual annotations,
// group membership and typed input dependencies have separate semantics.
func ValidateGraph(doc Document) error {
	if len(doc.Nodes) > 2000 || len(doc.Edges) > 4000 {
		return ErrInvalidCommand
	}
	nodes := make(map[uuid.UUID]Node, len(doc.Nodes))
	for _, node := range doc.Nodes {
		if node.ID == uuid.Nil {
			return ErrInvalidCommand
		}
		if _, exists := nodes[node.ID]; exists {
			return ErrInvalidCommand
		}
		nodes[node.ID] = node
		if len(node.Config.unknown) > 0 {
			return fmt.Errorf("%w: unknown node configuration", ErrUnsupportedCommand)
		}
		switch node.NodeType {
		case "config", "text", "group", "image", "video", "audio", "model", "image_generation", "video_generation", "batch_table", "timeline", "director", "generation":
		default:
			return fmt.Errorf("%w: unknown node type", ErrUnsupportedCommand)
		}
		if !validConfig(node.NodeType, node.Config) || !validEditorFiles(node) {
			return ErrInvalidCommand
		}
	}
	if !validParents(doc.Nodes) || !validToolReferences(doc.Nodes) {
		return ErrInvalidCommand
	}
	ids := make(map[uuid.UUID]bool, len(doc.Edges))
	relations := make(map[string]bool, len(doc.Edges))
	for _, edge := range doc.Edges {
		if !annotation(edge) {
			return fmt.Errorf("%w: unclassified stored relation", ErrUnsupportedCommand)
		}
		_, sourceOK := nodes[edge.SourceNodeID]
		_, targetOK := nodes[edge.TargetNodeID]
		pair := edge.SourceNodeID.String() + "/" + edge.TargetNodeID.String() + "/" + string(edge.Binding)
		if edge.ID == uuid.Nil || ids[edge.ID] || !sourceOK || !targetOK || edge.SourceNodeID == edge.TargetNodeID || relations[pair] {
			return ErrInvalidCommand
		}
		ids[edge.ID] = true
		relations[pair] = true
	}
	if err := validateEditorConnections(doc); err != nil {
		return err
	}
	return validateInputDAG(doc.Nodes)
}

// inputSources visits current canvas-node bindings only. Asset identities and
// frozen generation provenance never become node dependencies by UUID equality.
func inputSources(node Node, visit func(uuid.UUID)) {
	if c := node.Config.BatchTable; c != nil {
		for _, row := range c.Rows {
			for _, id := range row.InputNodeIDs {
				if id != nil {
					visit(*id)
				}
			}
		}
	}
	if c := node.Config.Director; c != nil {
		for _, object := range c.Objects {
			if object.SourceNodeID != nil {
				visit(*object.SourceNodeID)
			}
		}
	}
	if c := node.Config.Timeline; c != nil {
		for _, clip := range c.Clips {
			if clip.NodeID != nil {
				visit(*clip.NodeID)
			}
		}
	}
}

func validateInputDAG(nodes []Node) error {
	adjacency := make(map[uuid.UUID]map[uuid.UUID]bool, len(nodes))
	indegree := make(map[uuid.UUID]int, len(nodes))
	for _, node := range nodes {
		indegree[node.ID] = 0
	}
	for _, node := range nodes {
		inputSources(node, func(source uuid.UUID) {
			if adjacency[source] == nil {
				adjacency[source] = make(map[uuid.UUID]bool)
			}
			if !adjacency[source][node.ID] {
				adjacency[source][node.ID] = true
				indegree[node.ID]++
			}
		})
	}
	ready := make([]uuid.UUID, 0, len(nodes))
	for id, count := range indegree {
		if count == 0 {
			ready = append(ready, id)
		}
	}
	visited := 0
	for len(ready) > 0 {
		id := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		visited++
		for target := range adjacency[id] {
			indegree[target]--
			if indegree[target] == 0 {
				ready = append(ready, target)
			}
		}
	}
	if visited != len(nodes) {
		return fmt.Errorf("%w: input dependency cycle", ErrInvalidCommand)
	}
	return nil
}

func protectedConsumer(nodes []Node, removed map[uuid.UUID]bool) bool {
	for _, node := range nodes {
		if removed[node.ID] || editable(node) {
			continue
		}
		blocked := false
		inputSources(node, func(source uuid.UUID) {
			if removed[source] {
				blocked = true
			}
		})
		if blocked {
			return true
		}
	}
	return false
}
