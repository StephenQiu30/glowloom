// Package event verifies durable local processing commands before Temporal delivery.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// Topic carries server-issued export and cancellation identities, never URLs.
const Topic = "lanverse.mediatool.export_command.v1"

// ProcessedStore owns existing durable external-delivery deduplication.
type ProcessedStore interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// DeliveryStore proves a command receipt and fences delayed attempt events.
type DeliveryStore interface {
	VerifyDelivery(context.Context, application.Delivery) (bool, error)
}

// Starter starts or signals the one exact committed export attempt.
type Starter interface {
	Deliver(context.Context, application.Delivery) error
}

// Handler adapts verified outbox events to the existing inbox relay.
type Handler struct {
	processed ProcessedStore
	store     DeliveryStore
	starter   Starter
}

// NewHandler injects durable delivery proof and the configured Temporal client.
func NewHandler(processed ProcessedStore, store DeliveryStore, starter Starter) *Handler {
	return &Handler{processed: processed, store: store, starter: starter}
}

// Handle rejects injected events and marks delivery only after scheduling succeeds.
func (h *Handler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil || record.Topic != Topic || len(record.Value) == 0 || len(record.Value) > 8192 {
		return application.ErrInvalidExport
	}
	var body struct {
		EventID    uuid.UUID            `json:"event_id"`
		EventType  string               `json:"event_type"`
		OccurredAt time.Time            `json:"occurred_at"`
		OrgID      uuid.UUID            `json:"org_id"`
		Data       application.Delivery `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(record.Value))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF || body.EventType != Topic || body.EventID != body.Data.EventID || body.OrgID != body.Data.OrgID || body.OccurredAt.IsZero() || string(record.Key) != body.OrgID.String() {
		return application.ErrInvalidExport
	}
	_, err := h.processed.ProcessExternalOnce(ctx, "media-export-command", body.EventID.String(), func(ctx context.Context) error {
		allowed, err := h.store.VerifyDelivery(ctx, body.Data)
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
		return h.starter.Deliver(ctx, body.Data)
	})
	return err
}
