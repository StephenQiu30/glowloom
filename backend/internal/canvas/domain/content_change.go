package domain

import (
	"encoding/json"
	"reflect"

	"github.com/google/uuid"
)

// HasContentChanges ignores view, metadata and transport versions while retaining editable graph semantics.
func HasContentChanges(before, after Document) bool {
	if len(before.Nodes) != len(after.Nodes) || len(before.Edges) != len(after.Edges) {
		return true
	}
	left := make(map[uuid.UUID]Node, len(before.Nodes))
	right := make(map[uuid.UUID]Node, len(after.Nodes))
	for _, node := range before.Nodes {
		node.InputRevision = 0
		node.LastOperationID = nil
		left[node.ID] = node
	}
	for _, node := range after.Nodes {
		node.InputRevision = 0
		node.LastOperationID = nil
		if prior, ok := left[node.ID]; ok && prior.Config.Draft != nil && node.Config.Draft != nil && SameInput(*prior.Config.Draft, *node.Config.Draft) {
			prior.Config.Draft = nil
			node.Config.Draft = nil
			left[node.ID] = prior
		}
		right[node.ID] = node
	}
	leftEdges := make(map[uuid.UUID]Edge, len(before.Edges))
	rightEdges := make(map[uuid.UUID]Edge, len(after.Edges))
	for _, edge := range before.Edges {
		leftEdges[edge.ID] = edge
	}
	for _, edge := range after.Edges {
		rightEdges[edge.ID] = edge
	}
	return !sameContentJSON(left, right) || !sameContentJSON(leftEdges, rightEdges)
}

func sameContentJSON(left, right any) bool {
	l, err := json.Marshal(left)
	if err != nil {
		return false
	}
	r, err := json.Marshal(right)
	if err != nil {
		return false
	}
	var a, b any
	if json.Unmarshal(l, &a) != nil || json.Unmarshal(r, &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
