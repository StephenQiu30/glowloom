package gormdb

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	agentgorm "github.com/StephenQiu30/lanverse/backend/internal/agent/adapter/gormdb"
	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	referencegorm "github.com/StephenQiu30/lanverse/backend/internal/production/reference/adapter/gormdb"
)

type referenceAuthorizationRepository struct{ repository }

func (store *Store) WithinReferenceGenerationAuthorization(ctx context.Context, operation func(application.ReferenceGenerationAuthorizationRepository) error) error {
	if store == nil || store.database == nil {
		return errors.New("Reference generation authorization store is unavailable")
	}
	return platformdatabase.WithinSerializableTransaction(ctx, store.database, func(tx *gorm.DB) error { return operation(&referenceAuthorizationRepository{repository{database: tx}}) })
}

func (repo *referenceAuthorizationRepository) AuthorizeReferenceGenerationProject(ctx context.Context, actor application.Actor, workspaceID, projectID string) error {
	// Keep membership and token revocation locked until the user intent is stored.
	var workspace model.Workspace
	var user model.UserAccount
	var membership model.Membership
	var project model.Project
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&workspace, "id = ?", workspaceID).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&user, "id = ?", actor.UserID).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("workspace_id = ? AND user_id = ?", workspaceID, actor.UserID).First(&membership).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	if err := repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND workspace_id = ?", projectID, workspaceID).First(&project).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	return repo.AuthorizeProject(ctx, actor, workspaceID, projectID, true)
}

func (repo *referenceAuthorizationRepository) ReadReferenceGenerationBrief(ctx context.Context, workspaceID, projectID, revisionID, revisionHash string) (agentapp.AcceptedReferenceBrief, error) {
	reader, err := agentgorm.NewReferenceBriefStore(repo.database, referencegorm.ValidateCurrentReferenceBriefInput)
	if err != nil {
		return agentapp.AcceptedReferenceBrief{}, err
	}
	return reader.ReadAcceptedReferenceBrief(ctx, workspaceID, projectID, revisionID, revisionHash)
}

var _ application.ReferenceGenerationAuthorizationTransactions = (*Store)(nil)
