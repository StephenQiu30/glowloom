package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// ErrInvalidBatchFreeQuote means a batch request has no stable scope or size.
var ErrInvalidBatchFreeQuote = errors.New("invalid batch free quote request")

// CreateBatchFreeQuoteInput holds ordered canvas items under one request key.
type CreateBatchFreeQuoteInput struct {
	ProjectID uuid.UUID
	RequestID string
	Items     []FreeQuoteItemInput
}

// Validate checks the envelope. Individual item failures are reported in order.
func (i CreateBatchFreeQuoteInput) Validate() error {
	requestID, err := uuid.Parse(i.RequestID)
	if i.ProjectID == uuid.Nil || err != nil || requestID == uuid.Nil ||
		requestID.String() != i.RequestID || len(i.Items) < 2 || len(i.Items) > 150 {
		return ErrInvalidBatchFreeQuote
	}
	return nil
}

// BatchFreeQuoteItemResult maps to one requested item at the same position.
type BatchFreeQuoteItemResult struct {
	OperationID  *uuid.UUID      `json:"operation_id,omitempty"`
	ModelKey     string          `json:"model_key"`
	Mode         string          `json:"mode"`
	QuoteMicros  *int64          `json:"quote_micros,omitempty"`
	QuoteDetail  json.RawMessage `json:"quote_detail,omitempty"`
	ReusedFromID *uuid.UUID      `json:"reused_from_id,omitempty"`
	Region       string          `json:"region,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
}

// CreateBatchFreeQuoteResult is the committed selection and observed balance.
type CreateBatchFreeQuoteResult struct {
	BatchID         *uuid.UUID                 `json:"batch_id,omitempty"`
	ExpiresAt       time.Time                  `json:"expires_at"`
	Items           []BatchFreeQuoteItemResult `json:"items"`
	TotalMicros     int64                      `json:"total_micros"`
	AvailableMicros int64                      `json:"available_micros"`
	Confirmable     bool                       `json:"confirmable"`
}

// BatchFreeQuoteCreator persists the valid items and their request outcome.
type BatchFreeQuoteCreator interface {
	CreateBatchFreeQuote(context.Context, identityapp.Principal, CreateBatchFreeQuoteInput) (CreateBatchFreeQuoteResult, error)
}

// CreateBatchFreeQuoteCommand validates the envelope before persistence.
type CreateBatchFreeQuoteCommand struct{ store BatchFreeQuoteCreator }

// NewCreateBatchFreeQuoteCommand injects the batch quote store.
func NewCreateBatchFreeQuoteCommand(store BatchFreeQuoteCreator) *CreateBatchFreeQuoteCommand {
	return &CreateBatchFreeQuoteCommand{store: store}
}

// Execute records one batch quote or a stable all-invalid item result.
func (c *CreateBatchFreeQuoteCommand) Execute(ctx context.Context, actor identityapp.Principal, input CreateBatchFreeQuoteInput) (CreateBatchFreeQuoteResult, error) {
	if c == nil || c.store == nil || input.Validate() != nil {
		return CreateBatchFreeQuoteResult{}, ErrInvalidBatchFreeQuote
	}
	result, err := c.store.CreateBatchFreeQuote(ctx, actor, input)
	if err != nil {
		return CreateBatchFreeQuoteResult{}, fmt.Errorf("create batch free quote: %w", err)
	}
	return result, nil
}
