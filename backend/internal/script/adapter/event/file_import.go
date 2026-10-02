// Package event delivers only exact permanent script commands through the existing inbox.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ImportTopic is the single persistent file-import command topic.
const ImportTopic = "lanverse.script.import_command.v1"

// ProcessedStore serializes successful external delivery with permanent inbox evidence.
type ProcessedStore interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// ImportDeliveryStore proves exact original immutable command/outbox facts.
type ImportDeliveryStore interface {
	VerifyImportDelivery(context.Context, application.ImportDelivery) (bool, error)
}

// ImportStarter schedules only an owning, proven work or control command.
type ImportStarter interface {
	Deliver(context.Context, application.ImportDelivery) error
}

// ImportHandler rejects spoofed or ambiguous envelopes before inbox acknowledgement.
type ImportHandler struct {
	processed ProcessedStore
	store     ImportDeliveryStore
	starter   ImportStarter
}

// NewImportHandler injects the actual inbox, owning delivery proof and Temporal starter.
func NewImportHandler(p ProcessedStore, s ImportDeliveryStore, start ImportStarter) *ImportHandler {
	return &ImportHandler{processed: p, store: s, starter: start}
}

// Handle checks typed closed JSON and verifies current attempt under inbox serialization.
func (h *ImportHandler) Handle(ctx context.Context, r inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil || r.Topic != ImportTopic || len(r.Value) == 0 || len(r.Value) > 8192 {
		return domain.ErrInvalidSource
	}
	if err := domain.ValidateJSON(r.Value); err != nil {
		return err
	}
	var envelope struct {
		EventID    uuid.UUID                  `json:"event_id"`
		EventType  string                     `json:"event_type"`
		OccurredAt time.Time                  `json:"occurred_at"`
		OrgID      uuid.UUID                  `json:"org_id"`
		Data       application.ImportDelivery `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(r.Value))
	dec.DisallowUnknownFields()
	if dec.Decode(&envelope) != nil || dec.Decode(new(any)) != io.EOF || envelope.EventID != envelope.Data.EventID || envelope.EventType != ImportTopic || envelope.OrgID != envelope.Data.OrgID || envelope.OccurredAt.IsZero() || string(r.Key) != envelope.Data.ProjectID.String() {
		return domain.ErrInvalidSource
	}
	ok, err := h.store.VerifyImportDelivery(ctx, envelope.Data)
	if err != nil || !ok {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "script-import-command", envelope.EventID.String(), func(ctx context.Context) error {
		ok, err := h.store.VerifyImportDelivery(ctx, envelope.Data)
		if err != nil || !ok {
			return err
		}
		return h.starter.Deliver(ctx, envelope.Data)
	})
	return err
}
