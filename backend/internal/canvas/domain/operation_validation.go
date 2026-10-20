package domain

import "github.com/google/uuid"

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
	invalid := false
	switch op.Type {
	case "migrate_composer":
		invalid = op.NodeID == uuid.Nil || op.Expected == nil || op.Path != nil || op.Value != nil || op.Node != nil || op.Edge != nil || op.Text != nil || op.Context != nil || op.Unset || op.ItemID != "" || op.BeforeID != "" || op.ExpectedExists != nil
	case "noop":
		invalid = op.NodeID != uuid.Nil || op.Path != nil || op.Node != nil || op.Edge != nil || op.Value != nil || op.Text != nil
	case "set", "text", "insert", "restore_item", "remove", "move":
		invalid = op.NodeID == uuid.Nil || len(op.Path) == 0 || op.Node != nil || op.Edge != nil
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
		invalid = op.Path != nil || op.Value != nil || op.Expected != nil || op.ExpectedExists != nil || op.Context != nil || op.Text != nil || op.Unset || op.ItemID != "" || op.BeforeID != ""
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
