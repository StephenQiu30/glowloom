package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

var (
	// ErrInvalidFreeQuote means the canvas request has invalid identifiers or shape.
	ErrInvalidFreeQuote = errors.New("invalid free quote request")
	// ErrFreeQuoteModelMissing means no current usable model and price match.
	ErrFreeQuoteModelMissing = errors.New("free quote model unavailable")
	// ErrFreeQuoteInputNotReady means the request violates published model limits.
	ErrFreeQuoteInputNotReady = errors.New("free quote input not ready")
	// ErrQuoteKeyReused means one UUID names a different quote request.
	ErrQuoteKeyReused = errors.New("quote idempotency key reused")
)

// FreeQuoteItemInput describes one canvas request without project metadata.
type FreeQuoteItemInput struct {
	ModelKey        string
	Capability      string
	Mode            string
	Prompt          string
	Params          json.RawMessage
	OutputCount     int32
	ForceRegenerate bool
	MediaInputs     []FreeQuoteMediaInput
}

// FreeQuoteMediaInput names an ordered canvas asset and its model input role.
type FreeQuoteMediaInput struct {
	Role         string
	MediaAssetID uuid.UUID
}

// Validate checks prompt, optional model selection and JSON bounds.
func (i FreeQuoteItemInput) Validate() error {
	if strings.TrimSpace(i.Capability) == "" || strings.TrimSpace(i.Mode) == "" ||
		strings.TrimSpace(i.Prompt) == "" || len(i.Prompt) > 64*1024 ||
		!utf8.ValidString(i.Prompt) || i.OutputCount < 1 || i.OutputCount > 8 ||
		len(i.Params) > 64*1024 {
		return ErrInvalidFreeQuote
	}
	if len(i.Params) == 0 {
		i.Params = json.RawMessage(`{}`)
	}
	if _, err := fingerprintObject(i.Params); err != nil {
		return ErrInvalidFreeQuote
	}
	if len(i.MediaInputs) > 255 {
		return ErrInvalidFreeQuote
	}
	for _, media := range i.MediaInputs {
		if media.MediaAssetID == uuid.Nil || strings.TrimSpace(media.Role) == "" || media.Role == "prompt" {
			return ErrInvalidFreeQuote
		}
	}
	return nil
}

// CreateFreeQuoteInput is an internal canvas request with a stable request key.
type CreateFreeQuoteInput struct {
	ProjectID uuid.UUID
	RequestID string
	FreeQuoteItemInput
}

// Validate checks request identity and the single item.
func (i CreateFreeQuoteInput) Validate() error {
	requestID, err := uuid.Parse(i.RequestID)
	if i.ProjectID == uuid.Nil || err != nil || requestID == uuid.Nil ||
		requestID.String() != i.RequestID {
		return ErrInvalidFreeQuote
	}
	return i.FreeQuoteItemInput.Validate()
}

// FreeQuoteCatalog is a locked, current model and effective price snapshot.
type FreeQuoteCatalog struct {
	ModelVersionID  uuid.UUID
	ProviderModelID string
	Region          string
	Modes           []string
	InputRoles      []string
	Limits          json.RawMessage
	ParamSchema     json.RawMessage
	Price           catalogdomain.PriceRuleVersion
}

// FreeQuoteMediaFact is a project-scoped locked media snapshot supplied by the adapter.
type FreeQuoteMediaFact struct {
	ID         uuid.UUID
	Kind       string
	SHA256     string
	ByteSize   int64
	DurationMS *int64
}

// PreparedFreeQuote holds a new immutable quote before the adapter writes it.
type PreparedFreeQuote struct {
	Operation domain.Operation
	Inputs    []domain.OperationInput
	Reusable  bool
}

// PrepareFreeQuote validates the published model and actual request, then
// calculates the frozen fingerprint and exact price. The candidate is trusted
// only after a project-scoped completed-output lookup by the adapter.
func PrepareFreeQuote(input CreateFreeQuoteInput, catalog FreeQuoteCatalog, media []FreeQuoteMediaFact, now time.Time, candidate *uuid.UUID) (PreparedFreeQuote, error) {
	if len(input.Params) == 0 {
		input.Params = json.RawMessage(`{}`)
	}
	if input.Validate() != nil || now.IsZero() || catalog.ModelVersionID == uuid.Nil ||
		strings.TrimSpace(catalog.ProviderModelID) == "" ||
		(catalog.Region != "domestic" && catalog.Region != "overseas") ||
		catalog.Price.Validate() != nil {
		return PreparedFreeQuote{}, ErrInvalidFreeQuote
	}
	supported := false
	for _, mode := range catalog.Modes {
		if mode == input.Mode {
			supported = true
			break
		}
	}
	if !supported {
		return PreparedFreeQuote{}, ErrFreeQuoteModelMissing
	}
	params, err := validateQuoteParams(input.Params, catalog.ParamSchema, input.Mode)
	if err != nil {
		return PreparedFreeQuote{}, err
	}
	var limits struct {
		MaxOutputs   int64    `json:"max_outputs"`
		DurationsMS  []int64  `json:"durations_ms"`
		Resolutions  []string `json:"resolutions"`
		AspectRatios []string `json:"aspect_ratios"`
	}
	if err := json.Unmarshal(catalog.Limits, &limits); err != nil ||
		limits.MaxOutputs < int64(input.OutputCount) {
		return PreparedFreeQuote{}, ErrFreeQuoteInputNotReady
	}
	for _, selected := range []struct {
		name    string
		allowed []string
	}{{"resolution", limits.Resolutions}, {"aspect_ratio", limits.AspectRatios}} {
		if value, present := params[selected.name]; present && len(selected.allowed) > 0 {
			text, ok := value.(string)
			if !ok || !slices.Contains(selected.allowed, text) {
				return PreparedFreeQuote{}, ErrFreeQuoteInputNotReady
			}
		}
	}
	operationID := uuid.New()
	part := FingerprintPart{SeqNo: 0, Role: "prompt", RefType: "text", TextValue: input.Prompt}
	parts := []FingerprintPart{part}
	text := input.Prompt
	inputs := []domain.OperationInput{{
		ID: uuid.New(), OperationID: operationID, SeqNo: 0,
		Role: "prompt", RefType: "text", TextValue: &text,
	}}
	mediaParts, mediaInputs, err := prepareFreeQuoteMedia(input.MediaInputs, media, catalog.InputRoles, catalog.Limits, operationID)
	if err != nil {
		return PreparedFreeQuote{}, err
	}
	parts = append(parts, mediaParts...)
	inputs = append(inputs, mediaInputs...)
	hash, reusable, err := InputHash(FingerprintInput{
		Capability: input.Capability, Mode: input.Mode,
		ModelProfileVersionID: catalog.ModelVersionID,
		ProviderModelID:       catalog.ProviderModelID,
		OutputCount:           input.OutputCount, Params: input.Params,
		ParamSchema: catalog.ParamSchema, Inputs: parts,
		FinalPrompt: input.Prompt,
	})
	if err != nil {
		return PreparedFreeQuote{}, fmt.Errorf("fingerprint free quote: %w", err)
	}
	if candidate != nil && (*candidate == uuid.Nil || !reusable || input.ForceRegenerate) {
		return PreparedFreeQuote{}, ErrInvalidFreeQuote
	}
	pricing := PricingInput{OutputCount: int64(input.OutputCount), Mode: input.Mode,
		CharacterCount:     int64(utf8.RuneCountInString(input.Prompt)),
		AllowedDurationsMS: limits.DurationsMS,
	}
	if resolution, ok := params["resolution"].(string); ok {
		pricing.Resolution = resolution
	}
	if duration, ok := params["duration_ms"].(json.Number); ok {
		exact, parseErr := exactFingerprintNumber(duration.String())
		if parseErr != nil || !exact.IsInt() || !exact.Num().IsInt64() {
			return PreparedFreeQuote{}, ErrFreeQuoteInputNotReady
		}
		pricing.TargetDurationMS = exact.Num().Int64()
	}
	amount, costs, err := CalculateQuoteDetailed(catalog.Price, pricing)
	if err != nil {
		return PreparedFreeQuote{}, fmt.Errorf("price free quote: %w", err)
	}
	if candidate != nil {
		amount = 0
	}
	quoteDetail, err := json.Marshal(struct {
		QuoteCostDetail
		Region             string    `json:"region"`
		PriceRuleVersionID uuid.UUID `json:"price_rule_version_id"`
		Reused             bool      `json:"reused"`
	}{
		QuoteCostDetail: costs, Region: catalog.Region,
		PriceRuleVersionID: catalog.Price.ID, Reused: candidate != nil,
	})
	if err != nil {
		return PreparedFreeQuote{}, fmt.Errorf("encode free quote detail: %w", err)
	}
	now = now.UTC()
	expires := now.Add(15 * time.Minute)
	region := catalog.Region
	versionID, priceID := catalog.ModelVersionID, catalog.Price.ID
	op := domain.Operation{
		ID: operationID, ProjectID: input.ProjectID, TargetType: "free",
		Capability: input.Capability, Mode: input.Mode,
		ModelProfileVersionID: &versionID, PriceRuleVersionID: &priceID,
		Params: input.Params, OutputCount: input.OutputCount,
		InputHash: hash, Origin: "canvas", Status: domain.StatusQuoted,
		QuoteMicros: &amount, QuoteDetail: quoteDetail, QuoteExpiresAt: &expires,
		ReusedFromID: candidate, ForceRegenerate: input.ForceRegenerate,
		Region: &region, CreateTime: now,
	}
	prepared := PreparedFreeQuote{Operation: op, Reusable: reusable,
		Inputs: inputs,
	}
	if err := op.Validate(); err != nil {
		return PreparedFreeQuote{}, fmt.Errorf("validate free quote: %w", err)
	}
	return prepared, nil
}

// CreateFreeQuoteResult contains the committed quote and observed balance.
type CreateFreeQuoteResult struct {
	OperationID     uuid.UUID
	QuoteMicros     int64
	QuoteDetail     json.RawMessage
	AvailableMicros int64
	ExpiresAt       time.Time
	ReusedFromID    *uuid.UUID
	Confirmable     bool
}

// FreeQuoteCreator persists one project-scoped free quote transaction.
type FreeQuoteCreator interface {
	CreateFreeQuote(context.Context, identityapp.Principal, CreateFreeQuoteInput) (CreateFreeQuoteResult, error)
}

// CreateFreeQuoteCommand validates a canvas quote request before persistence.
type CreateFreeQuoteCommand struct{ store FreeQuoteCreator }

// NewCreateFreeQuoteCommand injects the transactional quote store.
func NewCreateFreeQuoteCommand(store FreeQuoteCreator) *CreateFreeQuoteCommand {
	return &CreateFreeQuoteCommand{store: store}
}

// Execute creates an immutable quote without reserving any budget.
func (c *CreateFreeQuoteCommand) Execute(ctx context.Context, actor identityapp.Principal, input CreateFreeQuoteInput) (CreateFreeQuoteResult, error) {
	if len(input.Params) == 0 {
		input.Params = json.RawMessage(`{}`)
	}
	if c == nil || c.store == nil || input.Validate() != nil {
		return CreateFreeQuoteResult{}, ErrInvalidFreeQuote
	}
	result, err := c.store.CreateFreeQuote(ctx, actor, input)
	if err != nil {
		return CreateFreeQuoteResult{}, fmt.Errorf("create free quote: %w", err)
	}
	return result, nil
}
