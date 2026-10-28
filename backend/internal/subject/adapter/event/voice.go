// Package event proves committed Subject voice deliveries before orchestration.
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// Inbox acknowledges durable external delivery only after orchestration.
type Inbox interface {
	ProcessExternalOnce(context.Context, string, string, func(context.Context) error) (bool, error)
}

// DeliveryStore proves immutable admission and lists safe recovery work.
type DeliveryStore interface {
	VerifyDelivery(context.Context, application.VoiceDelivery) (bool, error)
	Recoverable(context.Context, int) ([]application.VoiceDelivery, error)
}

// Starter schedules only a previously proven delivery.
type Starter interface {
	Deliver(context.Context, application.VoiceDelivery) error
}

// VoiceHandler validates committed outbox evidence before starting work.
type VoiceHandler struct {
	inbox   Inbox
	store   DeliveryStore
	starter Starter
}

// NewVoiceHandler injects inbox ownership, job proof and a workflow starter.
func NewVoiceHandler(i Inbox, s DeliveryStore, start Starter) *VoiceHandler {
	return &VoiceHandler{inbox: i, store: s, starter: start}
}

// Handle rejects forged deliveries before permanent acknowledgement.
func (h *VoiceHandler) Handle(ctx context.Context, record inbox.Record) error {
	if h == nil || h.inbox == nil || h.store == nil || h.starter == nil {
		return application.ErrVoiceUnavailable
	}
	if record.Topic != application.VoiceTopic || len(record.Value) == 0 || len(record.Value) > 8192 {
		return application.ErrInvalidVoiceJob
	}
	var body struct {
		EventID    uuid.UUID                 `json:"event_id"`
		EventType  string                    `json:"event_type"`
		OccurredAt time.Time                 `json:"occurred_at"`
		Data       application.VoiceDelivery `json:"data"`
	}
	d := json.NewDecoder(bytes.NewReader(record.Value))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || d.Decode(new(any)) != io.EOF || body.EventID == uuid.Nil || body.EventID != body.Data.EventID || body.EventType != application.VoiceTopic || body.OccurredAt.IsZero() || string(record.Key) != body.Data.JobID.String() {
		return application.ErrInvalidVoiceJob
	}
	valid, err := h.store.VerifyDelivery(ctx, body.Data)
	if err != nil || !valid {
		return err
	}
	_, err = h.inbox.ProcessExternalOnce(ctx, "subject-voice", body.EventID.String(), func(ctx context.Context) error {
		valid, err := h.store.VerifyDelivery(ctx, body.Data)
		if err != nil || !valid {
			return err
		}
		return h.starter.Deliver(ctx, body.Data)
	})
	return err
}

// SweepOnce recovers missed admission or saved-receipt import without resubmission.
func (h *VoiceHandler) SweepOnce(ctx context.Context) error {
	if h == nil || h.store == nil || h.starter == nil {
		return application.ErrVoiceUnavailable
	}
	deliveries, err := h.store.Recoverable(ctx, 120)
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		valid, err := h.store.VerifyDelivery(ctx, d)
		if err != nil {
			return err
		}
		if valid {
			if err := h.starter.Deliver(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ inbox.Handler = (*VoiceHandler)(nil)
