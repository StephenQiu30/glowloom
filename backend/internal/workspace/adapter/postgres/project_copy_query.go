package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// List returns one authorized source's copy jobs, including hidden targets and failures.
func (s *ProjectCopyStore) List(ctx context.Context, actor identityapp.Principal, input application.ProjectCopyListInput) (application.ProjectCopyPage, error) {
	if s == nil || s.db == nil || input.Limit < 1 || input.Limit > 100 {
		return application.ProjectCopyPage{}, application.ErrProjectDependencyUnavailable
	}
	page := application.ProjectCopyPage{Jobs: []domain.ProjectCopyJob{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireRequest(ctx, tx, actor, false); err != nil {
			return err
		}
		query := `SELECT ` + copyJobColumns + `,create_time FROM workspace.project_copy_job WHERE org_id=? AND source_project_id=? AND owner_scope_id=?`
		args := []any{actor.OrgID, input.SourceProjectID, actor.ScopeID}
		if input.After != nil {
			query += ` AND (create_time,id)<(?,?)`
			args = append(args, input.After.CreatedAt, input.After.ID)
		}
		query += ` ORDER BY create_time DESC,id DESC LIMIT ?`
		args = append(args, input.Limit+1)
		var rows []copyJobRow
		if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
			return fmt.Errorf("list project copies: %w", err)
		}
		if len(rows) > input.Limit {
			rows = rows[:input.Limit]
			last := rows[len(rows)-1]
			page.Next = &application.ProjectCopyCursor{CreatedAt: last.CreateTime, ID: last.ID}
		}
		jobs, err := lockCopyPage(tx, actor, input.SourceProjectID, rows)
		if err != nil {
			return err
		}
		page.Jobs = jobs
		return nil
	})
	return page, err
}

// lockCopyPage acquires the entire page's projects in the same UUID order as
// workers, before taking any job locks. A source prelock or per-job acquisition
// can deadlock with a worker whose target sorts before that source.
func lockCopyPage(tx *gorm.DB, actor identityapp.Principal, source uuid.UUID, candidates []copyJobRow) ([]domain.ProjectCopyJob, error) {
	projectSet := map[uuid.UUID]bool{source: true}
	jobIDs := make([]uuid.UUID, 0, len(candidates))
	for _, row := range candidates {
		projectSet[row.SourceProjectID] = true
		projectSet[row.TargetProjectID] = true
		jobIDs = append(jobIDs, row.ID)
	}
	projectIDs := make([]uuid.UUID, 0, len(projectSet))
	for id := range projectSet {
		projectIDs = append(projectIDs, id)
	}
	var projects []struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM workspace.project WHERE org_id=? AND owner_scope_id=? AND id IN ? ORDER BY id FOR SHARE`, actor.OrgID, actor.ScopeID, projectIDs).Scan(&projects).Error; err != nil {
		return nil, fmt.Errorf("lock copy page projects: %w", err)
	}
	// Empty pages still authorize their source; archived and deleted projects
	// remain readable under the existing lifecycle contract.
	if len(projects) != len(projectIDs) {
		return nil, application.ErrProjectNotFound
	}
	jobs := make([]domain.ProjectCopyJob, 0, len(candidates))
	if len(candidates) == 0 {
		return jobs, nil
	}
	var locked []copyJobRow
	if err := tx.Raw(`SELECT `+copyJobColumns+` FROM workspace.project_copy_job WHERE org_id=? AND owner_scope_id=? AND source_project_id=? AND id IN ? ORDER BY id FOR SHARE`, actor.OrgID, actor.ScopeID, source, jobIDs).Scan(&locked).Error; err != nil {
		return nil, fmt.Errorf("lock copy page jobs: %w", err)
	}
	if len(locked) != len(candidates) {
		return nil, application.ErrProjectNotFound
	}
	byID := make(map[uuid.UUID]copyJobRow, len(locked))
	for _, row := range locked {
		byID[row.ID] = row
	}
	for _, candidate := range candidates {
		row, ok := byID[candidate.ID]
		// Recheck bindings under locks without acquiring any new project locks.
		if !ok || row.SourceProjectID != candidate.SourceProjectID || row.TargetProjectID != candidate.TargetProjectID || row.ActorID != candidate.ActorID || row.ExecutionActorID != candidate.ExecutionActorID {
			return nil, application.ErrProjectNotFound
		}
		job, err := row.job()
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// WorkerActor restores only the job's current command-bound account, never a synthesized role.
func (s *ProjectCopyStore) WorkerActor(ctx context.Context, org, id uuid.UUID) (identityapp.Principal, error) {
	if s == nil || s.db == nil || org == uuid.Nil || id == uuid.Nil {
		return identityapp.Principal{}, application.ErrProjectDependencyUnavailable
	}
	var actor identityapp.Principal
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Raw(`SELECT u.id,j.org_id,j.owner_scope_id AS scope_id FROM identity."user" u JOIN workspace.project_copy_job j ON COALESCE(j.execution_actor_id,j.actor_id)=u.id WHERE j.id=? AND j.org_id=? AND NOT u.is_delete AND u.status='active'`, id, org).Scan(&actor)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return identityapp.ErrForbidden
		}
		_, err := s.authorizedCopy(ctx, tx, actor, id, true, true)
		return err
	})
	return actor, err
}
