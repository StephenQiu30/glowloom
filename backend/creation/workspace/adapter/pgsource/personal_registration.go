package pgsource

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// PersonalRegistrationStore commits the source receipt and both local resources.
// Only the private Identity-authenticated preparation service may consume this port.
type PersonalRegistrationStore struct{ db *gorm.DB }

// NewPersonalRegistrationStore injects the restricted Creation connection.
func NewPersonalRegistrationStore(db *gorm.DB) *PersonalRegistrationStore {
	return &PersonalRegistrationStore{db: db}
}

func (s *PersonalRegistrationStore) transaction(ctx context.Context, run func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return workspacesource.ErrPersonalRegistration
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.ErrTransaction
	}
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return workspacesource.ErrPersonalRegistration
		}
		return run(tx)
	})
}
func readPersonalRegistration(tx *gorm.DB, p workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, bool, error) {
	var receipt workspacesource.PersonalRegistration
	r := tx.Raw(`SELECT operation_id,account_id,organization_id,scope_id,input_sha256 FROM workspace.personal_scope_preparation WHERE operation_id=?`, p.OperationID).Scan(&receipt)
	if r.Error != nil {
		return receipt, false, r.Error
	}
	if r.RowsAffected == 1 && receipt != p {
		return receipt, true, workspacesource.ErrPersonalRegistration
	}
	return receipt, r.RowsAffected == 1, nil
}

// PreparePersonalRegistration is idempotent on the immutable original operation.
// Conflicting account or resource identities never adopt an existing scope.
func (s *PersonalRegistrationStore) PreparePersonalRegistration(ctx context.Context, p workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, error) {
	if err := p.Validate(); err != nil {
		return workspacesource.PersonalRegistration{}, err
	}
	var receipt workspacesource.PersonalRegistration
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		// A transaction advisory lock serializes this absent command; unique account,
		// scope and organization keys separately reject cross-command collisions.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, p.OperationID.String()).Error; err != nil {
			return err
		}
		var found bool
		var err error
		receipt, found, err = readPersonalRegistration(tx, p)
		if err != nil || found {
			return err
		}
		if err := tx.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'个人创作空间')`, p.OrganizationID).Error; err != nil {
			return fmt.Errorf("prepare personal organization: %w", err)
		}
		if err := tx.Exec(`INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES(?,'personal',?,?)`, p.ScopeID, p.AccountID, p.OrganizationID).Error; err != nil {
			return fmt.Errorf("prepare personal scope: %w", err)
		}
		if err := tx.Exec(`INSERT INTO workspace.personal_scope_preparation(operation_id,account_id,organization_id,scope_id,input_sha256) VALUES(?,?,?,?,?)`, p.OperationID, p.AccountID, p.OrganizationID, p.ScopeID, p.InputSHA256).Error; err != nil {
			return fmt.Errorf("record personal scope preparation: %w", err)
		}
		receipt, found, err = readPersonalRegistration(tx, p)
		if err != nil {
			return err
		}
		if !found {
			return workspacesource.ErrPersonalRegistration
		}
		return nil
	})
	if err != nil {
		return workspacesource.PersonalRegistration{}, err
	}
	return receipt, nil
}

// ResolvePersonalRegistration returns only a previously committed exact receipt.
// An absent command is unresolved and never licenses activation or cleanup.
func (s *PersonalRegistrationStore) ResolvePersonalRegistration(ctx context.Context, p workspacesource.PersonalRegistration) (workspacesource.PersonalRegistration, error) {
	if err := p.Validate(); err != nil {
		return workspacesource.PersonalRegistration{}, err
	}
	var receipt workspacesource.PersonalRegistration
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var found bool
		var err error
		receipt, found, err = readPersonalRegistration(tx, p)
		if err != nil {
			return err
		}
		if !found {
			return workspacesource.ErrPersonalRegistration
		}
		return nil
	})
	if err != nil {
		return workspacesource.PersonalRegistration{}, err
	}
	return receipt, nil
}
