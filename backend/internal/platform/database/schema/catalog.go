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
		&model.ScriptDocument{},
		&model.DocumentRevision{},
		&model.NodeDefinitionVersion{},
		&model.NodeCatalogVersion{},
		&model.WorkflowTask{},
		&model.ProductionBible{},
		&model.AgentInvocation{},
		&model.EpisodePlan{},
		&model.Episode{},
		&model.EpisodeScriptVersion{},
		&model.EpisodeStructure{},
		&model.ImportCommit{},
		&model.StoryboardDraftBatch{},
		&model.StoryboardShot{},
		&model.StoryboardExport{},
		&model.CommandReceipt{},
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
