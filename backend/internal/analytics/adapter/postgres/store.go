// Package postgres reads canonical execution facts without shadow events or billing writes.
package postgres

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/analytics/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Access reuses the identity authority in the same consumer transaction.
type Access interface {
	CheckSession(context.Context, identity.Principal, func() time.Time) error
	CheckPlatformSession(context.Context, identity.Principal, func() time.Time) error
}

// Store owns a scoped database and transaction-bound access factory.
type Store struct {
	db     *gorm.DB
	access func(*gorm.DB) Access
}

// NewStore injects existing identity checks without coupling the projection to identity storage.
func NewStore(db *gorm.DB, access func(*gorm.DB) Access) *Store { return &Store{db, access} }

// CheckSession rechecks revoked, disabled and epoch-invalid callers.
func (s *Store) CheckSession(ctx context.Context, a identity.Principal, clock func() time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.access(tx).CheckSession(ctx, a, clock) })
}

// ReadCreation reads a maximum of 10,001 operations, then signals explicit truncation.
func (s *Store) ReadCreation(ctx context.Context, a identity.Principal, f domain.Filter, clock func() time.Time) ([]domain.Fact, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var facts []domain.Fact
	truncated := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := f.Validate(clock()); err != nil {
			return err
		}
		access := s.access(tx)
		if f.Scope == "platform" {
			if err := access.CheckPlatformSession(ctx, a, clock); err != nil {
				return err
			}
		} else if err := access.CheckSession(ctx, a, clock); err != nil {
			return err
		}
		if err := tx.Exec(`SET LOCAL statement_timeout='3s'`).Error; err != nil {
			return err
		}
		query := tx.Table("operation.operation AS o").Select(`o.id,o.confirmed_at AS accepted_at,o.status,o.model_profile_version_id AS model_id,o.origin,left(m.display_name,100) AS model_name,
   (SELECT count(DISTINCT c.attempt) FROM operation.provider_call c WHERE c.operation_id=o.id AND c.action='submit' AND c.dispatch_started_at IS NOT NULL AND NOT c.is_delete) AS attempts,
   (SELECT count(*) FROM operation.operation_output x WHERE x.operation_id=o.id AND NOT x.is_delete) AS outputs`).Joins(`JOIN workspace.project p ON p.id=o.project_id`).Joins(`LEFT JOIN catalog.model_profile_version v ON v.id=o.model_profile_version_id`).Joins(`LEFT JOIN catalog.model_profile m ON m.id=v.model_profile_id`).Where(`NOT o.is_delete AND NOT p.is_delete AND o.confirmed_at>=? AND o.confirmed_at<? AND o.confirmed_by IS NOT NULL AND o.origin<>'upload' AND o.status NOT IN ('draft','quoted','expired')`, f.From, f.To)
		if f.Scope == "personal" {
			query = query.Where(`o.confirmed_by=? AND EXISTS(SELECT 1 FROM workspace.creation_scope sc WHERE sc.id=p.owner_scope_id AND sc.physical_scope_id=p.org_id AND (
    (sc.kind='personal' AND sc.owner_user_id=?) OR (sc.kind='team' AND EXISTS(SELECT 1 FROM workspace.team t JOIN workspace.team_member m ON m.team_id=t.id WHERE t.id=sc.team_id AND t.status IN ('active','archived') AND m.user_id=? AND m.status='active' AND m.role IN ('member','team_admin')))))`, a.ID, a.ID, a.ID)
		}
		if f.ProjectID != nil {
			query = query.Where("o.project_id=?", *f.ProjectID)
		}
		if f.ModelID != nil {
			query = query.Where("o.model_profile_version_id=?", *f.ModelID)
		}
		if f.Origin != "" {
			query = query.Where("o.origin=?", f.Origin)
		}
		if err := query.Order("o.confirmed_at DESC,o.id DESC").Limit(10001).Scan(&facts).Error; err != nil {
			return err
		}
		if len(facts) > 10000 {
			facts = facts[:10000]
			truncated = true
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return facts, truncated, err
}
