// Package redis adapts Redis caching and rate limits for the billing module.
package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
)

// ErrCreditLimiterUnconfigured means a required write limit is missing.
var ErrCreditLimiterUnconfigured = errors.New("credit command limiter is not configured")

const walletBalanceTTL = 5 * time.Second

// WalletBalanceCache stores short-lived, versioned wallet balance projections.
type WalletBalanceCache struct{ client *redisclient.Client }

// NewWalletBalanceCache constructs a wallet balance cache using Redis.
func NewWalletBalanceCache(client *redisclient.Client) *WalletBalanceCache {
	return &WalletBalanceCache{client: client}
}

// Get returns a cached balance only when its version watermark is current.
func (c *WalletBalanceCache) Get(ctx context.Context, scopeID uuid.UUID) (application.WalletBalance, bool, error) {
	if c == nil || c.client == nil {
		return application.WalletBalance{}, false, fmt.Errorf("wallet balance cache unavailable")
	}
	key := balanceKey(scopeID)
	result, err := getWalletBalanceScript.Run(ctx, c.client, []string{key, versionKey(scopeID)}).Result()
	if errors.Is(err, redisclient.Nil) {
		return application.WalletBalance{}, false, nil
	}
	if err != nil {
		return application.WalletBalance{}, false, fmt.Errorf("read wallet balance cache: %w", err)
	}
	value, ok := result.(string)
	if !ok {
		return application.WalletBalance{}, false, fmt.Errorf("decode wallet balance cache: unexpected Redis value %T", result)
	}
	var balance application.WalletBalance
	if err := json.Unmarshal([]byte(value), &balance); err != nil {
		return application.WalletBalance{}, false, fmt.Errorf("decode wallet balance cache: %w", err)
	}
	if balance.Version < 0 {
		return application.WalletBalance{}, false, fmt.Errorf("invalid cached wallet version")
	}
	return balance, true, nil
}

// Put writes a balance only when its version is not older than the watermark.
func (c *WalletBalanceCache) Put(ctx context.Context, scopeID uuid.UUID, balance application.WalletBalance) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("wallet balance cache unavailable")
	}
	if scopeID == uuid.Nil || balance.Version < 0 {
		return fmt.Errorf("invalid wallet balance cache value")
	}
	value, err := json.Marshal(balance)
	if err != nil {
		return fmt.Errorf("encode wallet balance cache: %w", err)
	}
	result, err := putWalletBalanceScript.Run(ctx, c.client, []string{balanceKey(scopeID), versionKey(scopeID)}, value, balance.Version, walletBalanceTTL.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("write wallet balance cache: %w", err)
	}
	if result == 0 {
		return nil
	}
	return nil
}

// Invalidate raises the watermark and evicts an older cached balance.
func (c *WalletBalanceCache) Invalidate(ctx context.Context, scopeID uuid.UUID, version int64) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("wallet balance cache unavailable")
	}
	if scopeID == uuid.Nil || version < 1 {
		return fmt.Errorf("invalid wallet balance version event")
	}
	if err := invalidateWalletBalanceScript.Run(ctx, c.client, []string{balanceKey(scopeID), versionKey(scopeID)}, version).Err(); err != nil {
		return fmt.Errorf("invalidate wallet balance cache: %w", err)
	}
	return nil
}

// CreditCommandLimiter enforces caller, scope, IP, and amount limits.
type CreditCommandLimiter struct {
	client            *redisclient.Client
	requestsPerMinute int
	maxAmountMicros   int64
}

// NewCreditCommandLimiter constructs the billing limiter with reviewed limits.
func NewCreditCommandLimiter(client *redisclient.Client, requestsPerMinute int, maxAmountMicros int64) *CreditCommandLimiter {
	return &CreditCommandLimiter{client: client, requestsPerMinute: requestsPerMinute, maxAmountMicros: maxAmountMicros}
}

// AllowCreditCommand checks a write request against configured rate and amount caps.
func (l *CreditCommandLimiter) AllowCreditCommand(ctx context.Context, callerID string, scopeID uuid.UUID, requestIP string, amountMicros int64, now time.Time) (time.Duration, error) {
	if l == nil || l.client == nil || l.requestsPerMinute < 1 || l.maxAmountMicros < 1 {
		return 0, ErrCreditLimiterUnconfigured
	}
	if amountMicros < 1 {
		return 0, fmt.Errorf("invalid credit command amount")
	}
	if amountMicros > l.maxAmountMicros {
		return time.Minute, nil
	}
	return l.allow(ctx, "command", callerID, scopeID, requestIP, now)
}

// AllowWalletRead checks a balance read against the read request budget.
func (l *CreditCommandLimiter) AllowWalletRead(ctx context.Context, callerID string, scopeID uuid.UUID, requestIP string, now time.Time) (time.Duration, error) {
	return l.allow(ctx, "read", callerID, scopeID, requestIP, now)
}

func (l *CreditCommandLimiter) allow(ctx context.Context, category, callerID string, scopeID uuid.UUID, requestIP string, now time.Time) (time.Duration, error) {
	if l == nil || l.client == nil || l.requestsPerMinute < 1 {
		return 0, ErrCreditLimiterUnconfigured
	}
	window := now.UTC().Format("200601021504")
	keys := []string{
		"credit:limit:" + category + ":caller:" + digest(callerID) + ":" + window,
		"credit:limit:" + category + ":scope:" + digest(scopeID.String()) + ":" + window,
		"credit:limit:" + category + ":ip:" + digest(requestIP) + ":" + window,
	}
	wait, err := creditRateLimitScript.Run(ctx, l.client, keys, l.requestsPerMinute, time.Minute.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("check credit command rate limit: %w", err)
	}
	if wait <= 0 {
		return 0, nil
	}
	return time.Duration(wait) * time.Millisecond, nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}

func balanceKey(scopeID uuid.UUID) string { return "credit:balance:" + scopeID.String() }
func versionKey(scopeID uuid.UUID) string { return "credit:balance-version:" + scopeID.String() }

var getWalletBalanceScript = redisclient.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then return false end
local watermark = tonumber(redis.call('GET', KEYS[2]) or '0')
local decoded = cjson.decode(data)
local version = tonumber(decoded.version or '0')
if version < watermark then
  redis.call('DEL', KEYS[1])
  return false
end
return data
`)

var putWalletBalanceScript = redisclient.NewScript(`
local nextVersion = tonumber(ARGV[2])
local watermark = tonumber(redis.call('GET', KEYS[2]) or '0')
local current = redis.call('GET', KEYS[1])
if current then
  local currentVersion = tonumber(cjson.decode(current).version or '0')
  if currentVersion > watermark then watermark = currentVersion end
end
if nextVersion < watermark then return 0 end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
redis.call('SET', KEYS[2], tostring(math.max(watermark, nextVersion)))
return 1
`)

var invalidateWalletBalanceScript = redisclient.NewScript(`
local incoming = tonumber(ARGV[1])
local current = tonumber(redis.call('GET', KEYS[2]) or '0')
local watermark = math.max(current, incoming)
redis.call('SET', KEYS[2], tostring(watermark))
local data = redis.call('GET', KEYS[1])
if data and tonumber(cjson.decode(data).version or '0') < watermark then redis.call('DEL', KEYS[1]) end
return 1
`)

var creditRateLimitScript = redisclient.NewScript(`
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local wait = 0
for i,key in ipairs(KEYS) do
  local count = redis.call('INCR', key)
  if count == 1 then redis.call('PEXPIRE', key, ttl) end
  if count > limit then
    local remaining = redis.call('PTTL', key)
    if remaining > wait then wait = remaining end
  end
end
return wait
`)

var _ interface {
	Get(context.Context, uuid.UUID) (application.WalletBalance, bool, error)
	Put(context.Context, uuid.UUID, application.WalletBalance) error
	Invalidate(context.Context, uuid.UUID, int64) error
} = (*WalletBalanceCache)(nil)
var _ application.CreditCommandLimiter = (*CreditCommandLimiter)(nil)
