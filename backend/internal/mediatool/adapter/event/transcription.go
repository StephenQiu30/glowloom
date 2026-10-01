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

// TranscriptionTopic carries server-issued speech job identities, never URLs.
const TranscriptionTopic = "lanverse.mediatool.transcription_command.v1"

// TranscriptionDeliveryStore proves a command receipt and fences delayed attempt events.
type TranscriptionDeliveryStore interface {
	VerifyDelivery(context.Context, application.TranscriptionDelivery) (bool, error)
}

// TranscriptionStarter starts or signals the one exact committed speech attempt.
type TranscriptionStarter interface {
	Deliver(context.Context, application.TranscriptionDelivery) error
}

// TranscriptionHandler adapts verified outbox events to the existing inbox relay.
type TranscriptionHandler struct {
	processed ProcessedStore
	store     TranscriptionDeliveryStore
	starter   TranscriptionStarter
}

// NewTranscriptionHandler injects durable delivery proof and the configured Temporal client.
func NewTranscriptionHandler(processed ProcessedStore, store TranscriptionDeliveryStore, starter TranscriptionStarter) *TranscriptionHandler {
	return &TranscriptionHandler{processed: processed, store: store, starter: starter}
}

// Handle rejects injected events and marks delivery only after scheduling succeeds.
func (h *TranscriptionHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil || record.Topic != TranscriptionTopic || len(record.Value) == 0 || len(record.Value) > 8192 {
		return application.ErrInvalidTranscription
	}
	var body struct {
		EventID    uuid.UUID                         `json:"event_id"`
		EventType  string                            `json:"event_type"`
		OccurredAt time.Time                         `json:"occurred_at"`
		OrgID      uuid.UUID                         `json:"org_id"`
		Data       application.TranscriptionDelivery `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(record.Value))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF || body.EventType != TranscriptionTopic || body.EventID != body.Data.EventID || body.OrgID != body.Data.OrgID || body.OccurredAt.IsZero() || string(record.Key) != body.OrgID.String() {
		return application.ErrInvalidTranscription
	}
	_, err := h.processed.ProcessExternalOnce(ctx, "media-transcription-command", body.EventID.String(), func(ctx context.Context) error {
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
