package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
)

var (
	ErrIdentityStateHeadNotFound         = errors.New("Asset identity-state Head not found")
	ErrProductionWorldAssetNotFound      = errors.New("Production World Asset not found")
	ErrProductionWorldAssetStateNotFound = errors.New("Production World AssetState not found")
	ErrIdentityStateHeadConflict         = errors.New("Asset identity-state Head conflict")
)

type ProductionWorldAssetStateInput struct {
	StateKey string
	Snapshot json.RawMessage
}

type ProductionWorldAssetIdentityInput struct {
	IdentityKey string
	Kind        string
	States      []ProductionWorldAssetStateInput
}

type ApplyProductionWorldAssetsCommand struct {
	WorkspaceID, ProjectID, ActorID string
	ExpectedHeadRevision            int64
	ExpectedHeadHash                string
	ExpectedBusinessKeyRoot         string
	Identities                      []ProductionWorldAssetIdentityInput
}

type ApplyProductionWorldAssetsResult struct {
	Assets []domain.Asset
	States []domain.AssetState
	Head   domain.IdentityStateCollectionHead
}

type ProductionWorldAssetRepository interface {
	GetIdentityStateHead(context.Context, string, string, bool) (domain.IdentityStateCollectionHead, error)
	GetAssetByIdentityKey(context.Context, string, string, bool) (domain.Asset, error)
	CreateProductionWorldAsset(context.Context, domain.Asset) error
	GetLatestAssetState(context.Context, string, string, bool) (domain.AssetState, error)
	CreateProductionWorldAssetState(context.Context, domain.AssetState) error
	CreateIdentityStateMemberships(context.Context, domain.IdentityStateCollectionHead) error
	SaveIdentityStateHead(context.Context, domain.IdentityStateCollectionHead, int64, string) error
}

type ProductionWorldAssetOwner struct {
	now   func() time.Time
	newID func() string
}

func NewProductionWorldAssetOwner(now func() time.Time, newID func() string) *ProductionWorldAssetOwner {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = uuid.NewString
	}
	return &ProductionWorldAssetOwner{now: now, newID: newID}
}

func (owner *ProductionWorldAssetOwner) ApplyProductionWorldAssets(
	ctx context.Context,
	repository ProductionWorldAssetRepository,
	command ApplyProductionWorldAssetsCommand,
) (ApplyProductionWorldAssetsResult, error) {
	if owner == nil || repository == nil {
		return ApplyProductionWorldAssetsResult{}, errors.New("Production World Asset owner is unavailable")
	}
	if err := validateProductionWorldAssetCommand(command); err != nil {
		return ApplyProductionWorldAssetsResult{}, err
	}
	head, headErr := repository.GetIdentityStateHead(ctx, command.WorkspaceID, command.ProjectID, true)
	if command.ExpectedHeadRevision == 0 {
		if headErr == nil || !errors.Is(headErr, ErrIdentityStateHeadNotFound) {
			return ApplyProductionWorldAssetsResult{}, ErrIdentityStateHeadConflict
		}
	} else if headErr != nil || head.HeadRevision != command.ExpectedHeadRevision ||
		head.HeadContentHash != command.ExpectedHeadHash {
		return ApplyProductionWorldAssetsResult{}, ErrIdentityStateHeadConflict
	}

	now := owner.now().UTC()
	assets := make([]domain.Asset, 0, len(command.Identities))
	states := make([]domain.AssetState, 0)
	for _, identity := range command.Identities {
		asset, err := owner.ensureProductionWorldAsset(ctx, repository, command, identity, now)
		if err != nil {
			return ApplyProductionWorldAssetsResult{}, err
		}
		assets = append(assets, asset)
		for _, stateInput := range identity.States {
			state, stateErr := owner.ensureProductionWorldAssetState(ctx, repository, command, asset, stateInput, now)
			if stateErr != nil {
				return ApplyProductionWorldAssetsResult{}, stateErr
			}
			states = append(states, state)
		}
	}
	if headErr == nil {
		preview, previewErr := domain.NewIdentityStateCollectionHead(
			command.WorkspaceID, command.ProjectID, head.ScopeRevision, assets, states, head.UpdatedAt,
		)
		if previewErr != nil {
			return ApplyProductionWorldAssetsResult{}, previewErr
		}
		if preview.CollectionRootHash == head.CollectionRootHash && reflect.DeepEqual(preview.Members, head.Members) {
			return ApplyProductionWorldAssetsResult{Assets: assets, States: states, Head: head}, nil
		}
	}
	nextHead, err := domain.NewIdentityStateCollectionHead(
		command.WorkspaceID, command.ProjectID, command.ExpectedHeadRevision+1, assets, states, now,
	)
	if err != nil {
		return ApplyProductionWorldAssetsResult{}, err
	}
	if err = repository.CreateIdentityStateMemberships(ctx, nextHead); err != nil {
		return ApplyProductionWorldAssetsResult{}, err
	}
	if err = repository.SaveIdentityStateHead(ctx, nextHead, command.ExpectedHeadRevision, command.ExpectedHeadHash); err != nil {
		return ApplyProductionWorldAssetsResult{}, err
	}
	return ApplyProductionWorldAssetsResult{Assets: assets, States: states, Head: nextHead}, nil
}

func (owner *ProductionWorldAssetOwner) ensureProductionWorldAsset(
	ctx context.Context,
	repository ProductionWorldAssetRepository,
	command ApplyProductionWorldAssetsCommand,
	input ProductionWorldAssetIdentityInput,
	now time.Time,
) (domain.Asset, error) {
	asset, err := repository.GetAssetByIdentityKey(ctx, command.ProjectID, input.IdentityKey, true)
	if err == nil {
		if asset.WorkspaceID != command.WorkspaceID || asset.ProjectID != command.ProjectID || asset.Kind != input.Kind ||
			domain.ValidateAsset(asset) != nil {
			return domain.Asset{}, errors.New("Production World Asset identity has drifted")
		}
		return asset, nil
	}
	if !errors.Is(err, ErrProductionWorldAssetNotFound) {
		return domain.Asset{}, err
	}
	asset, err = domain.NewAsset(domain.AssetInput{
		ID: owner.newID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		Kind: input.Kind, IdentityKey: input.IdentityKey, CreatedBy: command.ActorID, CreatedAt: now,
	})
	if err != nil {
		return domain.Asset{}, err
	}
	if err = repository.CreateProductionWorldAsset(ctx, asset); err != nil {
		return domain.Asset{}, err
	}
	return asset, nil
}

func (owner *ProductionWorldAssetOwner) ensureProductionWorldAssetState(
	ctx context.Context,
	repository ProductionWorldAssetRepository,
	command ApplyProductionWorldAssetsCommand,
	asset domain.Asset,
	input ProductionWorldAssetStateInput,
	now time.Time,
) (domain.AssetState, error) {
	latest, err := repository.GetLatestAssetState(ctx, asset.ID, input.StateKey, true)
	revision := 1
	if err == nil {
		revision = latest.Revision + 1
	} else if !errors.Is(err, ErrProductionWorldAssetStateNotFound) {
		return domain.AssetState{}, err
	}
	desired, err := domain.NewAssetState(domain.AssetStateInput{
		ID: owner.newID(), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID, AssetID: asset.ID,
		StateKey: input.StateKey, Label: input.StateKey, Revision: revision, Snapshot: input.Snapshot,
		CreatedBy: command.ActorID, CreatedAt: now,
	})
	if err != nil {
		return domain.AssetState{}, err
	}
	if latest.ID != "" && latest.ContentHash == desired.ContentHash {
		return latest, nil
	}
	if err = repository.CreateProductionWorldAssetState(ctx, desired); err != nil {
		return domain.AssetState{}, err
	}
	return desired, nil
}

func validateProductionWorldAssetCommand(command ApplyProductionWorldAssetsCommand) error {
	for _, identifier := range []string{command.WorkspaceID, command.ProjectID, command.ActorID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Production World Asset command identity")
		}
	}
	if command.ExpectedHeadRevision < 0 ||
		(command.ExpectedHeadRevision == 0 && command.ExpectedHeadHash != "") ||
		(command.ExpectedHeadRevision > 0 && !domain.ValidContentHash(command.ExpectedHeadHash)) ||
		!domain.ValidContentHash(command.ExpectedBusinessKeyRoot) || len(command.Identities) == 0 {
		return errors.New("invalid Production World Asset command")
	}
	keys := make([]string, 0)
	previousIdentity := ""
	for _, identity := range command.Identities {
		if identity.IdentityKey <= previousIdentity || len(identity.States) == 0 {
			return errors.New("Production World Asset identities are not canonical")
		}
		keys = append(keys, "asset_identity:"+identity.IdentityKey)
		previousState := ""
		for _, state := range identity.States {
			if state.StateKey <= previousState || len(state.Snapshot) == 0 {
				return errors.New("Production World Asset states are not canonical")
			}
			keys = append(keys, "asset_state:"+state.StateKey)
			previousState = state.StateKey
		}
		previousIdentity = identity.IdentityKey
	}
	slices.Sort(keys)
	encoded, _ := json.Marshal(keys)
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != command.ExpectedBusinessKeyRoot {
		return errors.New("Production World Asset business key coverage has drifted")
	}
	return nil
}
