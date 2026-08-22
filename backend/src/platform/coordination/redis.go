package coordination

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCoordinator is the only Redis adapter exposed to business modules. Redis stores
// coordination state only; callers must persist the resulting fencing token or
// decision in PostgreSQL before changing business facts.
type RedisCoordinator struct {
	redis *redis.Client
}

func NewRedisCoordinator() *RedisCoordinator {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	return &RedisCoordinator{redis: redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD"), DB: 0})}
}

func (c *RedisCoordinator) Ping(ctx context.Context) error {
	if c == nil || c.redis == nil {
		return errors.New("redis client is not configured")
	}
	return c.redis.Ping(ctx).Err()
}

func (c *RedisCoordinator) Close() error { return c.redis.Close() }

type RedisLease struct {
	Key   string
	Owner string
	Fence int64
	TTL   time.Duration
	redis *redis.Client
}

var acquireLease = redis.NewScript(`
local token = ARGV[1]
local ttl = ARGV[2]
if redis.call('SET', KEYS[1], token, 'NX', 'PX', ttl) then
  local fence = redis.call('INCR', KEYS[2])
  return fence
end
return 0
`)

var releaseLease = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

var refreshLease = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 0
`)

func (c *RedisCoordinator) Acquire(ctx context.Context, key string, ttl time.Duration) (*RedisLease, error) {
	if ttl <= 0 || ttl > 5*time.Minute {
		return nil, fmt.Errorf("lease ttl must be between 1ns and 5m")
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("create lease owner: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	leaseKey := "lanverse:lock:" + key
	fenceKey := "lanverse:fence:" + key
	value, err := acquireLease.Run(ctx, c.redis, []string{leaseKey, fenceKey}, token, ttl.Milliseconds()).Int64()
	if err != nil {
		return nil, fmt.Errorf("acquire lease: %w", err)
	}
	if value == 0 {
		return nil, fmt.Errorf("lease is held: %s", key)
	}
	return &RedisLease{Key: leaseKey, Owner: token, Fence: value, TTL: ttl, redis: c.redis}, nil
}

func (l *RedisLease) Refresh(ctx context.Context) error {
	if l == nil {
		return errors.New("nil lease")
	}
	result, err := refreshLease.Run(ctx, l.redis, []string{l.Key}, l.Owner, l.TTL.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("refresh lease: %w", err)
	}
	if result != 1 {
		return errors.New("lease owner or ttl is no longer valid")
	}
	return nil
}

func (l *RedisLease) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if _, err := releaseLease.Run(ctx, l.redis, []string{l.Key}, l.Owner).Int(); err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	return nil
}

type RedisRateDecision struct {
	Allowed    bool
	RetryAfter time.Duration
	Remaining  int64
}

// AllowGCRA implements a single-key, bounded GCRA gate. It intentionally
// returns a decision instead of sleeping, so Kafka partitions are never held
// by a rate limiter; the caller persists a delayed task in PostgreSQL.
func (c *RedisCoordinator) AllowGCRA(ctx context.Context, key string, limit int64, period time.Duration, burst int64) (RedisRateDecision, error) {
	if limit <= 0 || period <= 0 || burst < 0 {
		return RedisRateDecision{}, errors.New("invalid GCRA policy")
	}
	interval := period.Microseconds() / limit
	if interval < 1 {
		interval = 1
	}
	now := time.Now().UnixMicro()
	key = "lanverse:rate:" + key
	script := redis.NewScript(`
local now = tonumber(ARGV[1])
local interval = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local tat = tonumber(redis.call('GET', KEYS[1]) or '0')
local allow_at = tat - burst * interval
if now < allow_at then
  return {0, allow_at - now, 0}
end
local next = math.max(now, tat) + interval
redis.call('SET', KEYS[1], next, 'PX', ttl)
local remaining = math.max(0, math.floor((burst * interval - (next - now)) / interval))
return {1, 0, remaining}
`)
	values, err := script.Run(ctx, c.redis, []string{key}, now, interval, burst, period.Milliseconds()*2).Result()
	if err != nil {
		return RedisRateDecision{}, fmt.Errorf("gcra decision: %w", err)
	}
	items, ok := values.([]any)
	if !ok || len(items) != 3 {
		return RedisRateDecision{}, errors.New("invalid GCRA response")
	}
	toInt := func(value any) int64 {
		switch typed := value.(type) {
		case int64:
			return typed
		case string:
			parsed, _ := strconv.ParseInt(typed, 10, 64)
			return parsed
		default:
			return 0
		}
	}
	return RedisRateDecision{Allowed: toInt(items[0]) == 1, RetryAfter: time.Duration(toInt(items[1])) * time.Microsecond, Remaining: toInt(items[2])}, nil
}
