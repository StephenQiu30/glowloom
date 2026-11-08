package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
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
	cleanupKeys := []string{}
	defer func() {
		if len(cleanupKeys) > 0 {
			_ = client.Del(context.Background(), cleanupKeys...).Err()
		}
		_ = client.Close()
	}()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect Redis: %v", err)
	}

	scopeID := uuid.New()
	cache := NewWalletBalanceCache(client)
	cleanupKeys = append(cleanupKeys, balanceKey(scopeID), versionKey(scopeID))
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

	limiter := NewCreditCommandLimiter(client, 2, 1000, 10000)
	callerID := "redis-limit-" + uuid.NewString()
	now := time.Now().UTC()
	window := now.Format("200601021504")
	day := now.Format("20060102")
	requestIPs := make([]string, 3)
	keys := []string{
		"credit:limit:command:caller:" + digest(callerID) + ":" + window,
		"credit:limit:command:scope:" + digest(scopeID.String()) + ":" + window,
		"credit:limit:command:daily:caller:" + digest(callerID) + ":" + day,
		"credit:limit:command:daily:scope:" + digest(scopeID.String()) + ":" + day,
	}
	cleanupKeys = append(cleanupKeys, keys...)
	for attempt := 1; attempt <= 3; attempt++ {
		requestIPs[attempt-1] = fmt.Sprintf("redis-limit-ip-%d-%s", attempt, uuid.NewString())
		keys = append(keys, "credit:limit:command:ip:"+digest(requestIPs[attempt-1])+":"+window)
		cleanupKeys = append(cleanupKeys, keys[len(keys)-1])
		wait, err := limiter.AllowCreditCommand(ctx, callerID, scopeID, requestIPs[attempt-1], 1000, now)
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

func TestCreditCommandLimiterEnforcesDailyAmountAndNonceStoreIsSingleUse(t *testing.T) {
	address := os.Getenv("GLOWLOOM_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("set GLOWLOOM_TEST_REDIS_ADDR to a disposable or isolated local Redis endpoint")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := redisclient.NewClient(&redisclient.Options{Addr: address})
	cleanupKeys := []string{}
	defer func() {
		if len(cleanupKeys) > 0 {
			_ = client.Del(context.Background(), cleanupKeys...).Err()
		}
		_ = client.Close()
	}()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect Redis: %v", err)
	}

	cache := NewWalletBalanceCache(client)
	callerID, keyID, nonce := "daily-limit-"+uuid.NewString(), "key-1", "nonce_0123456789abcdefghijklmnop"
	nonceIdentity := sha256.Sum256([]byte(callerID + "\n" + keyID + "\n" + nonce))
	nonceKey := "credit:nonce:" + hex.EncodeToString(nonceIdentity[:])
	cleanupKeys = append(cleanupKeys, nonceKey)
	claimed, err := cache.Claim(ctx, callerID, keyID, nonce, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim first nonce = (%t, %v), want (true, nil)", claimed, err)
	}
	claimed, err = cache.Claim(ctx, callerID, keyID, nonce, time.Minute)
	if err != nil || claimed {
		t.Fatalf("claim replayed nonce = (%t, %v), want (false, nil)", claimed, err)
	}

	scopeID := uuid.New()
	now := time.Now().UTC()
	limiter := NewCreditCommandLimiter(client, 100, 1000, 1500)
	callerID = "daily-limit-" + uuid.NewString()
	window := now.Format("200601021504")
	day := now.Format("20060102")
	requestIP := "daily-limit-ip-" + uuid.NewString()
	keys := []string{
		"credit:limit:command:caller:" + digest(callerID) + ":" + window,
		"credit:limit:command:scope:" + digest(scopeID.String()) + ":" + window,
		"credit:limit:command:ip:" + digest(requestIP) + ":" + window,
		"credit:limit:command:daily:caller:" + digest(callerID) + ":" + day,
		"credit:limit:command:daily:scope:" + digest(scopeID.String()) + ":" + day,
	}
	cleanupKeys = append(cleanupKeys, keys...)
	wait, err := limiter.AllowCreditCommand(ctx, callerID, scopeID, requestIP, 1000, now)
	if err != nil || wait != 0 {
		t.Fatalf("first daily command wait = %s, error = %v", wait, err)
	}
	wait, err = limiter.AllowCreditCommand(ctx, callerID, scopeID, requestIP, 1000, now)
	if err != nil || wait <= 0 {
		t.Fatalf("daily overage wait = %s, error = %v, want a positive wait", wait, err)
	}
}

func TestCreditCommandLimiterComparesInt64DailyAmountsExactly(t *testing.T) {
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

	callerID := "daily-int64-" + uuid.NewString()
	scopeID := uuid.New()
	requestIP := "daily-int64-ip-" + uuid.NewString()
	now := time.Now().UTC()
	limiter := NewCreditCommandLimiter(client, 100, math.MaxInt64, math.MaxInt64)
	keys := []string{
		"credit:limit:command:caller:" + digest(callerID) + ":" + now.Format("200601021504"),
		"credit:limit:command:scope:" + digest(scopeID.String()) + ":" + now.Format("200601021504"),
		"credit:limit:command:ip:" + digest(requestIP) + ":" + now.Format("200601021504"),
		"credit:limit:command:daily:caller:" + digest(callerID) + ":" + now.Format("20060102"),
		"credit:limit:command:daily:scope:" + digest(scopeID.String()) + ":" + now.Format("20060102"),
	}
	defer func() { _ = client.Del(context.Background(), keys...).Err() }()

	if wait, err := limiter.AllowCreditCommand(ctx, callerID, scopeID, requestIP, math.MaxInt64-1, now); err != nil || wait != 0 {
		t.Fatalf("first near-limit command wait = %s, error = %v", wait, err)
	}
	for _, key := range keys[3:] {
		ttl, err := client.PTTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("daily counter TTL for %q = %s, error = %v, want positive TTL", key, ttl, err)
		}
	}
	if wait, err := limiter.AllowCreditCommand(ctx, callerID, scopeID, requestIP, 2, now); err != nil || wait <= 0 {
		t.Fatalf("overflowing daily command wait = %s, error = %v, want positive wait", wait, err)
	}
	for _, key := range keys[3:] {
		value, err := client.Get(ctx, key).Int64()
		if err != nil || value != math.MaxInt64-1 {
			t.Fatalf("daily counter %q = %d, error = %v, want exact unchanged value %d", key, value, err, int64(math.MaxInt64-1))
		}
	}
}
