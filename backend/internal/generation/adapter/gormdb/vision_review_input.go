package gormdb

import (
	"context"
	"errors"
	"reflect"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	"gorm.io/gorm"
)

// CompileBaseVisionReviewInput prepares exact input in its own consistent
// transaction. It does not publish a Stage, authorize dispatch, or send bytes.
func (store *Store) CompileBaseVisionReviewInput(ctx context.Context, actor application.Actor, projectID, executionID string, bundleIndex int, stageReleaseHash string) (contract.VisionReviewInput, error) {
	input, _, err := store.compileBaseVisionReviewMediaFacts(ctx, actor, projectID, executionID, bundleIndex, stageReleaseHash)
	return input, err
}

// ReadBaseVisionReviewMediaFacts is an internal exact read, not a download or
// dispatch endpoint. Private locations remain inside the Backend service.
func (store *Store) ReadBaseVisionReviewMediaFacts(ctx context.Context, actor application.Actor, expected contract.VisionReviewInput) ([]domain.ReferenceStagedMedia, error) {
	if err := expected.Validate(); err != nil {
		return nil, err
	}
	subject := expected.Subject
	input, media, err := store.compileBaseVisionReviewMediaFacts(ctx, actor, subject.ProjectID, subject.ExecutionRef.ID, subject.CandidateBundleIndex, subject.StageReleaseHash)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(input, expected) {
		return nil, errors.New("Vision Review input differs from current facts")
	}
	return media, nil
}

func (store *Store) compileBaseVisionReviewMediaFacts(ctx context.Context, actor application.Actor, projectID, executionID string, bundleIndex int, stageReleaseHash string) (contract.VisionReviewInput, []domain.ReferenceStagedMedia, error) {
	if store == nil || store.database == nil || store.database.Statement == nil {
		return contract.VisionReviewInput{}, nil, errors.New("Vision Review facts reader is unavailable")
	}
	if _, nested := store.database.Statement.ConnPool.(gorm.TxCommitter); nested {
		return contract.VisionReviewInput{}, nil, errors.New("Vision Review preparation requires its own consistent snapshot")
	}
	var result contract.VisionReviewInput
	var media []domain.ReferenceStagedMedia
	err := platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error {
		repo := repository{database: tx}
		scope, err := repo.authorizeProject(ctx, actor, "", projectID, "write")
		if err != nil {
			return err
		}
		snapshot, err := loadReferenceBundleSnapshot(ctx, tx, scope.WorkspaceID, projectID, executionID)
		if err != nil {
			return err
		}
		targets := &referenceTargetRepository{referenceAuthorizationRepository{repo}}
		executions := &referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx}
		if err := executions.ValidateReferenceExecutionHead(ctx, snapshot.execution); err != nil {
			return err
		}
		target, err := application.ReadCurrentReferenceGenerationTarget(ctx, targets, actor, application.ReadReferenceGenerationTargetQuery{
			WorkspaceID: scope.WorkspaceID, ProjectID: projectID, TargetRef: snapshot.bundles.TargetRef,
		})
		if err != nil {
			return err
		}
		brief, err := targets.ReadReferenceGenerationBrief(ctx, scope.WorkspaceID, projectID, target.ReferenceBriefRevisionRef.ID, target.ReferenceBriefRevisionRef.RevisionHash)
		if err != nil {
			return err
		}
		var styleRow model.EffectiveStyleSnapshot
		var policyRow model.EffectivePolicySnapshot
		if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND content_hash = ?", target.EffectiveStyleSnapshotRef.OwnerVersionID, scope.WorkspaceID, projectID, target.EffectiveStyleSnapshotRef.OwnerContentHash).First(&styleRow).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND content_hash = ?", target.EffectivePolicySnapshotRef.OwnerVersionID, scope.WorkspaceID, projectID, target.EffectivePolicySnapshotRef.OwnerContentHash).First(&policyRow).Error; err != nil {
			return err
		}
		// The accepted Brief read already verifies relational columns and current
		// Preset Head; the compiler also recomputes these complete payload hashes.
		var style preset.EffectiveStyleSnapshot
		var policy preset.EffectivePolicySnapshot
		if err := canonical.Decode(styleRow.Content, &style); err != nil {
			return err
		}
		if err := canonical.Decode(policyRow.Content, &policy); err != nil {
			return err
		}
		media = make([]domain.ReferenceStagedMedia, 0)
		for _, item := range snapshot.media {
			if item.Call.BundleIndex == bundleIndex {
				media = append(media, item)
			}
		}
		result, err = application.CompileBaseVisionReviewInput(application.BaseVisionReviewCompilationFacts{
			Target: target, Brief: brief, Style: style, Policy: policy, Bundles: snapshot.bundles, Media: media,
			CandidateBundleIndex: bundleIndex, StageReleaseHash: stageReleaseHash,
		})
		return err
	})
	if err != nil {
		return contract.VisionReviewInput{}, nil, err
	}
	return result, media, nil
}

var _ application.VisionReviewMediaFactsReader = (*Store)(nil)
