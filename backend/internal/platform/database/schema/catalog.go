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
		&model.MediaObject{},
		&model.MediaVersion{},
		&model.UploadSession{},
		&model.Artifact{},
		&model.ArtifactLocation{},
		&model.GenerationCandidate{},
		&model.GenerationQCReport{},
		&model.ScriptDocument{},
		&model.DocumentRevision{},
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
		&model.HumanTask{},
		&model.ReviewDecision{},
		&model.GenerationCandidateSelection{},
		&model.CostBudgetPolicy{},
		&model.CostPriceQuote{},
		&model.CostEstimate{},
		&model.CostReservation{},
		&model.CostLedgerEntry{},
		&model.QuotaPolicy{},
		&model.QuotaCounter{},
		&model.QuotaReservation{},
		&model.GenerationIntent{},
		&model.GenerationProviderBindingVersion{},
		&model.GenerationRequest{},
		&model.GenerationProviderJob{},
		&model.GenerationProviderResultReceipt{},
		&model.WorkflowHumanGateApplyReceipt{},
		&model.WorkflowSignalIntent{},
		&model.WorkflowSignalReceipt{},
		&model.WorkflowTask{},
		&model.ProductionBible{},
		&model.AgentInvocation{},
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
		&model.CommandReceipt{},
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
	return nil
}
