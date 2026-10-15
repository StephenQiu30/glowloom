package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// WorkerScopeFactory binds live worker membership checks to the current transaction.
type WorkerScopeFactory func(*gorm.DB) application.WorkerScopeResolver

// NewScopedProjectCopyStore requires separate request and worker authority for the installed owners.
func NewScopedProjectCopyStore(db *gorm.DB, requests RequestScopeFactory, workers WorkerScopeFactory, work ProjectWorkFactory, owners ProjectCopyOwnersFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, requests: requests, workers: workers, work: work, owners: owners}
}

// NewScopedProjectCopyStoreWithBible explicitly separates HTTP session and durable worker authority.
func NewScopedProjectCopyStoreWithBible(db *gorm.DB, requests RequestScopeFactory, workers WorkerScopeFactory, work ProjectWorkFactory, owners ProjectCopyOwnersFactory, script ProjectCopyScriptOwnerFactory, bible ProjectCopyBibleOwnerFactory) *ProjectCopyStore {
	return &ProjectCopyStore{db: db, requests: requests, workers: workers, work: work, owners: owners, script: script, withScript: true, bible: bible, withBible: true}
}

func (s *ProjectCopyStore) requireRequest(ctx context.Context, tx *gorm.DB, actor identity.Principal, write bool) error {
	return (&Store{scopes: s.requests}).requireScope(ctx, tx, actor, write)
}

func (s *ProjectCopyStore) authorizedCopy(ctx context.Context, tx *gorm.DB, actor identity.Principal, id uuid.UUID, write, worker bool) (domain.ProjectCopyJob, error) {
	if !worker {
		if err := s.requireRequest(ctx, tx, actor, write); err != nil {
			return domain.ProjectCopyJob{}, err
		}
	}
	var candidate struct{ OwnerScopeID, ActorID uuid.UUID }
	read := tx.Raw(`SELECT owner_scope_id,COALESCE(execution_actor_id,actor_id) AS actor_id FROM workspace.project_copy_job WHERE id=? AND org_id=?`, id, actor.OrgID).Scan(&candidate)
	if read.Error != nil {
		return domain.ProjectCopyJob{}, read.Error
	}
	if read.RowsAffected != 1 || candidate.OwnerScopeID == uuid.Nil {
		return domain.ProjectCopyJob{}, application.ErrProjectNotFound
	}
	if worker {
		if s.workers == nil || s.workers(tx) == nil {
			return domain.ProjectCopyJob{}, application.ErrProjectDependencyUnavailable
		}
		if actor.ID != candidate.ActorID {
			return domain.ProjectCopyJob{}, identity.ErrForbidden
		}
		scope, err := s.workers(tx).Resolve(ctx, candidate.ActorID, candidate.OwnerScopeID, write)
		if err != nil {
			return domain.ProjectCopyJob{}, err
		}
		if scope.PhysicalScopeID != actor.OrgID || scope.ID != candidate.OwnerScopeID {
			return domain.ProjectCopyJob{}, application.ErrScopeNotFound
		}
	} else if candidate.OwnerScopeID != actor.ScopeID {
		return domain.ProjectCopyJob{}, application.ErrProjectNotFound
	}
	job, err := readCopyJob(tx, actor, id, write)
	if err != nil {
		return job, err
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM workspace.project WHERE org_id=? AND owner_scope_id=? AND id IN (?,?)`, job.OrgID, candidate.OwnerScopeID, job.SourceProjectID, job.TargetProjectID).Scan(&count).Error; err != nil {
		return job, err
	}
	if count != 2 || job.ExecutionActor() != candidate.ActorID {
		return domain.ProjectCopyJob{}, application.ErrProjectNotFound
	}
	return job, nil
}

// NewScopedProjectCopyAccessStore retains exact authority in each child owner transaction.
func NewScopedProjectCopyAccessStore(tx *gorm.DB, authority application.ProjectCopyAuthority, requests RequestScopeFactory, workers WorkerScopeFactory) *ProjectCopyAccessStore {
	return &ProjectCopyAccessStore{tx: tx, authority: authority, requests: requests, workers: workers}
}
