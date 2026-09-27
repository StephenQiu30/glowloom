package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	redisidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/redis"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestSessionStoreUsesDigestAndHonorsBothDeadlines(t *testing.T) {
	url := os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_IDENTITY_REDIS_URL to a disposable Redis database")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	store, err := redisidentity.NewSessionStore(conn.Client, 10*time.Minute, 25*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	orgID, userID := uuid.New(), uuid.New()
	token, created, err := store.Create(ctx, orgID, userID, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Destroy(context.Background(), token) })
	digest := sha256.Sum256([]byte(token))
	key := "sess:" + hex.EncodeToString(digest[:])
	value, err := conn.Client.Get(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(value, token) || strings.Contains(key, token) || len(token) < 40 {
		t.Fatal("session storage exposed the raw token or token has insufficient entropy")
	}
	if created.OrgID != orgID || created.UserID != userID || created.SessionEpoch != 4 {
		t.Fatalf("created session identity = %+v", created)
	}
	if _, err := store.Load(ctx, token+"invalid"); !errors.Is(err, redisidentity.ErrSessionNotFound) {
		t.Fatalf("invalid token lookup = %v", err)
	}
	now = now.Add(9 * time.Minute)
	loaded, err := store.Load(ctx, token)
	if err != nil || loaded.OrgID != orgID || loaded.UserID != userID {
		t.Fatalf("load before idle deadline = %+v, %v", loaded, err)
	}
	if err := store.Touch(ctx, token, loaded); err != nil {
		t.Fatalf("first touch: %v", err)
	}
	now = now.Add(9 * time.Minute)
	loaded, err = store.Load(ctx, token)
	if err != nil {
		t.Fatalf("load after idle renewal: %v", err)
	}
	if err := store.Touch(ctx, token, loaded); err != nil {
		t.Fatalf("second touch: %v", err)
	}
	ttl, err := conn.Client.PTTL(ctx, key).Result()
	if err != nil || ttl <= 0 || ttl > 7*time.Minute {
		t.Fatalf("absolute deadline cap = %v, error %v", ttl, err)
	}
	now = now.Add(7 * time.Minute)
	if _, err := store.Load(ctx, token); !errors.Is(err, redisidentity.ErrSessionNotFound) {
		t.Fatalf("absolute deadline lookup = %v", err)
	}
	idleToken, _, err := store.Create(ctx, orgID, userID, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Destroy(context.Background(), idleToken) })
	now = now.Add(10 * time.Minute)
	if _, err := store.Load(ctx, idleToken); !errors.Is(err, redisidentity.ErrSessionNotFound) {
		t.Fatalf("idle deadline lookup = %v", err)
	}
}

func TestSessionStoreDestroyDoesNotResurrectOnTouch(t *testing.T) {
	url := os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_IDENTITY_REDIS_URL to a disposable Redis database")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	store, err := redisidentity.NewSessionStore(conn.Client, time.Hour, 24*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	token, session, err := store.Create(ctx, uuid.New(), uuid.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Destroy(context.Background(), token) })
	if err := store.Destroy(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(ctx, token, session); !errors.Is(err, redisidentity.ErrSessionNotFound) {
		t.Fatalf("touch after logout = %v", err)
	}
	if _, err := store.Load(ctx, token); !errors.Is(err, redisidentity.ErrSessionNotFound) {
		t.Fatalf("load after logout = %v", err)
	}
}

func TestSessionStoreRejectsInvalidConfigurationAndIdentity(t *testing.T) {
	if _, err := redisidentity.NewSessionStore(nil, time.Hour, 24*time.Hour, time.Now); !errors.Is(err, redisidentity.ErrInvalidSessionStore) {
		t.Fatalf("nil client = %v", err)
	}
	client := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := redisidentity.NewSessionStore(client, 25*time.Hour, 24*time.Hour, time.Now); !errors.Is(err, redisidentity.ErrInvalidSessionStore) {
		t.Fatalf("invalid durations = %v", err)
	}
	if _, err := redisidentity.NewSessionStore(client, time.Nanosecond, time.Hour, time.Now); !errors.Is(err, redisidentity.ErrInvalidSessionStore) {
		t.Fatalf("submillisecond idle TTL = %v", err)
	}
}

func TestSessionStoreFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	client := redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:0", MaxRetries: 0, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	store, err := redisidentity.NewSessionStore(client, time.Hour, 24*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := store.Load(ctx, token); err == nil || errors.Is(err, redisidentity.ErrSessionNotFound) {
		t.Fatalf("unavailable Redis lookup = %v, want service error", err)
	}
	if token, _, err := store.Create(ctx, uuid.New(), uuid.New(), 1); err == nil || token != "" {
		t.Fatalf("unavailable Redis creation = token %q, error %v", token, err)
	}
}

func TestLoginLimiterEnforcesTwentyAttemptsPerMinute(t *testing.T) {
	url := os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if url == "" {
		t.Skip("set LV_TEST_IDENTITY_REDIS_URL to a disposable Redis database")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	limiter, err := redisidentity.NewLoginLimiter(conn.Client)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	ip := fmt.Sprintf("2001:db8:%x:%x:%x:%x::1", id[0:2], id[2:4], id[4:6], id[6:8])
	canonicalIP, err := netip.ParseAddr(ip)
	if err != nil {
		t.Fatal(err)
	}
	ipDigest := sha256.Sum256([]byte(canonicalIP.String()))
	key := "login_fail_ip:" + hex.EncodeToString(ipDigest[:])
	t.Cleanup(func() { _ = conn.Client.Del(context.Background(), key).Err() })
	for i := 0; i < 20; i++ {
		allowed, retryAfter, err := limiter.AllowAttempt(ctx, ip)
		if err != nil || !allowed || retryAfter != 0 {
			t.Fatalf("attempt %d = allowed %t, retry %s, error %v", i+1, allowed, retryAfter, err)
		}
	}
	allowed, retryAfter, err := limiter.AllowAttempt(ctx, ip)
	if err != nil || allowed || retryAfter <= 0 || retryAfter > time.Minute {
		t.Fatalf("attempt 21 = allowed %t, retry %s, error %v", allowed, retryAfter, err)
	}
	if _, _, err := limiter.AllowAttempt(ctx, "invalid-address"); !errors.Is(err, redisidentity.ErrInvalidIP) {
		t.Fatalf("invalid IP = %v", err)
	}
	if err := conn.Client.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	allowed, _, err = limiter.AllowAttempt(ctx, ip)
	if err != nil || !allowed {
		t.Fatalf("attempt after key cleanup = allowed %t, error %v", allowed, err)
	}
}
