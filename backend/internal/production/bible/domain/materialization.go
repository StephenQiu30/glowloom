package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	assetdomain "github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
)

type SpecificationVersionInput struct {
	ID, WorkspaceID, ProjectID, AssetID string
	Kind, EntityKey                     string
	Version                             int
	SourceBibleVersionID                string
	Snapshot                            json.RawMessage
	CreatedBy                           string
	CreatedAt                           time.Time
}

type SpecificationVersion struct {
	ID                   string          `json:"id"`
	WorkspaceID          string          `json:"workspace_id"`
	ProjectID            string          `json:"project_id"`
	AssetID              string          `json:"asset_id"`
	Kind                 string          `json:"kind"`
	EntityKey            string          `json:"entity_key"`
	Version              int             `json:"version"`
	SourceBibleVersionID string          `json:"source_bible_version_id"`
	Snapshot             json.RawMessage `json:"snapshot"`
	ContentHash          string          `json:"content_hash"`
	CreatedBy            string          `json:"created_by"`
	CreatedAt            time.Time       `json:"created_at"`
}

func NewSpecificationVersion(input SpecificationVersionInput) (SpecificationVersion, error) {
	for _, value := range []string{
		input.ID, input.WorkspaceID, input.ProjectID, input.AssetID, input.SourceBibleVersionID, input.CreatedBy,
	} {
		if _, err := uuid.Parse(value); err != nil {
			return SpecificationVersion{}, errors.New("invalid SpecificationVersion identity")
		}
	}
	if !oneOf(input.Kind, assetdomain.AssetKindCharacter, assetdomain.AssetKindLocation, assetdomain.AssetKindProp) ||
		!keyPattern.MatchString(input.EntityKey) || input.Version < 1 || input.CreatedAt.IsZero() {
		return SpecificationVersion{}, errors.New("invalid SpecificationVersion input")
	}
	canonical, err := canonicalMaterializationJSON(input.Snapshot)
	if err != nil {
		return SpecificationVersion{}, errors.New("invalid SpecificationVersion snapshot")
	}
	value := SpecificationVersion{
		ID: input.ID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, AssetID: input.AssetID,
		Kind: input.Kind, EntityKey: input.EntityKey, Version: input.Version,
		SourceBibleVersionID: input.SourceBibleVersionID, Snapshot: canonical,
		CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt.UTC(),
	}
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, AssetID, Kind, EntityKey string
		Snapshot                         json.RawMessage
	}{"production-bible-specification", value.AssetID, value.Kind, value.EntityKey, value.Snapshot})
	if err != nil {
		return SpecificationVersion{}, err
	}
	return value, nil
}

type MaterializedAsset struct {
	ID          string `json:"id"`
	IdentityKey string `json:"identity_key"`
	Kind        string `json:"kind"`
	Revision    int    `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type MaterializedSpecification struct {
	ID          string `json:"id"`
	AssetID     string `json:"asset_id"`
	Kind        string `json:"kind"`
	EntityKey   string `json:"entity_key"`
	Version     int    `json:"version"`
	ContentHash string `json:"content_hash"`
}

type MaterializedState struct {
	ID          string `json:"id"`
	AssetID     string `json:"asset_id"`
	StateKey    string `json:"state_key"`
	Revision    int    `json:"revision"`
	ContentHash string `json:"content_hash"`
}

type MaterializedBinding struct {
	ID                     string `json:"id"`
	EntityKey              string `json:"entity_key"`
	AssetID                string `json:"asset_id"`
	SpecificationVersionID string `json:"specification_version_id"`
	Revision               int    `json:"revision"`
	ContentHash            string `json:"content_hash"`
}

type ProductionBindingInput struct {
	ID, WorkspaceID, ProjectID       string
	BibleVersionID, BibleVersionHash string
	EntityKey                        string
	Asset                            MaterializedAsset
	Specification                    MaterializedSpecification
	States                           []MaterializedState
	CreatedBy                        string
	CreatedAt                        time.Time
}

type ProductionBinding struct {
	ID               string                    `json:"id"`
	WorkspaceID      string                    `json:"workspace_id"`
	ProjectID        string                    `json:"project_id"`
	BibleVersionID   string                    `json:"bible_version_id"`
	BibleVersionHash string                    `json:"bible_version_hash"`
	EntityKey        string                    `json:"entity_key"`
	Asset            MaterializedAsset         `json:"asset"`
	Specification    MaterializedSpecification `json:"specification"`
	States           []MaterializedState       `json:"states"`
	Revision         int                       `json:"revision"`
	ContentHash      string                    `json:"content_hash"`
	CreatedBy        string                    `json:"created_by"`
	CreatedAt        time.Time                 `json:"created_at"`
}

func NewProductionBinding(input ProductionBindingInput) (ProductionBinding, error) {
	for _, value := range []string{input.ID, input.WorkspaceID, input.ProjectID, input.BibleVersionID, input.CreatedBy} {
		if _, err := uuid.Parse(value); err != nil {
			return ProductionBinding{}, errors.New("invalid ProductionBinding identity")
		}
	}
	if !hashPattern.MatchString(input.BibleVersionHash) || !keyPattern.MatchString(input.EntityKey) ||
		input.CreatedAt.IsZero() || input.Asset.IdentityKey != input.EntityKey || input.Asset.ID == "" ||
		input.Specification.AssetID != input.Asset.ID || input.Specification.EntityKey != input.EntityKey ||
		input.Specification.Kind != input.Asset.Kind || len(input.States) == 0 {
		return ProductionBinding{}, errors.New("invalid ProductionBinding input")
	}
	states := append([]MaterializedState(nil), input.States...)
	slices.SortFunc(states, func(left, right MaterializedState) int {
		if compared := strings.Compare(left.StateKey, right.StateKey); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	seenStates := map[string]struct{}{}
	for _, state := range states {
		if state.AssetID != input.Asset.ID || !statePattern.MatchString(state.StateKey) || state.Revision < 1 ||
			!hashPattern.MatchString(state.ContentHash) {
			return ProductionBinding{}, errors.New("invalid ProductionBinding state")
		}
		if _, exists := seenStates[state.StateKey]; exists {
			return ProductionBinding{}, errors.New("ProductionBinding states must be unique")
		}
		seenStates[state.StateKey] = struct{}{}
	}
	value := ProductionBinding{
		ID: input.ID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		BibleVersionID: input.BibleVersionID, BibleVersionHash: input.BibleVersionHash,
		EntityKey: input.EntityKey, Asset: input.Asset, Specification: input.Specification,
		States: states, Revision: 1, CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt.UTC(),
	}
	var err error
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, BibleVersionID, BibleVersionHash, EntityKey string
		Asset                                               MaterializedAsset
		Specification                                       MaterializedSpecification
		States                                              []MaterializedState
	}{
		"production-binding", value.BibleVersionID, value.BibleVersionHash, value.EntityKey,
		value.Asset, value.Specification, value.States,
	})
	if err != nil {
		return ProductionBinding{}, err
	}
	return value, nil
}

type Materialization struct {
	BibleVersionID   string                      `json:"bible_version_id"`
	BibleVersionHash string                      `json:"bible_version_hash"`
	Assets           []MaterializedAsset         `json:"assets"`
	Specifications   []MaterializedSpecification `json:"specifications"`
	States           []MaterializedState         `json:"states"`
	Bindings         []MaterializedBinding       `json:"bindings"`
	ContentHash      string                      `json:"content_hash"`
}

func NewMaterialization(
	bibleVersionID string,
	bibleVersionHash string,
	assets []MaterializedAsset,
	specifications []MaterializedSpecification,
	states []MaterializedState,
	bindings []MaterializedBinding,
) (Materialization, error) {
	if _, err := uuid.Parse(bibleVersionID); err != nil || !hashPattern.MatchString(bibleVersionHash) || len(bindings) == 0 {
		return Materialization{}, errors.New("invalid Production Bible materialization input")
	}
	assets = append([]MaterializedAsset(nil), assets...)
	specifications = append([]MaterializedSpecification(nil), specifications...)
	states = append([]MaterializedState(nil), states...)
	bindings = append([]MaterializedBinding(nil), bindings...)
	slices.SortFunc(assets, func(left, right MaterializedAsset) int { return strings.Compare(left.IdentityKey, right.IdentityKey) })
	slices.SortFunc(specifications, func(left, right MaterializedSpecification) int {
		return strings.Compare(left.EntityKey, right.EntityKey)
	})
	slices.SortFunc(states, func(left, right MaterializedState) int {
		if compared := strings.Compare(left.AssetID, right.AssetID); compared != 0 {
			return compared
		}
		return strings.Compare(left.StateKey, right.StateKey)
	})
	slices.SortFunc(bindings, func(left, right MaterializedBinding) int { return strings.Compare(left.EntityKey, right.EntityKey) })
	if len(assets) != len(bindings) || len(specifications) != len(bindings) {
		return Materialization{}, errors.New("Production Bible materialization is incomplete")
	}
	assetsByID := map[string]MaterializedAsset{}
	assetIDsByKey := map[string]string{}
	specificationsByID := map[string]MaterializedSpecification{}
	bindingIDs, bindingKeys := map[string]struct{}{}, map[string]struct{}{}
	stateKeys := map[string]struct{}{}
	stateCountByAsset := map[string]int{}
	for _, asset := range assets {
		if _, err := uuid.Parse(asset.ID); err != nil || !keyPattern.MatchString(asset.IdentityKey) || asset.Revision != 1 ||
			!oneOf(asset.Kind, assetdomain.AssetKindCharacter, assetdomain.AssetKindLocation, assetdomain.AssetKindProp) ||
			!hashPattern.MatchString(asset.ContentHash) {
			return Materialization{}, errors.New("Production Bible materialization contains an invalid Asset")
		}
		if _, exists := assetsByID[asset.ID]; exists || assetIDsByKey[asset.IdentityKey] != "" {
			return Materialization{}, errors.New("Production Bible materialization contains a duplicate Asset")
		}
		assetsByID[asset.ID] = asset
		assetIDsByKey[asset.IdentityKey] = asset.ID
	}
	for _, specification := range specifications {
		if _, err := uuid.Parse(specification.ID); err != nil || specification.Version < 1 ||
			!keyPattern.MatchString(specification.EntityKey) ||
			!hashPattern.MatchString(specification.ContentHash) {
			return Materialization{}, errors.New("Production Bible materialization contains an invalid SpecificationVersion")
		}
		asset, assetExists := assetsByID[specification.AssetID]
		if _, exists := specificationsByID[specification.ID]; exists || !assetExists ||
			asset.IdentityKey != specification.EntityKey || asset.Kind != specification.Kind {
			return Materialization{}, errors.New("Production Bible materialization SpecificationVersion has no exact Asset")
		}
		specificationsByID[specification.ID] = specification
	}
	for _, state := range states {
		if _, err := uuid.Parse(state.ID); err != nil || state.Revision < 1 ||
			!statePattern.MatchString(state.StateKey) || !hashPattern.MatchString(state.ContentHash) {
			return Materialization{}, errors.New("Production Bible materialization contains an invalid AssetState")
		}
		if _, assetExists := assetsByID[state.AssetID]; !assetExists {
			return Materialization{}, errors.New("Production Bible materialization AssetState has no exact Asset")
		}
		stateIdentity := state.AssetID + ":" + state.StateKey
		if _, exists := stateKeys[stateIdentity]; exists {
			return Materialization{}, errors.New("Production Bible materialization contains a duplicate AssetState")
		}
		stateKeys[stateIdentity] = struct{}{}
		stateCountByAsset[state.AssetID]++
	}
	for _, binding := range bindings {
		if _, err := uuid.Parse(binding.ID); err != nil || binding.Revision != 1 ||
			!hashPattern.MatchString(binding.ContentHash) {
			return Materialization{}, errors.New("Production Bible materialization contains an invalid ProductionBinding")
		}
		if _, exists := bindingIDs[binding.ID]; exists {
			return Materialization{}, errors.New("Production Bible materialization contains duplicate binding identities")
		}
		bindingIDs[binding.ID] = struct{}{}
		if _, exists := bindingKeys[binding.EntityKey]; exists || !keyPattern.MatchString(binding.EntityKey) {
			return Materialization{}, errors.New("Production Bible materialization contains duplicate entity bindings")
		}
		bindingKeys[binding.EntityKey] = struct{}{}
		asset, assetExists := assetsByID[binding.AssetID]
		if !assetExists || asset.IdentityKey != binding.EntityKey || stateCountByAsset[binding.AssetID] == 0 {
			return Materialization{}, errors.New("Production Bible materialization binding has no Asset or State")
		}
		specification, specificationExists := specificationsByID[binding.SpecificationVersionID]
		if !specificationExists || specification.AssetID != binding.AssetID ||
			specification.EntityKey != binding.EntityKey || specification.Kind != asset.Kind {
			return Materialization{}, errors.New("Production Bible materialization binding has no SpecificationVersion")
		}
	}
	value := Materialization{
		BibleVersionID: bibleVersionID, BibleVersionHash: bibleVersionHash,
		Assets: assets, Specifications: specifications, States: states, Bindings: bindings,
	}
	var err error
	value.ContentHash, err = CanonicalStoryHash(struct {
		Schema, BibleVersionID, BibleVersionHash string
		Assets                                   []MaterializedAsset
		Specifications                           []MaterializedSpecification
		States                                   []MaterializedState
		Bindings                                 []MaterializedBinding
	}{
		"production-bible-materialization", bibleVersionID, bibleVersionHash,
		assets, specifications, states, bindings,
	})
	if err != nil {
		return Materialization{}, err
	}
	return value, nil
}

func MaterializedAssetRef(value assetdomain.Asset) MaterializedAsset {
	return MaterializedAsset{
		ID: value.ID, IdentityKey: value.IdentityKey, Kind: value.Kind,
		Revision: value.Revision, ContentHash: value.ContentHash,
	}
}

func MaterializedSpecificationRef(value SpecificationVersion) MaterializedSpecification {
	return MaterializedSpecification{
		ID: value.ID, AssetID: value.AssetID, Kind: value.Kind, EntityKey: value.EntityKey,
		Version: value.Version, ContentHash: value.ContentHash,
	}
}

func MaterializedStateRef(value assetdomain.AssetState) MaterializedState {
	return MaterializedState{
		ID: value.ID, AssetID: value.AssetID, StateKey: value.StateKey,
		Revision: value.Revision, ContentHash: value.ContentHash,
	}
}

func MaterializedBindingRef(value ProductionBinding) MaterializedBinding {
	return MaterializedBinding{
		ID: value.ID, EntityKey: value.EntityKey, AssetID: value.Asset.ID,
		SpecificationVersionID: value.Specification.ID, Revision: value.Revision, ContentHash: value.ContentHash,
	}
}

func ValidateProductionBinding(value ProductionBinding) error {
	rebuilt, err := NewProductionBinding(ProductionBindingInput{
		ID: value.ID, WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		BibleVersionID: value.BibleVersionID, BibleVersionHash: value.BibleVersionHash,
		EntityKey: value.EntityKey, Asset: value.Asset, Specification: value.Specification,
		States: value.States, CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt,
	})
	if err != nil || value.Revision != 1 || value.ContentHash != rebuilt.ContentHash || !slices.Equal(value.States, rebuilt.States) {
		return errors.New("ProductionBinding has drifted")
	}
	return nil
}

func ValidateSpecificationVersion(value SpecificationVersion) error {
	rebuilt, err := NewSpecificationVersion(SpecificationVersionInput{
		ID: value.ID, WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, AssetID: value.AssetID,
		Kind: value.Kind, EntityKey: value.EntityKey, Version: value.Version,
		SourceBibleVersionID: value.SourceBibleVersionID, Snapshot: value.Snapshot,
		CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt,
	})
	if err != nil || value.ContentHash != rebuilt.ContentHash || !bytes.Equal(value.Snapshot, rebuilt.Snapshot) {
		return errors.New("SpecificationVersion has drifted")
	}
	return nil
}

func canonicalMaterializationJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("snapshot must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("snapshot contains multiple JSON values")
	}
	return json.Marshal(value)
}
