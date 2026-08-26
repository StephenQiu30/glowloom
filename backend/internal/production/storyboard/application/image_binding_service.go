package application

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

const bindSelectedImageOperation = "storyboard.shot.bind_selected_image"

var bindingHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type SelectedImageSnapshot struct {
	ID, WorkspaceID, ProjectID string
	Revision                   int
	ContentHash                string
	CandidateID                string
	CandidateRevision          int
	ArtifactID                 string
	ArtifactRevision           int
	ArtifactSHA256             string
}

type SelectedImageSource interface {
	RequireSelectedImage(context.Context, Actor, string) (SelectedImageSnapshot, error)
}

type ShotImageBindingRepository interface {
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
	LockShotImageTarget(context.Context, Actor, string, bool) (domain.Shot, error)
	FindCurrentShotImageBinding(context.Context, string) (domain.ShotImageBindingVersion, error)
	GetShotImageBinding(context.Context, string) (domain.ShotImageBindingVersion, error)
	CreateShotImageBinding(context.Context, domain.ShotImageBindingVersion) error
}

type ShotImageBindingTransactionManager interface {
	WithinShotImageBindingTransaction(context.Context, func(ShotImageBindingRepository) error) error
}

type ShotImageBindingService struct {
	transactions ShotImageBindingTransactionManager
	selections   SelectedImageSource
	config       Config
}

type BindSelectedImageCommand struct {
	ShotID, CandidateSelectionID, IdempotencyKey  string
	ExpectedShotContentHash                       string
	ExpectedShotRevision, ExpectedCurrentRevision int
}

type BindSelectedImageResult struct {
	Binding domain.ShotImageBindingVersion
	Receipt platformcommand.Receipt
}

type shotImageBindingReceipt struct {
	BindingID string `json:"binding_id"`
}

func NewShotImageBindingService(
	transactions ShotImageBindingTransactionManager,
	selections SelectedImageSource,
	config Config,
) *ShotImageBindingService {
	return &ShotImageBindingService{transactions: transactions, selections: selections, config: config}
}

func (service *ShotImageBindingService) BindSelectedImage(
	ctx context.Context,
	actor Actor,
	command BindSelectedImageCommand,
) (BindSelectedImageResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.ShotID = strings.TrimSpace(command.ShotID)
	command.CandidateSelectionID = strings.TrimSpace(command.CandidateSelectionID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.selections == nil ||
		service.config.Now == nil || service.config.NewID == nil || actor.TokenVersion < 1 ||
		!validBindingUUID(actor.UserID) || !validBindingUUID(command.ShotID) ||
		!validBindingUUID(command.CandidateSelectionID) || command.ExpectedCurrentRevision < 0 ||
		command.ExpectedShotRevision < 1 || !validBindingHash(command.ExpectedShotContentHash) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return BindSelectedImageResult{}, invalid("Invalid Shot image binding request")
	}
	selection, err := service.selections.RequireSelectedImage(ctx, actor, command.CandidateSelectionID)
	if err != nil {
		return BindSelectedImageResult{}, err
	}
	if err = validateSelectedImage(selection, command.CandidateSelectionID); err != nil {
		return BindSelectedImageResult{}, err
	}

	var result BindSelectedImageResult
	err = service.transactions.WithinShotImageBindingTransaction(ctx, func(repo ShotImageBindingRepository) error {
		shot, loadErr := repo.LockShotImageTarget(ctx, actor, command.ShotID, true)
		if loadErr != nil {
			return loadErr
		}
		if shot.Status != "active" || shot.Revision != command.ExpectedShotRevision ||
			shot.ContentHash != command.ExpectedShotContentHash ||
			shot.WorkspaceID != selection.WorkspaceID || shot.ProjectID != selection.ProjectID ||
			shot.Revision < 1 || !validBindingHash(shot.ContentHash) {
			return conflict("Shot and selected image do not share an active production boundary")
		}
		inputHash, hashErr := platformcommand.InputHash(struct {
			ActorID                 string
			ShotID                  string
			ShotRevision            int
			ShotContentHash         string
			Selection               SelectedImageSnapshot
			ExpectedCurrentRevision int
		}{actor.UserID, shot.ID, shot.Revision, shot.ContentHash, selection, command.ExpectedCurrentRevision})
		if hashErr != nil {
			return hashErr
		}
		if receipt, findErr := repo.FindReceipt(
			ctx, shot.WorkspaceID, bindSelectedImageOperation, command.IdempotencyKey,
		); findErr == nil {
			return replayShotImageBinding(ctx, repo, receipt, inputHash, &result)
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}

		current, currentErr := repo.FindCurrentShotImageBinding(ctx, shot.ID)
		switch {
		case errors.Is(currentErr, ErrNotFound) && command.ExpectedCurrentRevision != 0:
			return conflict("Shot image binding revision has changed")
		case currentErr != nil && !errors.Is(currentErr, ErrNotFound):
			return currentErr
		case currentErr == nil && current.Revision != command.ExpectedCurrentRevision:
			return conflict("Shot image binding revision has changed")
		}
		revision := command.ExpectedCurrentRevision + 1
		contentHash, hashErr := shotImageBindingContentHash(shot, selection, revision)
		if hashErr != nil {
			return hashErr
		}
		bindingID := strings.TrimSpace(service.config.NewID())
		receiptID := strings.TrimSpace(service.config.NewID())
		if !validBindingUUID(bindingID) || !validBindingUUID(receiptID) {
			return errors.New("Shot image binding identifier is invalid")
		}
		now := service.config.Now().UTC()
		binding := domain.ShotImageBindingVersion{
			ID: bindingID, WorkspaceID: shot.WorkspaceID, ProjectID: shot.ProjectID, EpisodeID: shot.EpisodeID,
			ShotID: shot.ID, ShotRevision: shot.Revision, ShotContentHash: shot.ContentHash,
			CandidateSelectionID: selection.ID, CandidateSelectionRevision: selection.Revision,
			CandidateSelectionContentHash: selection.ContentHash,
			CandidateID:                   selection.CandidateID, CandidateRevision: selection.CandidateRevision,
			ArtifactID: selection.ArtifactID, ArtifactRevision: selection.ArtifactRevision,
			ArtifactSHA256: selection.ArtifactSHA256, Revision: revision, ContentHash: contentHash,
			CreatedBy: actor.UserID, CreatedAt: now,
		}
		if createErr := repo.CreateShotImageBinding(ctx, binding); createErr != nil {
			return createErr
		}
		encoded, encodeErr := platformcommand.Result(shotImageBindingReceipt{BindingID: binding.ID})
		if encodeErr != nil {
			return encodeErr
		}
		receipt := platformcommand.Receipt{
			ID: receiptID, WorkspaceID: binding.WorkspaceID, Operation: bindSelectedImageOperation,
			IdempotencyKey: command.IdempotencyKey, InputHash: inputHash, ResourceID: binding.ID,
			Result: encoded, CreatedBy: actor.UserID, CreatedAt: now,
		}
		if createErr := repo.CreateReceipt(ctx, receipt); createErr != nil {
			return createErr
		}
		result = BindSelectedImageResult{Binding: binding, Receipt: receipt}
		return nil
	})
	return result, normalizeShotImageBindingError(err)
}

func (service *ShotImageBindingService) RequireCurrentShotImage(
	ctx context.Context,
	actor Actor,
	shotID string,
) (domain.ShotImageBindingVersion, error) {
	actor.UserID, shotID = strings.TrimSpace(actor.UserID), strings.TrimSpace(shotID)
	if service == nil || service.transactions == nil || actor.TokenVersion < 1 ||
		!validBindingUUID(actor.UserID) || !validBindingUUID(shotID) {
		return domain.ShotImageBindingVersion{}, ErrNotFound
	}
	var binding domain.ShotImageBindingVersion
	err := service.transactions.WithinShotImageBindingTransaction(ctx, func(repo ShotImageBindingRepository) error {
		shot, loadErr := repo.LockShotImageTarget(ctx, actor, shotID, false)
		if loadErr != nil {
			return loadErr
		}
		if shot.Status != "active" {
			return ErrNotFound
		}
		binding, loadErr = repo.FindCurrentShotImageBinding(ctx, shot.ID)
		if loadErr != nil {
			return loadErr
		}
		if binding.WorkspaceID != shot.WorkspaceID || binding.ProjectID != shot.ProjectID ||
			binding.EpisodeID != shot.EpisodeID || binding.ShotID != shot.ID ||
			binding.ShotRevision != shot.Revision || binding.ShotContentHash != shot.ContentHash ||
			validateShotImageBinding(binding) != nil {
			return errors.New("Shot image binding has drifted")
		}
		return nil
	})
	return binding, normalizeShotImageBindingError(err)
}

func replayShotImageBinding(
	ctx context.Context,
	repo ShotImageBindingRepository,
	receipt platformcommand.Receipt,
	inputHash string,
	result *BindSelectedImageResult,
) error {
	replayed, err := platformcommand.Replay[shotImageBindingReceipt](receipt, inputHash)
	if err != nil {
		return err
	}
	binding, err := repo.GetShotImageBinding(ctx, replayed.BindingID)
	if err != nil {
		return err
	}
	if receipt.ResourceID != binding.ID || validateShotImageBinding(binding) != nil {
		return platformcommand.ErrInputMismatch
	}
	*result = BindSelectedImageResult{Binding: binding, Receipt: receipt}
	return nil
}

func shotImageBindingContentHash(
	shot domain.Shot,
	selection SelectedImageSnapshot,
	revision int,
) (string, error) {
	return platformcommand.InputHash(struct {
		SchemaVersion string
		Shot          struct {
			ID, ContentHash string
			Revision        int
		}
		Selection SelectedImageSnapshot
		Revision  int
	}{
		SchemaVersion: "shot-image-binding-v1",
		Shot: struct {
			ID, ContentHash string
			Revision        int
		}{shot.ID, shot.ContentHash, shot.Revision},
		Selection: selection, Revision: revision,
	})
}

func validateSelectedImage(value SelectedImageSnapshot, expectedID string) error {
	for _, identifier := range []string{value.ID, value.WorkspaceID, value.ProjectID, value.CandidateID, value.ArtifactID} {
		if !validBindingUUID(identifier) {
			return errors.New("Generation selected image returned an invalid identifier")
		}
	}
	if value.ID != expectedID || value.Revision != 1 || value.CandidateRevision < 1 || value.ArtifactRevision < 1 ||
		!validBindingHash(value.ContentHash) || !validBindingHash(value.ArtifactSHA256) {
		return errors.New("Generation selected image has drifted")
	}
	return nil
}

func validateShotImageBinding(value domain.ShotImageBindingVersion) error {
	identifiers := []string{
		value.ID, value.WorkspaceID, value.ProjectID, value.EpisodeID, value.ShotID,
		value.CandidateSelectionID, value.CandidateID, value.ArtifactID, value.CreatedBy,
	}
	for _, identifier := range identifiers {
		if !validBindingUUID(identifier) {
			return errors.New("invalid Shot image binding identifier")
		}
	}
	if value.ShotRevision < 1 || value.CandidateSelectionRevision != 1 || value.CandidateRevision < 1 ||
		value.ArtifactRevision < 1 || value.Revision < 1 || !validBindingHash(value.ShotContentHash) ||
		!validBindingHash(value.CandidateSelectionContentHash) || !validBindingHash(value.ArtifactSHA256) ||
		!validBindingHash(value.ContentHash) || value.CreatedAt.IsZero() {
		return errors.New("invalid Shot image binding snapshot")
	}
	expectedHash, err := shotImageBindingContentHash(domain.Shot{
		ID: value.ShotID, Revision: value.ShotRevision, ContentHash: value.ShotContentHash,
	}, SelectedImageSnapshot{
		ID: value.CandidateSelectionID, WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		Revision: value.CandidateSelectionRevision, ContentHash: value.CandidateSelectionContentHash,
		CandidateID: value.CandidateID, CandidateRevision: value.CandidateRevision,
		ArtifactID: value.ArtifactID, ArtifactRevision: value.ArtifactRevision,
		ArtifactSHA256: value.ArtifactSHA256,
	}, value.Revision)
	if err != nil || expectedHash != value.ContentHash {
		return errors.New("Shot image binding content hash has drifted")
	}
	return nil
}

func validBindingUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func validBindingHash(value string) bool { return bindingHashPattern.MatchString(value) }

func normalizeShotImageBindingError(err error) error {
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return conflict("Shot image binding idempotency input has changed")
	}
	return normalizeError(err)
}
