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

const batchConfirmedTopic = "lanverse.batch.confirmed.v1"

// ErrInvalidConfirmBatchQuote means a batch confirmation lacks stable identities.
var ErrInvalidConfirmBatchQuote = errors.New("invalid batch quote confirmation")

// ConfirmBatchQuoteInput identifies a quoted batch and the items explicitly removed by its user.
type ConfirmBatchQuoteInput struct {
	ProjectID           uuid.UUID
	BatchID             uuid.UUID
	ExcludeOperationIDs []uuid.UUID
	RequestID           string
}

// Validate rejects missing, duplicate, and malformed request identities.
func (i ConfirmBatchQuoteInput) Validate() error {
	requestID, err := uuid.Parse(i.RequestID)
	if i.ProjectID == uuid.Nil || i.BatchID == uuid.Nil || err != nil ||
		requestID == uuid.Nil || requestID.String() != i.RequestID || len(i.ExcludeOperationIDs) > 300 {
		return ErrInvalidConfirmBatchQuote
	}
	seen := make(map[uuid.UUID]bool, len(i.ExcludeOperationIDs))
	for _, id := range i.ExcludeOperationIDs {
		if id == uuid.Nil || seen[id] {
			return ErrInvalidConfirmBatchQuote
		}
		seen[id] = true
	}
	return nil
}

// BatchConfirmationItem records one selected or removed quote without losing its batch identity.
type BatchConfirmationItem struct {
	OperationID uuid.UUID
	Status      domain.Status
	Reasons     []string
}

// ConfirmBatchQuoteResult includes committed expired items on a rejected batch
// and the reserved amount and balance after a successful confirmation.
type ConfirmBatchQuoteResult struct {
	Items            []BatchConfirmationItem
	ConfirmedCount   int32
	QuoteTotalMicros int64
	AvailableMicros  int64
	BudgetRevision   int64
}

// BatchQuoteConfirmer persists all selected reservations and the parent event together.
type BatchQuoteConfirmer interface {
	ConfirmBatchQuote(context.Context, identityapp.Principal, ConfirmBatchQuoteInput) (ConfirmBatchQuoteResult, error)
}

// ConfirmBatchQuoteCommand is the internal use case for a quoted batch.
type ConfirmBatchQuoteCommand struct{ store BatchQuoteConfirmer }

// NewConfirmBatchQuoteCommand injects the transactional batch store.
func NewConfirmBatchQuoteCommand(store BatchQuoteConfirmer) *ConfirmBatchQuoteCommand {
	return &ConfirmBatchQuoteCommand{store: store}
}

// Execute validates the request before the store rechecks durable actor rights.
func (c *ConfirmBatchQuoteCommand) Execute(ctx context.Context, actor identityapp.Principal, input ConfirmBatchQuoteInput) (ConfirmBatchQuoteResult, error) {
	if c == nil || c.store == nil {
		return ConfirmBatchQuoteResult{}, ErrInvalidConfirmBatchQuote
	}
	if err := input.Validate(); err != nil {
		return ConfirmBatchQuoteResult{}, err
	}
	result, err := c.store.ConfirmBatchQuote(ctx, actor, input)
	if err != nil {
		return result, fmt.Errorf("confirm batch quote: %w", err)
	}
	return result, nil
}

// BatchConfirmationEvents emits one parent starter event and one bounded user audit.
func BatchConfirmationEvents(actor identityapp.Principal, batchID, projectID uuid.UUID,
	totalCount int32, totalMicros int64, expiredCount int, requestID string, confirmedAt time.Time,
) ([]identityapp.OutboxEvent, error) {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || batchID == uuid.Nil || projectID == uuid.Nil ||
		totalCount < 1 || totalCount > 300 || totalMicros < 0 || expiredCount < 0 ||
		confirmedAt.IsZero() {
		return nil, ErrInvalidConfirmBatchQuote
	}
	if err := (ConfirmBatchQuoteInput{ProjectID: projectID, BatchID: batchID, RequestID: requestID}).Validate(); err != nil {
		return nil, err
	}
	confirmedID, auditID := uuid.New(), uuid.New()
	base := map[string]any{
		"occurred_at": confirmedAt.UTC(), "org_id": actor.OrgID, "project_id": projectID,
		"actor": map[string]any{"kind": "user", "id": actor.ID},
	}
	confirmed := make(map[string]any, len(base)+4)
	audit := make(map[string]any, len(base)+4)
	for key, value := range base {
		confirmed[key], audit[key] = value, value
	}
	confirmed["event_id"], confirmed["event_type"] = confirmedID, batchConfirmedTopic
	confirmed["aggregate"] = map[string]any{"type": "batch", "id": batchID}
	confirmed["data"] = map[string]any{
		"batch_id": batchID, "total_count": totalCount, "quote_total_micros": totalMicros,
	}
	audit["event_id"], audit["event_type"] = auditID, operationAuditTopic
	audit["aggregate"] = map[string]any{"type": "audit", "id": auditID}
	audit["data"] = map[string]any{
		"action": "batch.confirmed", "object": map[string]any{"type": "batch", "id": batchID},
		"request_id": requestID,
		"after": map[string]any{
			"quote_total_micros": totalMicros, "total_count": totalCount,
			"expired_count": expiredCount,
		},
	}
	confirmedPayload, err := json.Marshal(confirmed)
	if err != nil {
		return nil, fmt.Errorf("encode batch confirmed event: %w", err)
	}
	auditPayload, err := json.Marshal(audit)
	if err != nil {
		return nil, fmt.Errorf("encode batch confirmation audit: %w", err)
	}
	key := projectID.String()
	return []identityapp.OutboxEvent{
		{ID: confirmedID, Topic: batchConfirmedTopic, PartitionKey: key, Payload: confirmedPayload},
		{ID: auditID, Topic: operationAuditTopic, PartitionKey: key, Payload: auditPayload},
	}, nil
}
