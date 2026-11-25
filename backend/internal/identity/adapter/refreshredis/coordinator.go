// Package refreshredis provides derived coordination using the existing Redis client.
package refreshredis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const prefix = "glowloom:auth:v2:"

// Admission is atomic: bounded fixed windows and a five-second owner lease.
const admitScript = `local peer=redis.call('INCR',KEYS[1]); if peer==1 then redis.call('PEXPIRE',KEYS[1],60000) end; local sid=redis.call('INCR',KEYS[2]); if sid==1 then redis.call('PEXPIRE',KEYS[2],60000) end; if peer>60 or sid>12 then return 2 end; if redis.call('SET',KEYS[3],ARGV[1],'PX',5000,'NX') then return 0 end; return 1`
const releaseScript = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end; return 0`

// Coordinator stores non-authoritative refresh coordination state.
type Coordinator struct{ client *redis.Client }

// New reuses an injected Redis client without opening a connection.
func New(client *redis.Client) (*Coordinator, error) {
	if client == nil {
		return nil, errors.New("existing redis client required")
	}
	return &Coordinator{client}, nil
}
func dependency(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", app.ErrRefreshDependency, err)
}

// Admit applies source/session budgets and acquires an owner lease.
func (c *Coordinator) Admit(ctx context.Context, sid uuid.UUID, source string) (func(context.Context) error, error) {
	if sid == uuid.Nil || source == "" {
		return nil, errors.New("refresh session and source required")
	}
	d := sha256.Sum256([]byte(source))
	lock := prefix + "refresh-lock:" + sid.String()
	owner := uuid.NewString()
	keys := []string{prefix + "refresh-budget:peer:" + hex.EncodeToString(d[:]), prefix + "refresh-budget:sid:" + sid.String(), lock}
	n, e := c.client.Eval(ctx, admitScript, keys, owner).Int()
	if e != nil {
		return nil, dependency(e)
	}
	switch n {
	case 1:
		return nil, app.ErrRefreshBusy
	case 2:
		return nil, app.ErrRefreshRateLimited
	case 0:
	default:
		return nil, dependency(errors.New("unexpected admission response"))
	}
	return func(ctx context.Context) error {
		return dependency(c.client.Eval(ctx, releaseScript, []string{lock}, owner).Err())
	}, nil
}
func receiptKey(s uuid.UUID, g int64) string {
	return prefix + "refresh-receipt:" + s.String() + ":" + strconv.FormatInt(g, 10)
}

// ReadReceipt returns ciphertext; callers must revalidate PostgreSQL authority.
func (c *Coordinator) ReadReceipt(ctx context.Context, s uuid.UUID, g int64) ([]byte, error) {
	b, e := c.client.Get(ctx, receiptKey(s, g)).Bytes()
	if errors.Is(e, redis.Nil) {
		return nil, nil
	}
	return b, dependency(e)
}

// WriteReceipt caps retention to ten seconds and the supplied PG recovery window.
func (c *Coordinator) WriteReceipt(ctx context.Context, s uuid.UUID, g int64, b []byte, until time.Time) error {
	now := time.Now()
	if !until.After(now) {
		return nil
	}
	deadline := until
	if limit := now.Add(10 * time.Second); deadline.After(limit) {
		deadline = limit
	}
	// Absolute millisecond expiry survives transport queueing and client retries.
	return dependency(c.client.Do(ctx, "SET", receiptKey(s, g), b, "PXAT", deadline.UnixMilli()).Err())
}

// MarkRevoked retains a negative hint only until the original absolute deadline.
func (c *Coordinator) MarkRevoked(ctx context.Context, s uuid.UUID, until time.Time) error {
	ttl := time.Until(until)
	if ttl <= 0 {
		return nil
	}
	// SetArgs.ExpireAt uses second-precision EXAT; preserve the PG deadline
	// at millisecond precision, including transport delay and command retries.
	return dependency(c.client.Do(ctx, "SET", prefix+"revoked:"+s.String(), "1", "PXAT", until.UnixMilli()).Err())
}

// IsRevoked reports a negative hint; absence cannot authorize requests.
func (c *Coordinator) IsRevoked(ctx context.Context, s uuid.UUID) (bool, error) {
	n, e := c.client.Exists(ctx, prefix+"revoked:"+s.String()).Result()
	return n > 0, dependency(e)
}
