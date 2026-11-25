// Package event projects source-authorized event envelopes into Identity inboxes.
package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

const operationStatusTopic = "glowloom.operation.status_changed.v1"

type operationNotifications struct {
	next          inbox.Handler
	notifications *application.Notifications
}

// WithOperationNotifications preserves the existing realtime consumer effect,
// then projects completed/failed operations through the account-owned store.
// It adds no topic, queue or consumer group and accepts no public recipients.
func WithOperationNotifications(next inbox.Handler, notifications *application.Notifications) inbox.Handler {
	return &operationNotifications{next: next, notifications: notifications}
}

func (h *operationNotifications) Handle(ctx context.Context, record inbox.Record) error {
	if err := h.next.Handle(ctx, record); err != nil {
		return err
	}
	if record.Topic != operationStatusTopic {
		return nil
	}
	var envelope struct {
		EventID     uuid.UUID  `json:"event_id"`
		EventType   string     `json:"event_type"`
		OccurredAt  time.Time  `json:"occurred_at"`
		ProjectID   uuid.UUID  `json:"project_id"`
		RecipientID *uuid.UUID `json:"recipient_id"`
		Aggregate   struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregate"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Value, &envelope); err != nil {
		return fmt.Errorf("decode operation notification: %w", err)
	}
	if envelope.EventType != record.Topic || envelope.ProjectID.String() != string(record.Key) || envelope.Aggregate.Type != "operation" {
		return application.ErrInvalidNotification
	}
	if envelope.Data.Status != "completed" && envelope.Data.Status != "failed" {
		return nil
	}
	// Historical source envelopes did not declare recipients. Do not infer one
	// from the project, event actor or another owner's database.
	if envelope.RecipientID == nil {
		return nil
	}
	return h.notifications.ProjectTask(ctx, application.TaskNotificationInput{EventID: envelope.EventID, RecipientID: *envelope.RecipientID, OperationID: envelope.Aggregate.ID, Status: envelope.Data.Status, OccurredAt: envelope.OccurredAt})
}
