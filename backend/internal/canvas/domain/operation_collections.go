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
	return op.NodeID == id || op.Node != nil && op.Node.ID == id || op.Edge != nil && (op.Edge.SourceNodeID == id || op.Edge.TargetNodeID == id)
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
	case "restore_node":
		if op.Node == nil || !editable(*op.Node) || nodeIndex(doc, op.Node.ID) >= 0 {
			return operationError("undo_conflict")
		}
		doc.Nodes = append(doc.Nodes, *op.Node)
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
