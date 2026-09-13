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

func (store *Store) ReadReferenceBundleInputs(ctx context.Context, actor application.Actor, projectID, executionID string) (domain.ReferenceBundleInputCollection, error) {
	if store == nil || store.database == nil || store.database.Statement == nil {
		return domain.ReferenceBundleInputCollection{}, errors.New("Reference Bundle reader is unavailable")
	}
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return domain.ReferenceBundleInputCollection{}, errors.New("Reference Bundle query requires its own consistent snapshot")
	}
	var result domain.ReferenceBundleInputCollection
	err := store.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repository{database: tx}
		scope, err := repo.authorizeProject(ctx, actor, "", projectID, "read")
		if err != nil {
			return err
		}
		snapshot, err := loadReferenceBundleSnapshot(ctx, tx, scope.WorkspaceID, projectID, executionID)
		if err != nil {
			return err
		}
		result = snapshot.bundles
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domain.ReferenceBundleInputCollection{}, err
	}
	return result, nil
}

var _ application.ReferenceBundleInputReader = (*Store)(nil)

// Private Owner facts never leave the Backend transaction as object locations.
type referenceBundleSnapshot struct {
	execution domain.ReferenceExecution
	bundles   domain.ReferenceBundleInputCollection
	media     []domain.ReferenceStagedMedia
}

func loadReferenceBundleSnapshot(ctx context.Context, tx *gorm.DB, workspaceID, projectID, executionID string) (referenceBundleSnapshot, error) {
	repo := repository{database: tx}
	targets := &referenceTargetRepository{referenceAuthorizationRepository{repo}}
	executions := &referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx}
	execution, err := executions.FindReferenceExecution(ctx, workspaceID, projectID, executionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return referenceBundleSnapshot{}, notFound("Reference execution not found")
	}
	if err != nil {
		return referenceBundleSnapshot{}, err
	}
	target, err := targets.FindReferenceGenerationTarget(ctx, workspaceID, projectID, execution.ReadSet.TargetRef.ID)
	if err != nil {
		return referenceBundleSnapshot{}, err
	}
	if execution.ReadSet.TargetRef != (domain.GenerationRevisionRef{ID: target.ID, Revision: target.Revision, ContentHash: target.ContentHash}) || execution.ReadSet.TargetReadSetRoot != target.TargetReadSetRoot {
		return referenceBundleSnapshot{}, errors.New("Reference Bundle target has drifted")
	}
	job, calls, states, err := executions.readReferenceProviderJob(ctx, workspaceID, projectID, executionID)
	if err != nil {
		return referenceBundleSnapshot{}, err
	}
	if job.ExecutionRef != (domain.GenerationRevisionRef{ID: execution.ID, Revision: execution.Revision, ContentHash: execution.ContentHash}) {
		return referenceBundleSnapshot{}, errors.New("Reference Bundle execution has drifted")
	}
	progress, err := domain.BuildReferenceJobProgress(job, calls, states)
	if err != nil {
		return referenceBundleSnapshot{}, err
	}
	if !progress.Terminal {
		return referenceBundleSnapshot{}, &application.Error{Code: "reference_results_unresolved", Message: "Reference Bundle requires all explicit outcomes", Status: 409}
	}
	var rows []model.GenerationReferenceStagedMedia
	// Include all frozen members, even corrupt scope or failed-call media; the
	// content compiler must reject them rather than silently filter them out.
	if err := tx.WithContext(ctx).Where("call_key IN ?", job.CallKeys).Find(&rows).Error; err != nil {
		return referenceBundleSnapshot{}, err
	}
	media := make([]domain.ReferenceStagedMedia, len(rows))
	for i, row := range rows {
		media[i], err = referenceStagedMediaFromRecord(row)
		if err != nil {
			return referenceBundleSnapshot{}, err
		}
	}
	bundles, err := domain.BuildReferenceBundleInputs(domain.ReferenceBundleFacts{WorkspaceID: workspaceID, ProjectID: projectID, TargetRef: execution.ReadSet.TargetRef, GenerationRound: target.GenerationRound, TargetKind: target.TargetKind, DependencyRootHash: target.DependencyRootHash, Output: target.OutputContract, Job: job, Calls: calls, States: states, Media: media})
	if err != nil {
		return referenceBundleSnapshot{}, &application.Error{Code: "reference_bundle_incomplete", Message: "Reference Bundle media or identity is incomplete", Status: 409}
	}
	return referenceBundleSnapshot{execution: execution, bundles: bundles, media: media}, nil
}
