package application

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ProviderTrialQuote requires an explicit project and separate customer/supplier ceilings.
// The quote is free; the existing confirmation endpoint reserves the project budget.
type ProviderTrialQuote struct {
	ProjectID         uuid.UUID         `json:"project_id"`
	Mode              string            `json:"mode"`
	Prompt            string            `json:"prompt"`
	Params            json.RawMessage   `json:"params" swaggertype:"object"`
	MediaInputs       []TrialMediaInput `json:"media_inputs"`
	MaxChargeMicros   int64             `json:"max_charge_micros"`
	SupplierMaxMicros int64             `json:"supplier_max_micros"`
	SupplierCurrency  string            `json:"supplier_currency"`
	RequestID         uuid.UUID         `json:"request_id"`
}

// TrialMediaInput uses the same project-owned assets and role validation as creation.
type TrialMediaInput struct {
	Role         string    `json:"role"`
	MediaAssetID uuid.UUID `json:"media_asset_id"`
}

// ProviderTrials is implemented by the transactional quote adapter.
type ProviderTrials interface {
	CreateProviderTrialQuote(context.Context, identityapp.Principal, uuid.UUID, ProviderTrialQuote) (CreateFreeQuoteResult, error)
}
