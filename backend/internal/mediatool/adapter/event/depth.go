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

// DepthTopic carries exact committed depth commands without private file paths.
const DepthTopic = "lanverse.mediatool.depth_command.v1"

// DepthDeliveryStore proves immutable command and original outbox data before delivery.
type DepthDeliveryStore interface {
	VerifyDelivery(context.Context, application.DepthDelivery) (bool, error)
}

// DepthStarter schedules only a proven physical attempt or saved control command.
type DepthStarter interface {
	Deliver(context.Context, application.DepthDelivery) error
}

// DepthHandler keeps injected records outside permanent inbox acknowledgement.
type DepthHandler struct {
	processed ProcessedStore
	store     DepthDeliveryStore
	starter   DepthStarter
}

// NewDepthHandler injects permanent inbox, owning proof and Temporal delivery.
func NewDepthHandler(p ProcessedStore, s DepthDeliveryStore, start DepthStarter) *DepthHandler {
	return &DepthHandler{processed: p, store: s, starter: start}
}

// Handle rechecks delivery within inbox serialization and acknowledges only success.
func (h *DepthHandler) Handle(ctx context.Context, r inbox.Record) error {
	if h == nil || h.processed == nil || h.store == nil || h.starter == nil || r.Topic != DepthTopic || len(r.Value) == 0 || len(r.Value) > 8192 {
		return application.ErrInvalidDepthInput
	}
	var body struct {
		EventID    uuid.UUID                 `json:"event_id"`
		EventType  string                    `json:"event_type"`
		OccurredAt time.Time                 `json:"occurred_at"`
		OrgID      uuid.UUID                 `json:"org_id"`
		Data       application.DepthDelivery `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(r.Value))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF || body.EventID == uuid.Nil || body.EventID != body.Data.EventID || body.OrgID != body.Data.OrgID || body.EventType != DepthTopic || body.OccurredAt.IsZero() || string(r.Key) != body.Data.JobID.String() {
		return application.ErrInvalidDepthInput
	}
	ok, err := h.store.VerifyDelivery(ctx, body.Data)
	if err != nil || !ok {
		return err
	}
	_, err = h.processed.ProcessExternalOnce(ctx, "media-depth-command", body.EventID.String(), func(ctx context.Context) error {
		ok, err := h.store.VerifyDelivery(ctx, body.Data)
		if err != nil || !ok {
			return err
		}
		return h.starter.Deliver(ctx, body.Data)
	})
	return err
}
