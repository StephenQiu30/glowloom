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
	ReadProviderTrialRecovery(context.Context, identityapp.Principal, uuid.UUID) (ProviderTrialRecovery, error)
}

// ProviderTrialRecovery restores only the requesting administrator's durable trial.
type ProviderTrialRecovery struct {
	Found             bool      `json:"found"`
	OperationID       uuid.UUID `json:"operation_id"`
	ProjectID         uuid.UUID `json:"project_id"`
	RequestID         uuid.UUID `json:"request_id"`
	ConfirmRequestID  uuid.UUID `json:"confirm_request_id"`
	Status            string    `json:"status"`
	MaxChargeMicros   int64     `json:"max_charge_micros"`
	SupplierMaxMicros int64     `json:"supplier_max_micros"`
	SupplierCurrency  string    `json:"supplier_currency"`
}

// ProviderTrialConfirmKey is stable across refreshes; it is an idempotency ID, not a credential.
func ProviderTrialConfirmKey(operation uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(operation, []byte("provider-trial-confirmation"))
}
