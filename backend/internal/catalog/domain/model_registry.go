package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidCapability means a capability declaration is incomplete.
	ErrInvalidCapability = errors.New("invalid capability")
	// ErrInvalidModel means a model identity or state is invalid.
	ErrInvalidModel = errors.New("invalid model profile")
	// ErrInvalidModelVersion means an immutable model version is invalid.
	ErrInvalidModelVersion = errors.New("invalid model version")
	// ErrInvalidPriceRule means an immutable price version is invalid.
	ErrInvalidPriceRule = errors.New("invalid price rule")
	// ErrModelRevisionConflict means the model changed since it was read.
	ErrModelRevisionConflict = errors.New("model revision conflict")
	// ErrModelNotPublishable means a model lacks a current version or price.
	ErrModelNotPublishable = errors.New("model is not publishable")
	// ErrInvalidModelTransition means the requested status equals the current status.
	ErrInvalidModelTransition = errors.New("invalid model status transition")
)

// OutputType is the kind of result produced by a capability.
type OutputType string

// Capability output types identify the produced artifact.
const (
	OutputJSON  OutputType = "json"
	OutputImage OutputType = "image"
	OutputVideo OutputType = "video"
	OutputAudio OutputType = "audio"
	OutputNone  OutputType = "none"
)

// Capability describes model-independent modes and input roles.
type Capability struct {
	ID         uuid.UUID
	Key        string
	OutputType OutputType
	Modes      []string
	InputRoles []string
}

// Validate checks the stable capability declaration.
func (c Capability) Validate() error {
	if c.ID == uuid.Nil || strings.TrimSpace(c.Key) == "" ||
		!uniqueNames(c.Modes, true) || !uniqueNames(c.InputRoles, false) {
		return ErrInvalidCapability
	}
	switch c.OutputType {
	case OutputJSON, OutputImage, OutputVideo, OutputAudio, OutputNone:
		return nil
	default:
		return ErrInvalidCapability
	}
}

// ModelStatus controls whether a model accepts new quotes.
type ModelStatus string

// Model statuses distinguish available and disabled models.
const (
	ModelActive   ModelStatus = "active"
	ModelDisabled ModelStatus = "disabled"
)

// ModelProfile is the mutable identity and current-version pointer.
type ModelProfile struct {
	ID               uuid.UUID
	Key              string
	ProviderID       uuid.UUID
	Capability       string
	DisplayName      string
	Status           ModelStatus
	CurrentVersionID uuid.UUID
	Revision         int64
	CreateTime       time.Time
	UpdateTime       time.Time
}

// Validate checks identity and the status machine without querying storage.
func (m ModelProfile) Validate() error {
	if m.ID == uuid.Nil || m.ProviderID == uuid.Nil || strings.TrimSpace(m.Key) == "" ||
		strings.TrimSpace(m.Capability) == "" || strings.TrimSpace(m.DisplayName) == "" ||
		(m.Status != ModelActive && m.Status != ModelDisabled) ||
		m.Revision < 1 || m.Revision > math.MaxInt32 ||
		(m.Status == ModelActive && m.CurrentVersionID == uuid.Nil) {
		return ErrInvalidModel
	}
	return nil
}

// AttachVersion advances the current pointer after a new version is inserted.
func (m *ModelProfile) AttachVersion(version ModelVersion, expectedRevision int64) error {
	if m == nil || m.Validate() != nil {
		return ErrInvalidModel
	}
	if m.Revision != expectedRevision || m.Revision == math.MaxInt32 {
		return ErrModelRevisionConflict
	}
	if version.Validate() != nil || version.ModelID != m.ID {
		return ErrInvalidModelVersion
	}
	m.CurrentVersionID = version.ID
	m.Revision++
	return nil
}

// SetStatus changes availability; activation requires a version and price.
func (m *ModelProfile) SetStatus(status ModelStatus, hasPrice bool) error {
	if m == nil || m.Validate() != nil {
		return ErrInvalidModel
	}
	if status != ModelActive && status != ModelDisabled {
		return ErrInvalidModelTransition
	}
	if m.Status == status {
		return ErrInvalidModelTransition
	}
	if status == ModelActive && (m.CurrentVersionID == uuid.Nil || !hasPrice) {
		return ErrModelNotPublishable
	}
	if m.Revision == math.MaxInt32 {
		return ErrModelRevisionConflict
	}
	m.Status = status
	m.Revision++
	return nil
}

// Moderation identifies who checks generated content.
type Moderation string

// Moderation modes identify the content-checking owner.
const (
	ModerationProvider Moderation = "provider"
	ModerationPlatform Moderation = "platform"
	ModerationBoth     Moderation = "both"
)

// ModelVersion is a published, append-only model configuration.
type ModelVersion struct {
	ID               uuid.UUID
	ModelID          uuid.UUID
	VersionNo        int
	ProviderModelID  string
	Modes            []string
	Limits           json.RawMessage
	ParamSchema      json.RawMessage
	SupportsQuery    bool
	SupportsCancel   bool
	SupportsCallback bool
	ExpectedMaxMS    int
	Moderation       Moderation
	Queue            string
	CreateBy         uuid.UUID
	CreateTime       time.Time
}

// Validate checks immutable version metadata and JSON container shapes.
func (v ModelVersion) Validate() error {
	if v.ID == uuid.Nil || v.ModelID == uuid.Nil || v.VersionNo < 1 || v.VersionNo > math.MaxInt32 ||
		strings.TrimSpace(v.ProviderModelID) == "" || !uniqueNames(v.Modes, true) ||
		!jsonShape(v.Limits, '{') || !jsonShape(v.ParamSchema, '[') ||
		v.ExpectedMaxMS < 1 || v.ExpectedMaxMS > math.MaxInt32 || strings.TrimSpace(v.Queue) == "" {
		return ErrInvalidModelVersion
	}
	switch v.Moderation {
	case ModerationProvider, ModerationPlatform, ModerationBoth:
		return nil
	default:
		return ErrInvalidModelVersion
	}
}

// PriceUnit defines the denominator used by a price rule.
type PriceUnit string

// Price units define the billable quantity for a rule.
const (
	PricePerImage    PriceUnit = "per_image"
	PricePerSecond   PriceUnit = "per_second"
	PricePerRequest  PriceUnit = "per_request"
	PricePer1KTokens PriceUnit = "per_1k_tokens"
	PricePer1KChars  PriceUnit = "per_1k_chars"
)

// PriceRuleVersion is a published, append-only price and exchange rate.
// FXRateToCNY is decimal text to retain exact monetary input.
type PriceRuleVersion struct {
	ID            uuid.UUID
	ModelID       uuid.UUID
	VersionNo     int
	Unit          PriceUnit
	Rule          json.RawMessage
	Currency      string
	FXRateToCNY   string
	EffectiveFrom time.Time
	CreateBy      uuid.UUID
	CreateTime    time.Time
}

// Validate checks currency, exact exchange rate, and rule shape.
func (p PriceRuleVersion) Validate() error {
	if p.ID == uuid.Nil || p.ModelID == uuid.Nil || p.VersionNo < 1 || p.VersionNo > math.MaxInt32 ||
		!jsonShape(p.Rule, '{') || p.EffectiveFrom.IsZero() || len(p.Currency) != 3 {
		return ErrInvalidPriceRule
	}
	for _, letter := range p.Currency {
		if letter < 'A' || letter > 'Z' {
			return ErrInvalidPriceRule
		}
	}
	switch p.Unit {
	case PricePerImage, PricePerSecond, PricePerRequest, PricePer1KTokens, PricePer1KChars:
	default:
		return ErrInvalidPriceRule
	}
	if p.Currency != "CNY" && p.FXRateToCNY == "" {
		return ErrInvalidPriceRule
	}
	if p.FXRateToCNY != "" && !validPositiveRate(p.FXRateToCNY) {
		return ErrInvalidPriceRule
	}
	return nil
}

func uniqueNames(names []string, required bool) bool {
	if required && len(names) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return false
		}
		if _, exists := seen[name]; exists {
			return false
		}
		seen[name] = struct{}{}
	}
	return true
}

func jsonShape(raw json.RawMessage, opening byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 1 && trimmed[0] == opening && json.Valid(trimmed)
}

func validPositiveRate(rate string) bool {
	if rate == "" || rate[0] == '-' || rate[0] == '+' {
		return false
	}
	whole, fraction, dotted := strings.Cut(rate, ".")
	if len(whole) == 0 || len(whole) > 6 ||
		(dotted && (len(fraction) == 0 || len(fraction) > 6)) {
		return false
	}
	positive := false
	for _, digit := range whole + fraction {
		if digit < '0' || digit > '9' {
			return false
		}
		positive = positive || digit != '0'
	}
	return positive
}
