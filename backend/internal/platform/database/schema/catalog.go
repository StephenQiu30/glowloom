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
		&model.ProjectPresetSelection{},
		&model.ProjectPresetSelectionHead{},
		&model.ProjectPresetBindingVersion{},
		&model.EffectiveStyleSnapshot{},
		&model.EffectivePolicySnapshot{},
		&model.PresetEffectiveScopeHead{},
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
		&model.StructureIdentitySetVersion{},
		&model.StructureIdentityScopeHead{},
		&model.StructureIdentityCollectionReceipt{},
		&model.Asset{},
		&model.AssetState{},
		&model.AssetIdentityStateMembership{},
		&model.AssetIdentityStateScopeHead{},
		&model.ProductionWorldEvidence{},
		&model.ProductionWorldSpecification{},
		&model.ProductionWorldClaim{},
		&model.ProductionWorldBinding{},
		&model.ProductionWorldBindingState{},
		&model.ProductionWorldBibleVersion{},
		&model.ProductionWorldBibleScopeHead{},
		&model.Episode{},
		&model.ProductionWorldPlanningScene{},
		&model.ProductionWorldPlanningDialogue{},
		&model.ProductionWorldPlanningBeat{},
		&model.ProductionWorldPlanningOccurrence{},
		&model.ProductionWorldPlanningClaim{},
		&model.ProductionWorldPlanningMembership{},
		&model.ProductionWorldPlanningEpisodeHead{},
		&model.ProductionWorldPlanningRebaseHead{},
		&model.ProductionWorldCommandDedup{},
		&model.ProductionWorldCollectionReceipt{},
		&model.ApprovedReferencePlanVersion{},
		&model.ReferencePlanTargetVersion{},
		&model.GenerationReferenceTargetHead{},
		&model.GenerationReferenceExecution{},
		&model.GenerationReferenceExecutionHead{},
		&model.GenerationReferenceProviderJob{},
		&model.GenerationReferenceProviderCall{},
		&model.GenerationReferenceStagedMedia{},
		&model.ReferencePlanScopeHead{},
		&model.ProjectReferencePlanActivationHead{},
		&model.VisualFoundationScopeCollectionReceipt{},
		&model.ProductionBibleSpecificationVersion{},
		&model.ProductionBinding{},
		&model.ProductionBindingState{},
		&model.StageInstanceStaleness{},
		&model.EpisodePlan{},
		&model.EpisodeScriptVersion{},
		&model.ProjectEpisodeVersion{},
		&model.ProjectEpisodeMembership{},
		&model.ProjectEpisodeScopeHead{},
		&model.ProjectEpisodeCollectionReceipt{},
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
		{model: &model.SceneAnalysisRelease{}, name: "ck_agt_scene_release_capability"},
		{model: &model.SceneAnalysisInvocationRecord{}, name: "ck_agt_scene_invocation_stage"},
		{model: &model.SceneAnalysisInvocationRecord{}, name: "ck_agt_scene_invocation_profile"},
		{model: &model.SceneAnalysisInvocationRecord{}, name: "ck_agt_scene_invocation_source"},
		{model: &model.SceneAnalysisCandidateRevision{}, name: "ck_agt_scene_candidate_type"},
		{model: &model.WorkflowHumanGateInput{}, name: "ck_wrk_human_gate_input_key"},
		{model: &model.WorkflowHumanGateInput{}, name: "ck_wrk_human_gate_input_subject"},
		{model: &model.ProductionWorldClaim{}, name: "ck_scr_world_claim_type"},
		{model: &model.GenerationTarget{}, name: "ck_gen_target_kind"},
		{model: &model.GenerationReferenceProviderCall{}, name: "ck_gen_ref_call_status"},
		{model: &model.GenerationReferenceProviderCall{}, name: "ck_gen_ref_call_revision"},
		{model: &model.GenerationReferenceProviderCall{}, name: "ck_gen_ref_call_dispatch_metadata"},
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
