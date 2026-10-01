package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// List returns one authorized source's copy jobs, including hidden targets and failures.
func (s *ProjectCopyStore) List(ctx context.Context, actor identityapp.Principal, input application.ProjectCopyListInput) (application.ProjectCopyPage, error) {
	if s == nil || s.db == nil || input.Limit < 1 || input.Limit > 100 {
		return application.ProjectCopyPage{}, application.ErrProjectDependencyUnavailable
	}
	page := application.ProjectCopyPage{Jobs: []domain.ProjectCopyJob{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := readLifecycleProject(tx, actor.OrgID, input.SourceProjectID, false); err != nil {
			return err
		}
		query := `SELECT ` + copyJobColumns + `,create_time FROM workspace.project_copy_job WHERE org_id=? AND source_project_id=?`
		args := []any{actor.OrgID, input.SourceProjectID}
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
		for _, row := range rows {
			job, err := row.job()
			if err != nil {
				return err
			}
			page.Jobs = append(page.Jobs, job)
		}
		return nil
	})
	return page, err
}

// WorkerActor restores only the job's initiating current account, never a synthesized role.
func (s *ProjectCopyStore) WorkerActor(ctx context.Context, org, id uuid.UUID) (identityapp.Principal, error) {
	if s == nil || s.db == nil || org == uuid.Nil || id == uuid.Nil {
		return identityapp.Principal{}, application.ErrProjectDependencyUnavailable
	}
	var actor identityapp.Principal
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Raw(`SELECT u.id,u.org_id,u.role,u.must_change_password FROM identity."user" u JOIN workspace.project_copy_job j ON j.actor_id=u.id AND j.org_id=u.org_id WHERE j.id=? AND j.org_id=? AND NOT u.is_delete AND u.status='active'`, id, org).Scan(&actor)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return identityapp.ErrForbidden
		}
		return requireCurrentActor(tx, actor)
	})
	return actor, err
}
