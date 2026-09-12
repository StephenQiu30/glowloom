package gormdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"gorm.io/gorm"
)

func (store *Store) ReadReferenceExecutionProgress(ctx context.Context, actor application.Actor, projectID, executionID string) (domain.ReferenceJobProgress, error) {
	if store == nil || store.database == nil || store.database.Statement == nil {
		return domain.ReferenceJobProgress{}, errors.New("Reference execution reader is unavailable")
	}
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return domain.ReferenceJobProgress{}, errors.New("Reference execution query requires its own consistent snapshot")
	}
	var result domain.ReferenceJobProgress
	err := store.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository{database: tx}
		scope, err := repo.authorizeProject(ctx, actor, "", projectID, "read")
		if err != nil {
			return err
		}
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repo}}
		executions := &referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx}
		execution, err := executions.FindReferenceExecution(ctx, scope.WorkspaceID, projectID, executionID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound("Reference execution not found")
		}
		if err != nil {
			return err
		}
		job, calls, states, err := executions.readReferenceProviderJob(ctx, scope.WorkspaceID, projectID, executionID)
		if err != nil {
			return err
		}
		if job.ExecutionRef != (domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}) {
			return errors.New("Reference job execution identity has drifted")
		}
		result, err = domain.BuildReferenceJobProgress(job, calls, states)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domain.ReferenceJobProgress{}, err
	}
	return result, nil
}

var _ application.ReferenceExecutionProgressReader = (*Store)(nil)
