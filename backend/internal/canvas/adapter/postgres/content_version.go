package postgres

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// Versions belong to accepted content, not layout changes or OT simulations.
// Effects include these increments so replay and undo can reconstruct any base.
func advanceContentVersions(before domain.Document, after *domain.Document) error {
	previous := make(map[uuid.UUID]domain.Node, len(before.Nodes))
	for _, node := range before.Nodes {
		previous[node.ID] = node
	}
	for i := range after.Nodes {
		node := &after.Nodes[i]
		prior, exists := previous[node.ID]
		if !exists {
			// A trusted undo may restore a former entity. Its content identity
			// must advance as well instead of reusing a historical version.
			if node.ContentVersion == math.MaxInt64 {
				return &domain.OperationError{Code: "content_version_limit"}
			}
			node.ContentVersion++
			continue
		}
		left, err := json.Marshal(prior.Config)
		if err != nil {
			return err
		}
		right, err := json.Marshal(node.Config)
		if err != nil {
			return err
		}
		changed := prior.RefType != node.RefType || (prior.RefID == nil) != (node.RefID == nil) || prior.RefID != nil && node.RefID != nil && *prior.RefID != *node.RefID || !bytes.Equal(left, right)
		node.ContentVersion = prior.ContentVersion
		if changed {
			if node.ContentVersion == math.MaxInt64 {
				return &domain.OperationError{Code: "content_version_limit"}
			}
			node.ContentVersion++
		}
	}
	return nil
}
