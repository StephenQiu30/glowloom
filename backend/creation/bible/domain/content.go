// Package domain defines project Bible identities and immutable typed content.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidContent means a typed Bible snapshot violates its closed contract.
var ErrInvalidContent = errors.New("invalid Bible content")

// Kind identifies one of the three independently versioned Bible identities.
type Kind string

// Identity kinds and budgets define the closed immutable content contract.
const (
	KindCharacter         Kind = "character"
	KindLocation          Kind = "location"
	KindProp              Kind = "prop"
	MaxVersionBytes            = 4 << 20
	MaxNameScalars             = 512
	MaxDescriptionScalars      = 8192
	MaxLooks                   = 200
)

// Valid reports whether the kind belongs to the closed identity set.
func (k Kind) Valid() bool { return k == KindCharacter || k == KindLocation || k == KindProp }

// CharacterDefinition preserves the fixed source's meaningful character fields.
type CharacterDefinition struct {
	Role              string `json:"role,omitempty"`
	Appearance        string `json:"appearance,omitempty"`
	Physique          string `json:"physique,omitempty"`
	Clothing          string `json:"clothing,omitempty"`
	Personality       string `json:"personality,omitempty"`
	Props             string `json:"props,omitempty"`
	ConsistencyPrompt string `json:"consistency_prompt,omitempty"`
	MultiViewPrompt   string `json:"multi_view_prompt,omitempty"`
	VoiceLanguage     string `json:"voice_language,omitempty"`
	VoiceAge          string `json:"voice_age,omitempty"`
	VoiceTimbre       string `json:"voice_timbre,omitempty"`
}

// ImageRole states the exact purpose of a frozen character representation.
type ImageRole string

// Image roles preserve the fixed source's six supported purposes.
const (
	RolePrimary    ImageRole = "primary"
	RoleFront      ImageRole = "front"
	RoleSide       ImageRole = "side"
	RoleBack       ImageRole = "back"
	RoleTurnaround ImageRole = "turnaround_sheet"
	RoleExpression ImageRole = "expression_sheet"
)

// Valid reports whether the image role belongs to the source's closed set.
func (r ImageRole) Valid() bool {
	switch r {
	case RolePrimary, RoleFront, RoleSide, RoleBack, RoleTurnaround, RoleExpression:
		return true
	default:
		return false
	}
}

// MediaFact holds owner-proven content and rendition identities without private URLs.
type MediaFact struct {
	AssetID         uuid.UUID `json:"asset_id"`
	Revision        int64     `json:"revision"`
	SHA256          string    `json:"sha256"`
	ByteSize        int64     `json:"byte_size"`
	Kind            string    `json:"kind"`
	RenditionID     uuid.UUID `json:"rendition_id,omitempty"`
	RenditionSHA256 string    `json:"rendition_sha256,omitempty"`
}

// Validate checks frozen content evidence; current authorization remains an owning port.
func (m MediaFact) Validate(kind string) error {
	if m.AssetID == uuid.Nil || m.Revision < 1 || m.Revision > math.MaxInt32 || !validSHA(m.SHA256) || m.ByteSize < 1 || m.Kind != kind {
		return ErrInvalidContent
	}
	if kind == "image" && (m.RenditionID == uuid.Nil || !validSHA(m.RenditionSHA256)) {
		return ErrInvalidContent
	}
	if kind == "audio" && (m.RenditionID != uuid.Nil || m.RenditionSHA256 != "") {
		return ErrInvalidContent
	}
	return nil
}

// ImageReference is an immutable purpose-to-media binding.
type ImageReference struct {
	Role  ImageRole `json:"role"`
	Media MediaFact `json:"media"`
}

// LookScope binds a look to a formal episode and optional stable scene key.
type LookScope struct {
	EpisodeID uuid.UUID  `json:"episode_id"`
	SceneKey  *uuid.UUID `json:"scene_key,omitempty"`
}

// LookContent is the complete immutable appearance within a character version.
type LookContent struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Default     bool             `json:"default"`
	AppliesTo   []LookScope      `json:"applies_to,omitempty"`
	References  []ImageReference `json:"references,omitempty"`
}

// VoiceKind distinguishes a configured catalog voice from an uploaded sample.
type VoiceKind string

// Voice sources preserve the catalog/sample distinction.
const (
	VoiceCatalog VoiceKind = "catalog"
	VoiceSample  VoiceKind = "sample"
)

// VoiceParams contains the supported optional user choices, checked against a real schema by its owner.
type VoiceParams struct {
	Speed    *float64 `json:"speed,omitempty"`
	Pitch    *float64 `json:"pitch,omitempty"`
	Volume   *float64 `json:"volume,omitempty"`
	Emotion  string   `json:"emotion,omitempty"`
	Language string   `json:"language,omitempty"`
}

// Equal compares values rather than pointer addresses in independently decoded parameters.
func (p VoiceParams) Equal(other VoiceParams) bool {
	if p.Emotion != other.Emotion || p.Language != other.Language {
		return false
	}
	pairs := [][2]*float64{{p.Speed, other.Speed}, {p.Pitch, other.Pitch}, {p.Volume, other.Volume}}
	for _, pair := range pairs {
		if (pair[0] == nil) != (pair[1] == nil) || pair[0] != nil && *pair[0] != *pair[1] {
			return false
		}
	}
	return true
}

// Equal compares complete catalog evidence including parameter values.
func (c CatalogVoice) Equal(other CatalogVoice) bool {
	return c.ModelKey == other.ModelKey && c.ModelVersionID == other.ModelVersionID && c.ModelVersion == other.ModelVersion && c.VoiceKey == other.VoiceKey && c.ParamSchemaSHA256 == other.ParamSchemaSHA256 && c.Params.Equal(other.Params)
}

// CatalogVoice freezes a real published model version and its selected voice.
type CatalogVoice struct {
	ModelKey          string      `json:"model_key"`
	ModelVersionID    uuid.UUID   `json:"model_version_id"`
	ModelVersion      int         `json:"model_version"`
	VoiceKey          string      `json:"voice_key"`
	ParamSchemaSHA256 string      `json:"param_schema_sha256"`
	Params            VoiceParams `json:"params"`
}

// SampleVoice is a frozen audio reference, not evidence of cloning or TTS execution.
type SampleVoice struct {
	Name  string    `json:"name"`
	Media MediaFact `json:"media"`
}

// VoiceContent freezes one closed voice source and delivery instructions.
type VoiceContent struct {
	Kind         VoiceKind     `json:"kind"`
	Instructions string        `json:"instructions,omitempty"`
	Catalog      *CatalogVoice `json:"catalog,omitempty"`
	Sample       *SampleVoice  `json:"sample,omitempty"`
}

// CharacterContent contains the complete content of an immutable character version.
type CharacterContent struct {
	Name        string              `json:"name"`
	Aliases     []string            `json:"aliases,omitempty"`
	Description string              `json:"description,omitempty"`
	Definition  CharacterDefinition `json:"definition"`
	Looks       []LookContent       `json:"looks"`
	Voice       *VoiceContent       `json:"voice,omitempty"`
}

// LocationContent contains the source location fields without arbitrary provider JSON.
type LocationContent struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
}

// PropContent contains the source prop fields without arbitrary provider JSON.
type PropContent struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
}

func validText(s string, limit int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= limit && !strings.ContainsRune(s, '\x00')
}
func validName(s string) bool { return strings.TrimSpace(s) != "" && validText(s, MaxNameScalars) }
func validSHA(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}
func validateLabels(name string, aliases []string, descriptions ...string) error {
	if !validName(name) || len(aliases) > 64 {
		return ErrInvalidContent
	}
	seen := make(map[string]bool, len(aliases))
	for _, a := range aliases {
		if !validName(a) || seen[a] {
			return ErrInvalidContent
		}
		seen[a] = true
	}
	for _, d := range descriptions {
		if !validText(d, MaxDescriptionScalars) {
			return ErrInvalidContent
		}
	}
	return nil
}

// Validate checks the complete version and stable appearance identities.
func (c CharacterContent) Validate() error {
	d := c.Definition
	if err := validateLabels(c.Name, c.Aliases, c.Description, d.Role, d.Appearance, d.Physique, d.Clothing, d.Personality, d.Props, d.ConsistencyPrompt, d.MultiViewPrompt, d.VoiceLanguage, d.VoiceAge, d.VoiceTimbre); err != nil {
		return err
	}
	if len(c.Looks) < 1 || len(c.Looks) > MaxLooks {
		return ErrInvalidContent
	}
	ids := make(map[uuid.UUID]bool, len(c.Looks))
	defaults := 0
	for _, l := range c.Looks {
		if l.ID == uuid.Nil || ids[l.ID] || validateLabels(l.Name, nil, l.Description) != nil || len(l.References) > 8 || len(l.AppliesTo) > 2500 {
			return ErrInvalidContent
		}
		ids[l.ID] = true
		if l.Default {
			defaults++
		}
		roles := make(map[ImageRole]bool, len(l.References))
		for _, r := range l.References {
			if !r.Role.Valid() || roles[r.Role] || r.Media.Validate("image") != nil {
				return ErrInvalidContent
			}
			roles[r.Role] = true
		}
		type scopeKey struct {
			episode uuid.UUID
			scene   uuid.UUID
		}
		scopes := make(map[scopeKey]bool, len(l.AppliesTo))
		for _, s := range l.AppliesTo {
			if s.EpisodeID == uuid.Nil || (s.SceneKey != nil && *s.SceneKey == uuid.Nil) {
				return ErrInvalidContent
			}
			key := scopeKey{episode: s.EpisodeID}
			if s.SceneKey != nil {
				key.scene = *s.SceneKey
			}
			if scopes[key] {
				return ErrInvalidContent
			}
			scopes[key] = true
		}
	}
	if defaults != 1 {
		return ErrInvalidContent
	}
	if c.Voice != nil {
		return c.Voice.Validate()
	}
	return nil
}

// Validate checks the exclusive voice source and bounded typed parameters.
func (v VoiceContent) Validate() error {
	if !validText(v.Instructions, MaxDescriptionScalars) {
		return ErrInvalidContent
	}
	switch v.Kind {
	case VoiceSample:
		if v.Sample == nil || v.Catalog != nil || !validName(v.Sample.Name) || v.Sample.Media.Validate("audio") != nil {
			return ErrInvalidContent
		}
	case VoiceCatalog:
		c := v.Catalog
		if c == nil || v.Sample != nil || !validName(c.ModelKey) || !validName(c.VoiceKey) || c.ModelVersionID == uuid.Nil || c.ModelVersion < 1 || !validSHA(c.ParamSchemaSHA256) {
			return ErrInvalidContent
		}
		if !validText(c.Params.Emotion, 512) || !validText(c.Params.Language, 512) {
			return ErrInvalidContent
		}
		for _, n := range []*float64{c.Params.Speed, c.Params.Pitch, c.Params.Volume} {
			if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0)) {
				return ErrInvalidContent
			}
		}
	default:
		return ErrInvalidContent
	}
	return nil
}

// Validate checks location content budgets.
func (c LocationContent) Validate() error {
	return validateLabels(c.Name, c.Aliases, c.Description, c.Prompt)
}

// Validate checks prop content budgets.
func (c PropContent) Validate() error {
	return validateLabels(c.Name, c.Aliases, c.Description, c.Prompt)
}

// ContentSHA computes the exact serialized content digest.
func ContentSHA(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// EncodeCharacter validates and canonically encodes a complete immutable snapshot.
func EncodeCharacter(c CharacterContent) ([]byte, string, error) {
	if err := c.Validate(); err != nil {
		return nil, "", err
	}
	return encode(c)
}

// EncodeLocation validates and canonically encodes a location snapshot.
func EncodeLocation(c LocationContent) ([]byte, string, error) {
	if err := c.Validate(); err != nil {
		return nil, "", err
	}
	return encode(c)
}

// EncodeProp validates and canonically encodes a prop snapshot.
func EncodeProp(c PropContent) ([]byte, string, error) {
	if err := c.Validate(); err != nil {
		return nil, "", err
	}
	return encode(c)
}
func encode(v any) ([]byte, string, error) {
	b, err := json.Marshal(v)
	if err != nil || len(b) > MaxVersionBytes {
		return nil, "", ErrInvalidContent
	}
	return b, ContentSHA(b), nil
}

func decode(b []byte, v any) error {
	return DecodeClosedJSON(b, v)
}

// DecodeCharacter rejects unknown fields and invalid immutable content.
func DecodeCharacter(b []byte) (CharacterContent, error) {
	var c CharacterContent
	if decode(b, &c) != nil || c.Validate() != nil {
		return c, ErrInvalidContent
	}
	return c, nil
}

// DecodeLocation rejects unknown fields and invalid immutable content.
func DecodeLocation(b []byte) (LocationContent, error) {
	var c LocationContent
	if decode(b, &c) != nil || c.Validate() != nil {
		return c, ErrInvalidContent
	}
	return c, nil
}

// DecodeProp rejects unknown fields and invalid immutable content.
func DecodeProp(b []byte) (PropContent, error) {
	var c PropContent
	if decode(b, &c) != nil || c.Validate() != nil {
		return c, ErrInvalidContent
	}
	return c, nil
}
