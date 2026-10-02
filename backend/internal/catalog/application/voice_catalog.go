package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
)

var (
	// ErrInvalidVoiceSelection refuses an unsupported voice or parameter choice.
	ErrInvalidVoiceSelection = errors.New("invalid catalog voice selection")
	// ErrVoiceVersionConflict refuses a stale published model selection.
	ErrVoiceVersionConflict = errors.New("catalog voice version conflict")
	// ErrVoiceCatalogUnavailable refuses an absent or corrupt owning dependency.
	ErrVoiceCatalogUnavailable = errors.New("catalog voices unavailable")
	// ErrVoiceModelNotFound hides absent, foreign or currently unavailable models.
	ErrVoiceModelNotFound = errors.New("catalog voice model not found")
)

// ConfiguredVoiceModel contains safe current published configuration, without credentials.
type ConfiguredVoiceModel struct {
	Key         string
	DisplayName string
	Version     domain.ModelVersion
}

// VoiceModelCursor allows remaining voices of the first model to be paged.
type VoiceModelCursor struct {
	Key       string
	Inclusive bool
}

// VoiceModelInput scopes a bounded owning read of configured TTS models.
type VoiceModelInput struct {
	ModelKey string
	After    *VoiceModelCursor
	Limit    int
}

// VoiceModelStore retains current authorization and model configuration locks in the caller transaction.
type VoiceModelStore interface {
	ReadConfiguredVoiceModels(context.Context, identityapp.Principal, uuid.UUID, VoiceModelInput) ([]ConfiguredVoiceModel, error)
}

// VoiceSelection carries only explicit user choices, not supplier execution facts.
type VoiceSelection struct {
	ModelKey        string
	ExpectedVersion int
	VoiceKey        string
	Params          json.RawMessage
}

// VoiceReference freezes the exact safe published configuration and provided parameters.
type VoiceReference struct {
	ModelKey          string
	ModelVersionID    uuid.UUID
	ModelVersion      int
	VoiceKey          string
	ParamSchemaSHA256 string
	Params            json.RawMessage
}

// VoiceCursor is a deterministic configured model/voice position.
type VoiceCursor struct {
	ModelKey string
	VoiceKey string
}

// VoiceChoice is a configured enum choice, without a fabricated sample or provider call.
type VoiceChoice struct {
	ModelKey     string
	ModelVersion int
	VoiceKey     string
	DisplayName  string
}

// VoicePage bounds actual configured choices after filtering and ordering.
type VoicePage struct {
	Voices []VoiceChoice
	Next   *VoiceCursor
}

// VoiceCatalog validates published schemas through the catalog's existing owner.
type VoiceCatalog struct {
	store     VoiceModelStore
	validator ModelVersionValidator
}

// NewVoiceCatalog injects the transaction-bound catalog reader and existing publication validator.
func NewVoiceCatalog(store VoiceModelStore, validator ModelVersionValidator) *VoiceCatalog {
	return &VoiceCatalog{store: store, validator: validator}
}

func (q *VoiceCatalog) authorize(actor identityapp.Principal, project uuid.UUID) error {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword || actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer {
		return identityapp.ErrForbidden
	}
	if q == nil || q.store == nil || q.validator == nil {
		return ErrVoiceCatalogUnavailable
	}
	if project == uuid.Nil {
		return ErrInvalidVoiceSelection
	}
	return nil
}

// Reference verifies one complete selection without materializing implicit defaults.
func (q *VoiceCatalog) Reference(ctx context.Context, actor identityapp.Principal, project uuid.UUID, input VoiceSelection) (VoiceReference, error) {
	if err := q.authorize(actor, project); err != nil {
		return VoiceReference{}, err
	}
	if strings.TrimSpace(input.ModelKey) == "" || strings.TrimSpace(input.VoiceKey) == "" || len(input.ModelKey) > 512 || len(input.VoiceKey) > 512 || input.ExpectedVersion < 1 || input.ExpectedVersion > math.MaxInt32 {
		return VoiceReference{}, ErrInvalidVoiceSelection
	}
	params, err := decodeVoiceParams(input.Params)
	if err != nil {
		return VoiceReference{}, err
	}
	models, err := q.store.ReadConfiguredVoiceModels(ctx, actor, project, VoiceModelInput{ModelKey: input.ModelKey, Limit: 1})
	if err != nil {
		return VoiceReference{}, fmt.Errorf("read selected voice model: %w", err)
	}
	if len(models) == 0 {
		return VoiceReference{}, ErrVoiceModelNotFound
	}
	if len(models) != 1 || models[0].Key != input.ModelKey {
		return VoiceReference{}, ErrVoiceCatalogUnavailable
	}
	model := models[0]
	fields, voices, err := q.fields(model)
	if err != nil {
		return VoiceReference{}, err
	}
	if model.Version.VersionNo != input.ExpectedVersion {
		return VoiceReference{}, ErrVoiceVersionConflict
	}
	if !slices.Contains(voices, input.VoiceKey) || !voiceParamsSupported(params, fields, model.Version.Modes) {
		return VoiceReference{}, ErrInvalidVoiceSelection
	}
	canonical, err := json.Marshal(params)
	if err != nil {
		return VoiceReference{}, errors.Join(ErrInvalidVoiceSelection, err)
	}
	sum := sha256.Sum256(model.Version.ParamSchema)
	return VoiceReference{ModelKey: model.Key, ModelVersionID: model.Version.ID, ModelVersion: model.Version.VersionNo, VoiceKey: input.VoiceKey, ParamSchemaSHA256: hex.EncodeToString(sum[:]), Params: canonical}, nil
}

// List preserves complete model pagination and sorts each real voice enum before slicing.
func (q *VoiceCatalog) List(ctx context.Context, actor identityapp.Principal, project uuid.UUID, limit int, after *VoiceCursor) (VoicePage, error) {
	if err := q.authorize(actor, project); err != nil {
		return VoicePage{}, err
	}
	if limit < 1 || limit > 200 || after != nil && (strings.TrimSpace(after.ModelKey) == "" || strings.TrimSpace(after.VoiceKey) == "" || len(after.ModelKey) > 512 || len(after.VoiceKey) > 512) {
		return VoicePage{}, ErrInvalidVoiceSelection
	}
	page := VoicePage{Voices: []VoiceChoice{}}
	var cursor *VoiceModelCursor
	if after != nil {
		cursor = &VoiceModelCursor{Key: after.ModelKey, Inclusive: true}
	}
	for {
		if err := ctx.Err(); err != nil {
			return VoicePage{}, err
		}
		models, err := q.store.ReadConfiguredVoiceModels(ctx, actor, project, VoiceModelInput{After: cursor, Limit: 50})
		if err != nil {
			return VoicePage{}, fmt.Errorf("read configured voice models: %w", err)
		}
		if len(models) > 50 {
			return VoicePage{}, ErrVoiceCatalogUnavailable
		}
		for i, model := range models {
			if strings.TrimSpace(model.Key) == "" || cursor != nil && (model.Key < cursor.Key || model.Key == cursor.Key && !cursor.Inclusive) || i > 0 && model.Key <= models[i-1].Key {
				return VoicePage{}, ErrVoiceCatalogUnavailable
			}
			_, voices, err := q.fields(model)
			if err != nil {
				return VoicePage{}, err
			}
			for _, voice := range voices {
				if after != nil && (model.Key < after.ModelKey || model.Key == after.ModelKey && voice <= after.VoiceKey) {
					continue
				}
				if len(page.Voices) == limit {
					last := page.Voices[len(page.Voices)-1]
					page.Next = &VoiceCursor{ModelKey: last.ModelKey, VoiceKey: last.VoiceKey}
					return page, nil
				}
				page.Voices = append(page.Voices, VoiceChoice{ModelKey: model.Key, ModelVersion: model.Version.VersionNo, VoiceKey: voice, DisplayName: model.DisplayName + " · " + voice})
			}
		}
		if len(models) < 50 {
			return page, nil
		}
		cursor = &VoiceModelCursor{Key: models[len(models)-1].Key}
	}
}

type voiceField struct {
	Field     string            `json:"field"`
	Type      string            `json:"type"`
	Component string            `json:"component"`
	Required  bool              `json:"required"`
	Default   json.RawMessage   `json:"default"`
	Enum      []json.RawMessage `json:"enum"`
	ForModes  []string          `json:"for_modes"`
	Min       json.RawMessage   `json:"min"`
	Max       json.RawMessage   `json:"max"`
	Step      json.RawMessage   `json:"step"`
}

func (q *VoiceCatalog) fields(model ConfiguredVoiceModel) ([]voiceField, []string, error) {
	if err := q.validator.Validate(model.Version); err != nil {
		return nil, nil, errors.Join(ErrVoiceCatalogUnavailable, err)
	}
	var fields []voiceField
	if json.Unmarshal(model.Version.ParamSchema, &fields) != nil {
		return nil, nil, ErrVoiceCatalogUnavailable
	}
	var voices []string
	voiceFields := 0
	for _, field := range fields {
		if field.Component != "voice" {
			continue
		}
		voiceFields++
		if voiceFields != 1 || field.Field != "voice_id" || field.Type != "string" {
			return nil, nil, ErrVoiceCatalogUnavailable
		}
		for _, option := range field.Enum {
			var key string
			if json.Unmarshal(option, &key) != nil || strings.TrimSpace(key) == "" || len(key) > 512 {
				return nil, nil, ErrVoiceCatalogUnavailable
			}
			voices = append(voices, key)
		}
	}
	slices.Sort(voices)
	return fields, voices, nil
}

func decodeVoiceParams(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > 16*1024 {
		return nil, ErrInvalidVoiceSelection
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, ErrInvalidVoiceSelection
	}
	params := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || params[key] != nil {
			return nil, ErrInvalidVoiceSelection
		}
		switch key {
		case "speed", "pitch", "volume", "emotion", "language":
		default:
			return nil, ErrInvalidVoiceSelection
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, ErrInvalidVoiceSelection
		}
		params[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, ErrInvalidVoiceSelection
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidVoiceSelection
	}
	return params, nil
}
