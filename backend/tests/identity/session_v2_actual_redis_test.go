package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/refreshredis"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Exact opt-in. Never pass a shared source string: peer hashes are fixture-owned.
func TestSessionV2ActualRedisLua(t *testing.T) {
	if os.Getenv("LV_TEST_SESSION_V2_REDIS_ACTUAL") != "reviewed-exact-keys" {
		t.Skip("actual Redis Lua NOT verified; safety review opt-in absent")
	}
	raw := os.Getenv("LV_TEST_SESSION_V2_REDIS_URL")
	if raw == "" {
		t.Fatal("reviewed existing Redis URL required")
	}
	options, err := redis.ParseURL(raw)
	if err != nil {
		t.Fatal("invalid existing Redis URL")
	}
	options.MaxRetries = -1
	options.DialTimeout, options.ReadTimeout, options.WriteTimeout = 2*time.Second, 2*time.Second, 2*time.Second
	client := redis.NewClient(options)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error("close Redis client failed")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	q, err := refreshredis.New(client)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "glowloom:auth:v2:"
	sid := uuid.New()
	other := uuid.New()
	sourceA, sourceB := uuid.NewString(), uuid.NewString()
	peerKey := func(source string) string {
		d := sha256.Sum256([]byte(source))
		return prefix + "refresh-budget:peer:" + hex.EncodeToString(d[:])
	}
	lock := prefix + "refresh-lock:" + sid.String()
	budget := prefix + "refresh-budget:sid:" + sid.String()
	receipt := prefix + "refresh-receipt:" + sid.String() + ":0"
	revoked := prefix + "revoked:" + sid.String()
	keys := []string{peerKey(sourceA), peerKey(sourceB), lock, budget, receipt, revoked, prefix + "refresh-lock:" + other.String(), prefix + "refresh-budget:sid:" + other.String()}
	if n, e := client.Exists(ctx, keys...).Result(); e != nil || n != 0 {
		t.Fatal("random Redis fixture keys unavailable")
	}
	t.Logf("exact Redis fixture cleanup keys: %v", keys)
	// Register before any write, independent of the operation timeout and test failure.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := client.Del(cleanup, keys...).Err(); err != nil {
			t.Error("exact Redis fixture cleanup failed")
		}
	})
	release, err := q.Admit(ctx, sid, sourceA)
	if err != nil {
		t.Fatal("Lua admission failed")
	}
	ttl, err := client.PTTL(ctx, lock).Result()
	if err != nil || ttl <= 0 || ttl > 5*time.Second {
		t.Fatal("lease is not bounded to five seconds")
	}
	for _, key := range []string{budget, peerKey(sourceA)} {
		if ttl, e := client.PTTL(ctx, key).Result(); e != nil || ttl <= 0 || ttl > time.Minute {
			t.Fatal("budget TTL is not bounded to one minute")
		}
	}
	if _, err = q.Admit(ctx, sid, sourceB); !errors.Is(err, identity.ErrRefreshBusy) {
		t.Fatal("cross-source lease did not serialize sid")
	}
	// Replace only our random lock to deterministically exercise owner-safe release.
	if err = client.Set(ctx, lock, "replacement-fixture-owner", time.Second).Err(); err != nil {
		t.Fatal("replace fixture lease failed")
	}
	if err = release(ctx); err != nil {
		t.Fatal("release failed")
	}
	if got, e := client.Get(ctx, lock).Result(); e != nil || got != "replacement-fixture-owner" {
		t.Fatal("old owner deleted replacement lease")
	}
	// Expire only this fixture lease in the past, without sleeping or changing Redis time.
	if err = client.PExpireAt(ctx, lock, time.Now().Add(-time.Second)).Err(); err != nil {
		t.Fatal("fixture lease expiry failed")
	}
	release, err = q.Admit(ctx, sid, sourceB)
	if err != nil {
		t.Fatal("expired lease not reacquired")
	}
	if err = release(ctx); err != nil {
		t.Fatal("owner release failed")
	}
	if n, e := client.Exists(ctx, lock).Result(); e != nil || n != 0 {
		t.Fatal("owner lease retained")
	}
	// Three admissions above count against one sid, despite distinct sources.
	for i := 3; i < 12; i++ {
		release, err = q.Admit(ctx, sid, sourceA)
		if err != nil {
			t.Fatal("sid budget prematurely exhausted")
		}
		if err = release(ctx); err != nil {
			t.Fatal("release failed")
		}
	}
	if _, err = q.Admit(ctx, sid, sourceB); !errors.Is(err, identity.ErrRefreshRateLimited) {
		t.Fatal("sid budget not shared across sources")
	}
	// Seed only our peer bucket at its real Lua boundary; no global traffic needed.
	if err = client.Set(ctx, peerKey(sourceA), 60, time.Minute).Err(); err != nil {
		t.Fatal("fixture peer budget setup failed")
	}
	if _, err = q.Admit(ctx, other, sourceA); !errors.Is(err, identity.ErrRefreshRateLimited) {
		t.Fatal("peer limit ignored")
	}
	release, err = q.Admit(ctx, other, sourceB)
	if err != nil {
		t.Fatal("distinct source incorrectly inherited exhausted peer budget")
	}
	if err = release(ctx); err != nil {
		t.Fatal("release failed")
	}
	before := time.Now()
	if err = q.WriteReceipt(ctx, sid, 0, []byte("fixture-ciphertext"), before.Add(time.Hour)); err != nil {
		t.Fatal("bounded receipt write failed")
	}
	capDeadline, capErr := client.Do(ctx, "PEXPIRETIME", receipt).Int64()
	if capErr != nil || capDeadline < before.Add(10*time.Second).UnixMilli() || capDeadline > time.Now().Add(10*time.Second).UnixMilli() {
		t.Fatal("receipt ten-second cap lost")
	}
	until := time.Now().Add(3 * time.Second).Truncate(time.Millisecond)
	if err = q.WriteReceipt(ctx, sid, 0, []byte("fixture-ciphertext"), until); err != nil {
		t.Fatal("receipt write failed")
	}
	if got, e := q.ReadReceipt(ctx, sid, 0); e != nil || string(got) != "fixture-ciphertext" {
		t.Fatal("receipt read failed")
	}
	deadline, e := client.Do(ctx, "PEXPIRETIME", receipt).Int64()
	if e != nil || deadline != until.UnixMilli() {
		t.Fatal("receipt PXAT does not preserve absolute deadline")
	}
	if err = q.MarkRevoked(ctx, sid, until); err != nil {
		t.Fatal("revocation hint write failed")
	}
	if yes, e := q.IsRevoked(ctx, sid); e != nil || !yes {
		t.Fatal("revocation hint unreadable")
	}
	deadline, e = client.Do(ctx, "PEXPIRETIME", revoked).Int64()
	if e != nil || deadline != until.UnixMilli() {
		t.Fatal("revocation PXAT differs from absolute deadline")
	}
	for _, key := range []string{receipt, revoked} {
		if err = client.PExpireAt(ctx, key, time.Now().Add(-time.Second)).Err(); err != nil {
			t.Fatal("fixture expiry failed")
		}
	}
	if got, e := q.ReadReceipt(ctx, sid, 0); e != nil || len(got) != 0 {
		t.Fatal("expired receipt retained")
	}
	if yes, e := q.IsRevoked(ctx, sid); e != nil || yes {
		t.Fatal("expired revocation retained")
	}
}
