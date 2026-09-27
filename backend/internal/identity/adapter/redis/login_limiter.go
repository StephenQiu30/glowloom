package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

const (
	loginAttemptsPerMinute = 20
	loginWindow            = time.Minute
)

var (
	// ErrInvalidLoginLimiter means the Redis client is missing.
	ErrInvalidLoginLimiter = errors.New("invalid login limiter configuration")
	// ErrInvalidIP means the caller supplied no valid client address.
	ErrInvalidIP = errors.New("invalid client IP")
	// ErrLimiterState means Redis returned an unusable rate-limit state.
	ErrLimiterState = errors.New("invalid login limiter state")
)

var recordLoginAttempt = redisclient.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return {count, redis.call('PTTL', KEYS[1])}
`)

// LoginLimiter caps login attempts to 20 per client IP in a one-minute window.
type LoginLimiter struct {
	client *redisclient.Client
}

// NewLoginLimiter creates a limiter backed by the shared Redis client.
func NewLoginLimiter(client *redisclient.Client) (*LoginLimiter, error) {
	if client == nil {
		return nil, ErrInvalidLoginLimiter
	}
	return &LoginLimiter{client: client}, nil
}

// AllowAttempt atomically records every login attempt, including rejected ones.
// Redis failures must be treated as unavailable authentication by the caller.
func (l *LoginLimiter) AllowAttempt(ctx context.Context, rawIP string) (bool, time.Duration, error) {
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return false, 0, ErrInvalidIP
	}
	ip = ip.Unmap()
	digest := sha256.Sum256([]byte(ip.String()))
	key := "login_fail_ip:" + hex.EncodeToString(digest[:])
	result, err := recordLoginAttempt.Run(ctx, l.client, []string{key}, loginWindow.Milliseconds()).Result()
	if err != nil {
		return false, 0, fmt.Errorf("record login attempt: %w", err)
	}
	parts, ok := result.([]any)
	if !ok || len(parts) != 2 {
		return false, 0, ErrLimiterState
	}
	count, countOK := parts[0].(int64)
	remainingMS, remainingOK := parts[1].(int64)
	if !countOK || !remainingOK || count <= 0 || remainingMS < 0 {
		return false, 0, ErrLimiterState
	}
	if count <= loginAttemptsPerMinute {
		return true, 0, nil
	}
	return false, max(time.Duration(remainingMS)*time.Millisecond, time.Millisecond), nil
}
