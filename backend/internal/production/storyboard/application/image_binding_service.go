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

type ShotImageBindingTarget struct {
	Shot                      domain.Shot
	ExpectedCurrentRevision   int
	CurrentBindingID          string
	CurrentBindingContentHash string
	ContentHash               string
}

type BindSelectedImageAtTargetCommand struct {
	ShotID, CandidateSelectionID, IdempotencyKey       string
	ExpectedShotContentHash, ExpectedBindingTargetHash string
	ExpectedShotRevision, ExpectedCurrentRevision      int
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

func (service *ShotImageBindingService) RequireActiveShot(
	ctx context.Context,
	actor Actor,
	shotID string,
) (domain.Shot, error) {
	actor.UserID, shotID = strings.TrimSpace(actor.UserID), strings.TrimSpace(shotID)
	if service == nil || service.transactions == nil || actor.TokenVersion < 1 ||
		!validBindingUUID(actor.UserID) || !validBindingUUID(shotID) {
		return domain.Shot{}, ErrNotFound
	}
	var shot domain.Shot
	err := service.transactions.WithinShotImageBindingTransaction(ctx, func(repo ShotImageBindingRepository) error {
		var loadErr error
		shot, loadErr = repo.LockShotImageTarget(ctx, actor, shotID, false)
		if loadErr != nil {
			return loadErr
		}
		for _, identifier := range []string{
			shot.ID, shot.WorkspaceID, shot.ProjectID, shot.EpisodeID, shot.BatchID, shot.CreatedBy,
		} {
			if !validBindingUUID(identifier) {
				return errors.New("active Shot contains an invalid identifier")
			}
		}
		if shot.ID != shotID || shot.Status != "active" || shot.Position < 1 || shot.Revision < 1 ||
			strings.TrimSpace(shot.ProposalKey) == "" || strings.TrimSpace(shot.Title) == "" ||
			!validBindingHash(shot.ContentHash) || shot.CreatedAt.IsZero() {
			return errors.New("active Shot snapshot has drifted")
		}
		return nil
	})
	return shot, normalizeError(err)
}

func (service *ShotImageBindingService) RequireShotImageBindingTarget(
	ctx context.Context,
	actor Actor,
	shotID string,
) (ShotImageBindingTarget, error) {
	actor.UserID, shotID = strings.TrimSpace(actor.UserID), strings.TrimSpace(shotID)
	if service == nil || service.transactions == nil || actor.TokenVersion < 1 ||
		!validBindingUUID(actor.UserID) || !validBindingUUID(shotID) {
		return ShotImageBindingTarget{}, ErrNotFound
	}
	var target ShotImageBindingTarget
	err := service.transactions.WithinShotImageBindingTransaction(ctx, func(repo ShotImageBindingRepository) error {
		shot, loadErr := repo.LockShotImageTarget(ctx, actor, shotID, true)
		if loadErr != nil {
			return loadErr
		}
		if err := validateActiveShotImageTarget(shot, shotID); err != nil {
			return err
		}
		current, currentErr := repo.FindCurrentShotImageBinding(ctx, shot.ID)
		switch {
		case errors.Is(currentErr, ErrNotFound):
			target, loadErr = buildShotImageBindingTarget(shot, domain.ShotImageBindingVersion{})
		case currentErr != nil:
			return currentErr
		default:
			if current.WorkspaceID != shot.WorkspaceID || current.ProjectID != shot.ProjectID ||
				current.EpisodeID != shot.EpisodeID || current.ShotID != shot.ID ||
				current.ShotRevision != shot.Revision || current.ShotContentHash != shot.ContentHash ||
				validateShotImageBinding(current) != nil {
				return errors.New("current Shot image binding has drifted")
			}
			target, loadErr = buildShotImageBindingTarget(shot, current)
		}
		return loadErr
	})
	return target, normalizeShotImageBindingError(err)
}

func (service *ShotImageBindingService) BindSelectedImage(
	ctx context.Context,
	actor Actor,
	command BindSelectedImageCommand,
) (BindSelectedImageResult, error) {
	return service.bindSelectedImage(ctx, actor, command, "")
}

func (service *ShotImageBindingService) BindSelectedImageAtTarget(
	ctx context.Context,
	actor Actor,
	command BindSelectedImageAtTargetCommand,
) (BindSelectedImageResult, error) {
	command.ExpectedBindingTargetHash = strings.TrimSpace(command.ExpectedBindingTargetHash)
	if !validBindingHash(command.ExpectedBindingTargetHash) {
		return BindSelectedImageResult{}, invalid("Invalid Shot image binding target request")
	}
	return service.bindSelectedImage(ctx, actor, BindSelectedImageCommand{
		ShotID: command.ShotID, CandidateSelectionID: command.CandidateSelectionID,
		ExpectedShotContentHash: command.ExpectedShotContentHash,
		ExpectedShotRevision:    command.ExpectedShotRevision,
		ExpectedCurrentRevision: command.ExpectedCurrentRevision,
		IdempotencyKey:          command.IdempotencyKey,
	}, command.ExpectedBindingTargetHash)
}

func (service *ShotImageBindingService) bindSelectedImage(
	ctx context.Context,
	actor Actor,
	command BindSelectedImageCommand,
	expectedBindingTargetHash string,
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
		inputHash, hashErr := shotImageBindingInputHash(
			actor.UserID, shot, selection, command.ExpectedCurrentRevision, expectedBindingTargetHash,
		)
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
		if currentErr == nil && (current.WorkspaceID != shot.WorkspaceID || current.ProjectID != shot.ProjectID ||
			current.EpisodeID != shot.EpisodeID || current.ShotID != shot.ID ||
			current.ShotRevision != shot.Revision || current.ShotContentHash != shot.ContentHash ||
			validateShotImageBinding(current) != nil) {
			return errors.New("current Shot image binding has drifted")
		}
		if expectedBindingTargetHash != "" {
			target, targetErr := buildShotImageBindingTarget(shot, current)
			if targetErr != nil {
				return targetErr
			}
			if target.ExpectedCurrentRevision != command.ExpectedCurrentRevision ||
				target.ContentHash != expectedBindingTargetHash {
				return conflict("Shot image binding target has changed")
			}
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

func shotImageBindingInputHash(
	actorID string,
	shot domain.Shot,
	selection SelectedImageSnapshot,
	expectedCurrentRevision int,
	expectedBindingTargetHash string,
) (string, error) {
	if expectedBindingTargetHash == "" {
		return platformcommand.InputHash(struct {
			ActorID                 string
			ShotID                  string
			ShotRevision            int
			ShotContentHash         string
			Selection               SelectedImageSnapshot
			ExpectedCurrentRevision int
		}{actorID, shot.ID, shot.Revision, shot.ContentHash, selection, expectedCurrentRevision})
	}
	return platformcommand.InputHash(struct {
		SchemaVersion             string
		ActorID                   string
		ShotID                    string
		ShotRevision              int
		ShotContentHash           string
		Selection                 SelectedImageSnapshot
		ExpectedCurrentRevision   int
		ExpectedBindingTargetHash string
	}{
		"shot-image-binding-command-v2", actorID, shot.ID, shot.Revision, shot.ContentHash, selection,
		expectedCurrentRevision, expectedBindingTargetHash,
	})
}

func buildShotImageBindingTarget(
	shot domain.Shot,
	current domain.ShotImageBindingVersion,
) (ShotImageBindingTarget, error) {
	target := ShotImageBindingTarget{Shot: shot}
	if current.ID != "" {
		target.ExpectedCurrentRevision = current.Revision
		target.CurrentBindingID = current.ID
		target.CurrentBindingContentHash = current.ContentHash
	}
	contentHash, err := platformcommand.InputHash(struct {
		SchemaVersion string
		Shot          struct {
			ID, ContentHash string
			Revision        int
		}
		CurrentBinding struct {
			ID, ContentHash string
			Revision        int
		}
	}{
		SchemaVersion: "shot-image-binding-target-v1",
		Shot: struct {
			ID, ContentHash string
			Revision        int
		}{shot.ID, shot.ContentHash, shot.Revision},
		CurrentBinding: struct {
			ID, ContentHash string
			Revision        int
		}{target.CurrentBindingID, target.CurrentBindingContentHash, target.ExpectedCurrentRevision},
	})
	if err != nil {
		return ShotImageBindingTarget{}, err
	}
	target.ContentHash = contentHash
	return target, nil
}

func validateActiveShotImageTarget(shot domain.Shot, expectedID string) error {
	for _, identifier := range []string{
		shot.ID, shot.WorkspaceID, shot.ProjectID, shot.EpisodeID, shot.BatchID, shot.CreatedBy,
	} {
		if !validBindingUUID(identifier) {
			return errors.New("active Shot contains an invalid identifier")
		}
	}
	if shot.ID != expectedID || shot.Status != "active" || shot.Position < 1 || shot.Revision < 1 ||
		strings.TrimSpace(shot.ProposalKey) == "" || strings.TrimSpace(shot.Title) == "" ||
		!validBindingHash(shot.ContentHash) || shot.CreatedAt.IsZero() {
		return errors.New("active Shot snapshot has drifted")
	}
	return nil
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
