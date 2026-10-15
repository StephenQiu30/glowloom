package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgbilling "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ProjectAccessFactory binds authorization to the operation transaction.
type ProjectAccessFactory func(*gorm.DB) application.ProjectAccess

// NewScopedStore injects independent request and worker authority.
func NewScopedStore(db *gorm.DB, request, worker ProjectAccessFactory, platform ...PlatformAccessFactory) *Store {
	s := &Store{db: db, requestAccess: request, workerAccess: worker}
	if len(platform) == 1 {
		s.platformAccess = platform[0]
	}
	return s
}

func (s *Store) withDB(db *gorm.DB) *Store { bound := *s; bound.db = db; return &bound }

func (s *Store) requireProject(tx *gorm.DB, actor identity.Principal, project uuid.UUID, write bool) error {
	if s.requestAccess == nil || s.requestAccess(tx) == nil {
		return identity.ErrForbidden
	}
	return s.requestAccess(tx).AuthorizeOperation(tx.Statement.Context, actor, project, write)
}

func (s *Store) requireWorker(tx *gorm.DB, operation uuid.UUID, write bool) error {
	if s.workerAccess == nil || s.workerAccess(tx) == nil {
		return identity.ErrForbidden
	}
	var row struct {
		ActorID          *uuid.UUID
		ProjectID, OrgID uuid.UUID
	}
	r := tx.Raw(`SELECT o.confirmed_by AS actor_id,o.project_id,p.org_id FROM operation.operation o JOIN workspace.project p ON p.id=o.project_id WHERE o.id=? AND NOT o.is_delete`, operation).Scan(&row)
	if r.Error != nil {
		return fmt.Errorf("read execution actor: %w", r.Error)
	}
	if r.RowsAffected != 1 || row.ActorID == nil || *row.ActorID == uuid.Nil {
		return application.ErrExecutionAuthorityRevoked
	}
	actor := identity.Principal{ID: *row.ActorID, OrgID: row.OrgID}
	if err := s.workerAccess(tx).AuthorizeOperation(tx.Statement.Context, actor, row.ProjectID, write); err != nil {
		if errors.Is(err, identity.ErrForbidden) {
			return application.ErrExecutionAuthorityRevoked
		}
		return err
	}
	var present int
	r = tx.Raw(`SELECT 1 FROM operation.operation WHERE id=? AND project_id=? AND confirmed_by=? AND NOT is_delete FOR SHARE`, operation, row.ProjectID, *row.ActorID).Scan(&present)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return application.ErrExecutionAuthorityRevoked
	}
	return nil
}

// AuthorizeWorker rechecks the durable confirming account before external execution.
func (s *Store) AuthorizeWorker(ctx context.Context, operation uuid.UUID, write bool) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.requireWorker(tx, operation, write) })
}

type billingAccess struct {
	store *Store
	tx    *gorm.DB
}

func (a billingAccess) AuthorizeBilling(ctx context.Context, actor identity.Principal, project uuid.UUID, write bool) error {
	return a.store.requireProject(a.tx.WithContext(ctx), actor, project, write)
}
func (s *Store) billingStore(tx *gorm.DB) *pgbilling.Store {
	return pgbilling.NewScopedStore(tx, func(db *gorm.DB) billing.ProjectAccess { return billingAccess{s, db} })
}

func (s *Store) requireBatchWorker(tx *gorm.DB, batch uuid.UUID, write bool) error {
	var rows []struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM operation.operation WHERE batch_id=? AND status NOT IN ('quoted','expired') AND NOT is_delete ORDER BY id`, batch).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return application.ErrExecutionAuthorityRevoked
	}
	for _, row := range rows {
		if err := s.requireWorker(tx, row.ID, write); err != nil {
			return err
		}
	}
	return nil
}

// PlatformAccessFactory binds privileged commands to the caller transaction.
type PlatformAccessFactory func(*gorm.DB) application.PlatformAccess

func (s *Store) requirePlatform(tx *gorm.DB, actor identity.Principal) error {
	if s.platformAccess == nil || s.platformAccess(tx) == nil {
		return identity.ErrForbidden
	}
	return s.platformAccess(tx).CheckPlatformSession(tx.Statement.Context, actor, time.Now().UTC())
}

// AuthorizeBatchWorker rechecks every durable confirmer before starting a batch.
func (s *Store) AuthorizeBatchWorker(ctx context.Context, batch uuid.UUID) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.requireBatchWorker(tx, batch, true) })
}
