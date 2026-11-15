package pgsource

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var _ source.ProjectClosureAccess = (*Store)(nil)

// ReadProjectClosureView reads actual current scope/project ownership. Historical
// soft-deletion is visible only to this lifecycle command's authorized replay.
func (s *Store) ReadProjectClosureView(ctx context.Context, identity source.Identity, scopeID, projectID uuid.UUID) (source.ProjectClosureView, error) {
	if identity.AccountID == uuid.Nil || identity.SessionID == uuid.Nil || identity.CredentialEpoch < 1 || scopeID == uuid.Nil || projectID == uuid.Nil {
		return source.ProjectClosureView{}, writefence.ErrInvalid
	}
	scope, err := s.ReadProjectCreationScope(ctx, identity.AccountID, scopeID)
	if err != nil {
		return source.ProjectClosureView{}, err
	}
	view := source.ProjectClosureView{Scope: scope}
	err = s.projectTransaction(ctx, func(tx *gorm.DB) error {
		org, err := lockScope(ctx, tx, identity.AccountID, scopeID)
		if err != nil {
			return err
		}
		if org != scope.OrganizationID {
			return writefence.ErrConflict
		}
		var binding struct{ OwnerScopeID uuid.UUID }
		read := tx.Raw(`SELECT owner_scope_id FROM workspace.project WHERE id=? AND org_id=? FOR SHARE`, projectID, org).Scan(&binding)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || binding.OwnerScopeID != scopeID {
			return source.ErrProjectNotFound
		}
		read = tx.Raw(`SELECT id,org_id,owner_id,initial_canvas_id AS canvas_id,style_key,name,description,aspect_ratio,style_type,COALESCE(style_subtype,'') AS style_subtype,style_preset_id,resolution,allow_overseas_models,status,archived_at,is_delete,delete_time,purge_after,revision,create_time,update_time,cover_asset_id,independent_canvas,independent_media FROM workspace.project WHERE id=?`, projectID).Scan(&view.Project)
		if read.Error != nil {
			return read.Error
		}
		if view.Project.Validate() != nil {
			return writefence.ErrConflict
		}
		var head struct{ Epoch int64 }
		read = tx.Raw(`SELECT epoch FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind='project' AND authority_id=?`, projectID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || head.Epoch < 1 {
			return writefence.ErrFenced
		}
		view.Authority = writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Project, ID: projectID, Epoch: head.Epoch}
		return nil
	})
	return view, err
}
