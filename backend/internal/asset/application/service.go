package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstore"
)

const (
	registerOperation = "asset.artifact.register_staged"
	validateOperation = "asset.artifact.validate_ready"
)

var (
	ErrNotFound      = errors.New("artifact not found")
	sha256Pattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	outputKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`)
)

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

type Repository interface {
	AuthorizeProject(context.Context, Actor, string, string, bool) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	EnsureReceipt(context.Context, platformcommand.Receipt) (platformcommand.Receipt, error)
	EnsureStaged(context.Context, domain.ArtifactWithLocation) (domain.ArtifactWithLocation, error)
	Get(context.Context, string, bool) (domain.ArtifactWithLocation, error)
	SaveReadiness(context.Context, domain.ArtifactWithLocation, int) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type ObjectReader interface {
	ReadVerified(context.Context, string, int64, string, int64) ([]byte, error)
}

type Config struct {
	Now                            func() time.Time
	NewID                          func() string
	Bucket, StorageProfile, Region string
	MaxImageBytes                  int64
}

type Service struct {
	transactions TransactionManager
	objects      ObjectReader
	config       Config
}

type RegisterStagedCommand struct {
	WorkspaceID, ProjectID, SourceType, SourceID, OutputKey string
	ProviderJobID, ProviderCallID                           string
	ObjectKey, MediaType, SHA256, IdempotencyKey            string
	SizeBytes                                               int64
}

type RegisterResult struct {
	Artifact domain.Artifact
	Location domain.Location
	Receipt  platformcommand.Receipt
}

type ValidateReadyCommand struct {
	ArtifactID, IdempotencyKey    string
	ExpectedRevision              int
	ExpectedWidth, ExpectedHeight int
}

type ValidateResult struct {
	Artifact domain.Artifact
	Location domain.Location
	Receipt  platformcommand.Receipt
}

type artifactReceipt struct {
	ArtifactID string `json:"artifact_id"`
}

type registerHashInput struct {
	WorkspaceID, ProjectID, SourceType, SourceID, OutputKey string
	ProviderJobID, ProviderCallID                           string
	ObjectKey, MediaType, SHA256                            string
	SizeBytes                                               int64
	Bucket, StorageProfile, Region                          string
}

type validateHashInput struct {
	ArtifactID, ObjectKey, MediaType, SHA256 string
	SizeBytes                                int64
	ExpectedRevision                         int
	ExpectedWidth, ExpectedHeight            int
}

func NewService(transactions TransactionManager, objects ObjectReader, config Config) *Service {
	return &Service{transactions: transactions, objects: objects, config: config}
}

func (service *Service) RegisterStaged(ctx context.Context, actor Actor, command RegisterStagedCommand) (RegisterResult, error) {
	command.OutputKey = strings.TrimSpace(command.OutputKey)
	command.ObjectKey = strings.TrimSpace(command.ObjectKey)
	command.MediaType = strings.TrimSpace(command.MediaType)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if err := service.validateRegistration(command); err != nil {
		return RegisterResult{}, err
	}
	inputHash, err := platformcommand.InputHash(registerHashInput{
		WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, SourceType: command.SourceType,
		SourceID: command.SourceID, OutputKey: command.OutputKey,
		ProviderJobID: command.ProviderJobID, ProviderCallID: command.ProviderCallID, ObjectKey: command.ObjectKey,
		MediaType: command.MediaType, SHA256: command.SHA256, SizeBytes: command.SizeBytes,
		Bucket: service.config.Bucket, StorageProfile: service.config.StorageProfile, Region: service.config.Region,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	now := service.config.Now().UTC()
	var result RegisterResult
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if authorizeErr := repo.AuthorizeProject(ctx, actor, command.WorkspaceID, command.ProjectID, true); authorizeErr != nil {
			return authorizeErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, command.WorkspaceID, registerOperation, command.IdempotencyKey); findErr == nil {
			return service.replayRegistration(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		bundle, ensureErr := repo.EnsureStaged(ctx, domain.ArtifactWithLocation{
			Artifact: domain.Artifact{
				ID: service.config.NewID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
				SourceType: command.SourceType, SourceID: command.SourceID, OutputKey: command.OutputKey,
				MediaType: command.MediaType, SHA256: command.SHA256, SizeBytes: command.SizeBytes,
				Status: domain.ReadinessPendingValidation, Revision: 1, CreatedAt: now, UpdatedAt: now,
			},
			Location: domain.Location{
				ID: service.config.NewID(), WorkspaceID: command.WorkspaceID, LocationNo: 1,
				StorageProfile: service.config.StorageProfile, Bucket: service.config.Bucket,
				ObjectKey: command.ObjectKey, Region: service.config.Region, Checksum: command.SHA256,
				Status: domain.LocationStaging, CreatedAt: now, UpdatedAt: now,
			},
		})
		if ensureErr != nil {
			return normalizeError(ensureErr)
		}
		encoded, encodeErr := platformcommand.Result(artifactReceipt{ArtifactID: bundle.Artifact.ID})
		if encodeErr != nil {
			return encodeErr
		}
		receipt, ensureErr := repo.EnsureReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: command.WorkspaceID, Operation: registerOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bundle.Artifact.ID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
		if ensureErr != nil {
			return normalizeError(ensureErr)
		}
		result = RegisterResult{Artifact: bundle.Artifact, Location: bundle.Location, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) ValidateReady(ctx context.Context, actor Actor, command ValidateReadyCommand) (ValidateResult, error) {
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if _, err := uuid.Parse(command.ArtifactID); err != nil || command.ExpectedRevision < 1 ||
		command.ExpectedWidth < 1 || command.ExpectedHeight < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return ValidateResult{}, invalid("Invalid artifact validation request")
	}
	var observed domain.ArtifactWithLocation
	var inputHash string
	var replayed *ValidateResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bundle, loadErr := repo.Get(ctx, command.ArtifactID, false)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, bundle.Artifact.WorkspaceID, bundle.Artifact.ProjectID, true); loadErr != nil {
			return loadErr
		}
		inputHash, loadErr = platformcommand.InputHash(validateHashInput{
			ArtifactID: bundle.Artifact.ID, ObjectKey: bundle.Location.ObjectKey, MediaType: bundle.Artifact.MediaType,
			SHA256: bundle.Artifact.SHA256, SizeBytes: bundle.Artifact.SizeBytes, ExpectedRevision: command.ExpectedRevision,
			ExpectedWidth: command.ExpectedWidth, ExpectedHeight: command.ExpectedHeight,
		})
		if loadErr != nil {
			return loadErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, bundle.Artifact.WorkspaceID, validateOperation, command.IdempotencyKey); findErr == nil {
			var value ValidateResult
			if replayErr := service.replayValidation(ctx, repo, receipt, inputHash, &value); replayErr != nil {
				return replayErr
			}
			replayed = &value
			return nil
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if bundle.Artifact.Status != domain.ReadinessPendingValidation || bundle.Artifact.Revision != command.ExpectedRevision || bundle.Location.Status != domain.LocationStaging {
			return conflict("Artifact changed before validation")
		}
		observed = bundle
		return nil
	})
	if err != nil {
		return ValidateResult{}, normalizeError(err)
	}
	if replayed != nil {
		return *replayed, nil
	}

	contents, readErr := service.objects.ReadVerified(ctx, observed.Location.ObjectKey, observed.Artifact.SizeBytes, observed.Artifact.SHA256, service.config.MaxImageBytes)
	status, failureCode, width, height := domain.ReadinessReady, "", 0, 0
	if readErr != nil {
		switch {
		case errors.Is(readErr, objectstore.ErrObjectChecksumMismatch):
			status, failureCode = domain.ReadinessQuarantined, "checksum_mismatch"
		case errors.Is(readErr, objectstore.ErrObjectSizeMismatch):
			status, failureCode = domain.ReadinessQuarantined, "size_mismatch"
		case errors.Is(readErr, objectstore.ErrInvalidObjectDeclaration):
			return ValidateResult{}, invalid("Invalid artifact declaration")
		default:
			return ValidateResult{}, dependencyUnavailable("Object storage is unavailable")
		}
	} else if width, height, failureCode = validateImage(contents, observed.Artifact.MediaType); failureCode != "" {
		status = domain.ReadinessQuarantined
	} else if width != command.ExpectedWidth || height != command.ExpectedHeight {
		status, failureCode = domain.ReadinessQuarantined, "image_dimensions_mismatch"
	}

	now := service.config.Now().UTC()
	var result ValidateResult
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bundle, loadErr := repo.Get(ctx, command.ArtifactID, true)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, bundle.Artifact.WorkspaceID, bundle.Artifact.ProjectID, true); loadErr != nil {
			return loadErr
		}
		if receipt, findErr := repo.FindReceipt(ctx, bundle.Artifact.WorkspaceID, validateOperation, command.IdempotencyKey); findErr == nil {
			return service.replayValidation(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if bundle.Artifact.Status != domain.ReadinessPendingValidation || bundle.Artifact.Revision != command.ExpectedRevision ||
			bundle.Location.ID != observed.Location.ID || bundle.Artifact.SHA256 != observed.Artifact.SHA256 ||
			bundle.Artifact.SizeBytes != observed.Artifact.SizeBytes {
			return conflict("Artifact changed during validation")
		}
		bundle.Artifact.Status, bundle.Artifact.FailureCode = status, failureCode
		bundle.Artifact.Width, bundle.Artifact.Height = width, height
		bundle.Artifact.Revision++
		bundle.Artifact.UpdatedAt = now
		if status == domain.ReadinessReady {
			bundle.Location.Status = domain.LocationPrimary
			bundle.Location.UpdatedAt = now
		}
		if loadErr = repo.SaveReadiness(ctx, bundle, command.ExpectedRevision); loadErr != nil {
			return loadErr
		}
		encoded, encodeErr := platformcommand.Result(artifactReceipt{ArtifactID: bundle.Artifact.ID})
		if encodeErr != nil {
			return encodeErr
		}
		receipt, ensureErr := repo.EnsureReceipt(ctx, platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: bundle.Artifact.WorkspaceID, Operation: validateOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: bundle.Artifact.ID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		})
		if ensureErr != nil {
			return normalizeError(ensureErr)
		}
		result = ValidateResult{Artifact: bundle.Artifact, Location: bundle.Location, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) RequireReady(ctx context.Context, actor Actor, artifactID string) (domain.Artifact, error) {
	if _, err := uuid.Parse(artifactID); err != nil {
		return domain.Artifact{}, notFound("Artifact not found")
	}
	var result domain.Artifact
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		bundle, loadErr := repo.Get(ctx, artifactID, false)
		if loadErr != nil {
			return loadErr
		}
		if loadErr = repo.AuthorizeProject(ctx, actor, bundle.Artifact.WorkspaceID, bundle.Artifact.ProjectID, false); loadErr != nil {
			return loadErr
		}
		if bundle.Artifact.Status != domain.ReadinessReady || bundle.Location.Status != domain.LocationPrimary {
			return &Error{Code: "artifact_not_ready", Message: "Artifact is not ready", Status: 409, NextAction: "wait_or_replace"}
		}
		result = bundle.Artifact
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) validateRegistration(command RegisterStagedCommand) error {
	if _, err := uuid.Parse(command.WorkspaceID); err != nil {
		return invalid("Invalid staged artifact request")
	}
	if _, err := uuid.Parse(command.ProjectID); err != nil {
		return invalid("Invalid staged artifact request")
	}
	if _, err := uuid.Parse(command.SourceID); err != nil {
		return invalid("Invalid staged artifact request")
	}
	if _, err := uuid.Parse(command.ProviderJobID); err != nil {
		return invalid("Invalid staged artifact request")
	}
	if _, err := uuid.Parse(command.ProviderCallID); err != nil {
		return invalid("Invalid staged artifact request")
	}
	if command.SourceType != "generation_provider_receipt" || !outputKeyPattern.MatchString(command.OutputKey) ||
		(command.MediaType != "image/png" && command.MediaType != "image/jpeg") || !sha256Pattern.MatchString(command.SHA256) ||
		command.SizeBytes < 1 || command.SizeBytes > service.config.MaxImageBytes || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 ||
		service.config.Now == nil || service.config.NewID == nil || service.config.Bucket == "" || service.config.StorageProfile == "" || service.config.Region == "" {
		return invalid("Invalid staged artifact request")
	}
	prefix := fmt.Sprintf("staging/%s/%s/%s/", command.WorkspaceID, command.ProviderJobID, command.ProviderCallID)
	if !strings.HasPrefix(command.ObjectKey, prefix) || len(command.ObjectKey) > 600 || strings.Contains(command.ObjectKey, "..") || strings.HasSuffix(command.ObjectKey, "/") {
		return invalid("Invalid staged artifact object key")
	}
	return nil
}

func (service *Service) replayRegistration(ctx context.Context, repo Repository, receipt platformcommand.Receipt, inputHash string, result *RegisterResult) error {
	replayed, err := platformcommand.Replay[artifactReceipt](receipt, inputHash)
	if err != nil {
		return normalizeError(err)
	}
	bundle, err := repo.Get(ctx, replayed.ArtifactID, false)
	if err != nil {
		return err
	}
	if receipt.ResourceID != bundle.Artifact.ID {
		return errors.New("artifact registration receipt has drifted")
	}
	*result = RegisterResult{Artifact: bundle.Artifact, Location: bundle.Location, Receipt: receipt}
	return nil
}

func (service *Service) replayValidation(ctx context.Context, repo Repository, receipt platformcommand.Receipt, inputHash string, result *ValidateResult) error {
	replayed, err := platformcommand.Replay[artifactReceipt](receipt, inputHash)
	if err != nil {
		return normalizeError(err)
	}
	bundle, err := repo.Get(ctx, replayed.ArtifactID, false)
	if err != nil {
		return err
	}
	if receipt.ResourceID != bundle.Artifact.ID || (bundle.Artifact.Status != domain.ReadinessReady && bundle.Artifact.Status != domain.ReadinessQuarantined) {
		return errors.New("artifact validation receipt has drifted")
	}
	*result = ValidateResult{Artifact: bundle.Artifact, Location: bundle.Location, Receipt: receipt}
	return nil
}

func validateImage(contents []byte, declaredMediaType string) (int, int, string) {
	detected := http.DetectContentType(contents)
	if detected != declaredMediaType {
		return 0, 0, "media_type_mismatch"
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil || configuration.Width < 1 || configuration.Height < 1 {
		return 0, 0, "image_decode_failed"
	}
	if (declaredMediaType == "image/png" && format != "png") || (declaredMediaType == "image/jpeg" && format != "jpeg") {
		return 0, 0, "media_type_mismatch"
	}
	return configuration.Width, configuration.Height, ""
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Idempotency key was already used with different input")
	}
	if errors.Is(err, ErrNotFound) {
		return notFound("Artifact not found")
	}
	return err
}

func invalid(message string) error {
	return &Error{Code: "invalid_request", Message: message, Status: 422}
}
func conflict(message string) error {
	return &Error{Code: "state_conflict", Message: message, Status: 409}
}
func notFound(message string) error { return &Error{Code: "not_found", Message: message, Status: 404} }
func dependencyUnavailable(message string) error {
	return &Error{Code: "dependency_unavailable", Message: message, Status: 503, NextAction: "retry"}
}
