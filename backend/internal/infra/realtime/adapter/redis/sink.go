// Package redis stores short SSE replay buffers and publishes project events.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

const (
	replayWindow = 10 * time.Minute
	maxEvents    = 5000
)

// Sink uses the caller-owned Redis client.
type Sink struct {
	client *redisclient.Client
}

// NewSink constructs a project-scoped event sink.
func NewSink(client *redisclient.Client) *Sink {
	return &Sink{client: client}
}

// Publish stores a replayable event before publishing the same payload.
func (s *Sink) Publish(ctx context.Context, event application.Event) error {
	data, err := json.Marshal(struct {
		OperationID string          `json:"operation_id"`
		BatchID     string          `json:"batch_id,omitempty"`
		TargetType  string          `json:"target_type"`
		TargetID    string          `json:"target_id,omitempty"`
		Status      string          `json:"status"`
		Progress    json.RawMessage `json:"progress,omitempty"`
	}{event.OperationID, event.BatchID, event.TargetType, event.TargetID, event.Status, event.Progress})
	if err != nil {
		return fmt.Errorf("encode realtime event data: %w", err)
	}
	payload, err := json.Marshal(struct {
		ID    string          `json:"id"`
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}{event.ID, event.Type, data})
	if err != nil {
		return fmt.Errorf("encode realtime event: %w", err)
	}
	channel := "project:" + event.ProjectID
	stream := channel + ":events"
	cutoff := fmt.Sprintf("%d-0", time.Now().Add(-replayWindow).UnixMilli())
	pipe := s.client.TxPipeline()
	pipe.XAdd(ctx, &redisclient.XAddArgs{Stream: stream, Values: map[string]any{
		"id": event.ID, "event": event.Type, "data": string(data),
	}})
	pipe.XTrimMinID(ctx, stream, cutoff)
	pipe.XTrimMaxLen(ctx, stream, maxEvents)
	pipe.Expire(ctx, stream, replayWindow)
	pipe.Publish(ctx, channel, payload)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store and publish realtime event %s: %w", event.ID, err)
	}
	return nil
}

var _ application.Sink = (*Sink)(nil)
