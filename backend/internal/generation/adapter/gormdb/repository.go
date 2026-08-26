package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
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

func (repo *repository) EnsureCandidate(ctx context.Context, desired domain.CandidateWithReport) (domain.CandidateWithReport, error) {
	candidateRecord, err := candidateRecord(desired.Candidate)
	if err != nil {
		return domain.CandidateWithReport{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&candidateRecord).Error; err != nil {
		return domain.CandidateWithReport{}, err
	}
	var persistedCandidate model.GenerationCandidate
	err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND artifact_id = ?", candidateRecord.WorkspaceID, candidateRecord.ArtifactID).
		First(&persistedCandidate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND provider_job_id = ? AND output_key = ?", candidateRecord.WorkspaceID, candidateRecord.ProviderJobID, candidateRecord.OutputKey).
			First(&persistedCandidate).Error
	}
	if err != nil {
		return domain.CandidateWithReport{}, err
	}
	if !sameCandidate(persistedCandidate, candidateRecord) {
		return domain.CandidateWithReport{}, platformcommand.ErrInputMismatch
	}
	desired.Report.CandidateID = persistedCandidate.ID.String()
	reportRecord, err := reportRecord(desired.Report)
	if err != nil {
		return domain.CandidateWithReport{}, err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&reportRecord).Error; err != nil {
		return domain.CandidateWithReport{}, err
	}
	var persistedReport model.GenerationQCReport
	if err = repo.database.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("candidate_id = ?", persistedCandidate.ID).First(&persistedReport).Error; err != nil {
		return domain.CandidateWithReport{}, err
	}
	result, err := bundleDomain(persistedCandidate, persistedReport)
	if err != nil {
		return domain.CandidateWithReport{}, err
	}
	if !sameReport(result.Report, desired.Report) {
		return domain.CandidateWithReport{}, platformcommand.ErrInputMismatch
	}
	return result, nil
}

func (repo *repository) GetCandidate(ctx context.Context, candidateID string) (domain.CandidateWithReport, error) {
	parsed, err := uuid.Parse(candidateID)
	if err != nil {
		return domain.CandidateWithReport{}, application.ErrNotFound
	}
	var candidate model.GenerationCandidate
	if err = repo.database.WithContext(ctx).Where("id = ?", parsed).First(&candidate).Error; err != nil {
		return domain.CandidateWithReport{}, normalizeNotFound(err)
	}
	var report model.GenerationQCReport
	if err = repo.database.WithContext(ctx).Where("candidate_id = ?", parsed).First(&report).Error; err != nil {
		return domain.CandidateWithReport{}, normalizeNotFound(err)
	}
	if candidate.WorkspaceID != report.WorkspaceID {
		return domain.CandidateWithReport{}, errors.New("generation candidate QC workspace has drifted")
	}
	return bundleDomain(candidate, report)
}

func (repo *repository) GetCandidateByProviderOutput(
	ctx context.Context,
	providerJobID, outputKey string,
) (domain.CandidateWithReport, error) {
	jobID, err := uuid.Parse(providerJobID)
	if err != nil {
		return domain.CandidateWithReport{}, application.ErrNotFound
	}
	var candidate model.GenerationCandidate
	if err = repo.database.WithContext(ctx).
		Where("provider_job_id = ? AND output_key = ?", jobID, outputKey).First(&candidate).Error; err != nil {
		return domain.CandidateWithReport{}, normalizeNotFound(err)
	}
	var report model.GenerationQCReport
	if err = repo.database.WithContext(ctx).Where("candidate_id = ?", candidate.ID).First(&report).Error; err != nil {
		return domain.CandidateWithReport{}, normalizeNotFound(err)
	}
	return bundleDomain(candidate, report)
}

func candidateRecord(value domain.Candidate) (model.GenerationCandidate, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.GenerationCandidate{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.GenerationCandidate{}, err
	}
	projectID, err := uuid.Parse(value.ProjectID)
	if err != nil {
		return model.GenerationCandidate{}, err
	}
	providerJobID, err := uuid.Parse(value.ProviderJobID)
	if err != nil {
		return model.GenerationCandidate{}, err
	}
	artifactID, err := uuid.Parse(value.ArtifactID)
	if err != nil {
		return model.GenerationCandidate{}, err
	}
	createdBy, err := uuid.Parse(value.CreatedBy)
	if err != nil {
		return model.GenerationCandidate{}, err
	}
	return model.GenerationCandidate{
		ID: id, WorkspaceID: workspaceID, ProjectID: projectID, ProviderJobID: providerJobID,
		OutputKey: value.OutputKey, ArtifactID: artifactID, ArtifactRevision: value.ArtifactRevision,
		ArtifactSHA256: value.ArtifactSHA256, MediaType: value.MediaType, Width: value.Width, Height: value.Height,
		Status: value.Status, Revision: value.Revision, CreatedBy: createdBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}, nil
}

func candidateDomain(value model.GenerationCandidate) domain.Candidate {
	return domain.Candidate{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), ProjectID: value.ProjectID.String(),
		ProviderJobID: value.ProviderJobID.String(), OutputKey: value.OutputKey, ArtifactID: value.ArtifactID.String(),
		ArtifactRevision: value.ArtifactRevision, ArtifactSHA256: value.ArtifactSHA256, MediaType: value.MediaType,
		Width: value.Width, Height: value.Height, Status: value.Status, Revision: value.Revision,
		CreatedBy: value.CreatedBy.String(), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func reportRecord(value domain.QCReport) (model.GenerationQCReport, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.GenerationQCReport{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.GenerationQCReport{}, err
	}
	candidateID, err := uuid.Parse(value.CandidateID)
	if err != nil {
		return model.GenerationQCReport{}, err
	}
	policy, err := json.Marshal(value.Policy)
	if err != nil {
		return model.GenerationQCReport{}, err
	}
	return model.GenerationQCReport{
		ID: id, WorkspaceID: workspaceID, CandidateID: candidateID, PolicyVersion: value.Policy.Version,
		PolicyHash: value.PolicyHash, Policy: datatypes.JSON(policy), Status: value.Status,
		FailureCodes: datatypes.NewJSONSlice(append([]string{}, value.FailureCodes...)),
		ReportHash:   value.ReportHash, CreatedAt: value.CreatedAt,
	}, nil
}

func reportDomain(value model.GenerationQCReport) (domain.QCReport, error) {
	var policy domain.ImageQCPolicy
	if err := json.Unmarshal(value.Policy, &policy); err != nil {
		return domain.QCReport{}, err
	}
	return domain.QCReport{
		ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), CandidateID: value.CandidateID.String(),
		Policy: policy, PolicyHash: value.PolicyHash, Status: value.Status,
		FailureCodes: append([]string{}, value.FailureCodes...), ReportHash: value.ReportHash, CreatedAt: value.CreatedAt,
	}, nil
}

func bundleDomain(candidate model.GenerationCandidate, report model.GenerationQCReport) (domain.CandidateWithReport, error) {
	reportValue, err := reportDomain(report)
	if err != nil {
		return domain.CandidateWithReport{}, err
	}
	return domain.CandidateWithReport{Candidate: candidateDomain(candidate), Report: reportValue}, nil
}

func sameCandidate(left, right model.GenerationCandidate) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.ProviderJobID == right.ProviderJobID && left.OutputKey == right.OutputKey && left.ArtifactID == right.ArtifactID &&
		left.ArtifactRevision == right.ArtifactRevision && left.ArtifactSHA256 == right.ArtifactSHA256 &&
		left.MediaType == right.MediaType && left.Width == right.Width && left.Height == right.Height &&
		left.Status == right.Status && left.Revision == right.Revision
}

func sameReport(left, right domain.QCReport) bool {
	return left.WorkspaceID == right.WorkspaceID && left.CandidateID == right.CandidateID &&
		left.Policy.Version == right.Policy.Version && slices.Equal(left.Policy.AllowedMediaTypes, right.Policy.AllowedMediaTypes) &&
		left.Policy.MinWidth == right.Policy.MinWidth && left.Policy.MinHeight == right.Policy.MinHeight &&
		left.Policy.MaxPixels == right.Policy.MaxPixels && left.PolicyHash == right.PolicyHash && left.Status == right.Status &&
		slices.Equal(left.FailureCodes, right.FailureCodes) && left.ReportHash == right.ReportHash
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
