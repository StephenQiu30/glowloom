package postgres

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// yjsRestoration proves the whole candidate is the inverse of an accepted action
// by this actor. A browser restoration marker alone never grants identity reuse.
func yjsRestoration(tx *gorm.DB, actor uuid.UUID, doc domain.Document, row yjsRow, candidate *application.YjsCandidate) (string, error) {
	marker := candidate.Transition
	reject := &domain.OperationError{Code: "restoration_unavailable"}
	if marker.Epoch != row.Epoch || int64(marker.SourceCommitSeq) != row.CommitSeq {
		return "", &domain.OperationError{Code: "review_stale"}
	}
	var logs []collaborationRow
	if err := tx.Raw(`SELECT revision,op_id,create_by,commands FROM canvas.command_log WHERE document_id=? AND revision>? AND NOT is_delete ORDER BY revision DESC LIMIT 128`, doc.ID, max(int64(0), doc.Revision-128)).Scan(&logs).Error; err != nil {
		return "", err
	}
	for _, log := range logs {
		if log.CreateBy != actor || json.Unmarshal(log.Commands, &log.Log) != nil {
			continue
		}
		removed := false
		for _, op := range log.Log.Operations {
			if op.Type == "delete_node" || op.Type == "disconnect" {
				removed = true
			}
		}
		if !removed {
			continue
		}
		floor, history, err := collaborationUndoHistory(tx, doc, actor, log.OpID)
		if err != nil {
			var rejected *domain.OperationError
			if !errors.As(err, &rejected) {
				return "", err
			}
			continue
		}
		ops, err := domain.UndoContribution(floor, history, log.OpID, actor)
		if err != nil {
			continue
		}
		applied, err := domain.ApplyOperations(doc, ops)
		if err != nil {
			continue
		}
		nodes, edges := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
		for _, op := range ops {
			if op.Type == "restore_node" && op.Node != nil {
				nodes[op.Node.ID] = true
			}
			if op.Type == "restore_edge" && op.Edge != nil {
				edges[op.Edge.ID] = true
			}
		}
		if len(nodes) != len(marker.Nodes) || len(edges) != len(marker.Connections) {
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
		expected := application.Snapshot(applied.Document)
		// Runtime projections omit server-owned versions of absent nodes. Compare all
		// business fields; the store restores and advances versions from trusted history.
		if yjsEditableProjectionEqual(expected, candidate.Projection) {
			return log.OpID, nil
		}
	}
	return "", reject
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
