package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

func TestWalletBalanceCacheRejectsStaleVersionsAndLimiterEnforcesCallerQuota(t *testing.T) {
	address := os.Getenv("GLOWLOOM_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("set GLOWLOOM_TEST_REDIS_ADDR to a disposable or isolated local Redis endpoint")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := redisclient.NewClient(&redisclient.Options{Addr: address})
	defer func() { _ = client.Close() }()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect Redis: %v", err)
	}

	scopeID := uuid.New()
	cache := NewWalletBalanceCache(client)
	defer func() { _ = client.Del(context.Background(), balanceKey(scopeID), versionKey(scopeID)).Err() }()
	versionSeven := application.WalletBalance{Enabled: true, Version: 7, GeneralCredits: "10", HeldCredits: "1"}
	if err := cache.Put(ctx, scopeID, versionSeven); err != nil {
		t.Fatalf("cache version 7 balance: %v", err)
	}
	if err := cache.Invalidate(ctx, scopeID, 8); err != nil {
		t.Fatalf("invalidate through version 8: %v", err)
	}
	if _, found, err := cache.Get(ctx, scopeID); err != nil || found {
		t.Fatalf("stale balance cache = (found %t, error %v), want miss", found, err)
	}
	if err := cache.Put(ctx, scopeID, versionSeven); err != nil {
		t.Fatalf("write stale balance: %v", err)
	}
	if _, found, err := cache.Get(ctx, scopeID); err != nil || found {
		t.Fatalf("stale write bypassed version watermark = (found %t, error %v)", found, err)
	}
	versionNine := versionSeven
	versionNine.Version = 9
	versionNine.GeneralCredits = "8"
	if err := cache.Put(ctx, scopeID, versionNine); err != nil {
		t.Fatalf("cache version 9 balance: %v", err)
	}
	if err := cache.Invalidate(ctx, scopeID, 8); err != nil {
		t.Fatalf("replay older invalidation: %v", err)
	}
	cached, found, err := cache.Get(ctx, scopeID)
	if err != nil || !found || cached.Version != 9 || cached.GeneralCredits != "8" {
		t.Fatalf("version 9 balance after old event = (%+v, found %t, error %v)", cached, found, err)
	}

	limiter := NewCreditCommandLimiter(client, 2, 1000)
	callerID := "redis-limit-" + uuid.NewString()
	for attempt := 1; attempt <= 3; attempt++ {
		wait, err := limiter.AllowCreditCommand(ctx, callerID, scopeID, uuid.NewString(), 1000, time.Now().UTC())
		if err != nil {
			t.Fatalf("allow credit command attempt %d: %v", attempt, err)
		}
		if attempt < 3 && wait != 0 {
			t.Fatalf("attempt %d was limited for %s", attempt, wait)
		}
		if attempt == 3 && wait <= 0 {
			t.Fatalf("attempt %d was not limited", attempt)
		}
	}
}
