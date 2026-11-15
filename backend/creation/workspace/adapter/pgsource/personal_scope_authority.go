package pgsource

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

var _ workspacesource.PersonalScopeAuthorityStore = (*Store)(nil)

// PreparePersonalScopeAuthority creates an initial head only for the exact local
// preparation accepted through Identity's private registration contract. No caller
// can adopt an arbitrary existing scope or replace a control's original identity.
func (s *Store) PreparePersonalScopeAuthority(ctx context.Context, registration workspacesource.PersonalRegistration) (writefence.Control, error) {
	control := workspacesource.InitialScopeControl(registration)
	err := s.personalScopePolicy(ctx, registration, func(tx *gorm.DB) error {
		expected := control.Authority
		expected.Epoch = 0
		state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, expected, control.CoordinationID, false)
		if err != nil {
			return err
		}
		if state.Control != control {
			return writefence.ErrConflict
		}
		return nil
	})
	if err != nil {
		return writefence.Control{}, err
	}
	return control, nil
}

// CheckPersonalScopeAuthority checks today's exact head after all remote ACKs.
// An old completed opening is retained as history but cannot revive a closed head.
func (s *Store) CheckPersonalScopeAuthority(ctx context.Context, registration workspacesource.PersonalRegistration, control writefence.Control) error {
	if control != workspacesource.InitialScopeControl(registration) {
		return writefence.ErrConflict
	}
	return s.personalScopePolicy(ctx, registration, func(tx *gorm.DB) error {
		var head struct {
			Epoch             int64
			CoordinationID    uuid.UUID
			Blocked, Complete bool
		}
		read := tx.Raw(`SELECT epoch,coordination_id,blocked,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? FOR SHARE`, registration.ScopeID).Scan(&head)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || head.Epoch != control.Epoch || head.CoordinationID != control.CoordinationID || head.Blocked || !head.Complete {
			return writefence.ErrFenced
		}
		state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, control.CoordinationID)
		if err != nil {
			return err
		}
		if state.Control != control || !state.Complete {
			return writefence.ErrFenced
		}
		return nil
	})
}

func (s *Store) personalScopePolicy(ctx context.Context, registration workspacesource.PersonalRegistration, run func(*gorm.DB) error) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	if s == nil || s.db == nil || s.db.Statement == nil {
		return writefence.ErrUnavailable
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.ErrTransaction
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return writefence.ErrInvalid
		}
		var accepted workspacesource.PersonalRegistration
		read := tx.Raw(`SELECT operation_id,account_id,organization_id,scope_id,input_sha256 FROM workspace.personal_scope_preparation WHERE operation_id=?`, registration.OperationID).Scan(&accepted)
		if read.Error != nil {
			return fmt.Errorf("read accepted personal scope preparation: %w", read.Error)
		}
		if read.RowsAffected != 1 || accepted != registration {
			return workspacesource.ErrPersonalRegistration
		}
		var scope struct {
			Kind                string
			OwnerUserID, TeamID *uuid.UUID
			PhysicalScopeID     uuid.UUID
		}
		read = tx.Raw(`SELECT kind,owner_user_id,team_id,physical_scope_id FROM workspace.creation_scope WHERE id=? FOR SHARE`, registration.ScopeID).Scan(&scope)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || scope.Kind != "personal" || scope.OwnerUserID == nil || *scope.OwnerUserID != registration.AccountID || scope.TeamID != nil || scope.PhysicalScopeID != registration.OrganizationID {
			return workspacesource.ErrPersonalRegistration
		}
		var organization struct {
			Status   string
			IsDelete bool
		}
		read = tx.Raw(`SELECT status,is_delete FROM workspace.organization WHERE id=? FOR SHARE`, registration.OrganizationID).Scan(&organization)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || organization.Status != "active" || organization.IsDelete {
			return workspacesource.ErrPersonalRegistration
		}
		return run(tx)
	})
}
