package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"

	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// ErrInputStale identifies a saved source revision conflict.
var ErrInputStale = domain.ErrInputStale

// ReadInput retains owner share locks through the caller's transaction; confirm permits layout-only changes.
func (s *DraftStore) ReadInput(ctx context.Context, a identityapp.Principal, p uuid.UUID, source domain.InputSource, confirm bool) (domain.Draft, error) {
	var out domain.Draft
	if !source.Valid() {
		return out, domain.ErrInvalidDraft
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, true); err != nil {
			return err
		}
		if source.Kind == "prompt_draft" {
			saved, err := readDraft(tx, a.ID, p, source.DraftID)
			if err != nil {
				return err
			}
			if saved.Revision != source.Revision {
				return ErrInputStale
			}
			out = saved.Draft
		} else {
			var document struct {
				Revision      int64
				SchemaVersion int
			}
			r := tx.Raw(`SELECT revision,schema_version FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, source.CanvasID, p).Scan(&document)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || (!confirm && document.Revision != source.CanvasRevision) {
				return ErrInputStale
			}
			if document.SchemaVersion != canvasapp.SchemaVersion {
				return fmt.Errorf("%w: %w", ErrInputStale, canvasdomain.ErrGraphReadOnly)
			}
			graph, err := readCanvasInputGraph(tx, source.CanvasID)
			if err != nil {
				if errors.Is(err, canvasdomain.ErrGraphReadOnly) {
					return fmt.Errorf("%w: %w", ErrInputStale, err)
				}
				return err
			}
			var row struct {
				InputRevision int64
				Config        []byte
			}
			r = tx.Raw(`SELECT input_revision,config FROM canvas.node WHERE id=? AND document_id=? AND NOT is_delete AND (node_type IN ('image_generation','video_generation') OR (node_type IN ('text','image','video','audio') AND node_action='resource' AND config->'draft'->'composer' IS NOT NULL)) FOR SHARE`, source.NodeID, source.CanvasID).Scan(&row)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || row.InputRevision != source.InputRevision {
				return ErrInputStale
			}
			var config canvasdomain.NodeConfig
			if json.Unmarshal(row.Config, &config) != nil || config.Draft == nil || config.Draft.Composer == nil {
				return domain.ErrInvalidDraft
			}
			for _, node := range graph.Nodes {
				if node.ID == source.NodeID {
					effective, err := canvasdomain.GenerationInput(graph, node)
					if err != nil || effective.Composer == nil {
						return domain.ErrInvalidDraft
					}
					out = *effective.Composer
					if err := s.resolveGraphSubjectPins(ctx, tx, a, p, graph, node, &out); err != nil {
						return err
					}
				}
			}
		}
		return ValidateDraftSources(tx, a, p, out, true, s.sources(tx))
	})
	return out, err
}

// AuthorizeOwner deliberately ignores content readiness so unresolved submissions remain discoverable after edits.
func (s *DraftStore) AuthorizeOwner(ctx context.Context, a identityapp.Principal, p uuid.UUID, source domain.InputSource) error {
	if !source.Valid() {
		return domain.ErrInvalidDraft
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, true); err != nil {
			return err
		}
		var found int
		var result *gorm.DB
		if source.Kind == "prompt_draft" {
			result = tx.Raw(`SELECT 1 FROM workspace.prompt_draft WHERE id=? AND project_id=? AND actor_id=? FOR SHARE`, source.DraftID, p, a.ID).Scan(&found)
		} else {
			result = tx.Raw(`SELECT 1 FROM canvas.document d JOIN canvas.node n ON n.document_id=d.id WHERE d.id=? AND d.project_id=? AND n.id=? AND NOT d.is_delete AND NOT n.is_delete FOR SHARE OF d,n`, source.CanvasID, p, source.NodeID).Scan(&found)
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return application.ErrDraftNotFound
		}
		return nil
	})
}

// The document share lock held by ReadInput serializes this graph read with
// authoritative edits. Unknown graph fields must not bypass the canvas's
// read-only admission rule merely because the selected composer is understood.
func readCanvasInputGraph(tx *gorm.DB, id uuid.UUID) (canvasdomain.Document, error) {
	var rows []struct{ Body []byte }
	if err := tx.Raw(`SELECT row_to_json(n) AS body FROM (
SELECT id,title,node_type,node_action,ref_type,ref_id,config,x,y,width,height,parent_id,z_index,last_operation_id,COALESCE(input_revision,0) AS input_revision
FROM canvas.node WHERE document_id=? AND NOT is_delete ORDER BY id) n`, id).Scan(&rows).Error; err != nil {
		return canvasdomain.Document{}, err
	}
	doc := canvasdomain.Document{ID: id}
	for _, row := range rows {
		var node canvasdomain.Node
		if err := json.Unmarshal(row.Body, &node); err != nil {
			return canvasdomain.Document{}, fmt.Errorf("%w: decode input graph: %w", canvasdomain.ErrGraphReadOnly, err)
		}
		doc.Nodes = append(doc.Nodes, node)
	}
	if err := tx.Raw(`SELECT id,edge_type,source_node_id,target_node_id,role,binding FROM canvas.edge WHERE document_id=? AND NOT is_delete ORDER BY id`, id).Scan(&doc.Edges).Error; err != nil {
		return canvasdomain.Document{}, err
	}
	if err := canvasdomain.ValidateGraph(doc); err != nil {
		return canvasdomain.Document{}, fmt.Errorf("%w: %w", canvasdomain.ErrGraphReadOnly, err)
	}
	return doc, nil
}
