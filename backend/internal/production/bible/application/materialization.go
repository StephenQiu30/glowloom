package application

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

const materializeConfirmedOperation = "production_bible.materialize_confirmed"

type MaterializeCommand struct {
	BibleVersionID, ExpectedContentHash, IdempotencyKey string
	ExpectedVersion                                     int
}

type MaterializeResult struct {
	Materialization domain.Materialization
	Receipt         platformcommand.Receipt
}

type MaterializationScope struct {
	Version                 domain.ProductionBibleVersion
	AssetsByEntityKey       map[string]assetdomain.Asset
	SpecificationsByAssetID map[string][]domain.SpecificationVersion
	StatesByAssetID         map[string][]assetdomain.AssetState
	Bindings                []domain.ProductionBinding
}

type MaterializationWrite struct {
	NewAssets         []assetdomain.Asset
	NewSpecifications []domain.SpecificationVersion
	NewStates         []assetdomain.AssetState
	NewBindings       []domain.ProductionBinding
	Materialization   domain.Materialization
}

type materializationReceipt struct {
	Materialization domain.Materialization `json:"materialization"`
}

type materializedSpecificationSnapshot struct {
	CanonicalName  string                    `json:"canonical_name"`
	NormalizedName string                    `json:"normalized_name"`
	Aliases        []string                  `json:"aliases"`
	StableSpec     domain.AssetSpecCandidate `json:"stable_spec"`
	EpisodeNumbers []int                     `json:"episode_numbers"`
	Evidence       []domain.Evidence         `json:"evidence"`
	Ambiguities    []string                  `json:"ambiguities"`
}

type materializedStateSnapshot struct {
	StateSpec      domain.AssetSpecCandidate `json:"state_spec"`
	EpisodeNumbers []int                     `json:"episode_numbers"`
	Evidence       []domain.Evidence         `json:"evidence"`
	Ambiguities    []string                  `json:"ambiguities"`
}

func (service *Service) MaterializeConfirmedBible(
	ctx context.Context,
	actor Actor,
	command MaterializeCommand,
) (MaterializeResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.BibleVersionID = strings.TrimSpace(command.BibleVersionID)
	command.ExpectedContentHash = strings.ToLower(strings.TrimSpace(command.ExpectedContentHash))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.transactions == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.UserID == "" || actor.TokenVersion < 1 || command.ExpectedVersion < 1 ||
		len(command.ExpectedContentHash) != 64 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return MaterializeResult{}, invalid("Invalid Production Bible materialization")
	}
	if _, err := uuid.Parse(actor.UserID); err != nil {
		return MaterializeResult{}, invalid("Invalid Production Bible materialization")
	}
	if _, err := uuid.Parse(command.BibleVersionID); err != nil {
		return MaterializeResult{}, invalid("Invalid Production Bible materialization")
	}
	var result MaterializeResult
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		scope, err := repo.PrepareMaterialization(ctx, actor, command.BibleVersionID, true)
		if err != nil {
			return err
		}
		if scope.Version.ID != command.BibleVersionID || scope.Version.Version != command.ExpectedVersion ||
			scope.Version.ContentHash != command.ExpectedContentHash || scope.Version.CreatedBy == "" {
			return conflict("Production Bible Version changed before materialization")
		}
		inputHash, err := platformcommand.InputHash(command)
		if err != nil {
			return err
		}
		if receipt, findErr := repo.FindReceipt(
			ctx, scope.Version.WorkspaceID, materializeConfirmedOperation, command.IdempotencyKey,
		); findErr == nil {
			replayed, replayErr := platformcommand.Replay[materializationReceipt](receipt, inputHash)
			if errors.Is(replayErr, platformcommand.ErrInputMismatch) {
				return conflict("Idempotency key was already used with different input")
			}
			if replayErr != nil {
				return replayErr
			}
			if replayErr = repo.VerifyMaterialization(ctx, actor, replayed.Materialization); replayErr != nil {
				return replayErr
			}
			result = MaterializeResult{Materialization: replayed.Materialization, Receipt: receipt}
			return nil
		} else if !errors.Is(findErr, platformcommand.ErrReceiptNotFound) {
			return findErr
		}
		if len(scope.Bindings) != 0 {
			return conflict("Production Bible Version was already materialized with a different command")
		}
		write, err := service.buildMaterialization(scope, actor.UserID, service.config.Now().UTC())
		if err != nil {
			return err
		}
		if err = repo.CreateMaterialization(ctx, write); err != nil {
			return err
		}
		encoded, err := platformcommand.Result(materializationReceipt{Materialization: write.Materialization})
		if err != nil {
			return err
		}
		receipt := platformcommand.Receipt{
			ID: service.config.NewID(), WorkspaceID: scope.Version.WorkspaceID,
			Operation: materializeConfirmedOperation, IdempotencyKey: command.IdempotencyKey,
			InputHash: inputHash, ResourceID: scope.Version.ID, Result: encoded,
			CreatedBy: actor.UserID, CreatedAt: service.config.Now().UTC(),
		}
		if err = repo.CreateReceipt(ctx, receipt); err != nil {
			return err
		}
		result = MaterializeResult{Materialization: write.Materialization, Receipt: receipt}
		return nil
	})
	return result, normalizeError(err)
}

func (service *Service) buildMaterialization(
	scope MaterializationScope,
	createdBy string,
	createdAt time.Time,
) (MaterializationWrite, error) {
	var candidate domain.StoryReconciliationCandidate
	if err := json.Unmarshal(scope.Version.Snapshot, &candidate); err != nil ||
		domain.ValidateStoryReconciliationCandidate(candidate, domain.StoryReconciliationCandidateEvidence(candidate)) != nil {
		return MaterializationWrite{}, conflict("Production Bible Version snapshot is invalid")
	}
	entities := make([]domain.StoryEntityCandidate, 0, len(candidate.CanonicalEntities))
	for _, entity := range candidate.CanonicalEntities {
		if slices.Contains([]string{
			assetdomain.AssetKindCharacter, assetdomain.AssetKindLocation, assetdomain.AssetKindProp,
		}, entity.Kind) {
			entities = append(entities, entity)
		}
	}
	slices.SortFunc(entities, func(left, right domain.StoryEntityCandidate) int {
		return strings.Compare(left.EntityKey, right.EntityKey)
	})
	if len(entities) == 0 {
		return MaterializationWrite{}, conflict("Production Bible Version has no materializable identity")
	}
	write := MaterializationWrite{
		NewAssets: []assetdomain.Asset{}, NewSpecifications: []domain.SpecificationVersion{},
		NewStates: []assetdomain.AssetState{}, NewBindings: []domain.ProductionBinding{},
	}
	allAssets := make([]domain.MaterializedAsset, 0, len(entities))
	allSpecifications := make([]domain.MaterializedSpecification, 0, len(entities))
	allStates := make([]domain.MaterializedState, 0, len(entities))
	allBindings := make([]domain.MaterializedBinding, 0, len(entities))
	for _, entity := range entities {
		asset, exists := scope.AssetsByEntityKey[entity.EntityKey]
		if exists {
			if assetdomain.ValidateAsset(asset) != nil || asset.WorkspaceID != scope.Version.WorkspaceID ||
				asset.ProjectID != scope.Version.ProjectID || asset.Kind != entity.Kind {
				return MaterializationWrite{}, conflict("Production Bible Asset identity has drifted")
			}
		} else {
			var err error
			asset, err = assetdomain.NewAsset(assetdomain.AssetInput{
				ID: service.config.NewID(), WorkspaceID: scope.Version.WorkspaceID, ProjectID: scope.Version.ProjectID,
				Kind: entity.Kind, IdentityKey: entity.EntityKey, CreatedBy: createdBy, CreatedAt: createdAt,
			})
			if err != nil {
				return MaterializationWrite{}, err
			}
			write.NewAssets = append(write.NewAssets, asset)
		}
		specification, isNew, err := service.materializeSpecification(scope, entity, asset, createdBy, createdAt)
		if err != nil {
			return MaterializationWrite{}, err
		}
		if isNew {
			write.NewSpecifications = append(write.NewSpecifications, specification)
		}
		states, newStates, err := service.materializeStates(scope, entity, asset, createdBy, createdAt)
		if err != nil {
			return MaterializationWrite{}, err
		}
		write.NewStates = append(write.NewStates, newStates...)
		stateRefs := make([]domain.MaterializedState, 0, len(states))
		for _, state := range states {
			stateRefs = append(stateRefs, domain.MaterializedStateRef(state))
		}
		binding, err := domain.NewProductionBinding(domain.ProductionBindingInput{
			ID: service.config.NewID(), WorkspaceID: scope.Version.WorkspaceID, ProjectID: scope.Version.ProjectID,
			BibleVersionID: scope.Version.ID, BibleVersionHash: scope.Version.ContentHash, EntityKey: entity.EntityKey,
			Asset: domain.MaterializedAssetRef(asset), Specification: domain.MaterializedSpecificationRef(specification),
			States: stateRefs, CreatedBy: createdBy, CreatedAt: createdAt,
		})
		if err != nil {
			return MaterializationWrite{}, err
		}
		write.NewBindings = append(write.NewBindings, binding)
		allAssets = append(allAssets, binding.Asset)
		allSpecifications = append(allSpecifications, binding.Specification)
		allStates = append(allStates, binding.States...)
		allBindings = append(allBindings, domain.MaterializedBindingRef(binding))
	}
	materialization, err := domain.NewMaterialization(
		scope.Version.ID, scope.Version.ContentHash, allAssets, allSpecifications, allStates, allBindings,
	)
	if err != nil {
		return MaterializationWrite{}, err
	}
	write.Materialization = materialization
	return write, nil
}

func (service *Service) materializeSpecification(
	scope MaterializationScope,
	entity domain.StoryEntityCandidate,
	asset assetdomain.Asset,
	createdBy string,
	createdAt time.Time,
) (domain.SpecificationVersion, bool, error) {
	snapshot, err := json.Marshal(materializedSpecificationSnapshot{
		CanonicalName: entity.CanonicalName, NormalizedName: entity.NormalizedName,
		Aliases: entity.Aliases, StableSpec: entity.StableSpec,
		EpisodeNumbers: entity.EpisodeNumbers, Evidence: entity.Evidence, Ambiguities: entity.Ambiguities,
	})
	if err != nil {
		return domain.SpecificationVersion{}, false, err
	}
	existing := scope.SpecificationsByAssetID[asset.ID]
	nextVersion := 1
	for _, value := range existing {
		if domain.ValidateSpecificationVersion(value) != nil || value.AssetID != asset.ID || value.Kind != asset.Kind ||
			value.EntityKey != entity.EntityKey || value.ProjectID != scope.Version.ProjectID {
			return domain.SpecificationVersion{}, false, conflict("Production Bible SpecificationVersion has drifted")
		}
		if value.Version >= nextVersion {
			nextVersion = value.Version + 1
		}
	}
	candidate, err := domain.NewSpecificationVersion(domain.SpecificationVersionInput{
		ID: service.config.NewID(), WorkspaceID: scope.Version.WorkspaceID, ProjectID: scope.Version.ProjectID,
		AssetID: asset.ID, Kind: asset.Kind, EntityKey: entity.EntityKey, Version: nextVersion,
		SourceBibleVersionID: scope.Version.ID, Snapshot: snapshot, CreatedBy: createdBy, CreatedAt: createdAt,
	})
	if err != nil {
		return domain.SpecificationVersion{}, false, err
	}
	for _, value := range existing {
		if value.ContentHash == candidate.ContentHash {
			return value, false, nil
		}
	}
	return candidate, true, nil
}

func (service *Service) materializeStates(
	scope MaterializationScope,
	entity domain.StoryEntityCandidate,
	asset assetdomain.Asset,
	createdBy string,
	createdAt time.Time,
) ([]assetdomain.AssetState, []assetdomain.AssetState, error) {
	states := append([]domain.StoryEntityStateCandidate(nil), entity.States...)
	hasBase := false
	for _, state := range states {
		hasBase = hasBase || state.StateKey == "base"
	}
	if !hasBase {
		states = append(states, domain.StoryEntityStateCandidate{
			StateKey: "base", Label: "基础状态", StateSpec: entity.StableSpec,
			EpisodeNumbers: append([]int(nil), entity.EpisodeNumbers...), Evidence: append([]domain.Evidence(nil), entity.Evidence...),
			Ambiguities: append([]string(nil), entity.Ambiguities...),
		})
	}
	slices.SortFunc(states, func(left, right domain.StoryEntityStateCandidate) int {
		return strings.Compare(left.StateKey, right.StateKey)
	})
	existing := scope.StatesByAssetID[asset.ID]
	result, created := make([]assetdomain.AssetState, 0, len(states)), []assetdomain.AssetState{}
	for _, state := range states {
		snapshot, err := json.Marshal(materializedStateSnapshot{
			StateSpec: state.StateSpec, EpisodeNumbers: state.EpisodeNumbers,
			Evidence: state.Evidence, Ambiguities: state.Ambiguities,
		})
		if err != nil {
			return nil, nil, err
		}
		nextRevision := 1
		for _, value := range existing {
			if assetdomain.ValidateAssetState(value) != nil || value.AssetID != asset.ID ||
				value.ProjectID != scope.Version.ProjectID {
				return nil, nil, conflict("Production Bible AssetState has drifted")
			}
			if value.StateKey == state.StateKey && value.Revision >= nextRevision {
				nextRevision = value.Revision + 1
			}
		}
		candidate, err := assetdomain.NewAssetState(assetdomain.AssetStateInput{
			ID: service.config.NewID(), WorkspaceID: scope.Version.WorkspaceID, ProjectID: scope.Version.ProjectID,
			AssetID: asset.ID, StateKey: state.StateKey, Label: state.Label, Revision: nextRevision,
			Snapshot: snapshot, CreatedBy: createdBy, CreatedAt: createdAt,
		})
		if err != nil {
			return nil, nil, err
		}
		reused := false
		for _, value := range existing {
			if value.StateKey == state.StateKey && value.ContentHash == candidate.ContentHash {
				result, reused = append(result, value), true
				break
			}
		}
		if !reused {
			result, created = append(result, candidate), append(created, candidate)
		}
	}
	return result, created, nil
}
