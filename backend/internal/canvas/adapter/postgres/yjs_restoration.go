package postgres

import (
	"encoding/json"
	"reflect"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// yjsRestoration verifies Yjs undo against an accepted deletion by this actor.
// It restores absent identities and unchanged detached memberships only. It never
// transforms an operation stream or rewinds another actor's later field writes.
func yjsRestoration(tx *gorm.DB, actor uuid.UUID, doc domain.Document, row yjsRow, candidate *application.YjsCandidate) ([]domain.Operation, error) {
	marker := candidate.Transition
	if marker.Epoch != row.Epoch || int64(marker.SourceCommitSeq) != row.CommitSeq {
		return nil, &domain.OperationError{Code: "review_stale"}
	}
	var logs []struct {
		Revision int64
		CreateBy uuid.UUID
		Commands []byte
	}
	if err := tx.Raw(`SELECT revision,create_by,commands FROM canvas.command_log WHERE document_id=? AND revision>? AND NOT is_delete ORDER BY revision DESC LIMIT 128`, doc.ID, max(int64(0), doc.Revision-128)).Scan(&logs).Error; err != nil {
		return nil, err
	}
	// Most recent structural fact fences an earlier deletion from authorizing a
	// later deletion by somebody else (including delete/restore/delete ABA).
	seen := map[uuid.UUID]bool{}
	for _, log := range logs {
		var entry projectionLog
		if json.Unmarshal(log.Commands, &entry) != nil {
			return nil, &domain.OperationError{Code: "restoration_unavailable"}
		}
		ops := []domain.Operation{}
		nodes, edges := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
		proofNodes := map[uuid.UUID]domain.Node{}
		for _, effect := range entry.Effects {
			if len(effect.Path) != 0 {
				continue
			}
			latest := !seen[effect.ID]
			seen[effect.ID] = true
			if !latest || log.CreateBy != actor || !effect.BeforeExists || effect.AfterExists {
				continue
			}
			switch effect.Entity {
			case "node":
				var n domain.Node
				if json.Unmarshal(effect.Before, &n) != nil || n.ID != effect.ID {
					continue
				}
				nodes[n.ID] = true
				proofNodes[n.ID] = n
				ops = append(ops, domain.Operation{Type: "restore_node", Node: &n})
			case "edge":
				var e domain.Edge
				if json.Unmarshal(effect.Before, &e) != nil || e.ID != effect.ID {
					continue
				}
				edges[e.ID] = true
				ops = append(ops, domain.Operation{Type: "restore_edge", Edge: &e})
			}
		}
		if len(nodes) != len(marker.Nodes) || len(edges) != len(marker.Connections) || len(ops) == 0 {
			continue
		}
		match := true
		for _, id := range marker.Nodes {
			match = match && nodes[id]
		}
		for _, id := range marker.Connections {
			match = match && edges[id]
		}
		if !match {
			continue
		}
		// Reconstruct only this deletion's proved preimages. Fields changed
		// since the deletion are retained, including changes on live dependents.
		expected := doc
		expected.Nodes = slices.Clone(doc.Nodes)
		expected.Edges = slices.Clone(doc.Edges)
		for _, op := range ops {
			if op.Node != nil {
				expected.Nodes = append(expected.Nodes, *op.Node)
			}
			if op.Edge != nil {
				expected.Edges = append(expected.Edges, *op.Edge)
			}
		}
		for _, effect := range entry.Effects {
			if effect.Entity != "node" || len(effect.Path) == 0 {
				continue
			}
			if effect.Path[0] == "content_version" || effect.Path[0] == "input_revision" || effect.Path[0] == "last_operation_id" {
				continue
			}
			if reverted, err := domain.RewindEffects(expected, []domain.Effect{effect}); err == nil {
				expected = reverted
			}
		}
		// Convert a trusted final projection to exact-head domain assignments.
		// No remote operation stream is transformed and no intervening edit is replayed.
		ops, err := application.YjsProjectionOperations(doc, application.Snapshot(expected))
		if err != nil {
			continue
		}
		for i := range ops {
			if ops[i].Type == "create_node" && nodes[ops[i].Node.ID] {
				ops[i].Type = "restore_node"
				node := proofNodes[ops[i].Node.ID]
				ops[i].Node = &node
			}
			if ops[i].Type == "connect" && edges[ops[i].Edge.ID] {
				ops[i].Type = "restore_edge"
			}
		}
		applied, err := domain.ApplyOperations(doc, ops)
		if err == nil && yjsEditableProjectionEqual(application.Snapshot(applied.Document), candidate.Projection) {
			return ops, nil
		}
	}
	return nil, &domain.OperationError{Code: "restoration_unavailable"}
}

func yjsEditableProjectionEqual(a, b application.DocumentDTO) bool {
	normalize := func(doc application.DocumentDTO) any {
		data, _ := json.Marshal(doc)
		var v map[string]any
		_ = json.Unmarshal(data, &v)
		nodes := map[string]any{}
		for _, value := range v["nodes"].([]any) {
			node := value.(map[string]any)
			for _, key := range []string{"content_version", "input_revision", "last_operation_id"} {
				delete(node, key)
			}
			nodes[node["id"].(string)] = node
		}
		edges := map[string]any{}
		for _, value := range v["edges"].([]any) {
			edge := value.(map[string]any)
			edges[edge["id"].(string)] = edge
		}
		return []any{doc.Name, nodes, edges}
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}
