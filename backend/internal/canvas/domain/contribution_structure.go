package domain

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"
)

func inverseStructure(before Document, op Operation) ([]Operation, error) {
	switch op.Type {
	case "create_node", "restore_node":
		if op.Node == nil {
			return nil, operationError("undo_unavailable")
		}
		return []Operation{{Type: "delete_node", NodeID: op.Node.ID}}, nil
	case "connect", "restore_edge":
		if op.Edge == nil {
			return nil, operationError("undo_unavailable")
		}
		return []Operation{{Type: "disconnect", NodeID: op.Edge.ID}}, nil
	case "disconnect":
		for _, edge := range before.Edges {
			if edge.ID == op.NodeID {
				return []Operation{{Type: "restore_edge", Edge: &edge}}, nil
			}
		}
	case "delete_node":
		index := nodeIndex(&before, op.NodeID)
		if index < 0 {
			return nil, operationError("undo_unavailable")
		}
		node := before.Nodes[index]
		out := []Operation{{Type: "restore_node", Node: &node}}
		for _, edge := range before.Edges {
			if edge.SourceNodeID == node.ID || edge.TargetNodeID == node.ID {
				out = append(out, Operation{Type: "restore_edge", Edge: &edge})
			}
		}
		after, err := applyOperations(before, []Operation{op}, false)
		if err != nil {
			return nil, err
		}
		for _, n := range before.Nodes {
			if n.ID == node.ID {
				continue
			}
			addField := func(path []string) error {
				inv, err := restoreFieldFromDocuments(before, after.Document, n.ID, path)
				if err != nil {
					return err
				}
				out = append(out, inv)
				return nil
			}
			if n.ParentID != nil && *n.ParentID == node.ID {
				if err := addField([]string{"parent_id"}); err != nil {
					return nil, err
				}
			}
			if table := n.Config.BatchTable; table != nil {
				for _, row := range table.Rows {
					for col, id := range row.InputNodeIDs {
						if id != nil && *id == node.ID {
							out = append(out, Operation{Type: "set", NodeID: n.ID, Path: []string{"config", "batch_table", "rows", "@" + row.ID.String(), "input_node_ids", "@" + table.ReferenceColumns[col].ID.String()}, Value: effectJSON(node.ID)})
						}
					}
				}
			}
			if timeline := n.Config.Timeline; timeline != nil {
				for _, clip := range timeline.Clips {
					if clip.NodeID != nil && *clip.NodeID == node.ID {
						if err := addField([]string{"config", "timeline", "clips", "@" + clip.ID.String(), "source"}); err != nil {
							return nil, err
						}
					}
				}
			}
			if director := n.Config.Director; director != nil {
				for _, object := range director.Objects {
					if object.SourceNodeID != nil && *object.SourceNodeID == node.ID {
						if err := addField([]string{"config", "director", "objects", "@" + object.ID.String(), "source"}); err != nil {
							return nil, err
						}
					}
				}
			}
		}
		return out, nil
	case "insert", "restore_item":
		return []Operation{{Type: "remove", NodeID: op.NodeID, Path: slices.Clone(op.Path), ItemID: op.ItemID, Context: op.Context}}, nil
	case "remove", "move":
		index := nodeIndex(&before, op.NodeID)
		if index < 0 {
			return nil, operationError("undo_unavailable")
		}
		object, err := nodeObject(before.Nodes[index])
		if err != nil {
			return nil, err
		}
		parent, key, err := fieldContainer(object, op.Path)
		if err != nil {
			return nil, err
		}
		items, _ := parent[key].([]any)
		item := collectionIndex(items, op.ItemID)
		if item < 0 {
			return nil, operationError("undo_unavailable")
		}
		inverse := Operation{Type: "move", NodeID: op.NodeID, Path: slices.Clone(op.Path), ItemID: op.ItemID, Context: op.Context}
		if item+1 < len(items) {
			inverse.BeforeID = itemIdentity(items[item+1].(map[string]any))
		}
		if op.Type == "move" {
			return []Operation{inverse}, nil
		}
		inverse.Type = "restore_item"
		inverse.Value = effectJSON(items[item])
		out := []Operation{inverse}
		if isReferenceCollection(op.Path) {
			sequence, err := PromptSequence(before.Nodes[index].Config.Draft.Composer.Document)
			if err != nil {
				return nil, err
			}
			cleanup, _, err := referenceCleanup(before, op)
			if err != nil {
				return nil, err
			}
			text, err := InvertSequence(sequence, cleanup)
			if err != nil {
				return nil, err
			}
			out = append(out, Operation{Type: "text", NodeID: op.NodeID, Path: []string{"config", "draft", "composer", "document"}, Text: text})
		}
		if slices.Equal(op.Path, []string{"config", "batch_table", "reference_columns"}) {
			table := before.Nodes[index].Config.BatchTable
			for _, row := range table.Rows {
				if item < len(row.InputNodeIDs) && row.InputNodeIDs[item] != nil {
					out = append(out, Operation{Type: "set", NodeID: op.NodeID, Path: []string{"config", "batch_table", "rows", "@" + row.ID.String(), "input_node_ids", "@" + op.ItemID}, Value: effectJSON(row.InputNodeIDs[item])})
				}
			}
		}
		return out, nil
	}
	return nil, operationError("undo_unavailable")
}

func restoreFieldFromDocuments(before, after Document, nodeID uuid.UUID, path []string) (Operation, error) {
	i := nodeIndex(&before, nodeID)
	j := nodeIndex(&after, nodeID)
	if i < 0 || j < 0 {
		return Operation{}, operationError("undo_unavailable")
	}
	old, err := nodeObject(before.Nodes[i])
	if err != nil {
		return Operation{}, err
	}
	current, err := nodeObject(after.Nodes[j])
	if err != nil {
		return Operation{}, err
	}
	parent, key, err := fieldContainer(old, path)
	if err != nil {
		return Operation{}, err
	}
	rule, _ := policy(path)
	value, exists := fieldValue(parent, key, rule)
	nextParent, nextKey, err := fieldContainer(current, path)
	if err != nil {
		return Operation{}, err
	}
	expected, expectedExists := fieldValue(nextParent, nextKey, rule)
	inverse := Operation{Type: "set", NodeID: nodeID, Path: path, Value: value, Unset: !exists, Expected: expected, ExpectedExists: &expectedExists}
	if rule.context != "" {
		inverse.Context, err = fieldContext(current, path, rule)
	}
	return inverse, err
}

func touchesSameItem(op, change Operation) bool {
	if change.NodeID != op.NodeID {
		return false
	}
	return operationTargetsItem(change, op) || structureOperation(change.Type) && slices.Equal(change.Path, op.Path) && (change.ItemID == op.ItemID || op.Type == "move")
}

// Remapping a restored row uses stable column identities when another editor
// changed columns while that row was absent.
func transformRestoredRow(base Document, columns, row Operation) (Operation, error) {
	if row.Type != "restore_item" || !slices.Equal(row.Path, []string{"config", "batch_table", "rows"}) || !slices.Equal(columns.Path, []string{"config", "batch_table", "reference_columns"}) || row.NodeID != columns.NodeID {
		return row, nil
	}
	index := nodeIndex(&base, row.NodeID)
	if index < 0 {
		return row, operationError("undo_conflict")
	}
	old := base.Nodes[index].Config.BatchTable.ReferenceColumns
	after, err := applyOperations(base, []Operation{columns}, false)
	if err != nil {
		return row, err
	}
	var restored BatchTableRow
	if json.Unmarshal(row.Value, &restored) != nil {
		return row, operationError("undo_conflict")
	}
	slots := map[uuid.UUID]*uuid.UUID{}
	for i, col := range old {
		if i < len(restored.InputNodeIDs) {
			slots[col.ID] = restored.InputNodeIDs[i]
		}
	}
	restored.InputNodeIDs = []*uuid.UUID{}
	for _, col := range after.Document.Nodes[nodeIndex(&after.Document, row.NodeID)].Config.BatchTable.ReferenceColumns {
		restored.InputNodeIDs = append(restored.InputNodeIDs, slots[col.ID])
	}
	row.Value = effectJSON(restored)
	return row, nil
}
