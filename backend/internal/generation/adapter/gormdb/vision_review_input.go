package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	flow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ValidateCurrentVisionReviewInput joins the dispatch/result transaction; it
// never opens a nested transaction or performs private object IO.
func ValidateCurrentVisionReviewInput(ctx context.Context, tx *gorm.DB, command agentapp.ExecuteVisionReviewCommand) error {
	if tx == nil || tx.Statement == nil {
		return errors.New("Vision Review transaction required")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return errors.New("Vision Review transaction required")
	}
	var run model.WorkflowRun
	var node model.NodeRunProjection
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", command.WorkflowRunID).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, "id = ?", command.NodeRunID).Error; err != nil {
		return err
	}
	subject := command.Input.Subject
	if node.WorkflowRunID != run.ID || node.WorkspaceID != run.WorkspaceID || run.WorkspaceID.String() != subject.WorkspaceID || run.ProjectID.String() != subject.ProjectID ||
		run.CreatedBy.String() != command.UserID || run.InitiatorTokenVersion != command.TokenVersion || node.Executor != "activity.review_reference_artifact" || node.DefinitionKey != "agent.vision_review" ||
		(run.Status != "RUNNING" && run.Status != "RETRYING" && run.Status != "SUCCEEDED") || (node.Status != "RUNNING" && node.Status != "RETRYING" && node.Status != "SUCCEEDED") {
		return errors.New("Vision Review Workflow ownership changed")
	}
	input, _, hash, err := flow.ParseNodeInput(json.RawMessage(node.Input))
	if err != nil || node.InputHash == nil || hash != *node.InputHash || len(input.Bindings) != 0 || len(input.FrozenInputs) != 1 {
		return errors.New("Vision Review node input changed")
	}
	var config struct {
		ExecutionRef domain.GenerationRevisionRef `json:"execution_ref"`
		BundleIndex  *int                         `json:"bundle_index"`
	}
	if canonical.Decode(input.Config, &config) != nil || config.ExecutionRef != subject.ExecutionRef || config.BundleIndex == nil || *config.BundleIndex != subject.CandidateBundleIndex {
		return errors.New("Vision Review node scope changed")
	}
	frozen := input.FrozenInputs[0]
	if frozen.Kind != "reference_execution" || frozen.ID != config.ExecutionRef.ID || frozen.Version != "1" || frozen.Hash != config.ExecutionRef.ContentHash {
		return errors.New("Vision Review frozen Execution changed")
	}
	current, _, err := compileBaseVisionReviewMediaFactsInTransaction(ctx, tx, application.Actor{UserID: command.UserID, TokenVersion: command.TokenVersion}, subject.ProjectID, subject.ExecutionRef.ID, subject.CandidateBundleIndex, subject.StageReleaseHash)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, command.Input) {
		return errors.New("Vision Review current facts changed")
	}
	return nil
}

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
		var err error
		result, media, err = compileBaseVisionReviewMediaFactsInTransaction(ctx, tx, actor, projectID, executionID, bundleIndex, stageReleaseHash)
		return err
	})
	if err != nil {
		return contract.VisionReviewInput{}, nil, err
	}
	return result, media, nil
}

func compileBaseVisionReviewMediaFactsInTransaction(ctx context.Context, tx *gorm.DB, actor application.Actor, projectID, executionID string, bundleIndex int, stageReleaseHash string) (contract.VisionReviewInput, []domain.ReferenceStagedMedia, error) {
	var result contract.VisionReviewInput
	var media []domain.ReferenceStagedMedia
	repo := repository{database: tx}
	scope, err := repo.authorizeProject(ctx, actor, "", projectID, "write")
	if err != nil {
		return result, nil, err
	}
	snapshot, err := loadReferenceBundleSnapshot(ctx, tx, scope.WorkspaceID, projectID, executionID)
	if err != nil {
		return result, nil, err
	}
	targets := &referenceTargetRepository{referenceAuthorizationRepository{repo}}
	executions := &referenceExecutionRepository{referenceExecutionAuthorizationRepository{targets, &providerConfigurationRepository{database: tx}}, tx}
	if err := executions.ValidateReferenceExecutionHead(ctx, snapshot.execution); err != nil {
		return result, nil, err
	}
	target, err := application.ReadCurrentReferenceGenerationTarget(ctx, targets, actor, application.ReadReferenceGenerationTargetQuery{
		WorkspaceID: scope.WorkspaceID, ProjectID: projectID, TargetRef: snapshot.bundles.TargetRef,
	})
	if err != nil {
		return result, nil, err
	}
	brief, err := targets.ReadReferenceGenerationBrief(ctx, scope.WorkspaceID, projectID, target.ReferenceBriefRevisionRef.ID, target.ReferenceBriefRevisionRef.RevisionHash)
	if err != nil {
		return result, nil, err
	}
	var styleRow model.EffectiveStyleSnapshot
	var policyRow model.EffectivePolicySnapshot
	if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND content_hash = ?", target.EffectiveStyleSnapshotRef.OwnerVersionID, scope.WorkspaceID, projectID, target.EffectiveStyleSnapshotRef.OwnerContentHash).First(&styleRow).Error; err != nil {
		return result, nil, err
	}
	if err := tx.WithContext(ctx).Where("id = ? AND workspace_id = ? AND project_id = ? AND content_hash = ?", target.EffectivePolicySnapshotRef.OwnerVersionID, scope.WorkspaceID, projectID, target.EffectivePolicySnapshotRef.OwnerContentHash).First(&policyRow).Error; err != nil {
		return result, nil, err
	}
	// The accepted Brief read already verifies relational columns and current
	// Preset Head; the compiler also recomputes these complete payload hashes.
	var style preset.EffectiveStyleSnapshot
	var policy preset.EffectivePolicySnapshot
	if err := canonical.Decode(styleRow.Content, &style); err != nil {
		return result, nil, err
	}
	if err := canonical.Decode(policyRow.Content, &policy); err != nil {
		return result, nil, err
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
	return result, media, err
}

var _ application.VisionReviewMediaFactsReader = (*Store)(nil)
