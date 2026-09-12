package gormdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"gorm.io/gorm"
)

func (store *Store) ReadReferenceGenerationProgress(ctx context.Context, actor application.Actor, projectID, targetID string) (application.ReferenceGenerationProgress, error) {
	if store == nil || store.database == nil || store.database.Statement == nil {
		return application.ReferenceGenerationProgress{}, errors.New("Reference generation reader is unavailable")
	}
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return application.ReferenceGenerationProgress{}, errors.New("Reference generation query requires its own consistent snapshot")
	}
	var result application.ReferenceGenerationProgress
	err := store.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository{database: tx}
		scope, err := repo.authorizeProject(ctx, actor, "", projectID, "read")
		if err != nil {
			return err
		}
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repo}}
		target, err := targets.FindReferenceGenerationTarget(ctx, scope.WorkspaceID, projectID, targetID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFound("Reference generation Target not found")
		}
		if err != nil {
			return err
		}
		expected, err := referenceTargetHead(target)
		if err != nil {
			return err
		}
		var head model.GenerationReferenceTargetHead
		if err := tx.Where("workspace_id = ? AND project_id = ? AND plan_version_id = ? AND reference_target_version_id = ?", expected.WorkspaceID, expected.ProjectID, expected.PlanVersionID, expected.ReferenceTargetVersionID).First(&head).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return referenceGenerationProgressConflict("Reference generation Target Head is missing")
			}
			return err
		}
		if head.CurrentTargetID != expected.CurrentTargetID || head.CurrentTargetHash != expected.CurrentTargetHash || head.Revision != expected.Revision {
			return referenceGenerationProgressConflict("Reference generation Target Head has drifted")
		}
		result = application.ReferenceGenerationProgress{
			WorkspaceID: scope.WorkspaceID, ProjectID: projectID,
			GenerationTargetRef: domain.GenerationRevisionRef{ID: target.ID, Revision: target.Revision, ContentHash: target.ContentHash},
			PlanRef:             domain.GenerationRevisionRef{ID: target.ApprovedReferencePlanVersionRef.OwnerVersionID, Revision: target.ApprovedReferencePlanVersionRef.OwnerRevision, ContentHash: target.ApprovedReferencePlanVersionRef.OwnerContentHash},
			ReferenceTargetRef:  domain.GenerationRevisionRef{ID: target.ReferencePlanTargetRef.OwnerVersionID, Revision: target.ReferencePlanTargetRef.OwnerRevision, ContentHash: target.ReferencePlanTargetRef.OwnerContentHash},
			GenerationRound:     target.GenerationRound,
		}
		var executionHead model.GenerationReferenceExecutionHead
		err = tx.Where("workspace_id = ? AND project_id = ? AND target_id = ?", scope.WorkspaceID, projectID, targetID).First(&executionHead).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var count int64
			if err := tx.Model(&model.GenerationReferenceExecution{}).Where("workspace_id = ? AND project_id = ? AND target_id = ?", scope.WorkspaceID, projectID, targetID).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return referenceGenerationProgressConflict("Published Reference execution Head is missing")
			}
			return nil
		}
		if err != nil {
			return err
		}
		executions := &referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx}
		execution, err := executions.FindReferenceExecution(ctx, scope.WorkspaceID, projectID, executionHead.CurrentExecutionID.String())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return referenceGenerationProgressConflict("Reference execution Head has no execution")
		}
		if err != nil {
			return err
		}
		if executionHead.TargetHash != target.ContentHash || executionHead.CurrentExecutionHash != execution.ContentHash || executionHead.Revision != execution.Revision || execution.ReadSet.TargetRef != result.GenerationTargetRef || execution.ReadSet.TargetReadSetRoot != target.TargetReadSetRoot {
			return referenceGenerationProgressConflict("Reference execution Head or Target identity has drifted")
		}
		job, calls, states, err := executions.readReferenceProviderJob(ctx, scope.WorkspaceID, projectID, execution.ID)
		if err != nil {
			return err
		}
		if job.ExecutionRef != (domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}) {
			return referenceGenerationProgressConflict("Reference job execution identity has drifted")
		}
		progress, err := domain.BuildReferenceJobProgress(job, calls, states)
		if err != nil {
			return err
		}
		result.Execution = &progress
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return application.ReferenceGenerationProgress{}, err
	}
	return result, nil
}

func referenceGenerationProgressConflict(message string) error {
	return &application.Error{Code: "state_conflict", Message: message, Status: 409}
}

var _ application.ReferenceGenerationProgressReader = (*Store)(nil)
