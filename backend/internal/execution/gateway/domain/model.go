// Package domain defines the independent AI request, supply and money contracts.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalid rejects a contract outside the supported native subset.
	ErrInvalid = errors.New("invalid AI contract")
	// ErrConflict rejects reusing an immutable command for different input.
	ErrConflict = errors.New("AI idempotency conflict")
	// ErrFunds indicates that the account cannot cover the reservation.
	ErrFunds = errors.New("insufficient AI credits")
	// ErrBudget indicates that the approved spending limit is exhausted.
	ErrBudget = errors.New("AI budget exceeded")
	// ErrUnavailable indicates no eligible configured execution route.
	ErrUnavailable = errors.New("AI supply unavailable")
	// ErrProviderKeyRejected indicates that the provider rejected credential authentication.
	ErrProviderKeyRejected = errors.New("provider API key rejected")
	// ErrProviderKeyScope indicates that the key cannot use the provider's read-only endpoint.
	ErrProviderKeyScope = errors.New("provider API key lacks required access")
	// ErrProviderKeyCheckUnsupported identifies a route without an official read-only check.
	ErrProviderKeyCheckUnsupported = errors.New("provider does not support read-only key verification")
	// ErrTrialConfirmationRequired indicates a metered trial was not approved.
	ErrTrialConfirmationRequired = errors.New("generation trial requires separate confirmation")
	// ErrForbidden rejects missing or revoked caller authority.
	ErrForbidden = errors.New("AI scope forbidden")
	// ErrUnknown preserves funds until original evidence resolves the result.
	ErrUnknown = errors.New("AI result requires reconciliation")
)

// Price holds immutable micro-currency rates per million tokens and per request.
// Input includes both cache buckets. Output includes reasoning tokens.
type Price struct {
	ExplicitFree bool         `json:"explicit_free,omitempty"`
	Input        int64        `json:"input"`
	CacheRead    int64        `json:"cache_read"`
	CacheWrite   int64        `json:"cache_write"`
	Output       int64        `json:"output"`
	Request      int64        `json:"request"`
	Currency     string       `json:"currency"`
	FX           string       `json:"fx"`
	Multiplier   string       `json:"multiplier"`
	Discount     string       `json:"discount"`
	TextTariff   *TextTariff  `json:"text_tariff,omitempty"`
	MediaTariff  *MediaTariff `json:"media_tariff,omitempty"`
}

// Usage contains normalized disjoint billing evidence, with cache within Input.
type Usage struct {
	Input             int64       `json:"input"`
	CacheRead         int64       `json:"cache_read"`
	CacheWrite        int64       `json:"cache_write"`
	Output            int64       `json:"output"`
	VideoDurationMS   int64       `json:"video_duration_ms,omitempty"`
	ProviderAmount    json.Number `json:"provider_amount,omitempty"`
	Pixels            int64       `json:"pixels,omitempty"`
	ReferenceImages   int64       `json:"reference_images,omitempty"`
	Characters        int64       `json:"characters,omitempty"`
	OriginalSeconds   json.Number `json:"original_seconds,omitempty"`
	Reasoning         int64       `json:"reasoning,omitempty"`
	BillingStartedAt  time.Time   `json:"billing_started_at,omitzero"`
	BillingReceivedAt time.Time   `json:"billing_received_at,omitzero"`
}

// Validate checks exact rates, currency and commercial multipliers.
func (p Price) Validate() error {
	structured := p.TextTariff != nil || p.MediaTariff != nil
	if structured && (p.ExplicitFree || p.Input != 0 || p.Output != 0 || p.CacheRead != 0 || p.CacheWrite != 0 || p.Request != 0) {
		return ErrInvalid
	}
	if p.TextTariff != nil && !p.TextTariff.valid() || p.MediaTariff != nil &&
		(p.TextTariff != nil || !p.MediaTariff.valid() || (p.MediaTariff.Meter == ReportedImageAmount || p.MediaTariff.Meter == ImageTokens) && p.Currency != "USD" || p.MediaTariff.Meter == OutputVideoSeconds && p.Currency != "CNY") {
		return ErrInvalid
	}
	if (!structured && !p.ExplicitFree && p.Input == 0 && p.CacheRead == 0 && p.CacheWrite == 0 && p.Output == 0 && p.Request == 0) || p.Currency == "" || p.Input < 0 || p.CacheRead < 0 || p.CacheWrite < 0 || p.Output < 0 || p.Request < 0 {
		return ErrInvalid
	}
	for _, s := range []string{p.FX, p.Multiplier, p.Discount} {
		r, ok := new(big.Rat).SetString(s)
		if !ok || r.Sign() <= 0 || r.Cmp(big.NewRat(1000000, 1)) > 0 {
			return ErrInvalid
		}
	}
	d, _ := new(big.Rat).SetString(p.Discount)
	if d.Cmp(big.NewRat(1, 1)) > 0 {
		return ErrInvalid
	}
	return nil
}

// Validate enforces disjoint cache buckets within total input usage.
func (u Usage) Validate() error {
	if u.VideoDurationMS < 0 || u.VideoDurationMS > 600000 || len(u.ProviderAmount) > 33 || u.Characters < 0 || u.Characters > 1<<40 || len(u.OriginalSeconds) > 64 || u.Input < 0 || u.Output < 0 || u.Reasoning < 0 || u.Reasoning > u.Output || u.CacheRead < 0 || u.CacheWrite < 0 || u.Input > 1<<40 || u.Output > 1<<40 || u.CacheRead > u.Input || u.CacheWrite > u.Input-u.CacheRead {
		return ErrInvalid
	}
	return nil
}

// Cost computes the CNY micro amount once, using exact rational arithmetic.
func (p Price) Cost(u Usage) (int64, error) {
	if p.Validate() != nil || u.Validate() != nil {
		return 0, ErrInvalid
	}
	if p.MediaTariff != nil {
		amount, err := p.MediaTariff.cost(u)
		if err != nil {
			return 0, err
		}
		return p.convert(amount)
	}
	if p.TextTariff != nil {
		rates, err := p.TextTariff.rates(u)
		if err != nil {
			return 0, err
		}
		return p.convert(rates.amountMicros(u, false))
	}
	sum := new(big.Int).SetInt64(0)
	for _, v := range [][2]int64{{u.Input - u.CacheRead - u.CacheWrite, p.Input}, {u.CacheRead, p.CacheRead}, {u.CacheWrite, p.CacheWrite}, {u.Output, p.Output}} {
		sum.Add(sum, new(big.Int).Mul(big.NewInt(v[0]), big.NewInt(v[1])))
	}
	amount := new(big.Rat).SetFrac(sum, big.NewInt(1000000))
	amount.Add(amount, new(big.Rat).SetInt64(p.Request))
	return p.convert(amount)
}

func (p Price) convert(amount *big.Rat) (int64, error) {
	for _, s := range []string{p.FX, p.Multiplier, p.Discount} {
		r, _ := new(big.Rat).SetString(s)
		amount.Mul(amount, r)
	}
	return ceil(amount)
}
func ceil(r *big.Rat) (int64, error) {
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() || q.Sign() < 0 {
		return 0, ErrInvalid
	}
	return q.Int64(), nil
}

// Credits rounds CNY micro-units up to whole platform credits.
func Credits(cnyMicros int64) (int64, error) {
	if cnyMicros < 0 {
		return 0, ErrInvalid
	}
	credits, err := ceil(big.NewRat(cnyMicros, 100000))
	if err != nil || credits > 92233720368547 {
		return 0, ErrInvalid
	}
	return credits, nil
}

// Digest computes an immutable SHA-256 evidence digest.
func Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// Model is a published immutable product contract, independent of old catalogs.
type Model struct {
	PresetKey string `json:"preset_key,omitempty"`
	// OutputPolicy declares the published provider success review contract; empty remains pending.
	OutputPolicy  string               `json:"output_policy,omitempty"`
	Capability    string               `json:"capability,omitempty"`
	Modes         []string             `json:"modes,omitempty"`
	Parameters    map[string]Parameter `json:"parameters,omitempty"`
	VideoMetering *VideoMetering       `json:"video_metering,omitempty"`
	ID            uuid.UUID            `json:"id"`
	Name          string               `json:"name"`
	Protocol      string               `json:"protocol"`
	Price         Price                `json:"price"`
	MaxInput      int64                `json:"max_input"`
	MaxOutput     int64                `json:"max_output"`
	PurchaseCap   int64                `json:"purchase_cap"`
	Tools         bool                 `json:"tools"`
	Stream        bool                 `json:"stream"`
}

// Validate checks the declared native protocol and bounded product contract.
func (m Model) Validate() error {
	if len(m.PresetKey) > 200 || m.ID == uuid.Nil || m.Name == "" || !Protocol(m.Protocol) || m.Price.Validate() != nil || m.MaxInput < 1 || m.MaxInput > 1<<20 || m.MaxOutput < 1 || m.MaxOutput > 1<<20 || m.PurchaseCap < 0 {
		return ErrInvalid
	}
	if t := m.Price.TextTariff; t != nil && (m.Protocol == "media" || m.Protocol == EmbeddingProtocol || t.VercelProvider != "" && m.Protocol != "chat" || t.MaxInput > 0 && m.MaxInput > t.MaxInput) {
		return ErrInvalid
	}
	if m.Price.ExplicitFree || m.Price.MediaTariff != nil && m.Protocol != "media" {
		return ErrInvalid
	}
	if m.OutputPolicy != "" && (m.Protocol != "media" || m.OutputPolicy != "provider_success_v1") {
		return ErrInvalid
	}
	if m.VideoMetering != nil && !m.validVideoContract() {
		return ErrInvalid
	}
	if (m.Capability == EmbeddingCapability) != (m.Protocol == EmbeddingProtocol) {
		return ErrInvalid
	}
	if m.Protocol == EmbeddingProtocol && (m.Tools || m.Stream || len(m.Parameters)+len(m.Modes) != 0 || m.Price.Input <= 0 || m.Price.Request != 0 || m.Price.Output != 0 || m.Price.CacheRead != 0 || m.Price.CacheWrite != 0) {
		return ErrInvalid
	}
	if m.Protocol == "media" {
		if !slices.Contains([]string{"image.generate", "video.generate", "audio.generate"}, m.Capability) || len(m.Modes) == 0 || m.Tools || m.Stream || !m.AcceptsMediaPrice(m.Price) {
			return ErrInvalid
		}
		for _, p := range m.Parameters {
			if p.Validate() != nil {
				return ErrInvalid
			}
		}
	}
	return nil
}

// Protocol reports whether the native text protocol is supported.
func Protocol(s string) bool {
	return s == "chat" || s == "responses" || s == "messages" || s == "media" || s == EmbeddingProtocol
}

// Route references its own purchase version and encrypted credential identity.
type Route struct {
	KeyVerification string          `json:"key_verification,omitempty"`
	DriverConfig    json.RawMessage `json:"driver_config,omitempty"`
	ID              uuid.UUID       `json:"id"`
	ModelID         uuid.UUID       `json:"model_id"`
	Supplier        uuid.UUID       `json:"supplier"`
	Credential      uuid.UUID       `json:"credential"`
	Group           string          `json:"group"`
	BaseURL         string          `json:"base_url"`
	UpstreamModel   string          `json:"upstream_model"`
	Protocol        string          `json:"protocol"`
	Purchase        Price           `json:"purchase"`
	KeyID           string          `json:"key_id"`
	Ciphertext      []byte          `json:"-"`
	Priority        int             `json:"priority"`
}

// Call is a server-authorized logical operation. Its limit is credits, not money.
type Call struct {
	ID                   uuid.UUID
	Scope                uuid.UUID
	Actor                uuid.UUID
	Project              uuid.UUID
	Run                  uuid.UUID
	Key                  string
	Hash                 string
	Model                Model
	Raw                  json.RawMessage
	Limit                int64
	EstimatedVideoTokens int64 `json:",omitempty"`
	Stream               bool
	Origin               string
	Deadline             time.Time
}

// Receipt persists protocol output before settlement so crash recovery never buys it again.
type Receipt struct {
	Pending           bool              `json:"pending,omitempty"`
	Body              json.RawMessage   `json:"body"`
	Events            []json.RawMessage `json:"events,omitempty"`
	Usage             Usage             `json:"usage"`
	UpstreamID        string            `json:"upstream_id"`
	PurchaseMicros    int64             `json:"purchase_micros"`
	PurchaseEstimated bool              `json:"purchase_estimated,omitempty"`
	PurchasePending   bool              `json:"purchase_pending,omitempty"`
	DeliveryFailed    bool              `json:"delivery_failed,omitempty"`
}

// Record is the durable response returned by submission, replay or recovery.
type Record struct {
	ID       uuid.UUID `json:"id"`
	Scope    uuid.UUID `json:"scope_id"`
	ModelID  uuid.UUID `json:"model_id"`
	State    string    `json:"state"`
	Held     int64     `json:"held"`
	Charged  int64     `json:"charged"`
	Released int64     `json:"released"`
	Receipt  *Receipt  `json:"receipt,omitempty"`
	Created  time.Time `json:"created"`
}

// Parameter is the small scalar parameter contract actually consumed by a media driver.
type Parameter struct {
	Type    string            `json:"type"`
	Min     *float64          `json:"min,omitempty"`
	Max     *float64          `json:"max,omitempty"`
	Enum    []json.RawMessage `json:"enum,omitempty"`
	Default json.RawMessage   `json:"default,omitempty"`
}

// Validate rejects unbounded or unsupported schema features before publication.
func (p Parameter) Validate() error {
	if !slices.Contains([]string{"string", "integer", "number", "boolean"}, p.Type) || p.Min != nil && p.Max != nil && *p.Min > *p.Max {
		return ErrInvalid
	}
	return nil
}

// Reference identifies an owned input; URLs are resolved inside the trusted worker.
type Reference struct {
	ID        uuid.UUID `json:"id"`
	BindingID uuid.UUID `json:"binding_id,omitempty"`
	Version   string    `json:"version,omitempty"`
	Role      string    `json:"role"`
}

// MediaInput is the gateway's immutable single-output media intent.
type MediaInput struct {
	Model       string         `json:"model"`
	Capability  string         `json:"capability"`
	Mode        string         `json:"mode"`
	Prompt      string         `json:"prompt"`
	Params      map[string]any `json:"params"`
	References  []Reference    `json:"references,omitempty"`
	OutputCount int            `json:"output_count"`
}
