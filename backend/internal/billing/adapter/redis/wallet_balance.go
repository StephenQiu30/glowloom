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

// Claim records a signed-request nonce once for its caller and key rotation window.
func (c *WalletBalanceCache) Claim(ctx context.Context, callerID, keyID, nonce string, ttl time.Duration) (bool, error) {
	if c == nil || c.client == nil || callerID == "" || keyID == "" || nonce == "" || ttl <= 0 {
		return false, fmt.Errorf("credit service nonce store unavailable")
	}
	identity := sha256.Sum256([]byte(callerID + "\n" + keyID + "\n" + nonce))
	key := "credit:nonce:" + hex.EncodeToString(identity[:])
	claimed, err := c.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("claim credit service nonce: %w", err)
	}
	return claimed, nil
}

// CreditCommandLimiter enforces caller, scope, IP, and amount limits.
type CreditCommandLimiter struct {
	client            *redisclient.Client
	requestsPerMinute int
	maxAmountMicros   int64
	maxDailyMicros    int64
}

// NewCreditCommandLimiter constructs the billing limiter with reviewed limits.
func NewCreditCommandLimiter(client *redisclient.Client, requestsPerMinute int, maxAmountMicros, maxDailyMicros int64) *CreditCommandLimiter {
	return &CreditCommandLimiter{client: client, requestsPerMinute: requestsPerMinute, maxAmountMicros: maxAmountMicros, maxDailyMicros: maxDailyMicros}
}

// AllowCreditCommand checks a write request against configured rate and amount caps.
func (l *CreditCommandLimiter) AllowCreditCommand(ctx context.Context, callerID string, scopeID uuid.UUID, requestIP string, amountMicros int64, now time.Time) (time.Duration, error) {
	if l == nil || l.client == nil || l.requestsPerMinute < 1 || l.maxAmountMicros < 1 || l.maxDailyMicros < 1 {
		return 0, ErrCreditLimiterUnconfigured
	}
	if amountMicros < 1 {
		return 0, fmt.Errorf("invalid credit command amount")
	}
	if amountMicros > l.maxAmountMicros {
		return time.Minute, nil
	}
	return l.allowCommand(ctx, callerID, scopeID, requestIP, amountMicros, now)
}

// AllowWalletRead checks a balance read against the read request budget.
func (l *CreditCommandLimiter) AllowWalletRead(ctx context.Context, callerID string, scopeID uuid.UUID, requestIP string, now time.Time) (time.Duration, error) {
	return l.allow(ctx, "read", callerID, scopeID, requestIP, now)
}

func (l *CreditCommandLimiter) allow(ctx context.Context, category, callerID string, scopeID uuid.UUID, requestIP string, now time.Time) (time.Duration, error) {
	return l.runLimitScript(ctx, category, callerID, scopeID, requestIP, 0, 0, 0, now)
}

func (l *CreditCommandLimiter) allowCommand(ctx context.Context, callerID string, scopeID uuid.UUID, requestIP string, amountMicros int64, now time.Time) (time.Duration, error) {
	utc := now.UTC()
	nextDay := time.Date(utc.Year(), utc.Month(), utc.Day()+1, 0, 0, 0, 0, time.UTC)
	return l.runLimitScript(ctx, "command", callerID, scopeID, requestIP, amountMicros, l.maxDailyMicros, nextDay.Sub(utc), utc)
}

func (l *CreditCommandLimiter) runLimitScript(ctx context.Context, category, callerID string, scopeID uuid.UUID, requestIP string, amountMicros, dailyLimitMicros int64, dailyTTL time.Duration, now time.Time) (time.Duration, error) {
	if l == nil || l.client == nil || l.requestsPerMinute < 1 {
		return 0, ErrCreditLimiterUnconfigured
	}
	window := now.UTC().Format("200601021504")
	keys := []string{
		"credit:limit:" + category + ":caller:" + digest(callerID) + ":" + window,
		"credit:limit:" + category + ":scope:" + digest(scopeID.String()) + ":" + window,
		"credit:limit:" + category + ":ip:" + digest(requestIP) + ":" + window,
	}
	args := []any{l.requestsPerMinute, time.Minute.Milliseconds()}
	if amountMicros > 0 {
		if dailyLimitMicros < 1 || dailyTTL <= 0 {
			return 0, ErrCreditLimiterUnconfigured
		}
		day := now.UTC().Format("20060102")
		keys = append(keys,
			"credit:limit:command:daily:caller:"+digest(callerID)+":"+day,
			"credit:limit:command:daily:scope:"+digest(scopeID.String())+":"+day,
		)
		args = append(args, amountMicros, dailyLimitMicros, dailyTTL.Milliseconds())
	}
	wait, err := creditRateLimitScript.Run(ctx, l.client, keys, args...).Int64()
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
for i=1,3 do
  local key = KEYS[i]
  local count = redis.call('INCR', key)
  if count == 1 then redis.call('PEXPIRE', key, ttl) end
  if count > limit then
    local remaining = redis.call('PTTL', key)
    if remaining > wait then wait = remaining end
  end
end
local amountText = ARGV[3] or '0'
if amountText ~= '0' then
  local dailyLimit = ARGV[4]
  local dailyTTL = tonumber(ARGV[5])
  local function normalizeDecimal(value)
    value = string.gsub(value, '^0+', '')
    if value == '' then return '0' end
    return value
  end
  local function compareDecimal(left, right)
    left = normalizeDecimal(left)
    right = normalizeDecimal(right)
    if string.len(left) < string.len(right) then return -1 end
    if string.len(left) > string.len(right) then return 1 end
    if left == right then return 0 end
    if left < right then return -1 end
    return 1
  end
  local function addDecimal(left, right)
    local carry = 0
    local result = ''
    local leftIndex = string.len(left)
    local rightIndex = string.len(right)
    while leftIndex > 0 or rightIndex > 0 or carry > 0 do
      local leftDigit = 0
      local rightDigit = 0
      if leftIndex > 0 then
        leftDigit = tonumber(string.sub(left, leftIndex, leftIndex))
        leftIndex = leftIndex - 1
      end
      if rightIndex > 0 then
        rightDigit = tonumber(string.sub(right, rightIndex, rightIndex))
        rightIndex = rightIndex - 1
      end
      local sum = leftDigit + rightDigit + carry
      carry = math.floor(sum / 10)
      result = tostring(sum % 10) .. result
    end
    return normalizeDecimal(result)
  end
  local exceedsDailyLimit = false
  for i=4,5 do
    local current = redis.call('GET', KEYS[i]) or '0'
    if compareDecimal(addDecimal(current, amountText), dailyLimit) > 0 then
      exceedsDailyLimit = true
      local remaining = redis.call('PTTL', KEYS[i])
      if remaining <= 0 then remaining = dailyTTL end
      if remaining > wait then wait = remaining end
    end
  end
  if wait == 0 and not exceedsDailyLimit then
    local callerCounterIsNew = redis.call('EXISTS', KEYS[4]) == 0
    local scopeCounterIsNew = redis.call('EXISTS', KEYS[5]) == 0
    for i=4,5 do
      redis.call('INCRBY', KEYS[i], amountText)
    end
    if callerCounterIsNew then redis.call('PEXPIRE', KEYS[4], dailyTTL) end
    if scopeCounterIsNew then redis.call('PEXPIRE', KEYS[5], dailyTTL) end
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
