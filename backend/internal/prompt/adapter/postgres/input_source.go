package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrInputStale = errors.New("quote_source_stale")

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
			var revision int64
			r := tx.Raw(`SELECT revision FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, source.CanvasID, p).Scan(&revision)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 || (!confirm && revision != source.CanvasRevision) {
				return ErrInputStale
			}
			var row struct {
				InputRevision int64
				Config        []byte
			}
			r = tx.Raw(`SELECT input_revision,config FROM canvas.node WHERE id=? AND document_id=? AND NOT is_delete AND node_type IN ('image_generation','video_generation') FOR SHARE`, source.NodeID, source.CanvasID).Scan(&row)
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
			out = *config.Draft.Composer
		}
		return ValidateDraftSources(tx, a, p, out, true)
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
