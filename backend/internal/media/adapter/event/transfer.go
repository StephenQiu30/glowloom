// Package event proves permanent media commands before external orchestration.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// TransferTopic carries only permanent media-owner transfer command identities.
const TransferTopic = "lanverse.media.transfer_command.v1"

// TransferInbox owns durable acknowledgement around external delivery.
type TransferInbox interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// TransferDeliveryStore proves the command and exact original outbox data.
type TransferDeliveryStore interface {
	VerifyTransferDelivery(context.Context, application.TransferDelivery) (bool, error)
}

// TransferStarter schedules an already proven exact physical attempt.
type TransferStarter interface {
	Deliver(context.Context, application.TransferDelivery) error
}

// TransferHandler keeps injected records outside permanent acknowledgement.
type TransferHandler struct {
	inbox   TransferInbox
	store   TransferDeliveryStore
	starter TransferStarter
}

// NewTransferHandler injects durable inbox, owning proof and Temporal delivery.
func NewTransferHandler(inbox TransferInbox, store TransferDeliveryStore, starter TransferStarter) *TransferHandler {
	return &TransferHandler{inbox: inbox, store: store, starter: starter}
}

// Handle acknowledges only a successfully delivered exact committed event.
func (h *TransferHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.inbox == nil || h.store == nil || h.starter == nil {
		return application.ErrUnavailable
	}
	if record.Topic != TransferTopic || len(record.Value) == 0 || len(record.Value) > 8192 {
		return domain.ErrInvalidLibrary
	}
	var body struct {
		EventID    uuid.UUID                    `json:"event_id"`
		EventType  string                       `json:"event_type"`
		OccurredAt time.Time                    `json:"occurred_at"`
		OrgID      uuid.UUID                    `json:"org_id"`
		Data       application.TransferDelivery `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(record.Value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || body.EventID == uuid.Nil || body.EventID != body.Data.EventID || body.EventType != TransferTopic || body.OrgID != body.Data.OrgID || body.OccurredAt.IsZero() || string(record.Key) != body.Data.JobID.String() {
		return domain.ErrInvalidLibrary
	}
	valid, err := h.store.VerifyTransferDelivery(ctx, body.Data)
	if err != nil || !valid {
		return err
	}
	_, err = h.inbox.ProcessExternalOnce(ctx, "media-transfer-command", body.EventID.String(), func(ctx context.Context) error {
		valid, err := h.store.VerifyTransferDelivery(ctx, body.Data)
		if err != nil || !valid {
			return err
		}
		return h.starter.Deliver(ctx, body.Data)
	})
	return err
}
