package domain

import (
	"encoding/json"

	"github.com/google/uuid"
)

// ValidateWireOperations rejects internal noops and unrelated payload fields.
// Transformed noops are server effects, never client supplied commands.
func ValidateWireOperations(operations []Operation) error {
	if len(operations) < 1 || len(operations) > 100 {
		return operationError("operation_limit")
	}
	for _, op := range operations {
		if op.Type == "noop" || op.Type == "restore_node" || op.Type == "restore_edge" || op.Type == "restore_item" {
			return operationError("invalid_operation")
		}
		if err := validOperationShape(op); err != nil {
			return err
		}
	}
	return nil
}

func validOperationShape(op Operation) error {
	invalid := len(op.unknown) > 0
	switch op.Type {
	case "migrate_composer":
		invalid = invalid || op.NodeID == uuid.Nil || op.Expected == nil || op.Path != nil || op.Value != nil || op.Node != nil || op.Edge != nil || op.Text != nil || op.Context != nil || op.Unset || op.ItemID != "" || op.BeforeID != "" || op.ExpectedExists != nil
	case "noop":
		invalid = invalid || op.NodeID != uuid.Nil || op.Path != nil || op.Node != nil || op.Edge != nil || op.Value != nil || op.Text != nil
	case "set", "text", "insert", "restore_item", "remove", "move":
		if op.Type == "set" && (op.Unset == (op.Value != nil)) {
			return operationError("invalid_operation")
		}
		invalid = invalid || op.NodeID == uuid.Nil || len(op.Path) == 0 || op.Node != nil || op.Edge != nil
		if op.Type == "set" || op.Type == "text" {
			invalid = invalid || op.ItemID != "" || op.BeforeID != ""
		}
		if op.Type == "text" {
			invalid = invalid || op.Value != nil || op.Unset
		}
		if op.Type != "text" {
			invalid = invalid || op.Text != nil
		}
		if op.Type == "remove" || op.Type == "move" {
			invalid = invalid || op.Value != nil || op.Unset
		}
		if op.Type == "remove" {
			invalid = invalid || op.BeforeID != ""
		}
	case "create_node", "restore_node", "connect", "restore_edge", "delete_node", "disconnect":
		invalid = invalid || op.Path != nil || op.Value != nil || op.Expected != nil || op.ExpectedExists != nil || op.Context != nil || op.Text != nil || op.Unset || op.ItemID != "" || op.BeforeID != ""
		switch op.Type {
		case "create_node", "restore_node":
			invalid = invalid || op.Node == nil || op.Edge != nil || op.NodeID != uuid.Nil
		case "connect", "restore_edge":
			invalid = invalid || op.Edge == nil || op.Node != nil || op.NodeID != uuid.Nil
		default:
			invalid = invalid || op.NodeID == uuid.Nil || op.Node != nil || op.Edge != nil
		}
	default:
		invalid = true
	}
	if invalid {
		return operationError("invalid_operation")
	}
	return nil
}

// UnmarshalJSON keeps unrecognized fields in the hashable request so a durable rejection cannot
// accidentally replay for a different request with the same idempotency key.
func (op *Operation) UnmarshalJSON(body []byte) error {
	type wire Operation
	var decoded wire
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	for _, key := range []string{"type", "node_id", "path", "value", "expected", "expected_exists", "context", "unset", "text", "item_id", "before_id", "node", "edge"} {
		delete(fields, key)
	}
	*op = Operation(decoded)
	if len(fields) > 0 {
		op.unknown = fields
	}
	return nil
}

// MarshalJSON preserves all fields of a rejected request for exact replay.
func (op Operation) MarshalJSON() ([]byte, error) {
	type wire Operation
	body, err := json.Marshal(wire(op))
	if err != nil || len(op.unknown) == 0 && op.NodeID != uuid.Nil && (op.Type != "text" || len(op.Text) != 0) {
		return body, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if op.NodeID == uuid.Nil {
		delete(fields, "node_id")
	}
	if op.Type == "text" && len(op.Text) == 0 {
		fields["text"] = json.RawMessage(`[]`)
	}
	for key, value := range op.unknown {
		fields[key] = value
	}
	return json.Marshal(fields)
}
