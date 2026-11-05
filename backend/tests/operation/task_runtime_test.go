package operation_test

import (
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestTaskQueuesHaveDistinctOwners(t *testing.T) {
	queues := []struct {
		kind     string
		register func(map[string]taskruntime.Handler)
	}{
		{"operation.run", func(h map[string]taskruntime.Handler) { task.RegisterFlow(h, nil, nil, nil) }},
		{"operation.provider", func(h map[string]taskruntime.Handler) { task.RegisterProvider(h, nil, nil) }},
		{"operation.codex", func(h map[string]taskruntime.Handler) { task.RegisterCodex(h, nil, nil) }},
		{"operation.ingest", func(h map[string]taskruntime.Handler) { task.RegisterMedia(h, nil, nil, nil) }},
	}
	seen := map[string]bool{}
	for _, q := range queues {
		h := map[string]taskruntime.Handler{}
		q.register(h)
		if h[q.kind] == nil {
			t.Fatal(q.kind)
		}
		for k := range h {
			if seen[k] {
				t.Fatalf("duplicate task queue for %s", k)
			}
			seen[k] = true
		}
	}
}
