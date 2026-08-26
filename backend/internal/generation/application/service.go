package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

const registerCandidateOperation = "generation.candidate.register_ready"

var ErrNotFound = errors.New("generation candidate not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (value *Error) Error() string { return value.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type ReadyArtifact struct {
	ID, WorkspaceID, ProjectID      string
	SourceType, SourceID, OutputKey string
	MediaType, SHA256               string
	Width, Height, Revision         int
}

type AssetReadiness interface {
	RequireReady(context.Context, Actor, string) (ReadyArtifact, error)
}

type Repository interface {
	AuthorizeProject(context.Context, Actor, string, string, bool) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	EnsureCandidate(context.Context, domain.CandidateWithReport) (domain.CandidateWithReport, error)
	GetCandidate(context.Context, string) (domain.CandidateWithReport, error)
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type ImageQCPolicy struct {
	Version           string
	AllowedMediaTypes []string
	MinWidth          int
	MinHeight         int
	MaxPixels         int64
}

type Config struct {
	Now     func() time.Time
	NewID   func() string
	ImageQC ImageQCPolicy
}

type Service struct {
	transactions TransactionManager
	assets       AssetReadiness
	config       Config
}

type RegisterReadyCandidateCommand struct {
	ArtifactID, IdempotencyKey string
}

type RegisterCandidateResult struct {
	Candidate domain.Candidate
	Report    domain.QCReport
	Receipt   platformcommand.Receipt
}

type candidateReceipt struct {
	CandidateID string `json:"candidate_id"`
}

type candidateHashInput struct {
	ArtifactID, WorkspaceID, ProjectID      string
	SourceType, ProviderJobID, OutputKey    string
	ArtifactSHA256, MediaType, QCPolicyHash string
	ArtifactRevision, Width, Height         int
}

type reportHashInput struct {
	ArtifactID, ArtifactSHA256, MediaType string
	ArtifactRevision, Width, Height       int
	PolicyHash, Status                    string
	FailureCodes                          []string
}

func NewService(transactions TransactionManager, assets AssetReadiness, config Config) *Service {
	return &Service{transactions: transactions, assets: assets, config: config}
}

func (service *Service) RegisterReadyCandidate(ctx context.Context, actor Actor, command RegisterReadyCandidateCommand) (RegisterCandidateResult, error) {
	command.ArtifactID = strings.TrimSpace(command.ArtifactID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.assets == nil || service.config.Now == nil || service.config.NewID == nil {
		return RegisterCandidateResult{}, invalid("Invalid generation candidate request")
	}
	if _, err := uuid.Parse(command.ArtifactID); err != nil || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return RegisterCandidateResult{}, invalid("Invalid generation candidate request")
	}
	policy, policyHash, err := normalizedPolicy(service.config.ImageQC)
	if err != nil {
		return RegisterCandidateResult{}, err
	}
	artifact, err := service.assets.RequireReady(ctx, actor, command.ArtifactID)
	if err != nil {
		return RegisterCandidateResult{}, normalizeError(err)
	}
	if err = validateArtifact(artifact); err != nil {
		return RegisterCandidateResult{}, err
	}
	failureCodes := domain.EvaluateImage(artifact.MediaType, artifact.Width, artifact.Height, policy)
	reportStatus, candidateStatus := domain.QCPassed, domain.CandidateQCPassed
	if len(failureCodes) > 0 {
		reportStatus, candidateStatus = domain.QCFailed, domain.CandidateQCFailed
	}
	reportHash, err := qcReportHash(
		artifact.ID, artifact.SHA256, artifact.MediaType, artifact.Revision, artifact.Width, artifact.Height,
		policyHash, reportStatus, failureCodes,
	)
	if err != nil {
		return RegisterCandidateResult{}, err
	}
	inputHash, err := platformcommand.InputHash(candidateHashInput{
		ArtifactID: artifact.ID, WorkspaceID: artifact.WorkspaceID, ProjectID: artifact.ProjectID,
		SourceType: artifact.SourceType, ProviderJobID: artifact.SourceID, OutputKey: artifact.OutputKey,
		ArtifactSHA256: artifact.SHA256, MediaType: artifact.MediaType, ArtifactRevision: artifact.Revision,
		Width: artifact.Width, Height: artifact.Height, QCPolicyHash: policyHash,
	})
	if err != nil {
		return RegisterCandidateResult{}, err
	}
	now := service.config.Now().UTC()
	desired := domain.CandidateWithReport{
		Candidate: domain.Candidate{
			ID: service.config.NewID(), WorkspaceID: artifact.WorkspaceID, ProjectID: artifact.ProjectID,
			ProviderJobID: artifact.SourceID, OutputKey: artifact.OutputKey, ArtifactID: artifact.ID,
			ArtifactRevision: artifact.Revision, ArtifactSHA256: artifact.SHA256, MediaType: artifact.MediaType,
			Width: artifact.Width, Height: artifact.Height, Status: candidateStatus, Revision: 1,
			CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		},
		Report: domain.QCReport{
			ID: service.config.NewID(), WorkspaceID: artifact.WorkspaceID, Policy: policy,
			PolicyHash: policyHash, Status: reportStatus, ReportHash: reportHash,
			FailureCodes: failureCodes, CreatedAt: now,
		},
	}

	var result RegisterCandidateResult
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if authorizeErr := repo.AuthorizeProject(ctx, actor, artifact.WorkspaceID, artifact.ProjectID, true); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, artifact.WorkspaceID, registerCandidateOperation, command.IdempotencyKey); findErr == nil {
			return replayCandidate(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		bundle, ensureErr := repo.EnsureCandidate(ctx, desired)
		if ensureErr != nil {
			return normalizeError(ensureErr)
		}
		encoded, encodeErr := platformcommand.Result(candidateReceipt{CandidateID: bundle.Candidate.ID})
		if encodeErr != nil {
			return encodeErr
		}
		receipt, ensureErr := repo.EnsureReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: artifact.WorkspaceID, Operation: registerCandidateOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bundle.Candidate.ID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
		if ensureErr != nil {
			return normalizeError(ensureErr)
		}
		result = RegisterCandidateResult{Candidate: bundle.Candidate, Report: bundle.Report, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) RequireQCPassed(ctx context.Context, actor Actor, candidateID string) (domain.CandidateWithReport, error) {
	candidateID = strings.TrimSpace(candidateID)
	if service == nil || service.transactions == nil || service.assets == nil {
		return domain.CandidateWithReport{}, notFound("Generation candidate not found")
	}
	if _, err := uuid.Parse(candidateID); err != nil {
		return domain.CandidateWithReport{}, notFound("Generation candidate not found")
	}
	var result domain.CandidateWithReport
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bundle, loadErr := repo.GetCandidate(ctx, candidateID)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, bundle.Candidate.WorkspaceID, bundle.Candidate.ProjectID, false); loadErr != nil {
			return loadErr
		}
		if loadErr = validateQCBundle(bundle); loadErr != nil {
			return loadErr
		}
		if bundle.Candidate.Status != domain.CandidateQCPassed {
			return conflict("Generation candidate did not pass deterministic QC")
		}
		result = bundle
		return nil
	})
	if err != nil {
		return domain.CandidateWithReport{}, normalizeError(err)
	}
	artifact, err := service.assets.RequireReady(ctx, actor, result.Candidate.ArtifactID)
	if err != nil {
		return domain.CandidateWithReport{}, normalizeError(err)
	}
	if !sameArtifactSnapshot(result.Candidate, artifact) {
		return domain.CandidateWithReport{}, conflict("Generation candidate artifact binding has drifted")
	}
	return result, nil
}

func normalizedPolicy(value ImageQCPolicy) (domain.ImageQCPolicy, string, error) {
	policy := domain.ImageQCPolicy{
		Version: strings.TrimSpace(value.Version), AllowedMediaTypes: append([]string(nil), value.AllowedMediaTypes...),
		MinWidth: value.MinWidth, MinHeight: value.MinHeight, MaxPixels: value.MaxPixels,
	}
	for index := range policy.AllowedMediaTypes {
		policy.AllowedMediaTypes[index] = strings.TrimSpace(policy.AllowedMediaTypes[index])
	}
	slices.Sort(policy.AllowedMediaTypes)
	policy.AllowedMediaTypes = slices.Compact(policy.AllowedMediaTypes)
	if policy.Version == "" || len(policy.Version) > 80 || len(policy.AllowedMediaTypes) == 0 ||
		policy.MinWidth < 1 || policy.MinHeight < 1 || policy.MaxPixels < 1 ||
		int64(policy.MinWidth) > policy.MaxPixels/int64(policy.MinHeight) {
		return domain.ImageQCPolicy{}, "", invalid("Invalid deterministic image QC policy")
	}
	for _, mediaType := range policy.AllowedMediaTypes {
		if mediaType != "image/jpeg" && mediaType != "image/png" {
			return domain.ImageQCPolicy{}, "", invalid("Invalid deterministic image QC policy")
		}
	}
	hash, err := platformcommand.InputHash(policy)
	if err != nil {
		return domain.ImageQCPolicy{}, "", err
	}
	return policy, hash, nil
}

func validateArtifact(value ReadyArtifact) error {
	for _, identifier := range []string{value.ID, value.WorkspaceID, value.ProjectID, value.SourceID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("asset readiness returned an invalid identifier")
		}
	}
	if value.SourceType != "generation_provider_job" || value.OutputKey == "" || value.MediaType == "" ||
		len(value.SHA256) != 64 || value.Width < 1 || value.Height < 1 || value.Revision < 1 {
		return invalid("Ready artifact is not a provider image output")
	}
	return nil
}

func replayCandidate(ctx context.Context, repo Repository, receipt platformcommand.Receipt, inputHash string, result *RegisterCandidateResult) error {
	replayed, err := platformcommand.Replay[candidateReceipt](receipt, inputHash)
	if err != nil {
		return normalizeError(err)
	}
	bundle, err := repo.GetCandidate(ctx, replayed.CandidateID)
	if err != nil {
		return err
	}
	if receipt.ResourceID != bundle.Candidate.ID {
		return errors.New("generation candidate receipt has drifted")
	}
	if err = validateQCBundle(bundle); err != nil {
		return err
	}
	*result = RegisterCandidateResult{Candidate: bundle.Candidate, Report: bundle.Report, Receipt: receipt}
	return nil
}

func validateQCBundle(bundle domain.CandidateWithReport) error {
	policy, policyHash, err := normalizedPolicy(ImageQCPolicy{
		Version: bundle.Report.Policy.Version, AllowedMediaTypes: bundle.Report.Policy.AllowedMediaTypes,
		MinWidth: bundle.Report.Policy.MinWidth, MinHeight: bundle.Report.Policy.MinHeight,
		MaxPixels: bundle.Report.Policy.MaxPixels,
	})
	if err != nil || policyHash != bundle.Report.PolicyHash ||
		policy.Version != bundle.Report.Policy.Version ||
		!slices.Equal(policy.AllowedMediaTypes, bundle.Report.Policy.AllowedMediaTypes) {
		return errors.New("generation QC policy snapshot has drifted")
	}
	failures := domain.EvaluateImage(bundle.Candidate.MediaType, bundle.Candidate.Width, bundle.Candidate.Height, policy)
	reportStatus, candidateStatus := domain.QCPassed, domain.CandidateQCPassed
	if len(failures) > 0 {
		reportStatus, candidateStatus = domain.QCFailed, domain.CandidateQCFailed
	}
	if bundle.Report.WorkspaceID != bundle.Candidate.WorkspaceID || bundle.Report.CandidateID != bundle.Candidate.ID ||
		bundle.Report.Status != reportStatus || bundle.Candidate.Status != candidateStatus ||
		!slices.Equal(bundle.Report.FailureCodes, failures) {
		return errors.New("generation candidate deterministic QC facts have drifted")
	}
	reportHash, err := qcReportHash(
		bundle.Candidate.ArtifactID, bundle.Candidate.ArtifactSHA256, bundle.Candidate.MediaType,
		bundle.Candidate.ArtifactRevision, bundle.Candidate.Width, bundle.Candidate.Height,
		bundle.Report.PolicyHash, bundle.Report.Status, bundle.Report.FailureCodes,
	)
	if err != nil {
		return err
	}
	if reportHash != bundle.Report.ReportHash {
		return errors.New("generation QC report hash has drifted")
	}
	return nil
}

func qcReportHash(artifactID, artifactSHA256, mediaType string, artifactRevision, width, height int, policyHash, status string, failureCodes []string) (string, error) {
	failureCodes = append([]string{}, failureCodes...)
	return platformcommand.InputHash(reportHashInput{
		ArtifactID: artifactID, ArtifactSHA256: artifactSHA256, MediaType: mediaType,
		ArtifactRevision: artifactRevision, Width: width, Height: height,
		PolicyHash: policyHash, Status: status, FailureCodes: failureCodes,
	})
}

func sameArtifactSnapshot(candidate domain.Candidate, artifact ReadyArtifact) bool {
	return candidate.WorkspaceID == artifact.WorkspaceID && candidate.ProjectID == artifact.ProjectID &&
		candidate.ProviderJobID == artifact.SourceID && candidate.OutputKey == artifact.OutputKey &&
		candidate.ArtifactID == artifact.ID && candidate.ArtifactRevision == artifact.Revision &&
		candidate.ArtifactSHA256 == artifact.SHA256 && candidate.MediaType == artifact.MediaType &&
		candidate.Width == artifact.Width && candidate.Height == artifact.Height
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Generation candidate command or binding has drifted")
	}
	if errors.Is(err, ErrNotFound) {
		return notFound("Generation candidate not found")
	}
	return err
}

func invalid(message string) error {
	return &Error{Code: "invalid_request", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "state_conflict", Message: message, Status: 409}
}

func notFound(message string) error {
	return &Error{Code: "not_found", Message: message, Status: 404}
}
