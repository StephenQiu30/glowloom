package postgres

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

// AssetCreationFactory supplies transaction-bound workspace ownership.
type AssetCreationFactory func(*gorm.DB) application.AssetCreationAccess

// AssetCreationStore owns atomic direct asset creation and its immutable replay.
type AssetCreationStore struct {
	db     *gorm.DB
	access AssetCreationFactory
}

// NewAssetCreationStore injects the database and owning-project creation capability.
func NewAssetCreationStore(db *gorm.DB, access AssetCreationFactory) *AssetCreationStore {
	return &AssetCreationStore{db: db, access: access}
}

// CreateAsset reuses workspace's project/main-canvas creation under the outer receipt.
func (s *AssetCreationStore) CreateAsset(ctx context.Context, actor identity.Principal, key uuid.UUID, input application.AssetCreationInput) (application.CreatedAssetCanvas, error) {
	var result application.CreatedAssetCanvas
	input, err := application.NormalizeAssetCreation(input)
	if err != nil {
		return result, err
	}
	if s == nil || s.db == nil || s.access == nil || key == uuid.Nil {
		return result, domain.ErrInvalidCommand
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := s.access(tx)
		if access == nil {
			return application.ErrNotFound
		}
		if err := access.AuthorizeAssetCreation(ctx, actor); err != nil {
			return err
		}
		body, err := json.Marshal(input)
		if err != nil {
			return err
		}
		hash := requestHash("POST /api/canvas-assets/"+actor.OrgID.String()+"/"+actor.ScopeID.String(), body)
		// Keep the existing global actor/key conflict guard, then consult our
		// durable creation receipt even after the common 24-hour cache expires.
		_, err = replay(tx, actor.ID, key.String(), hash, &result)
		if err != nil {
			return err
		}
		var recorded struct {
			RequestHash  string
			ResponseBody []byte
			ScopeID      uuid.UUID
			ProjectID    uuid.UUID
			DocumentID   uuid.UUID
		}
		read := tx.Raw(`SELECT request_hash,response_body,scope_id,project_id,document_id FROM canvas.asset_creation_receipt WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&recorded)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			if recorded.RequestHash != hash || recorded.ScopeID != actor.ScopeID {
				return application.ErrIdempotencyConflict
			}
			if err := json.Unmarshal(recorded.ResponseBody, &result); err != nil {
				return err
			}
			if result.ID != recorded.DocumentID || result.ProjectID != recorded.ProjectID || result.Name != input.Name || result.Kind != input.Kind || result.TemplateKey != input.TemplateKey || result.Collection != "current" || result.Revision != 1 || result.ProjectRevision != 1 || result.IsTemplate {
				return domain.ErrInvalidCommand
			}
			var visible bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project p JOIN canvas.document d ON d.id=p.initial_canvas_id AND d.project_id=p.id WHERE p.id=? AND d.id=? AND p.org_id=? AND p.owner_scope_id=? AND NOT p.is_delete AND NOT d.is_delete)`, result.ProjectID, result.ID, actor.OrgID, actor.ScopeID).Scan(&visible).Error; err != nil {
				return err
			}
			if !visible {
				return application.ErrNotFound
			}
			return nil
		}
		projectKey := uuid.NewSHA1(key, []byte("canvas-assets/owning-project"))
		project, err := access.CreateAssetProject(ctx, actor, projectKey, input.Name)
		if err != nil {
			return err
		}
		if project.ProjectID == uuid.Nil || project.CanvasID == uuid.Nil {
			return domain.ErrInvalidCommand
		}
		var matches bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project p JOIN canvas.document d ON d.id=p.initial_canvas_id AND d.project_id=p.id WHERE p.id=? AND d.id=? AND p.org_id=? AND p.owner_scope_id=? AND p.owner_id=? AND p.status='active' AND NOT p.is_delete AND NOT d.is_delete AND p.revision=1 AND d.revision=1 AND d.asset_collection='current')`, project.ProjectID, project.CanvasID, actor.OrgID, actor.ScopeID, actor.ID).Scan(&matches).Error; err != nil {
			return err
		}
		if !matches {
			return application.ErrNotFound
		}
		before, err := load(tx, project.CanvasID, true)
		if err != nil {
			return err
		}
		if len(before.Nodes) != 0 || len(before.Edges) != 0 {
			return domain.ErrInvalidCommand
		}
		if err := tx.Exec(`UPDATE canvas.document SET kind=? WHERE id=?`, input.Kind, project.CanvasID).Error; err != nil {
			return err
		}
		before.Kind = input.Kind
		if input.TemplateKey != "" {
			candidate, err := domain.ApplySystemAssetTemplate(before, input.TemplateKey)
			if err != nil {
				return err
			}
			if err := advanceContentVersions(before, &candidate.Document); err != nil {
				return err
			}
			if err := saveGraph(tx, before, candidate.Document); err != nil {
				return err
			}
			if err := saveInputs(tx, actor.ID, candidate.Inputs); err != nil {
				return err
			}
			if err := savedAudit(tx, actor, candidate.Document, key.String()); err != nil {
				return err
			}
		}
		if err := changed(tx, actor, project.CanvasID, project.ProjectID, 1); err != nil {
			return err
		}
		result = application.CreatedAssetCanvas{ID: project.CanvasID, ProjectID: project.ProjectID, Name: input.Name, Kind: input.Kind, Collection: "current", Revision: 1, ProjectRevision: 1, TemplateKey: input.TemplateKey, IsTemplate: false}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO canvas.asset_creation_receipt(actor_id,scope_id,idem_key,request_hash,project_id,document_id,response_body) VALUES(?,?,?,?,?,?,?::jsonb)`, actor.ID, actor.ScopeID, key, hash, result.ProjectID, result.ID, string(response)).Error; err != nil {
			return err
		}
		if err := receipt(tx, actor.ID, key.String(), hash, 201, result); err != nil {
			return fmt.Errorf("record asset creation: %w", err)
		}
		return nil
	})
	return result, err
}
