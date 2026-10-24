package canvasfixture

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// LegacySeeder inserts pre-Yjs test facts to exercise lossless migration. It is
// deliberately not a collaboration client and rejects already-initialized data.
type LegacySeeder struct {
	db *gorm.DB
	*application.Service
}

// NewLegacySeeder constructs a historical-data fixture.
func NewLegacySeeder(db *gorm.DB, store application.Store) *LegacySeeder {
	return &LegacySeeder{db, application.NewService(store)}
}

// Submit inserts initial legacy entities without starting collaboration.
func (s *LegacySeeder) Submit(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input EditInput) (EditReceipt, error) {
	result := EditReceipt{OpID: key}
	before, err := s.Get(ctx, actor, id)
	if err != nil {
		return result, err
	}
	after, err := domain.ApplyOperations(before, input.Operations)
	if err != nil {
		return result, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM canvas.collab_document WHERE document_id=?`, id).Scan(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("legacy fixture cannot change initialized Yjs state")
		}
		for _, op := range input.Operations {
			switch op.Type {
			case "create_node":
				var n domain.Node
				for _, candidate := range after.Document.Nodes {
					if candidate.ID == op.Node.ID {
						n = candidate
					}
				}
				config, err := json.Marshal(n.Config)
				if err != nil {
					return err
				}
				var ref, revision any
				if n.RefType != "" {
					ref = n.RefType
				}
				if n.InputRevision > 0 {
					revision = n.InputRevision
				}
				if err := tx.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,ref_type,ref_id,config,x,y,width,height,parent_id,z_index,input_revision,content_version) VALUES(?,?,?,?,?,?,?,?::jsonb,?,?,?,?,?,?,?,1)`, n.ID, id, n.Title, n.NodeType, n.NodeAction, ref, n.RefID, string(config), n.X, n.Y, n.Width, n.Height, n.ParentID, n.ZIndex, revision).Error; err != nil {
					return err
				}
			case "connect":
				e := op.Edge
				var binding any
				if len(e.Binding) > 0 {
					binding = string(e.Binding)
				}
				if err := tx.Exec(`INSERT INTO canvas.edge(id,document_id,edge_type,source_node_id,target_node_id,binding) VALUES(?,?,?,?,?,?::jsonb)`, e.ID, id, e.EdgeType, e.SourceNodeID, e.TargetNodeID, binding).Error; err != nil {
					return err
				}
			default:
				return fmt.Errorf("legacy fixture supports initial entities only")
			}
		}
		for _, input := range after.Inputs {
			body, err := json.Marshal(input.Draft)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO canvas.node_input(node_id,input_revision,draft,created_by) VALUES(?,?,?::jsonb,?)`, input.NodeID, input.Revision, string(body), actor.ID).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE canvas.document SET revision=revision+1 WHERE id=?`, id).Error
	})
	if err == nil {
		result.Status = "accepted"
		result.Revision = before.Revision + 1
	}
	return result, err
}

// Edit is only for constructing pre-Yjs historical fixtures.
func (s *LegacySeeder) Edit(ctx context.Context, actor identity.Principal, id uuid.UUID, key string, input EditInput) (domain.Document, error) {
	if _, err := s.Submit(ctx, actor, id, key, input); err != nil {
		return domain.Document{}, err
	}
	return s.Get(ctx, actor, id)
}
