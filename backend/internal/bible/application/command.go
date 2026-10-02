package application

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

// ReferenceInput contains only a purpose and an authorized media identity.
type ReferenceInput struct {
	Role    domain.ImageRole `json:"role"`
	AssetID uuid.UUID        `json:"asset_id"`
}

// LookInput edits an appearance without accepting frozen media facts from the client.
type LookInput struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Default     bool               `json:"default"`
	AppliesTo   []domain.LookScope `json:"applies_to,omitempty"`
}

// CharacterInput is the closed manual character definition; media and voice use dedicated actions.
type CharacterInput struct {
	Name        string                     `json:"name"`
	Aliases     []string                   `json:"aliases,omitempty"`
	Description string                     `json:"description,omitempty"`
	Definition  domain.CharacterDefinition `json:"definition"`
}

// SampleInput selects one formal uploaded audio source.
type SampleInput struct {
	Name    string    `json:"name"`
	AssetID uuid.UUID `json:"asset_id"`
}

// VoiceInput selects exactly one catalog or sample source and optional instructions.
type VoiceInput struct {
	Kind         domain.VoiceKind `json:"kind"`
	Instructions string           `json:"instructions,omitempty"`
	Catalog      *VoiceSelection  `json:"catalog,omitempty"`
	Sample       *SampleInput     `json:"sample,omitempty"`
}

// Command is an internal closed action envelope assembled by individual public routes.
type Command struct {
	ProjectID              uuid.UUID               `json:"project_id"`
	Key                    uuid.UUID               `json:"key"`
	RequestID              uuid.UUID               `json:"-"`
	Kind                   domain.Kind             `json:"kind"`
	Action                 string                  `json:"action"`
	EntryID                uuid.UUID               `json:"entry_id"`
	ExpectedRevision       int64                   `json:"expected_revision"`
	Character              *CharacterInput         `json:"character,omitempty"`
	Location               *domain.LocationContent `json:"location,omitempty"`
	Prop                   *domain.PropContent     `json:"prop,omitempty"`
	LookID                 *uuid.UUID              `json:"look_id,omitempty"`
	Look                   *LookInput              `json:"look,omitempty"`
	References             []ReferenceInput        `json:"references,omitempty"`
	Voice                  *VoiceInput             `json:"voice,omitempty"`
	TargetID               *uuid.UUID              `json:"target_id,omitempty"`
	ExpectedTargetRevision *int64                  `json:"expected_target_revision,omitempty"`
	OperationID            *uuid.UUID              `json:"operation_id,omitempty"`
	OutputID               *uuid.UUID              `json:"output_id,omitempty"`
	AcknowledgedImpact     *ImpactProof            `json:"acknowledged_impact,omitempty"`
}

// Fingerprint validates action shape before producing an exact permanent command digest.
func (c Command) Fingerprint() (string, error) {
	if c.ProjectID == uuid.Nil || c.Key == uuid.Nil || !c.Kind.Valid() || c.ExpectedRevision < 0 || c.RequestID == uuid.Nil {
		return "", domain.ErrInvalidContent
	}
	if c.Action == "create" || c.Action == "create_result" {
		if c.EntryID != uuid.Nil || c.ExpectedRevision != 0 {
			return "", domain.ErrInvalidContent
		}
	} else if c.EntryID == uuid.Nil || c.ExpectedRevision < 1 {
		return "", domain.ErrInvalidContent
	}
	fields := 0
	for _, present := range []bool{c.Character != nil, c.Location != nil, c.Prop != nil, c.Look != nil, c.References != nil, c.Voice != nil, c.TargetID != nil, c.OperationID != nil} {
		if present {
			fields++
		}
	}
	switch c.Action {
	case "create", "update", "split":
		if c.Action == "split" && c.Kind != domain.KindCharacter {
			return "", domain.ErrInvalidContent
		}
		if fields != 1 || (c.Kind == domain.KindCharacter && c.Character == nil) || (c.Kind == domain.KindLocation && c.Location == nil) || (c.Kind == domain.KindProp && c.Prop == nil) {
			return "", domain.ErrInvalidContent
		}
	case "look_create", "look_update":
		if c.Kind != domain.KindCharacter || c.Look == nil || fields != 1 || (c.Action == "look_update" && c.LookID == nil) || (c.Action == "look_create" && c.LookID != nil) {
			return "", domain.ErrInvalidContent
		}
	case "look_delete", "look_default":
		if c.Kind != domain.KindCharacter || c.LookID == nil || fields != 0 {
			return "", domain.ErrInvalidContent
		}
	case "references":
		if c.Kind != domain.KindCharacter || c.LookID == nil || c.References == nil || fields != 1 || len(c.References) > 8 {
			return "", domain.ErrInvalidContent
		}
	case "voice_bind":
		if c.Kind != domain.KindCharacter || c.Voice == nil || fields != 1 {
			return "", domain.ErrInvalidContent
		}
	case "voice_unbind":
		if c.Kind != domain.KindCharacter || fields != 0 {
			return "", domain.ErrInvalidContent
		}
	case "confirm", "delete", "restore":
		if fields != 0 {
			return "", domain.ErrInvalidContent
		}
	case "merge":
		if c.Kind != domain.KindCharacter || c.TargetID == nil || *c.TargetID == uuid.Nil || *c.TargetID == c.EntryID || c.ExpectedTargetRevision == nil || *c.ExpectedTargetRevision < 1 || fields != 1 {
			return "", domain.ErrInvalidContent
		}
	case "adopt_result", "create_result":
		if c.OperationID == nil || *c.OperationID == uuid.Nil || c.OutputID == nil || *c.OutputID == uuid.Nil || fields != 1 {
			return "", domain.ErrInvalidContent
		}
	default:
		return "", domain.ErrInvalidContent
	}
	if (c.LookID != nil && (*c.LookID == uuid.Nil || !strings.HasPrefix(c.Action, "look_") && c.Action != "references")) || (c.ExpectedTargetRevision != nil && c.Action != "merge") || (c.OutputID != nil && c.Action != "adopt_result" && c.Action != "create_result") {
		return "", domain.ErrInvalidContent
	}
	b, err := json.Marshal(c)
	if err != nil || len(b) > 1<<20 {
		return "", domain.ErrInvalidContent
	}
	return domain.ContentSHA(b), nil
}
