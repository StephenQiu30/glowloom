package application

import (
	"context"
	"time"
)

// StreamWatchdog monitors a StreamChunk channel and terminates stalled streams if idle for too long.
type StreamWatchdog struct {
	idleTimeout time.Duration
}

// NewStreamWatchdog creates a watchdog with the given idle timeout (default 45s).
func NewStreamWatchdog(idleTimeout time.Duration) *StreamWatchdog {
	if idleTimeout <= 0 {
		idleTimeout = 45 * time.Second
	}
	return &StreamWatchdog{idleTimeout: idleTimeout}
}

// Watch wraps an input stream channel with idle timeout detection.
func (w *StreamWatchdog) Watch(ctx context.Context, in <-chan StreamChunk) <-chan StreamChunk {
	out := make(chan StreamChunk, 16)
	go func() {
		defer close(out)
		timer := time.NewTimer(w.idleTimeout)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			case <-timer.C:
				// Idle timeout reached with no chunks received
				out <- StreamChunk{Error: ErrStreamStalled}
				return
			case chunk, ok := <-in:
				if !ok {
					return
				}
				// Reset idle timer upon receiving any chunk
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.idleTimeout)

				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
