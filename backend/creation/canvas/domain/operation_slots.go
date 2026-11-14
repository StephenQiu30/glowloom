package domain

import (
	"encoding/json"
	"slices"
	"strings"
)

func isBatchSlot(path []string) bool {
	return len(path) == 6 && slices.Equal(path[:3], []string{"config", "batch_table", "rows"}) && path[4] == "input_node_ids" && strings.HasPrefix(path[3], "@") && strings.HasPrefix(path[5], "@")
}
func applyBatchSlot(doc *Document, index int, object map[string]any, op Operation) (FieldChange, error) {
	if op.Type != "set" || op.Unset || op.Value == nil {
		return FieldChange{}, operationError("invalid_operation")
	}
	parent, key, err := fieldContainer(object, op.Path[:5])
	if err != nil {
		return FieldChange{}, err
	}
	table, _, err := fieldContainer(object, []string{"config", "batch_table", "reference_columns"})
	if err != nil {
		return FieldChange{}, err
	}
	columns, _ := table["reference_columns"].([]any)
	column := collectionIndex(columns, strings.TrimPrefix(op.Path[5], "@"))
	slots, _ := parent[key].([]any)
	if column < 0 || column >= len(slots) {
		return FieldChange{}, operationError("target_not_found")
	}
	before, _ := json.Marshal(slots[column])
	var value any
	if json.Unmarshal(op.Value, &value) != nil {
		return FieldChange{}, operationError("invalid_operation")
	}
	if value != nil {
		if _, ok := value.(string); !ok {
			return FieldChange{}, operationError("invalid_operation")
		}
	}
	slots[column] = value
	parent[key] = slots
	after, _ := json.Marshal(value)
	body, _ := json.Marshal(object)
	var node Node
	if json.Unmarshal(body, &node) != nil {
		return FieldChange{}, operationError("invalid_operation")
	}
	doc.Nodes[index] = node
	return FieldChange{NodeID: op.NodeID, Path: slices.Clone(op.Path), Before: before, After: after, BeforeExists: true, AfterExists: true}, nil
}
