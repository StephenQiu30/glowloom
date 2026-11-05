package taskruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// WithReference decodes a module's immutable identifiers and bounds each
// execution. The module remains responsible for authoritative reads and writes.
func WithReference[T any](timeout time.Duration, run func(context.Context, Job, T) error) Handler {
	return func(ctx context.Context, job Job) (time.Time, error) {
		var ref T
		if err := json.Unmarshal(job.Reference, &ref); err != nil {
			return time.Time{}, fmt.Errorf("invalid %s reference: %w", job.Kind, err)
		}
		step, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return time.Time{}, run(step, job, ref)
	}
}
