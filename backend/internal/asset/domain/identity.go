package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	AssetKindCharacter = "character"
	AssetKindLocation  = "location"
	AssetKindProp      = "prop"
)

var (
	identityKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,99}$`)
	stateKeyPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,79}$`)
	hashPattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type AssetInput struct {
	ID, WorkspaceID, ProjectID string
	Kind, IdentityKey          string
	CreatedBy                  string
	CreatedAt                  time.Time
}

type Asset struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	ProjectID   string    `json:"project_id"`
	Kind        string    `json:"kind"`
	IdentityKey string    `json:"identity_key"`
	Revision    int       `json:"revision"`
	ContentHash string    `json:"content_hash"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewAsset(input AssetInput) (Asset, error) {
	for _, value := range []string{input.ID, input.WorkspaceID, input.ProjectID, input.CreatedBy} {
		if _, err := uuid.Parse(value); err != nil {
			return Asset{}, errors.New("invalid Asset identity")
		}
	}
	if !oneOfAssetKind(input.Kind) || !identityKeyPattern.MatchString(input.IdentityKey) || input.CreatedAt.IsZero() {
		return Asset{}, errors.New("invalid Asset input")
	}
	asset := Asset{
		ID: input.ID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		Kind: input.Kind, IdentityKey: input.IdentityKey, Revision: 1,
		CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt.UTC(),
	}
	asset.ContentHash = identityHash(struct {
		Schema, WorkspaceID, ProjectID, Kind, IdentityKey string
	}{"asset-identity", asset.WorkspaceID, asset.ProjectID, asset.Kind, asset.IdentityKey})
	return asset, nil
}

type AssetStateInput struct {
	ID, WorkspaceID, ProjectID, AssetID string
	StateKey, Label                     string
	Revision                            int
	Snapshot                            json.RawMessage
	CreatedBy                           string
	CreatedAt                           time.Time
}

type AssetState struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	ProjectID   string          `json:"project_id"`
	AssetID     string          `json:"asset_id"`
	StateKey    string          `json:"state_key"`
	Label       string          `json:"label"`
	Revision    int             `json:"revision"`
	Snapshot    json.RawMessage `json:"snapshot"`
	ContentHash string          `json:"content_hash"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

func NewAssetState(input AssetStateInput) (AssetState, error) {
	for _, value := range []string{input.ID, input.WorkspaceID, input.ProjectID, input.AssetID, input.CreatedBy} {
		if _, err := uuid.Parse(value); err != nil {
			return AssetState{}, errors.New("invalid AssetState identity")
		}
	}
	if !stateKeyPattern.MatchString(input.StateKey) || strings.TrimSpace(input.Label) == "" ||
		input.Revision < 1 || input.CreatedAt.IsZero() {
		return AssetState{}, errors.New("invalid AssetState input")
	}
	canonical, err := canonicalAssetJSON(input.Snapshot)
	if err != nil {
		return AssetState{}, errors.New("invalid AssetState snapshot")
	}
	state := AssetState{
		ID: input.ID, WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, AssetID: input.AssetID,
		StateKey: input.StateKey, Label: strings.TrimSpace(input.Label), Revision: input.Revision,
		Snapshot: canonical, CreatedBy: input.CreatedBy, CreatedAt: input.CreatedAt.UTC(),
	}
	state.ContentHash = identityHash(struct {
		Schema, AssetID, StateKey, Label string
		Snapshot                         json.RawMessage
	}{"asset-state", state.AssetID, state.StateKey, state.Label, state.Snapshot})
	return state, nil
}

func ValidateAsset(value Asset) error {
	rebuilt, err := NewAsset(AssetInput{
		ID: value.ID, WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
		Kind: value.Kind, IdentityKey: value.IdentityKey, CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt,
	})
	if err != nil || value.Revision != 1 || value.ContentHash != rebuilt.ContentHash {
		return errors.New("Asset has drifted")
	}
	return nil
}

func ValidateAssetState(value AssetState) error {
	rebuilt, err := NewAssetState(AssetStateInput{
		ID: value.ID, WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, AssetID: value.AssetID,
		StateKey: value.StateKey, Label: value.Label, Revision: value.Revision,
		Snapshot: value.Snapshot, CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt,
	})
	if err != nil || value.ContentHash != rebuilt.ContentHash || !bytes.Equal(value.Snapshot, rebuilt.Snapshot) {
		return errors.New("AssetState has drifted")
	}
	return nil
}

func oneOfAssetKind(value string) bool {
	return slices.Contains([]string{AssetKindCharacter, AssetKindLocation, AssetKindProp}, value)
}

func canonicalAssetJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("snapshot must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("snapshot contains multiple JSON values")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func identityHash(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func ValidContentHash(value string) bool { return hashPattern.MatchString(value) }
