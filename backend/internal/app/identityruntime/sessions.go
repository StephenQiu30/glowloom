package identityruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/refreshredis"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// SessionDependencies supplies browser credential security through existing ports.
type SessionDependencies struct {
	Tokens      application.AccessTokenCodec
	Receipts    application.RefreshReceiptProtector
	Coordinator application.RefreshCoordinator
}

// NewSessionDependencies loads configured security and reuses the API Redis client.
func NewSessionDependencies(cfg config.Config, client *redis.Client) (SessionDependencies, error) {
	var deps SessionDependencies
	if err := cfg.ValidateIdentitySessionV2(); err != nil {
		return deps, err
	}
	if cfg.IdentityJWTVerifyKeys == "" {
		return deps, nil
	}
	tokens, err := sessiontoken.LoadValues(cfg.IdentityJWTPrivateKey, cfg.IdentityJWTVerifyKeys, cfg.IdentityJWTActiveKID)
	if err != nil {
		return deps, err
	}
	deps.Tokens = tokens
	if cfg.IdentityRefreshReceiptKey != "" {
		deps.Receipts, err = sessiontoken.LoadReceiptValue(cfg.IdentityRefreshReceiptKey)
		if err != nil {
			return SessionDependencies{}, err
		}
	}
	deps.Coordinator, err = refreshredis.New(client)
	if err != nil {
		return SessionDependencies{}, err
	}
	return deps, nil
}

// ConfigureSessionSecurity keeps verification during rollback and gates new issuance.
func ConfigureSessionSecurity(auth *application.Auth, cfg config.Config, deps SessionDependencies) error {
	if auth == nil {
		return fmt.Errorf("session authentication required")
	}
	auth.WithLegacyUpgradeUntil(cfg.IdentityLegacyUpgradeUntil)
	if deps.Tokens == nil && deps.Receipts == nil && deps.Coordinator == nil && !cfg.IdentitySessionV2Enabled {
		return nil
	}
	if deps.Tokens == nil || deps.Coordinator == nil || (cfg.IdentitySessionV2Enabled && deps.Receipts == nil) {
		return fmt.Errorf("session security dependencies required")
	}
	auth.WithSessionSecurity(deps.Tokens, deps.Receipts, deps.Coordinator).WithSessionIssuance(cfg.IdentitySessionV2Enabled)
	return nil
}

// StartSessionReceiptCleanupLoop owns bounded cleanup and joins it before DB close.
// Full batches drain within a shared <=10-second, <=32-batch tick budget. Errors
// and exhausted budgets wait for the next scheduled tick instead of hot retrying.
// The 60-second retention target requires measured capacity; outages or overload
// can exceed it. Backlog observations require operational capacity validation.
func StartSessionReceiptCleanupLoop(ctx context.Context, cleanup *application.SessionReceiptCleanup, logger *zap.Logger, interval time.Duration) (func(), error) {
	if cleanup == nil || logger == nil || interval <= 0 || interval > time.Minute {
		return nil, fmt.Errorf("receipt cleanup requires runner, logger and interval at most one minute")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if runCtx.Err() != nil {
					return
				}
				drainReceiptCleanup(runCtx, cleanup, logger, min(interval, 10*time.Second))
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}

// drainReceiptCleanup emits only fixed classifications, never a raw store error.
func drainReceiptCleanup(ctx context.Context, cleanup *application.SessionReceiptCleanup, logger *zap.Logger, budget time.Duration) {
	cycleCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	processed, batches := 0, 0
	reason, class := "batch_limit", "none"
	for batches < 32 {
		if cycleCtx.Err() != nil {
			reason, class = "budget_exhausted", cleanupErrorClass(cycleCtx.Err())
			break
		}
		n, err := cleanup.RunOnce(cycleCtx, 1000)
		batches++
		if err != nil {
			reason, class = "failed", cleanupErrorClass(err)
			if errors.Is(err, context.DeadlineExceeded) {
				reason = "budget_exhausted"
			}
			break
		}
		processed += n
		if n < 1000 {
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	logger.Warn("identity receipt cleanup incomplete", zap.String("reason", reason), zap.String("error_class", class), zap.Bool("backlog", processed > 0), zap.Int("processed", processed), zap.Int("batches", batches))
}

func cleanupErrorClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	default:
		return "store"
	}
}
