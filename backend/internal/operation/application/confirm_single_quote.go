package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

const (
	operationConfirmedTopic = "lanverse.operation.confirmed.v1"
	operationAuditTopic     = "lanverse.audit.recorded.v1"
)

var (
	// ErrInvalidConfirmSingleQuote means an internal confirmation lacks stable identities.
	ErrInvalidConfirmSingleQuote = errors.New("invalid single quote confirmation")
	// ErrConfirmationTargetUnavailable means the target has no lockable version source yet.
	ErrConfirmationTargetUnavailable = errors.New("confirmation target unavailable")
	// ErrQuoteStale means mutable facts changed since the quote was created.
	ErrQuoteStale = errors.New("quote stale")
	// ErrReuseSourceUnavailable means a frozen source can no longer be copied.
	ErrReuseSourceUnavailable = errors.New("reuse source unavailable")
	// ErrConfirmationKeyReused means one request key names a different confirmation.
	ErrConfirmationKeyReused = errors.New("confirmation idempotency key reused")
)

// ConfirmSingleQuoteInput identifies one quoted operation and the request trace.
type ConfirmSingleQuoteInput struct {
	ProjectID   uuid.UUID
	OperationID uuid.UUID
	RequestID   string
}

// Validate rejects missing IDs; the persistent idempotency layer is separate.
func (i ConfirmSingleQuoteInput) Validate() error {
	requestID, err := uuid.Parse(i.RequestID)
	if i.ProjectID == uuid.Nil || i.OperationID == uuid.Nil ||
		err != nil || requestID == uuid.Nil || requestID.String() != i.RequestID {
		return ErrInvalidConfirmSingleQuote
	}
	return nil
}

// ConfirmSingleQuoteResult contains only the committed reservation and balance.
type ConfirmSingleQuoteResult struct {
	ReservationID   uuid.UUID
	AvailableMicros int64
	BudgetRevision  int64
}

// SingleQuoteConfirmer rechecks current rights and quote facts, then commits
// reservation, operation status, and both events in one transaction.
type SingleQuoteConfirmer interface {
	ConfirmSingleQuote(context.Context, identityapp.Principal, ConfirmSingleQuoteInput) (ConfirmSingleQuoteResult, error)
}

// ConfirmSingleQuoteCommand is the internal use case for one quote.
type ConfirmSingleQuoteCommand struct{ store SingleQuoteConfirmer }

// NewConfirmSingleQuoteCommand injects the transactional confirmation store.
func NewConfirmSingleQuoteCommand(store SingleQuoteConfirmer) *ConfirmSingleQuoteCommand {
	return &ConfirmSingleQuoteCommand{store: store}
}

// Execute validates the request before the store rechecks durable actor rights.
func (c *ConfirmSingleQuoteCommand) Execute(ctx context.Context, actor identityapp.Principal, input ConfirmSingleQuoteInput) (ConfirmSingleQuoteResult, error) {
	if c == nil || c.store == nil {
		return ConfirmSingleQuoteResult{}, ErrInvalidConfirmSingleQuote
	}
	if err := input.Validate(); err != nil {
		return ConfirmSingleQuoteResult{}, err
	}
	result, err := c.store.ConfirmSingleQuote(ctx, actor, input)
	if err != nil {
		return ConfirmSingleQuoteResult{}, fmt.Errorf("confirm single quote: %w", err)
	}
	return result, nil
}

// SingleConfirmationEvents builds bounded durable workflow and audit messages.
// The adapter validates the audit envelope before writing either event.
func SingleConfirmationEvents(actor identityapp.Principal, operation domain.Operation, reservationID uuid.UUID, modelKey, requestID string, confirmedAt time.Time) ([]identityapp.OutboxEvent, error) {
	if operation.Validate() != nil || operation.Status != domain.StatusQuoted ||
		operation.BatchID != nil || operation.QuoteMicros == nil || operation.Region == nil ||
		actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || reservationID == uuid.Nil ||
		modelKey == "" || confirmedAt.IsZero() {
		return nil, ErrInvalidConfirmSingleQuote
	}
	input := ConfirmSingleQuoteInput{ProjectID: operation.ProjectID, OperationID: operation.ID, RequestID: requestID}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	confirmedID, auditID := uuid.New(), uuid.New()
	base := map[string]any{
		"occurred_at": confirmedAt.UTC(), "org_id": actor.OrgID,
		"project_id": operation.ProjectID,
		"actor":      map[string]any{"kind": "user", "id": actor.ID},
	}
	confirmed := make(map[string]any, len(base)+4)
	for key, value := range base {
		confirmed[key] = value
	}
	confirmed["event_id"], confirmed["event_type"] = confirmedID, operationConfirmedTopic
	confirmed["aggregate"] = map[string]any{"type": "operation", "id": operation.ID}
	confirmed["data"] = map[string]any{
		"operation_id": operation.ID, "reservation_id": reservationID,
		"quote_micros": *operation.QuoteMicros,
	}
	audit := make(map[string]any, len(base)+4)
	for key, value := range base {
		audit[key] = value
	}
	audit["event_id"], audit["event_type"] = auditID, operationAuditTopic
	audit["aggregate"] = map[string]any{"type": "audit", "id": auditID}
	after := map[string]any{
		"quote_micros": *operation.QuoteMicros, "model_key": modelKey,
		"region": *operation.Region, "origin": operation.Origin,
	}
	if operation.ReusedFromID != nil {
		after["reused_from"] = operation.ReusedFromID.String()
	}
	audit["data"] = map[string]any{
		"action":     "operation.confirmed",
		"object":     map[string]any{"type": "operation", "id": operation.ID},
		"request_id": requestID, "after": after,
	}
	confirmedPayload, err := json.Marshal(confirmed)
	if err != nil {
		return nil, fmt.Errorf("encode confirmed event: %w", err)
	}
	auditPayload, err := json.Marshal(audit)
	if err != nil {
		return nil, fmt.Errorf("encode confirmation audit: %w", err)
	}
	key := operation.ProjectID.String()
	return []identityapp.OutboxEvent{
		{ID: confirmedID, Topic: operationConfirmedTopic, PartitionKey: key, Payload: confirmedPayload},
		{ID: auditID, Topic: operationAuditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
