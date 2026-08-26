package gormdb

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
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
