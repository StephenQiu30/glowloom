package schema

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

func Catalog() []any {
	return []any{
		&model.UserAccount{},
		&model.Workspace{},
		&model.Membership{},
		&model.RegistrationVerification{},
		&model.AuthSession{},
		&model.Project{},
		&model.CommandReceipt{},
		&model.CreationRun{},
		&model.CreationCommandOutbox{},
		&model.MediaObject{},
		&model.MediaVersion{},
		&model.UploadSession{},
		&model.Artifact{},
		&model.ArtifactLocation{},
		&model.GenerationCandidate{},
		&model.GenerationQCReport{},
		&model.ScriptDocument{},
		&model.DocumentRevision{},
		&model.SourceSpanIndexVersion{},
		&model.ScriptSourceScopeHead{},
		&model.ScriptSourceCollectionReceipt{},
		&model.NodeDefinitionVersion{},
		&model.NodeCatalogVersion{},
		&model.AuthoringDraft{},
		&model.AuthoringRevision{},
		&model.WorkflowDefinitionVersion{},
		&model.RunInputSnapshot{},
		&model.WorkflowRun{},
		&model.NodeRunProjection{},
		&model.NodeCacheEntry{},
		&model.WorkflowStartIntent{},
		&model.WorkflowStartReceipt{},
		&model.WorkflowControlIntent{},
		&model.WorkflowControlReceipt{},
		&model.WorkflowHumanGateInput{},
		&model.HumanTask{},
		&model.ReviewDecision{},
		&model.CreationProposal{},
		&model.TextWorldVersion{},
		&model.TextIntentVersion{},
		&model.GenerationCandidateSelection{},
		&model.ProviderCredentialVersion{},
		&model.ProviderConnectionVersion{},
		&model.ProviderModelProfileVersion{},
		&model.ProjectProviderBindingVersion{},
		&model.CostBudgetPolicy{},
		&model.CostPriceQuote{},
		&model.CostEstimate{},
		&model.CostReservation{},
		&model.CostLedgerEntry{},
		&model.QuotaPolicy{},
		&model.QuotaCounter{},
		&model.QuotaReservation{},
		&model.GenerationTarget{},
		&model.GenerationIntent{},
		&model.GenerationRequest{},
		&model.GenerationProviderJob{},
		&model.GenerationProviderCall{},
		&model.GenerationProviderResultReceipt{},
		&model.WorkflowHumanGateApplyReceipt{},
		&model.WorkflowSignalIntent{},
		&model.WorkflowSignalReceipt{},
		&model.WorkflowTask{},
		&model.ProductionBible{},
		&model.ShardManifest{},
		&model.AgentInvocation{},
		&model.StageCandidateRevision{},
		&model.StageCandidateHead{},
		&model.SceneAnalysisRelease{},
		&model.SceneAnalysisControlHead{},
		&model.SceneAnalysisInvocationRecord{},
		&model.SceneAnalysisAttempt{},
		&model.SceneAnalysisDispatchAuthorization{},
		&model.SceneAnalysisResult{},
		&model.SceneAnalysisCandidateRevision{},
		&model.SceneAnalysisInvocationRead{},
		&model.SceneAnalysisCandidateHead{},
		&model.ProductionBibleVersion{},
		&model.Asset{},
		&model.AssetState{},
		&model.ProductionBibleSpecificationVersion{},
		&model.ProductionBinding{},
		&model.ProductionBindingState{},
		&model.StageInstanceStaleness{},
		&model.EpisodePlan{},
		&model.Episode{},
		&model.EpisodeScriptVersion{},
		&model.EpisodeStructure{},
		&model.StoryGraphVersion{},
		&model.StoryGraphHead{},
		&model.ImportCommit{},
		&model.StoryboardDraftSet{},
		&model.StoryboardDraftBatch{},
		&model.StoryboardShot{},
		&model.StoryboardShotImageBindingVersion{},
		&model.StoryboardExportSet{},
		&model.StoryboardExport{},
		&model.OutboxEvent{},
		&model.InboxEvent{},
		&model.EventCheckpoint{},
		&model.DeadLetter{},
		&model.AuditEvent{},
	}
}

func Sync(ctx context.Context, database *gorm.DB) error {
	models := Catalog()
	if len(models) == 0 {
		return errors.New("database model catalog must not be empty")
	}
	if err := database.WithContext(ctx).AutoMigrate(models...); err != nil {
		return fmt.Errorf("synchronize GORM model catalog: %w", err)
	}
	if err := refreshEvolvingConstraints(ctx, database); err != nil {
		return fmt.Errorf("synchronize GORM model constraints: %w", err)
	}
	return nil
}

func refreshEvolvingConstraints(ctx context.Context, database *gorm.DB) error {
	constraints := []struct {
		model any
		name  string
	}{
		{model: &model.ShardManifest{}, name: "ck_agt_manifest_stage"},
		{model: &model.SceneAnalysisRelease{}, name: "ck_agt_scene_release_stage"},
		{model: &model.SceneAnalysisRelease{}, name: "ck_agt_scene_release_profile"},
		{model: &model.SceneAnalysisInvocationRecord{}, name: "ck_agt_scene_invocation_stage"},
		{model: &model.SceneAnalysisInvocationRecord{}, name: "ck_agt_scene_invocation_profile"},
		{model: &model.SceneAnalysisCandidateRevision{}, name: "ck_agt_scene_candidate_type"},
	}
	return database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		for _, constraint := range constraints {
			if transaction.Migrator().HasConstraint(constraint.model, constraint.name) {
				if err := transaction.Migrator().DropConstraint(constraint.model, constraint.name); err != nil {
					return fmt.Errorf("drop %s: %w", constraint.name, err)
				}
			}
			if err := transaction.Migrator().CreateConstraint(constraint.model, constraint.name); err != nil {
				return fmt.Errorf("create %s: %w", constraint.name, err)
			}
		}
		return nil
	})
}
