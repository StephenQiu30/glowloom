package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// ProjectContentAccessStore binds script access to its already-open transaction.
type ProjectContentAccessStore struct {
	tx      *gorm.DB
	scopes  application.RequestScopeResolver
	workers application.WorkerScopeResolver
}

// NewProjectContentAccessStore is retained for callers awaiting scope migration.
// It has no authority and fails closed. Use a request or worker constructor.
func NewProjectContentAccessStore(tx *gorm.DB) *ProjectContentAccessStore {
	return &ProjectContentAccessStore{tx: tx}
}

// NewScopedProjectContentAccessStore binds explicit ownership and request-session
// authorization to the content transaction during the coordinated scope cutover.
func NewScopedProjectContentAccessStore(tx *gorm.DB, scopes application.RequestScopeResolver) *ProjectContentAccessStore {
	return &ProjectContentAccessStore{tx: tx, scopes: scopes}
}

// NewWorkerProjectContentAccessStore binds background work to live account and
// membership checks. The durable project row supplies ownership, never an event role.
func NewWorkerProjectContentAccessStore(tx *gorm.DB, scopes application.WorkerScopeResolver) *ProjectContentAccessStore {
	return &ProjectContentAccessStore{tx: tx, workers: scopes}
}

// Authorize checks current account and project facts, retaining its row lock.
func (s *ProjectContentAccessStore) Authorize(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, write bool) (application.ProjectContentAccess, error) {
	return s.authorizeStatus(ctx, actor, projectID, write, false)
}

// AuthorizeCopyTarget authorizes only unpublished targets in the coordinator transaction.
func (s *ProjectContentAccessStore) AuthorizeCopyTarget(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (application.ProjectContentAccess, error) {
	return s.authorizeStatus(ctx, actor, projectID, true, true)
}

func (s *ProjectContentAccessStore) authorizeStatus(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, write, copying bool) (application.ProjectContentAccess, error) {
	if s == nil || s.tx == nil || s.tx.Statement == nil {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	if _, transaction := s.tx.Statement.ConnPool.(gorm.TxCommitter); !transaction {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	if s.scopes == nil && s.workers == nil {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	ownerScope := actor.ScopeID
	if s.workers != nil {
		// Read an ownership candidate first, then acquire account/scope locks and
		// recheck the exact mapping in the locked project query below.
		var binding struct{ OwnerScopeID *uuid.UUID }
		read := s.tx.WithContext(ctx).Raw(`SELECT owner_scope_id FROM workspace.project WHERE id=? AND org_id=?`, projectID, actor.OrgID).Scan(&binding)
		if read.Error != nil {
			return application.ProjectContentAccess{}, fmt.Errorf("read worker project ownership: %w", read.Error)
		}
		if read.RowsAffected != 1 || binding.OwnerScopeID == nil {
			return application.ProjectContentAccess{}, application.ErrProjectNotFound
		}
		ownerScope = *binding.OwnerScopeID
		scope, err := s.workers.Resolve(ctx, actor.ID, ownerScope, write)
		if err != nil {
			return application.ProjectContentAccess{}, err
		}
		if scope.ID != ownerScope || scope.PhysicalScopeID != actor.OrgID {
			return application.ProjectContentAccess{}, application.ErrScopeNotFound
		}
	} else {
		scope, err := s.scopes.Resolve(ctx, actor, ownerScope, write)
		if err != nil {
			return application.ProjectContentAccess{}, err
		}
		if scope.ID != ownerScope || scope.PhysicalScopeID != actor.OrgID {
			return application.ProjectContentAccess{}, application.ErrScopeNotFound
		}
	}
	if projectID == uuid.Nil {
		return application.ProjectContentAccess{}, application.ErrProjectNotFound
	}
	query := `SELECT id AS project_id,org_id,owner_scope_id,revision,status FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete`
	if copying {
		query += ` AND status='copying'`
	} else {
		query += ` AND status IN ('active','archived')`
	}
	args := []any{projectID, actor.OrgID}
	query += ` AND owner_scope_id=?`
	args = append(args, ownerScope)
	if write {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	var row struct {
		application.ProjectContentAccess
		Status string
	}
	read := s.tx.WithContext(ctx).Raw(query, args...).Scan(&row)
	if read.Error != nil {
		return application.ProjectContentAccess{}, fmt.Errorf("authorize script project: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return application.ProjectContentAccess{}, application.ErrProjectNotFound
	}
	if write && !copying && row.Status != "active" {
		return application.ProjectContentAccess{}, domain.ErrProjectStateConflict
	}
	if row.ProjectID != projectID || row.OrgID != actor.OrgID || row.Revision < 1 {
		return application.ProjectContentAccess{}, application.ErrProjectDependencyUnavailable
	}
	return row.ProjectContentAccess, nil
}

// TouchContent advances the project in the same transaction as script changes.
func (s *ProjectContentAccessStore) TouchContent(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, expected int64) (int64, error) {
	if expected < 1 || expected >= math.MaxInt32 {
		return 0, domain.ErrProjectRevisionConflict
	}
	facts, err := s.Authorize(ctx, actor, projectID, true)
	if err != nil {
		return 0, err
	}
	if facts.Revision != expected {
		return 0, domain.ErrProjectRevisionConflict
	}
	changed := s.tx.WithContext(ctx).Exec(`UPDATE workspace.project SET revision=revision+1,update_time=now() WHERE id=? AND org_id=? AND revision=? AND NOT is_delete AND status='active'`, projectID, actor.OrgID, expected)
	if changed.Error != nil {
		return 0, fmt.Errorf("advance script project revision: %w", changed.Error)
	}
	if changed.RowsAffected != 1 {
		return 0, domain.ErrProjectRevisionConflict
	}
	var updatedAt time.Time
	if err := s.tx.WithContext(ctx).Raw(`SELECT update_time FROM workspace.project WHERE id=? AND org_id=?`, projectID, actor.OrgID).Scan(&updatedAt).Error; err != nil {
		return 0, fmt.Errorf("read content change time: %w", err)
	}
	event, err := application.ProjectContentChangedEvent(actor, projectID, expected+1, updatedAt)
	if err != nil {
		return 0, err
	}
	written := s.tx.WithContext(ctx).Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, event.ID, event.Topic, event.PartitionKey, string(event.Payload))
	if written.Error != nil {
		return 0, fmt.Errorf("persist content invalidation: %w", written.Error)
	}
	if written.RowsAffected != 1 {
		return 0, application.ErrProjectDependencyUnavailable
	}
	return expected + 1, nil
}

// OwnerScopeID returns an authorized project's durable mapping to transfer consumers.
func (s *ProjectContentAccessStore) OwnerScopeID(ctx context.Context, actor identityapp.Principal, id uuid.UUID, write bool) (uuid.UUID, error) {
	if _, err := s.Authorize(ctx, actor, id, write); err != nil {
		return uuid.Nil, err
	}
	var row struct{ OwnerScopeID uuid.UUID }
	r := s.tx.WithContext(ctx).Raw(`SELECT owner_scope_id FROM workspace.project WHERE id=? AND org_id=? FOR SHARE`, id, actor.OrgID).Scan(&row)
	if r.Error != nil {
		return uuid.Nil, r.Error
	}
	if r.RowsAffected != 1 || row.OwnerScopeID == uuid.Nil {
		return uuid.Nil, application.ErrProjectNotFound
	}
	return row.OwnerScopeID, nil
}
