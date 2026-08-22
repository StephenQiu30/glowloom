package runtime

import (
	"context"
	"log/slog"
	"time"
)

// RunIdleWorker gives each backend role an explicit process boundary while its
// module handlers are enabled. It does not invent a second queue: enabled
// roles consume the same Kafka cluster through their module-specific handler.
func RunIdleWorker(ctx context.Context, role string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	slog.Info("lanverse worker started", "role", role)
	for {
		select {
		case <-ctx.Done():
			slog.Info("lanverse worker stopped", "role", role)
			return
		case <-ticker.C:
			slog.Debug("lanverse worker heartbeat", "role", role)
		}
	}
}
