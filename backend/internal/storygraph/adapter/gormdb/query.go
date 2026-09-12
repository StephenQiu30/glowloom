package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func (store *Store) GetCurrentVersion(ctx context.Context, actor storygraphapp.Actor, projectID string) (storygraph.Version, error) {
	repo := &repository{database: store.database}
	project, err := repo.authorizeProject(ctx, actor, projectID, false, false)
	if err != nil {
		return storygraph.Version{}, err
	}
	var head model.StoryGraphHead
	err = store.database.WithContext(ctx).Where("workspace_id = ? AND project_id = ?", project.WorkspaceID, project.ID).First(&head).Error
	if err != nil {
		return storygraph.Version{}, normalizeNotFound(err)
	}
	version, err := store.versionForProject(ctx, project.WorkspaceID, project.ID, head.CurrentVersionID)
	if err != nil {
		return storygraph.Version{}, err
	}
	if version.VersionNo != head.Revision || version.ContentHash != head.CurrentContentHash {
		return storygraph.Version{}, errors.New("StoryGraph head does not match its current immutable version")
	}
	return version, nil
}

func (store *Store) GetExactVersion(ctx context.Context, actor storygraphapp.Actor, projectID, versionID string) (storygraph.Version, error) {
	repo := &repository{database: store.database}
	project, err := repo.authorizeProject(ctx, actor, projectID, false, false)
	if err != nil {
		return storygraph.Version{}, err
	}
	id, err := uuid.Parse(versionID)
	if err != nil {
		return storygraph.Version{}, storygraphapp.ErrNotFound
	}
	return store.versionForProject(ctx, project.WorkspaceID, project.ID, id)
}

func (store *Store) GetCurrentOwnerSetHash(ctx context.Context, actor storygraphapp.Actor, projectID string) (string, error) {
	repo := &repository{database: store.database}
	project, err := repo.authorizeProject(ctx, actor, projectID, false, false)
	if err != nil {
		return "", err
	}
	var head model.StoryGraphHead
	if err = store.database.WithContext(ctx).Where(
		"workspace_id = ? AND project_id = ?", project.WorkspaceID, project.ID,
	).First(&head).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	current, err := store.versionForProject(ctx, project.WorkspaceID, project.ID, head.CurrentVersionID)
	if err != nil {
		return "", err
	}
	if current.SchemaVersion == storygraph.ProductionSchemaID {
		return store.currentProductionOwnerSetHash(ctx, repo, project.WorkspaceID, project.ID)
	}
	snapshot, err := repo.LoadOwnerSnapshot(ctx, storygraph.PublicationState{
		WorkspaceID: project.WorkspaceID.String(), ProjectID: project.ID.String(),
	})
	if err != nil {
		var applicationError *storygraphapp.Error
		if errors.Is(err, storygraphapp.ErrNotFound) || errors.As(err, &applicationError) && applicationError.Code == "invalid_owner_snapshot" {
			return "", nil
		}
		return "", err
	}
	_, ownerSetHash, err := storygraph.CanonicalOwnerHeadRefs(snapshot.OwnerHeads)
	return ownerSetHash, err
}

// GetCurrentVisualFoundationWorld is the authorization-free worker query for a
// Backend-owned invocation transaction. Public callers must use QueryService.
func (store *Store) GetCurrentVisualFoundationWorld(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (storygraph.VisualFoundationWorldReadSet, error) {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil || workspace == uuid.Nil || project == uuid.Nil {
		return storygraph.VisualFoundationWorldReadSet{}, errors.New("invalid Visual Foundation worker scope")
	}
	var head model.StoryGraphHead
	if err := store.database.WithContext(ctx).Where(
		"workspace_id = ? AND project_id = ?", workspace, project,
	).First(&head).Error; err != nil {
		return storygraph.VisualFoundationWorldReadSet{}, normalizeNotFound(err)
	}
	version, err := store.versionForProject(ctx, workspace, project, head.CurrentVersionID)
	if err != nil {
		return storygraph.VisualFoundationWorldReadSet{}, err
	}
	if version.VersionNo != head.Revision || version.ContentHash != head.CurrentContentHash {
		return storygraph.VisualFoundationWorldReadSet{}, errors.New("StoryGraph head does not match its current immutable version")
	}
	repo := &repository{database: store.database}
	ownerSetHash, err := store.currentProductionOwnerSetHash(ctx, repo, workspace, project)
	if err != nil {
		return storygraph.VisualFoundationWorldReadSet{}, err
	}
	if ownerSetHash == "" || ownerSetHash != version.OwnerSetHash {
		return storygraph.VisualFoundationWorldReadSet{}, errors.New("Production World changed before Visual Foundation Candidate acceptance")
	}
	return storygraph.BuildVisualFoundationWorldReadSet(version)
}

// GetCurrentReferencePlanVersion is the authorization-free worker query used
// only inside a Backend-owned serializable invocation transaction.
func (store *Store) GetCurrentReferencePlanVersion(
	ctx context.Context,
	workspaceID string,
	projectID string,
) (storygraph.Version, error) {
	workspace, workspaceErr := uuid.Parse(workspaceID)
	project, projectErr := uuid.Parse(projectID)
	if workspaceErr != nil || projectErr != nil || workspace == uuid.Nil || project == uuid.Nil {
		return storygraph.Version{}, errors.New("invalid Reference Plan worker scope")
	}
	var head model.StoryGraphHead
	if err := store.database.WithContext(ctx).Where(
		"workspace_id = ? AND project_id = ?", workspace, project,
	).First(&head).Error; err != nil {
		return storygraph.Version{}, normalizeNotFound(err)
	}
	version, err := store.versionForProject(ctx, workspace, project, head.CurrentVersionID)
	if err != nil {
		return storygraph.Version{}, err
	}
	if version.VersionNo != head.Revision || version.ContentHash != head.CurrentContentHash {
		return storygraph.Version{}, errors.New("StoryGraph head does not match its current immutable version")
	}
	repo := &repository{database: store.database}
	ownerSetHash, err := store.currentProductionOwnerSetHash(ctx, repo, workspace, project)
	if err != nil {
		return storygraph.Version{}, err
	}
	if ownerSetHash == "" || ownerSetHash != version.OwnerSetHash {
		return storygraph.Version{}, errors.New("Production World changed before Reference Plan Candidate acceptance")
	}
	return version, nil
}

func (store *Store) currentProductionOwnerSetHash(
	ctx context.Context,
	repo *repository,
	workspaceID uuid.UUID,
	projectID uuid.UUID,
) (string, error) {
	var latest model.ProductionWorldCollectionReceipt
	if err := store.database.WithContext(ctx).Where(
		"workspace_id = ? AND project_id = ? AND version_family = ?",
		workspaceID, projectID, bibledomain.BibleProductionWorldFamily,
	).Order("scope_revision DESC").Order("id DESC").First(&latest).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	var receipt model.CommandReceipt
	if err := store.database.WithContext(ctx).Where(
		"workspace_id = ? AND operation = ? AND resource_id = ?",
		workspaceID, worlddomain.ConfirmProductionWorldOperation, latest.CommandID,
	).First(&receipt).Error; err != nil {
		return "", normalizeNotFound(err)
	}
	var confirmation worlddomain.ConfirmProductionWorldResult
	if err := json.Unmarshal(receipt.Result, &confirmation); err != nil {
		return "", errors.New("Production World command receipt is invalid")
	}
	verified, err := worlddomain.CompleteConfirmProductionWorldResult(confirmation)
	if err != nil || verified.ResultContentHash != confirmation.ResultContentHash ||
		verified.ReceiptContentHash != confirmation.ReceiptContentHash ||
		confirmation.CommandReceiptID != receipt.ID.String() || confirmation.CommandID != latest.CommandID.String() {
		return "", errors.New("Production World command receipt has drifted")
	}
	snapshot, err := repo.LoadProductionOwnerSnapshot(ctx, storygraph.PublicationState{
		WorkspaceID: workspaceID.String(), ProjectID: projectID.String(),
	}, confirmation.CommandReceiptID, confirmation.ReceiptContentHash)
	if err != nil {
		return "", err
	}
	compiled, err := storygraph.CompileProductionOwnerSnapshot(snapshot)
	if err != nil {
		return "", err
	}
	return compiled.OwnerSetHash, nil
}

func (store *Store) versionForProject(ctx context.Context, workspaceID, projectID, versionID uuid.UUID) (storygraph.Version, error) {
	var record model.StoryGraphVersion
	err := store.database.WithContext(ctx).
		Where("id = ? AND workspace_id = ? AND project_id = ?", versionID, workspaceID, projectID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storygraph.Version{}, storygraphapp.ErrNotFound
	}
	if err != nil {
		return storygraph.Version{}, err
	}
	return versionDomain(record)
}
