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

// PurgeTopic carries only permanent media-owner purge command identities.
const PurgeTopic = "lanverse.media.purge_command.v1"

// PurgeInbox owns durable acknowledgement around external delivery.
type PurgeInbox interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// PurgeDeliveryStore proves the command and exact original outbox data.
type PurgeDeliveryStore interface {
	VerifyPurgeDelivery(context.Context, application.PurgeDelivery) (bool, error)
}

// PurgeStarter schedules an already proven exact physical attempt.
type PurgeStarter interface {
	Deliver(context.Context, application.PurgeDelivery) error
}

// PurgeHandler keeps injected records outside permanent acknowledgement.
type PurgeHandler struct {
	inbox   PurgeInbox
	store   PurgeDeliveryStore
	starter PurgeStarter
}

// NewPurgeHandler injects durable inbox, owning proof and Temporal delivery.
func NewPurgeHandler(inbox PurgeInbox, store PurgeDeliveryStore, starter PurgeStarter) *PurgeHandler {
	return &PurgeHandler{inbox: inbox, store: store, starter: starter}
}

// Handle acknowledges only a successfully delivered exact committed event.
func (h *PurgeHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.inbox == nil || h.store == nil || h.starter == nil {
		return application.ErrUnavailable
	}
	if record.Topic != PurgeTopic || len(record.Value) == 0 || len(record.Value) > 8192 {
		return domain.ErrInvalidLibrary
	}
	var body struct {
		EventID    uuid.UUID                 `json:"event_id"`
		EventType  string                    `json:"event_type"`
		OccurredAt time.Time                 `json:"occurred_at"`
		OrgID      uuid.UUID                 `json:"org_id"`
		Data       application.PurgeDelivery `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(record.Value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || body.EventID == uuid.Nil || body.EventID != body.Data.EventID || body.EventType != PurgeTopic || body.OrgID != body.Data.OrgID || body.OccurredAt.IsZero() || string(record.Key) != body.Data.JobID.String() {
		return domain.ErrInvalidLibrary
	}
	valid, err := h.store.VerifyPurgeDelivery(ctx, body.Data)
	if err != nil || !valid {
		return err
	}
	_, err = h.inbox.ProcessExternalOnce(ctx, "media-purge-command", body.EventID.String(), func(ctx context.Context) error {
		valid, err := h.store.VerifyPurgeDelivery(ctx, body.Data)
		if err != nil || !valid {
			return err
		}
		return h.starter.Deliver(ctx, body.Data)
	})
	return err
}
