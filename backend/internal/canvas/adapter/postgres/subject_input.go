package postgres

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// ReadSubjectInput is an internal owner read after current project authorization.
// The caller keeps the transaction open and separately proves every media source.
func ReadSubjectInput(ctx context.Context, tx *gorm.DB, project, nodeID uuid.UUID, revision int64) (domain.Node, error) {
	var row struct {
		ID            uuid.UUID
		DocumentID    uuid.UUID
		Title         string
		Config        []byte
		InputRevision int64
		Input         []byte
	}
	result := tx.WithContext(ctx).Raw(`SELECT n.id,n.document_id,n.title,n.config,n.input_revision,i.draft AS input FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id JOIN canvas.node_input i ON i.node_id=n.id AND i.input_revision=n.input_revision WHERE n.id=? AND d.project_id=? AND NOT n.is_delete AND NOT d.is_delete AND d.schema_version=1 AND d.scope='{}'::jsonb AND n.node_type='subject' AND n.node_action='resource' AND n.input_revision=? FOR SHARE OF n,d`, nodeID, project, revision).Scan(&row)
	if result.Error != nil {
		return domain.Node{}, result.Error
	}
	if result.RowsAffected != 1 {
		return domain.Node{}, promptdomain.ErrReferenceUnavailable
	}
	node := domain.Node{ID: row.ID, Title: row.Title, NodeType: "subject", NodeAction: "resource", InputRevision: row.InputRevision}
	if json.Unmarshal(row.Config, &node.Config) != nil || node.Config.Subject == nil || node.Config.Subject.Local == nil || node.Config.Subject.Local.Primary == nil {
		return domain.Node{}, promptdomain.ErrReferenceUnavailable
	}
	graph := domain.Document{Nodes: []domain.Node{node}}
	var sourceIDs []uuid.UUID
	for _, media := range node.Config.Subject.Local.Media() {
		if media.SourceNodeID != nil {
			sourceIDs = append(sourceIDs, *media.SourceNodeID)
		}
	}
	if len(sourceIDs) > 0 {
		var sources []struct {
			ID         uuid.UUID
			Title      string
			NodeType   string
			NodeAction string
			RefType    string
			RefID      *uuid.UUID
			Config     []byte
		}
		if err := tx.WithContext(ctx).Raw(`SELECT id,title,node_type,node_action,ref_type,ref_id,config FROM canvas.node WHERE document_id=? AND id IN ? AND NOT is_delete ORDER BY id FOR SHARE`, row.DocumentID, sourceIDs).Scan(&sources).Error; err != nil {
			return domain.Node{}, err
		}
		for _, source := range sources {
			current := domain.Node{ID: source.ID, Title: source.Title, NodeType: source.NodeType, NodeAction: source.NodeAction, RefType: source.RefType, RefID: source.RefID}
			if json.Unmarshal(source.Config, &current.Config) != nil {
				return domain.Node{}, promptdomain.ErrReferenceUnavailable
			}
			graph.Nodes = append(graph.Nodes, current)
		}
		if err := tx.WithContext(ctx).Raw(`SELECT id,edge_type,source_node_id,target_node_id,role,binding FROM canvas.edge WHERE document_id=? AND target_node_id=? AND source_node_id IN ? AND NOT is_delete ORDER BY id FOR SHARE`, row.DocumentID, nodeID, sourceIDs).Scan(&graph.Edges).Error; err != nil {
			return domain.Node{}, err
		}
	}
	if domain.ValidateGraph(graph) != nil {
		return domain.Node{}, promptdomain.ErrReferenceUnavailable
	}
	var snapshot subjectInput
	if json.Unmarshal(row.Input, &snapshot) != nil || snapshot.Kind != "subject" || snapshot.Title != node.Title {
		return domain.Node{}, promptdomain.ErrReferenceUnavailable
	}
	current, _ := json.Marshal(node.Config.Subject)
	frozen, _ := json.Marshal(snapshot.Subject)
	if !bytes.Equal(current, frozen) {
		return domain.Node{}, promptdomain.ErrReferenceUnavailable
	}
	return node, nil
}
