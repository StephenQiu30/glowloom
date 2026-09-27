package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

// Replay returns events after lastEventID. A missing ID means the client must resync.
// The caller subscribes to project Pub/Sub before invoking Replay to cover the gap.
func (s *Sink) Replay(ctx context.Context, projectID, lastEventID string) ([]application.Message, bool, error) {
	if lastEventID == "" {
		return nil, false, nil
	}
	stream := "project:" + projectID + ":events"
	items, err := s.client.XRangeN(ctx, stream, "-", "+", maxEvents).Result()
	if err != nil {
		return nil, false, fmt.Errorf("read project replay buffer: %w", err)
	}
	start := -1
	for i, item := range items {
		// External effects can repeat after a database commit failure. Resume
		// from the first match so later unique events are never skipped.
		if id, ok := item.Values["id"].(string); ok && id == lastEventID && start < 0 {
			start = i
		}
	}
	if start < 0 {
		return nil, true, nil
	}
	events := make([]application.Message, 0, len(items)-start-1)
	seen := map[string]bool{lastEventID: true}
	for _, item := range items[start+1:] {
		id, idOK := item.Values["id"].(string)
		event, eventOK := item.Values["event"].(string)
		data, dataOK := item.Values["data"].(string)
		if !idOK || !eventOK || !dataOK || id == "" || event == "" || !json.Valid([]byte(data)) {
			return nil, false, fmt.Errorf("invalid replay entry %s", item.ID)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		events = append(events, application.Message{ID: id, Event: event, Data: json.RawMessage(data)})
	}
	return events, false, nil
}
