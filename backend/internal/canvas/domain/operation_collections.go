package domain

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
)

func equalJSON(a, b []byte) bool {
	var x, y any
	left := json.NewDecoder(bytes.NewReader(a))
	left.UseNumber()
	right := json.NewDecoder(bytes.NewReader(b))
	right.UseNumber()
	return left.Decode(&x) == nil && right.Decode(&y) == nil && reflect.DeepEqual(x, y)
}
func itemIdentity(object map[string]any) string {
	if id, ok := object["id"].(string); ok {
		return id
	}
	if bone, ok := object["bone"].(string); ok {
		return bone
	}
	return ""
}
func collectionIndex(items []any, id string) int {
	return slices.IndexFunc(items, func(item any) bool { object, ok := item.(map[string]any); return ok && itemIdentity(object) == id })
}
func insertionOperation(kind string) bool { return kind == "insert" || kind == "restore_item" }
func structureOperation(kind string) bool {
	return insertionOperation(kind) || kind == "remove" || kind == "move"
}
func operationTargetsItem(op, change Operation) bool {
	return len(op.Path) > len(change.Path) && slices.Equal(op.Path[:len(change.Path)], change.Path) && op.Path[len(change.Path)] == "@"+change.ItemID
}
func operationUsesNode(op Operation, id uuid.UUID) bool {
	if op.NodeID == id || op.Edge != nil && (op.Edge.SourceNodeID == id || op.Edge.TargetNodeID == id) {
		return true
	}
	if op.Node != nil {
		used := op.Node.ID == id || op.Node.ParentID != nil && *op.Node.ParentID == id
		inputSources(*op.Node, func(source uuid.UUID) { used = used || source == id })
		if used {
			return true
		}
	}
	if op.Type == "set" && (slices.Equal(op.Path, []string{"parent_id"}) || isBatchSlot(op.Path)) {
		var source uuid.UUID
		return json.Unmarshal(op.Value, &source) == nil && source == id
	}
	if insertionOperation(op.Type) && slices.Equal(op.Path, []string{"config", "batch_table", "rows"}) {
		var row struct {
			InputNodeIDs []*uuid.UUID `json:"input_node_ids"`
		}
		if json.Unmarshal(op.Value, &row) == nil {
			return slices.ContainsFunc(row.InputNodeIDs, func(source *uuid.UUID) bool { return source != nil && *source == id })
		}
	}
	// These are the only value containers in the finite vocabulary that introduce
	// a graph source premise. Asset and subject UUIDs are separate identities.
	if len(op.Path) >= 3 && (slices.Equal(op.Path[:3], []string{"config", "timeline", "clips"}) || slices.Equal(op.Path[:3], []string{"config", "director", "objects"})) {
		var source struct {
			NodeID       *uuid.UUID `json:"node_id"`
			SourceNodeID *uuid.UUID `json:"source_node_id"`
		}
		if json.Unmarshal(op.Value, &source) == nil {
			return source.NodeID != nil && *source.NodeID == id || source.SourceNodeID != nil && *source.SourceNodeID == id
		}
	}
	return false
}
func operationsOverlap(a, b Operation) bool {
	paths := func(op Operation) [][]string {
		r, _ := policy(op.Path)
		if len(r.group) == 0 {
			return [][]string{op.Path}
		}
		result := make([][]string, 0, len(r.group))
		for _, key := range r.group {
			result = append(result, append(slices.Clone(op.Path[:len(op.Path)-1]), key))
		}
		return result
	}
	for _, x := range paths(a) {
		for _, y := range paths(b) {
			n := min(len(x), len(y))
			if slices.Equal(x[:n], y[:n]) {
				return true
			}
		}
	}
	return false
}
func applyStructure(doc *Document, op Operation) error {
	switch op.Type {
	case "set_edge":
		index := slices.IndexFunc(doc.Edges, func(edge Edge) bool { return edge.ID == op.NodeID })
		if index < 0 {
			return operationError("target_not_found")
		}
		if op.Edge == nil || !annotation(doc.Edges[index]) || !annotation(*op.Edge) {
			return operationError("protected_reference")
		}
		if !equalJSON(effectJSON(doc.Edges[index]), op.Expected) {
			return operationError("edge_conflict")
		}
		for _, id := range []uuid.UUID{doc.Edges[index].SourceNodeID, doc.Edges[index].TargetNodeID, op.Edge.SourceNodeID, op.Edge.TargetNodeID} {
			i := nodeIndex(doc, id)
			if i < 0 || !editable(doc.Nodes[i]) {
				return operationError("protected_reference")
			}
		}
		doc.Edges[index] = *op.Edge
		return nil
	case "restore_node":
		if op.Node == nil || !editable(*op.Node) || nodeIndex(doc, op.Node.ID) >= 0 {
			return operationError("undo_conflict")
		}
		var restored Node
		if json.Unmarshal(effectJSON(op.Node), &restored) != nil {
			return operationError("undo_conflict")
		}
		if restored.Config.Draft != nil && restored.Config.Draft.Composer != nil {
			for i := range restored.Config.Draft.Composer.References {
				ref := &restored.Config.Draft.Composer.References[i]
				if ref.BindingGeneration >= 9007199254740991 {
					return operationError("binding_generation_limit")
				}
				ref.BindingGeneration++
			}
		}
		doc.Nodes = append(doc.Nodes, restored)
		return nil
	case "restore_edge":
		if op.Edge == nil || !annotation(*op.Edge) || slices.ContainsFunc(doc.Edges, func(e Edge) bool { return e.ID == op.Edge.ID }) {
			return operationError("undo_conflict")
		}
		doc.Edges = append(doc.Edges, *op.Edge)
		return nil
	case "create_node":
		if op.Node == nil {
			return operationError("invalid_operation")
		}
		return applyOne(doc, Command{Type: "AddNodes", Nodes: []Node{*op.Node}})
	case "delete_node":
		i := nodeIndex(doc, op.NodeID)
		if i >= 0 && IsDraft(doc.Nodes[i]) {
			if !editable(doc.Nodes[i]) {
				return operationError("protected_target")
			}
			for _, e := range doc.Edges {
				if (e.SourceNodeID == op.NodeID || e.TargetNodeID == op.NodeID) && !annotation(e) {
					return operationError("protected_reference")
				}
			}
			for j := range doc.Nodes {
				if doc.Nodes[j].ParentID != nil && *doc.Nodes[j].ParentID == op.NodeID {
					if !editable(doc.Nodes[j]) {
						return operationError("protected_reference")
					}
					doc.Nodes[j].ParentID = nil
				}
			}
			doc.Nodes = slices.Delete(doc.Nodes, i, i+1)
			doc.Edges = slices.DeleteFunc(doc.Edges, func(e Edge) bool { return e.SourceNodeID == op.NodeID || e.TargetNodeID == op.NodeID })
			return nil
		}
		return applyOne(doc, Command{Type: "DeleteNodes", IDs: []uuid.UUID{op.NodeID}})
	case "connect":
		if op.Edge == nil {
			return operationError("invalid_operation")
		}
		return applyOne(doc, Command{Type: "Connect", Edges: []Edge{*op.Edge}})
	case "disconnect":
		return applyOne(doc, Command{Type: "Disconnect", IDs: []uuid.UUID{op.NodeID}})
	}
	return operationError("unsupported_operation")
}
func applyCollection(doc *Document, op Operation) (FieldChange, error) {
	i := nodeIndex(doc, op.NodeID)
	if i < 0 {
		return FieldChange{}, operationError("target_not_found")
	}
	if !editable(doc.Nodes[i]) {
		return FieldChange{}, operationError("protected_target")
	}
	rule, ok := policy(op.Path)
	if !ok || rule.kind != "list" || op.ItemID == "" || strings.ContainsAny(op.ItemID, "\x00") {
		return FieldChange{}, operationError("unsupported_field")
	}
	object, err := nodeObject(doc.Nodes[i])
	if err != nil {
		return FieldChange{}, err
	}
	parent, key, err := fieldContainer(object, op.Path)
	if err != nil {
		return FieldChange{}, err
	}
	if rule.context != "" {
		context, err := fieldContext(object, op.Path, rule)
		if err != nil {
			return FieldChange{}, err
		}
		if !equalJSON(context, op.Context) {
			return FieldChange{}, operationError("source_changed")
		}
	}
	if err := timelineGate(object, op); err != nil {
		return FieldChange{}, err
	}
	before, exists := fieldValue(parent, key, rule)
	items, _ := parent[key].([]any)
	items = slices.Clone(items)
	original := slices.Clone(items)
	index := collectionIndex(items, op.ItemID)
	switch op.Type {
	case "insert", "restore_item":
		if index >= 0 {
			return FieldChange{}, operationError("id_conflict")
		}
		var item map[string]any
		decoder := json.NewDecoder(bytes.NewReader(op.Value))
		decoder.UseNumber()
		if decoder.Decode(&item) != nil || itemIdentity(item) != op.ItemID {
			return FieldChange{}, operationError("invalid_operation")
		}
		items = append(items, item)
		index = len(items) - 1
	case "remove", "move":
		if index < 0 {
			return FieldChange{}, operationError("target_not_found")
		}
	}
	if op.Type == "remove" {
		items = slices.Delete(items, index, index+1)
	} else {
		if op.BeforeID == op.ItemID {
			return FieldChange{}, operationError("order_conflict")
		}
		item := items[index]
		items = slices.Delete(items, index, index+1)
		anchor := len(items)
		if op.BeforeID != "" {
			anchor = collectionIndex(items, op.BeforeID)
			if anchor < 0 {
				return FieldChange{}, operationError("anchor_deleted")
			}
		}
		items = slices.Insert(items, anchor, item)
	}
	parent[key] = items
	if slices.Equal(op.Path, []string{"config", "batch_table", "reference_columns"}) {
		config := object["config"].(map[string]any)["batch_table"].(map[string]any)
		rows, _ := config["rows"].([]any)
		for _, value := range rows {
			row := value.(map[string]any)
			slots, _ := row["input_node_ids"].([]any)
			old := make(map[string]any)
			for j, col := range original {
				if j < len(slots) {
					old[itemIdentity(col.(map[string]any))] = slots[j]
				}
			}
			next := make([]any, len(items))
			for j, col := range items {
				next[j] = old[itemIdentity(col.(map[string]any))]
			}
			row["input_node_ids"] = next
		}
	}
	if isReferenceCollection(op.Path) {
		if err := normalizeReferenceCollection(object, op); err != nil {
			return FieldChange{}, err
		}
	}
	after, _ := fieldValue(parent, key, rule)
	body, _ := json.Marshal(object)
	var node Node
	if json.Unmarshal(body, &node) != nil {
		return FieldChange{}, operationError("invalid_operation")
	}
	doc.Nodes[i] = node
	return FieldChange{NodeID: op.NodeID, Path: slices.Clone(op.Path), Before: before, After: after, BeforeExists: exists, AfterExists: true}, nil
}

// HasCollectionItem checks only a declared editable collection, for scoped
// database tombstone reconciliation. Unknown paths never become successful noops.
func HasCollectionItem(document Document, id uuid.UUID, path []string, item string) (bool, error) {
	index := nodeIndex(&document, id)
	if index < 0 {
		return false, operationError("target_not_found")
	}
	if !editable(document.Nodes[index]) {
		return false, operationError("protected_target")
	}
	rule, ok := policy(path)
	if !ok || rule.kind != "list" {
		return false, operationError("unsupported_field")
	}
	object, err := nodeObject(document.Nodes[index])
	if err != nil {
		return false, err
	}
	parent, key, err := fieldContainer(object, path)
	if err != nil {
		return false, err
	}
	items, _ := parent[key].([]any)
	return collectionIndex(items, item) >= 0, nil
}
