package gormdb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) ProjectWorkspace(ctx context.Context, actor application.Actor, projectID string, write bool) (string, error) {
	userID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return "", unauthenticated()
	}
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return "", notFound("Project not found")
	}
	var user model.UserAccount
	if err = repo.database.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return "", unauthenticated()
	}
	var project model.Project
	if err = repo.database.WithContext(ctx).First(&project, "id = ?", parsedProjectID).Error; err != nil {
		return "", notFound("Project not found")
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", project.WorkspaceID).Error; err != nil {
		return "", notFound("Project not found")
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND user_id = ? AND status = ?", project.WorkspaceID, userID, "active").First(&membership).Error; err != nil {
		return "", notFound("Project not found")
	}
	if write && (membership.Role == "viewer" || workspace.Status != "active" || project.Status != "active") {
		return "", &application.Error{Code: "forbidden", Message: "Action is not allowed", Status: 403}
	}
	return project.WorkspaceID.String(), nil
}

func (repo *repository) FindReceipt(ctx context.Context, workspaceID, operation, key string) (platformcommand.Receipt, error) {
	parsedWorkspaceID, err := uuid.Parse(workspaceID)
	if err != nil {
		return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
	}
	var record model.CommandReceipt
	if err = repo.database.WithContext(ctx).Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", parsedWorkspaceID, operation, key).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformcommand.Receipt{}, platformcommand.ErrReceiptNotFound
		}
		return platformcommand.Receipt{}, err
	}
	return receiptDomain(record), nil
}

func (repo *repository) CreateReceipt(ctx context.Context, receipt platformcommand.Receipt) error {
	id, err := uuid.Parse(receipt.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(receipt.WorkspaceID)
	if err != nil {
		return err
	}
	resourceID, err := uuid.Parse(receipt.ResourceID)
	if err != nil {
		return err
	}
	createdBy, err := uuid.Parse(receipt.CreatedBy)
	if err != nil {
		return err
	}
	record := model.CommandReceipt{ID: id, WorkspaceID: workspaceID, Operation: receipt.Operation, IdempotencyKey: receipt.IdempotencyKey, InputHash: receipt.InputHash, ResourceID: resourceID, Result: datatypes.JSON(receipt.Result), CreatedBy: createdBy, CreatedAt: receipt.CreatedAt}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return &application.Error{Code: "resource_conflict", Message: "Idempotency key is already in use", Status: 409}
		}
		return err
	}
	return nil
}

func (repo *repository) CreateAnalysis(ctx context.Context, analysis domain.Analysis) error {
	document, err := documentRecord(analysis.Document)
	if err != nil {
		return err
	}
	revision, err := revisionRecord(analysis.Revision)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&document).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&revision).Error
}

func (repo *repository) GetAnalysis(ctx context.Context, revisionID string) (domain.Analysis, error) {
	parsedRevisionID, err := uuid.Parse(revisionID)
	if err != nil {
		return domain.Analysis{}, application.ErrNotFound
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).First(&revision, "id = ?", parsedRevisionID).Error; err != nil {
		return domain.Analysis{}, normalizeNotFound(err)
	}
	var document model.ScriptDocument
	if err = repo.database.WithContext(ctx).First(&document, "id = ?", revision.DocumentID).Error; err != nil {
		return domain.Analysis{}, normalizeNotFound(err)
	}
	return analysisDomain(document, revision)
}

func (repo *repository) GetCurrentAnalysis(ctx context.Context, projectID string) (domain.Analysis, error) {
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return domain.Analysis{}, application.ErrNotFound
	}
	var document model.ScriptDocument
	if err = repo.database.WithContext(ctx).
		Where("project_id = ? AND status = ?", parsedProjectID, "active").
		Order("created_at DESC").
		Order("id DESC").
		First(&document).Error; err != nil {
		return domain.Analysis{}, normalizeNotFound(err)
	}
	var revision model.DocumentRevision
	if err = repo.database.WithContext(ctx).
		Where("document_id = ?", document.ID).
		Order("version_no DESC").
		Order("id DESC").
		First(&revision).Error; err != nil {
		return domain.Analysis{}, normalizeNotFound(err)
	}
	return analysisDomain(document, revision)
}

func (repo *repository) ListDocuments(ctx context.Context, projectID string, limit, offset int) ([]domain.Document, int, error) {
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, 0, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Model(&model.ScriptDocument{}).Where("project_id = ?", parsedProjectID)
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []model.ScriptDocument
	if err = query.Order("created_at DESC").Order("id").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	items := make([]domain.Document, len(records))
	for index, record := range records {
		items[index] = documentDomain(record)
	}
	return items, int(total), nil
}

func documentRecord(value domain.Document) (model.ScriptDocument, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.ScriptDocument{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.ScriptDocument{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.ScriptDocument{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.ScriptDocument{}, err
	}
	mediaVersionID, err := optionalUUID(value.SourceMediaVersionID)
	if err != nil {
		return model.ScriptDocument{}, err
	}
	return model.ScriptDocument{ID: id, WorkspaceID: workspaceID, ProjectID: projectID, Title: value.Title, SourceType: value.SourceType, SourceMediaVersionID: mediaVersionID, Language: value.Language, RightsDeclaration: value.RightsDeclaration, Status: value.Status, Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.CreatedAt}, nil
}

func revisionRecord(value domain.Revision) (model.DocumentRevision, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	documentID, err := uuid.Parse(value.DocumentID)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	mediaVersionID, err := optionalUUID(value.SourceMediaVersionID)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	normalizationMap, err := json.Marshal(value.NormalizationMap)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	blocks, err := json.Marshal(value.Blocks)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	issues, err := json.Marshal(value.Issues)
	if err != nil {
		return model.DocumentRevision{}, err
	}
	return model.DocumentRevision{ID: id, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: value.VersionNo, SourceType: value.SourceType, SourceMediaVersionID: mediaVersionID, RawText: value.RawText, RawHash: value.RawHash, NormalizedText: value.NormalizedText, NormalizedHash: value.NormalizedHash, NormalizerVersion: value.NormalizerVersion, NormalizationMap: datatypes.JSON(normalizationMap), CodepointCount: value.CodepointCount, AnalysisStatus: value.AnalysisStatus, AnalyzerVersion: value.AnalyzerVersion, Blocks: datatypes.JSON(blocks), Issues: datatypes.JSON(issues), CreatedBy: createdBy, CreatedAt: value.CreatedAt}, nil
}

func documentDomain(value model.ScriptDocument) domain.Document {
	return domain.Document{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(), Title: value.Title, SourceType: value.SourceType, SourceMediaVersionID: optionalString(value.SourceMediaVersionID), Language: value.Language, RightsDeclaration: value.RightsDeclaration, Status: value.Status, Revision: value.Revision, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt}
}

func revisionDomain(value model.DocumentRevision) (domain.Revision, error) {
	revision := domain.Revision{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), DocumentID: value.DocumentID.String(), VersionNo: value.VersionNo, SourceType: value.SourceType, SourceMediaVersionID: optionalString(value.SourceMediaVersionID), RawText: value.RawText, RawHash: value.RawHash, NormalizedText: value.NormalizedText, NormalizedHash: value.NormalizedHash, NormalizerVersion: value.NormalizerVersion, CodepointCount: value.CodepointCount, AnalysisStatus: value.AnalysisStatus, AnalyzerVersion: value.AnalyzerVersion, CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt}
	if err := json.Unmarshal(value.NormalizationMap, &revision.NormalizationMap); err != nil {
		return domain.Revision{}, err
	}
	if err := json.Unmarshal(value.Blocks, &revision.Blocks); err != nil {
		return domain.Revision{}, err
	}
	if err := json.Unmarshal(value.Issues, &revision.Issues); err != nil {
		return domain.Revision{}, err
	}
	return revision, nil
}

func analysisDomain(document model.ScriptDocument, revision model.DocumentRevision) (domain.Analysis, error) {
	revisionValue, err := revisionDomain(revision)
	if err != nil {
		return domain.Analysis{}, err
	}
	return domain.Analysis{Document: documentDomain(document), Revision: revisionValue}, nil
}

func receiptDomain(value model.CommandReceipt) platformcommand.Receipt {
	return platformcommand.Receipt{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), Operation: value.Operation, IdempotencyKey: value.IdempotencyKey, InputHash: value.InputHash, ResourceID: value.ResourceID.String(), Result: append([]byte(nil), value.Result...), CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt}
}

func optionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := uuid.Parse(*value)
	return &parsed, err
}

func optionalString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	result := value.String()
	return &result
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
