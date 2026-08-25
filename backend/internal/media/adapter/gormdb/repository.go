package gormdb

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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

func (repo *repository) Authorize(ctx context.Context, actor application.Actor, workspaceID string, write bool) error {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return unauthenticated()
	}
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return notFound("Workspace not found")
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return unauthenticated()
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", workspaceUUID).Error; err != nil {
		return notFound("Workspace not found")
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceUUID, userID, "active").First(&membership).Error; err != nil {
		return notFound("Workspace not found")
	}
	if write && (workspace.Status != "active" || membership.Role == "viewer") {
		return &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return nil
}

func (repo *repository) FindUploadByKey(ctx context.Context, workspaceID, key string, lock bool) (domain.UploadSession, error) {
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.UploadSession{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("workspace_id = ? AND idempotency_key = ?", workspaceUUID, key)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.UploadSession
	if err = query.First(&record).Error; err != nil {
		return domain.UploadSession{}, normalizeNotFound(err)
	}
	return uploadDomain(record), nil
}

func (repo *repository) GetUpload(ctx context.Context, id string, lock bool) (domain.UploadSession, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return domain.UploadSession{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", parsed)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.UploadSession
	if err = query.First(&record).Error; err != nil {
		return domain.UploadSession{}, normalizeNotFound(err)
	}
	return uploadDomain(record), nil
}

func (repo *repository) CreateUpload(ctx context.Context, upload domain.UploadSession) error {
	record, err := uploadRecord(upload)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) SaveUpload(ctx context.Context, upload domain.UploadSession) error {
	record, err := uploadRecord(upload)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Model(&model.UploadSession{}).Where("id = ?", record.ID).Select("status", "media_object_id", "completed_at", "updated_at").Updates(&record).Error
}

func (repo *repository) CreateCompletion(ctx context.Context, object domain.MediaObject, version domain.MediaVersion, task domain.Task) error {
	objectRecord, err := objectRecord(object)
	if err != nil {
		return err
	}
	versionRecord, err := versionRecord(version)
	if err != nil {
		return err
	}
	taskRecord, err := taskRecord(task)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&objectRecord).Error; err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&versionRecord).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&taskRecord).Error
}

func (repo *repository) Completion(ctx context.Context, upload domain.UploadSession) (domain.MediaObject, domain.MediaVersion, domain.Task, error) {
	if upload.MediaObjectID == nil {
		return domain.MediaObject{}, domain.MediaVersion{}, domain.Task{}, application.ErrNotFound
	}
	objectID, err := uuid.Parse(*upload.MediaObjectID)
	if err != nil {
		return domain.MediaObject{}, domain.MediaVersion{}, domain.Task{}, application.ErrNotFound
	}
	var object model.MediaObject
	if err = repo.database.WithContext(ctx).First(&object, "id = ?", objectID).Error; err != nil {
		return domain.MediaObject{}, domain.MediaVersion{}, domain.Task{}, normalizeNotFound(err)
	}
	if object.CurrentVersionID == nil {
		return domain.MediaObject{}, domain.MediaVersion{}, domain.Task{}, application.ErrNotFound
	}
	var version model.MediaVersion
	if err = repo.database.WithContext(ctx).First(&version, "id = ?", *object.CurrentVersionID).Error; err != nil {
		return domain.MediaObject{}, domain.MediaVersion{}, domain.Task{}, normalizeNotFound(err)
	}
	var task model.WorkflowTask
	if err = repo.database.WithContext(ctx).Where("request_type = ? AND request_id = ?", "media_version", version.ID).First(&task).Error; err != nil {
		return domain.MediaObject{}, domain.MediaVersion{}, domain.Task{}, normalizeNotFound(err)
	}
	objectValue := mediaObjectDomain(object)
	versionValue := mediaVersionDomain(version, objectValue)
	return objectValue, versionValue, taskDomain(task), nil
}

func (repo *repository) GetVersion(ctx context.Context, id string) (domain.MediaVersion, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return domain.MediaVersion{}, application.ErrNotFound
	}
	var version model.MediaVersion
	if err = repo.database.WithContext(ctx).First(&version, "id = ?", parsed).Error; err != nil {
		return domain.MediaVersion{}, normalizeNotFound(err)
	}
	var object model.MediaObject
	if err = repo.database.WithContext(ctx).First(&object, "id = ?", version.MediaObjectID).Error; err != nil {
		return domain.MediaVersion{}, normalizeNotFound(err)
	}
	return mediaVersionDomain(version, mediaObjectDomain(object)), nil
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func unauthenticated() error {
	return &application.Error{Code: "unauthenticated", Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
func notFound(message string) error {
	return &application.Error{Code: "not_found", Message: message, Status: 404}
}

func uploadRecord(value domain.UploadSession) (model.UploadSession, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.UploadSession{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.UploadSession{}, err
	}
	var mediaObjectID *uuid.UUID
	if value.MediaObjectID != nil {
		parsed, parseErr := uuid.Parse(*value.MediaObjectID)
		if parseErr != nil {
			return model.UploadSession{}, parseErr
		}
		mediaObjectID = &parsed
	}
	return model.UploadSession{ID: id, WorkspaceID: workspaceID, MediaObjectID: mediaObjectID, Status: value.Status, Kind: value.Kind, Filename: value.Filename, SizeBytes: value.SizeBytes, MIMEType: value.MIMEType, SHA256: value.SHA256, ObjectKey: value.ObjectKey, IdempotencyKey: value.IdempotencyKey, ExpiresAt: value.ExpiresAt, CompletedAt: value.CompletedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func uploadDomain(value model.UploadSession) domain.UploadSession {
	var mediaObjectID *string
	if value.MediaObjectID != nil {
		parsed := value.MediaObjectID.String()
		mediaObjectID = &parsed
	}
	return domain.UploadSession{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), MediaObjectID: mediaObjectID, Status: value.Status, Kind: value.Kind, Filename: value.Filename, SizeBytes: value.SizeBytes, MIMEType: value.MIMEType, SHA256: value.SHA256, ObjectKey: value.ObjectKey, IdempotencyKey: value.IdempotencyKey, ExpiresAt: value.ExpiresAt, CompletedAt: value.CompletedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func objectRecord(value domain.MediaObject) (model.MediaObject, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.MediaObject{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.MediaObject{}, err
	}
	var currentVersionID *uuid.UUID
	if value.CurrentVersionID != nil {
		parsed, parseErr := uuid.Parse(*value.CurrentVersionID)
		if parseErr != nil {
			return model.MediaObject{}, parseErr
		}
		currentVersionID = &parsed
	}
	return model.MediaObject{ID: id, WorkspaceID: workspaceID, Kind: value.Kind, SourceType: value.SourceType, Status: value.Status, CurrentVersionID: currentVersionID, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func mediaObjectDomain(value model.MediaObject) domain.MediaObject {
	var currentVersionID *string
	if value.CurrentVersionID != nil {
		parsed := value.CurrentVersionID.String()
		currentVersionID = &parsed
	}
	return domain.MediaObject{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), Kind: value.Kind, SourceType: value.SourceType, Status: value.Status, CurrentVersionID: currentVersionID, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func versionRecord(value domain.MediaVersion) (model.MediaVersion, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.MediaVersion{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.MediaVersion{}, err
	}
	objectID, err := uuid.Parse(value.MediaObjectID)
	if err != nil {
		return model.MediaVersion{}, err
	}
	return model.MediaVersion{ID: id, WorkspaceID: workspaceID, MediaObjectID: objectID, VersionNo: value.VersionNo, Filename: value.Filename, SHA256: value.SHA256, SizeBytes: value.SizeBytes, MIMEType: value.MIMEType, ObjectKey: value.ObjectKey, ProbeStatus: value.ProbeStatus, ProbeAttempt: value.ProbeAttempt, ProbeErrorCode: value.ProbeErrorCode, ProbeSummary: value.ProbeSummary, ProbeNextAction: value.ProbeNextAction, Width: value.Width, Height: value.Height, DurationMS: value.DurationMS, Codec: value.Codec, Container: value.Container, CreatedAt: value.CreatedAt}, nil
}

func mediaVersionDomain(value model.MediaVersion, object domain.MediaObject) domain.MediaVersion {
	return domain.MediaVersion{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), MediaObjectID: value.MediaObjectID.String(), VersionNo: value.VersionNo, Filename: value.Filename, SHA256: value.SHA256, SizeBytes: value.SizeBytes, MIMEType: value.MIMEType, ObjectKey: value.ObjectKey, ProbeStatus: value.ProbeStatus, ProbeAttempt: value.ProbeAttempt, ProbeErrorCode: value.ProbeErrorCode, ProbeSummary: value.ProbeSummary, ProbeNextAction: value.ProbeNextAction, Width: value.Width, Height: value.Height, DurationMS: value.DurationMS, Codec: value.Codec, Container: value.Container, CreatedAt: value.CreatedAt, MediaObject: object}
}

func taskRecord(value domain.Task) (model.WorkflowTask, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.WorkflowTask{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.WorkflowTask{}, err
	}
	requestID, err := uuid.Parse(value.RequestID)
	if err != nil {
		return model.WorkflowTask{}, err
	}
	return model.WorkflowTask{ID: id, WorkspaceID: workspaceID, TaskType: value.TaskType, RequestType: value.RequestType, RequestID: requestID, Scope: datatypes.JSON(value.Scope), Status: value.Status, ProgressStage: value.ProgressStage, Error: datatypes.JSON(value.Error), NextAction: value.NextAction, CancelStatus: value.CancelStatus, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func taskDomain(value model.WorkflowTask) domain.Task {
	return domain.Task{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), TaskType: value.TaskType, RequestType: value.RequestType, RequestID: value.RequestID.String(), Scope: append([]byte(nil), value.Scope...), Status: value.Status, ProgressStage: value.ProgressStage, Error: append([]byte(nil), value.Error...), NextAction: value.NextAction, CancelStatus: value.CancelStatus, Revision: value.Revision, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}
