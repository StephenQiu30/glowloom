package pgsource

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var _ workspacesource.ProjectCreationSource = (*Store)(nil)

// ReadProjectCreationScope reads actual policy and an already complete/open head.
// It cannot open a missing head or derive its epoch from the scope revision.
func (s *Store) ReadProjectCreationScope(ctx context.Context, account, scopeID uuid.UUID) (workspacesource.ProjectCreationScope, error) {
	if account == uuid.Nil || scopeID == uuid.Nil {
		return workspacesource.ProjectCreationScope{}, writefence.ErrInvalid
	}
	var scope workspacesource.ProjectCreationScope
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		org, err := lockScope(ctx, tx, account, scopeID)
		if err != nil {
			return err
		}
		var resource struct{ Revision int64 }
		if err := tx.Raw(`SELECT revision FROM workspace.creation_scope WHERE id=? FOR SHARE`, scopeID).Scan(&resource).Error; err != nil {
			return err
		}
		var head struct {
			Epoch             int64
			Complete, Blocked bool
		}
		read := tx.Raw(`SELECT epoch,complete,blocked FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? FOR SHARE`, scopeID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || head.Epoch < 1 || !head.Complete || head.Blocked {
			return writefence.ErrFenced
		}
		scope = workspacesource.ProjectCreationScope{ScopeID: scopeID, OrganizationID: org, Revision: resource.Revision, Authority: writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scopeID, Epoch: head.Epoch}}
		return nil
	})
	return scope, err
}

func checkCreationReplayVisibility(tx *gorm.DB, account, scope, org, project uuid.UUID) error {
	var visible int
	read := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? AND org_id=? AND owner_scope_id=? AND owner_id=? AND NOT is_delete AND status IN ('active','archived') FOR SHARE`, project, org, scope, account).Scan(&visible)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 {
		return workspacesource.ErrProjectNotFound
	}
	return nil
}

// LookupProjectCreation returns the original receipt only after current local
// resource authorization. Legacy receipts remain replayable without fabricating
// a new source preparation, wallet ACK or authority head.
func (s *Store) LookupProjectCreation(ctx context.Context, id workspacesource.Identity, scope workspacesource.ProjectCreationScope, key uuid.UUID, hash string) (workspacesource.ProjectCreationLookup, error) {
	if id.AccountID == uuid.Nil || id.SessionID == uuid.Nil || id.CredentialEpoch < 1 || scope.ScopeID == uuid.Nil || scope.OrganizationID == uuid.Nil || key == uuid.Nil {
		return workspacesource.ProjectCreationLookup{}, writefence.ErrInvalid
	}
	var result workspacesource.ProjectCreationLookup
	err := s.projectTransaction(ctx, func(tx *gorm.DB) error {
		physical, err := lockScope(ctx, tx, id.AccountID, scope.ScopeID)
		if err != nil {
			return err
		}
		if physical != scope.OrganizationID {
			return workspacesource.ErrProjectPreparationConflict
		}
		var preparation struct{ PreparationID uuid.UUID }
		read := tx.Raw(`SELECT preparation_id FROM workspace.project_preparation WHERE actor_id=? AND idempotency_key=?`, id.AccountID, key).Scan(&preparation)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected == 1 {
			state, err := readProjectPreparation(tx, preparation.PreparationID, false)
			if err != nil {
				return err
			}
			if state.Spec.ActorID != id.AccountID || state.Spec.IdempotencyKey != key || state.Spec.ScopeID != scope.ScopeID || state.Spec.Project.OrgID != scope.OrganizationID || state.Spec.InputSHA256 != hash {
				return workspacesource.ErrIdempotencyConflict
			}
			if state.Activation != nil {
				if err := checkCreationReplayVisibility(tx, id.AccountID, scope.ScopeID, scope.OrganizationID, state.Spec.Project.ID); err != nil {
					return err
				}
			}
			result = workspacesource.ProjectCreationLookup{Spec: &state.Spec, Activation: state.Activation}
			return nil
		}
		var legacy struct {
			RequestHash  string
			ResponseBody []byte
		}
		read = tx.Raw(`SELECT request_hash,response_body FROM workspace.project_creation_receipt WHERE actor_id=? AND idem_key=?`, id.AccountID, key.String()).Scan(&legacy)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return workspacesource.ErrProjectPreparationUnknown
		}
		if legacy.RequestHash != hash {
			return workspacesource.ErrIdempotencyConflict
		}
		var project domain.Project
		if projectJSON(legacy.ResponseBody, &project) != nil || project.Validate() != nil || project.OrgID != scope.OrganizationID || project.OwnerID != id.AccountID || project.CanvasID == uuid.Nil || project.Status != domain.ProjectStatusActive {
			return workspacesource.ErrProjectPreparationConflict
		}
		if err := checkCreationReplayVisibility(tx, id.AccountID, scope.ScopeID, scope.OrganizationID, project.ID); err != nil {
			return err
		}
		created := createdProject(project)
		result.Activation = &created
		return nil
	})
	return result, err
}
