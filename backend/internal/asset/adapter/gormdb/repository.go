package gormdb

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/asset/application"
	"github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	commandgorm "github.com/StephenQiu30/lanverse/backend/internal/platform/command/adapter/gormdb"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) AuthorizeProject(ctx context.Context, actor application.Actor, workspaceID, projectID string, write bool) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound("Project not found")
	}
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return notFound("Project not found")
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", workspaceUUID).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceUUID, userID, "active").First(&membership).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).Where("id = ? AND workspace_id = ?", projectUUID, workspaceUUID).First(&project).Error; err != nil {
		return normalizeAuthorizationNotFound(err)
	}
	if user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	if write && (workspace.Status != "active" || project.Status != "active" || membership.Role == "viewer") {
		return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	return commandgorm.Find(ctx, repo.database, workspaceID, operation, key)
}

func (repo *repository) EnsureReceipt(ctx context.Context, receipt platformcommand.Receipt) (platformcommand.Receipt, error) {
	return commandgorm.Ensure(ctx, repo.database, receipt)
}

func (repo *repository) EnsureStaged(ctx context.Context, desired domain.ArtifactWithLocation) (domain.ArtifactWithLocation, error) {
	artifactRecord, err := artifactRecord(desired.Artifact)
	if err != nil {
		return domain.ArtifactWithLocation{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "source_type"}, {Name: "source_id"}, {Name: "output_key"}},
		DoNothing: true,
	}).Create(&artifactRecord).Error; err != nil {
		return domain.ArtifactWithLocation{}, err
	}
	var persistedArtifact model.Artifact
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND source_type = ? AND source_id = ? AND output_key = ?", artifactRecord.WorkspaceID, artifactRecord.SourceType, artifactRecord.SourceID, artifactRecord.OutputKey).
		First(&persistedArtifact).Error; err != nil {
		return domain.ArtifactWithLocation{}, err
	}
	if !sameStagedArtifact(persistedArtifact, artifactRecord) {
		return domain.ArtifactWithLocation{}, platformcommand.ErrInputMismatch
	}
	desired.Location.ArtifactID = persistedArtifact.ID.String()
	locationRecord, err := locationRecord(desired.Location)
	if err != nil {
		return domain.ArtifactWithLocation{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "artifact_id"}, {Name: "location_no"}}, DoNothing: true,
	}).Create(&locationRecord).Error; err != nil {
		return domain.ArtifactWithLocation{}, err
	}
	var persistedLocation model.ArtifactLocation
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("artifact_id = ? AND location_no = ?", persistedArtifact.ID, locationRecord.LocationNo).
		First(&persistedLocation).Error; err != nil {
		return domain.ArtifactWithLocation{}, err
	}
	if !sameStagedLocation(persistedLocation, locationRecord) {
		return domain.ArtifactWithLocation{}, platformcommand.ErrInputMismatch
	}
	return domain.ArtifactWithLocation{Artifact: artifactDomain(persistedArtifact), Location: locationDomain(persistedLocation)}, nil
}

func (repo *repository) Get(ctx context.Context, artifactID string, lock bool) (domain.ArtifactWithLocation, error) {
	parsed, err := uuid.Parse(artifactID)
	if err != nil {
		return domain.ArtifactWithLocation{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", parsed)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var artifact model.Artifact
	if err = query.First(&artifact).Error; err != nil {
		return domain.ArtifactWithLocation{}, normalizeNotFound(err)
	}
	locationQuery := repo.database.WithContext(ctx).Where("artifact_id = ? AND location_no = ?", parsed, 1)
	if lock {
		locationQuery = locationQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var location model.ArtifactLocation
	if err = locationQuery.First(&location).Error; err != nil {
		return domain.ArtifactWithLocation{}, normalizeNotFound(err)
	}
	if artifact.WorkspaceID != location.WorkspaceID {
		return domain.ArtifactWithLocation{}, errors.New("artifact location workspace has drifted")
	}
	return domain.ArtifactWithLocation{Artifact: artifactDomain(artifact), Location: locationDomain(location)}, nil
}

func (repo *repository) SaveReadiness(ctx context.Context, desired domain.ArtifactWithLocation, expectedRevision int) error {
	artifact, err := artifactRecord(desired.Artifact)
	if err != nil {
		return err
	}
	location, err := locationRecord(desired.Location)
	if err != nil {
		return err
	}
	updated := repo.database.WithContext(ctx).Model(&model.Artifact{}).
		Where("id = ? AND revision = ? AND status = ?", artifact.ID, expectedRevision, domain.ReadinessPendingValidation).
		Select("status", "failure_code", "width", "height", "revision", "updated_at").Updates(&artifact)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return &application.Error{Code: "state_conflict", Message: "Artifact changed during validation", Status: 409}
	}
	if desired.Artifact.Status != domain.ReadinessReady {
		return nil
	}
	locationUpdated := repo.database.WithContext(ctx).Model(&model.ArtifactLocation{}).
		Where("id = ? AND artifact_id = ? AND status = ?", location.ID, artifact.ID, domain.LocationStaging).
		Select("status", "updated_at").Updates(&location)
	if locationUpdated.Error != nil {
		return locationUpdated.Error
	}
	if locationUpdated.RowsAffected != 1 {
		return &application.Error{Code: "state_conflict", Message: "Artifact location changed during validation", Status: 409}
	}
	return nil
}

func artifactRecord(value domain.Artifact) (model.Artifact, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.Artifact{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.Artifact{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.Artifact{}, err
	}
	sourceID, err := uuid.Parse(value.SourceID)
	if err != nil {
		return model.Artifact{}, err
	}
	var failureCode *string
	if value.FailureCode != "" {
		failureCode = &value.FailureCode
	}
	var width, height *int
	if value.Width > 0 {
		width = &value.Width
	}
	if value.Height > 0 {
		height = &value.Height
	}
	return model.Artifact{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, SourceType: value.SourceType,
		SourceID: sourceID, OutputKey: value.OutputKey, MediaType: value.MediaType, SHA256: value.SHA256,
		SizeBytes: value.SizeBytes, Status: value.Status, FailureCode: failureCode, Width: width, Height: height,
		Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func artifactDomain(value model.Artifact) domain.Artifact {
	failureCode := ""
	if value.FailureCode != nil {
		failureCode = *value.FailureCode
	}
	width, height := 0, 0
	if value.Width != nil {
		width = *value.Width
	}
	if value.Height != nil {
		height = *value.Height
	}
	return domain.Artifact{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		SourceType: value.SourceType, SourceID: value.SourceID.String(), OutputKey: value.OutputKey,
		MediaType: value.MediaType, SHA256: value.SHA256, SizeBytes: value.SizeBytes, Status: value.Status,
		FailureCode: failureCode, Width: width, Height: height, Revision: value.Revision,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func locationRecord(value domain.Location) (model.ArtifactLocation, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ArtifactLocation{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ArtifactLocation{}, err
	}
	artifactID, err := uuid.Parse(value.ArtifactID)
	if err != nil {
		return model.ArtifactLocation{}, err
	}
	return model.ArtifactLocation{
		ID: id, WorkspaceID: workspaceID, ArtifactID: artifactID, LocationNo: value.LocationNo,
		StorageProfile: value.StorageProfile, Bucket: value.Bucket, ObjectKey: value.ObjectKey,
		Region: value.Region, Checksum: value.Checksum, Status: value.Status,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func locationDomain(value model.ArtifactLocation) domain.Location {
	return domain.Location{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ArtifactID: value.ArtifactID.String(),
		LocationNo: value.LocationNo, StorageProfile: value.StorageProfile, Bucket: value.Bucket,
		ObjectKey: value.ObjectKey, Region: value.Region, Checksum: value.Checksum, Status: value.Status,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func sameStagedArtifact(left, right model.Artifact) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID && left.SourceType == right.SourceType &&
		left.SourceID == right.SourceID && left.OutputKey == right.OutputKey && left.MediaType == right.MediaType &&
		left.SHA256 == right.SHA256 && left.SizeBytes == right.SizeBytes
}

func sameStagedLocation(left, right model.ArtifactLocation) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ArtifactID == right.ArtifactID && left.LocationNo == right.LocationNo &&
		left.StorageProfile == right.StorageProfile && left.Bucket == right.Bucket && left.ObjectKey == right.ObjectKey &&
		left.Region == right.Region && left.Checksum == right.Checksum
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func normalizeAuthorizationNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound("Project not found")
	}
	return err
}

func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}

func notFound(message string) error {
	return &application.Error{Code: "not_found", Message: message, Status: 404}
}
