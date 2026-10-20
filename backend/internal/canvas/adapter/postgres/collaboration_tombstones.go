package postgres

import (
	"encoding/json"
	"slices"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func knownRemoval(tx *gorm.DB, document domain.Document, op domain.Operation) (bool, error) {
	switch op.Type {
	case "delete_node", "disconnect":
		table := "canvas.node"
		if op.Type == "disconnect" {
			table = "canvas.edge"
		}
		var count int64
		if err := tx.Raw("SELECT count(*) FROM "+table+" WHERE id=? AND document_id=? AND is_delete", op.NodeID, document.ID).Scan(&count).Error; err != nil {
			return false, err
		}
		return count == 1, nil
	case "remove", "insert":
		needle, err := json.Marshal(map[string]any{"operations": []domain.Operation{{Type: "remove", NodeID: op.NodeID, Path: op.Path, ItemID: op.ItemID}}})
		if err != nil {
			return false, err
		}
		var count int
		if err := tx.Raw(`SELECT 1 FROM canvas.command_log WHERE document_id=? AND operation_version=1 AND commands @> ?::jsonb LIMIT 1`, document.ID, string(needle)).Scan(&count).Error; err != nil {
			return false, err
		}
		return count == 1, nil
	}
	return false, nil
}
func normalizeKnownRemovals(tx *gorm.DB, base domain.Document, operations []domain.Operation) ([]domain.Operation, error) {
	normalized := slices.Clone(operations)
	for i, op := range normalized {
		missing := false
		switch op.Type {
		case "delete_node":
			missing = !slices.ContainsFunc(base.Nodes, func(n domain.Node) bool { return n.ID == op.NodeID })
		case "disconnect":
			missing = !slices.ContainsFunc(base.Edges, func(e domain.Edge) bool { return e.ID == op.NodeID })
		case "remove":
			exists, err := domain.HasCollectionItem(base, op.NodeID, op.Path, op.ItemID)
			if err != nil {
				continue
			}
			missing = !exists
		}
		if !missing {
			continue
		}
		known, err := knownRemoval(tx, base, op)
		if err != nil {
			return nil, err
		}
		if known {
			normalized[i] = domain.Operation{Type: "noop"}
		}
	}
	return normalized, nil
}

func collectionIdentityUsed(tx *gorm.DB, doc domain.Document, op domain.Operation) error {
	if op.Type != "insert" {
		return nil
	}
	known, err := knownRemoval(tx, doc, op)
	if err != nil {
		return err
	}
	if known {
		return &domain.OperationError{Code: "id_conflict"}
	}
	return nil
}
