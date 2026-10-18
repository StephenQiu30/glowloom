package identity_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	adapter "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/refreshredis"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type offlineHook struct{}

func (offlineHook) DialHook(_ redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("offline") }
}
func (offlineHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(context.Context, redis.Cmder) error { return errors.New("offline") }
}
func (offlineHook) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return errors.New("offline") }
}
func TestSessionV2RedisContract(t *testing.T) {
	client := redis.NewClient(&redis.Options{})
	defer func() { _ = client.Close() }()
	client.AddHook(offlineHook{})
	c, err := adapter.New(client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Admit(context.Background(), uuid.New(), "peer")
	if !errors.Is(err, app.ErrRefreshDependency) {
		t.Fatalf("dependency %v", err)
	}
}

type contractHook struct {
	commands  [][]interface{}
	admission int
}

func (h *contractHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *contractHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *contractHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, c redis.Cmder) error {
		h.commands = append(h.commands, c.Args())
		switch v := c.(type) {
		case *redis.Cmd:
			v.SetVal(int64(h.admission))
		case *redis.StatusCmd:
			v.SetVal("OK")
		case *redis.StringCmd:
			v.SetErr(redis.Nil)
		case *redis.IntCmd:
			v.SetVal(1)
		}
		return nil
	}
}
func TestSessionV2RedisContractCommands(t *testing.T) {
	client := redis.NewClient(&redis.Options{})
	defer func() { _ = client.Close() }()
	h := &contractHook{}
	client.AddHook(h)
	c, _ := adapter.New(client)
	ctx := context.Background()
	sid := uuid.New()
	release, err := c.Admit(ctx, sid, "private-peer")
	if err != nil {
		t.Fatal(err)
	}
	args := h.commands[0]
	script := args[1].(string)
	for _, part := range []string{"60000", "5000", "peer>60", "sid>12", "'NX'"} {
		if !strings.Contains(script, part) {
			t.Fatalf("missing admission contract %s", part)
		}
	}
	if strings.Contains(fmt.Sprint(args[3:6]), "private-peer") {
		t.Fatal("unhashed peer")
	}
	owner := args[6]
	if err = release(ctx); err != nil {
		t.Fatal(err)
	}
	released := h.commands[1]
	if released[4] != owner || !strings.Contains(released[1].(string), "==ARGV[1]") {
		t.Fatal("release without same owner comparison")
	}
	for _, tc := range []struct {
		n    int
		want error
	}{{1, app.ErrRefreshBusy}, {2, app.ErrRefreshRateLimited}} {
		h.admission = tc.n
		if _, err = c.Admit(ctx, sid, "peer"); !errors.Is(err, tc.want) {
			t.Fatalf("admission %v", err)
		}
	}
	beforeExpired := len(h.commands)
	if err = c.WriteReceipt(ctx, sid, 1, []byte("sealed"), time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(h.commands) != beforeExpired {
		t.Fatal("expired receipt sent to Redis")
	}
	until := time.Now().Add(time.Minute)
	if err = c.WriteReceipt(ctx, sid, 1, []byte("sealed"), until); err != nil {
		t.Fatal(err)
	}
	last := h.commands[len(h.commands)-1]
	if strings.Contains(fmt.Sprint(last[1]), "sealed") {
		t.Fatal("secret in key")
	}
	if last[len(last)-2] != "PXAT" {
		t.Fatalf("absolute ttl %v", last)
	}
	if err = c.WriteReceipt(ctx, sid, 1, []byte("sealed"), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	last = h.commands[len(h.commands)-1]
	if last[len(last)-2] != "PXAT" {
		t.Fatalf("PG window not preserved %v", last)
	}
	if err = c.MarkRevoked(ctx, sid, until); err != nil {
		t.Fatal(err)
	}
	last = h.commands[len(h.commands)-1]
	if last[len(last)-2] != "PXAT" || fmt.Sprint(last[len(last)-1]) != fmt.Sprint(until.UnixMilli()) {
		t.Fatalf("absolute expiry %v", last)
	}
}

// delayedReceiptHook models transport queueing followed by replay of the same
// Redis command. Its server clock advances without opening a Redis connection.
type delayedReceiptHook struct {
	contractHook
	receiveAt   time.Time
	expirations []time.Time
}

func (h *delayedReceiptHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		mode := strings.ToLower(fmt.Sprint(args[len(args)-2]))
		value, err := strconv.ParseInt(fmt.Sprint(args[len(args)-1]), 10, 64)
		if err != nil {
			return err
		}
		for _, received := range []time.Time{h.receiveAt, h.receiveAt.Add(2 * time.Second)} {
			var expiry time.Time
			switch mode {
			case "pxat":
				expiry = time.UnixMilli(value)
			case "px":
				expiry = received.Add(time.Duration(value) * time.Millisecond)
			case "ex":
				expiry = received.Add(time.Duration(value) * time.Second)
			default:
				return fmt.Errorf("unexpected expiration mode %s", mode)
			}
			h.expirations = append(h.expirations, expiry)
		}
		switch v := cmd.(type) {
		case *redis.Cmd:
			v.SetVal("OK")
		case *redis.StatusCmd:
			v.SetVal("OK")
		}
		return nil
	}
}
func TestSessionV2RedisContractReceiptDeadlineSurvivesDelayAndReplay(t *testing.T) {
	for _, window := range []time.Duration{3 * time.Second, time.Minute} {
		t.Run(window.String(), func(t *testing.T) {
			client := redis.NewClient(&redis.Options{})
			defer func() { _ = client.Close() }()
			sampledBefore := time.Now()
			pgDeadline := sampledBefore.Add(window)
			h := &delayedReceiptHook{receiveAt: sampledBefore.Add(5 * time.Second)}
			client.AddHook(h)
			c, err := adapter.New(client)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.WriteReceipt(context.Background(), uuid.New(), 1, []byte("sealed"), pgDeadline); err != nil {
				t.Fatal(err)
			}
			for _, expiry := range h.expirations {
				if expiry.After(pgDeadline) {
					t.Fatalf("transport/replay extended PG deadline: expiry=%v PG=%v", expiry, pgDeadline)
				}
			}
			if len(h.expirations) != 2 || !h.expirations[0].Equal(h.expirations[1]) {
				t.Fatal("replay moved absolute expiry")
			}
			if window < 10*time.Second && !h.expirations[0].Equal(time.UnixMilli(pgDeadline.UnixMilli())) {
				t.Fatal("PG window must use its exact millisecond deadline")
			}
			if window > 10*time.Second {
				if h.expirations[0].Before(sampledBefore.Add(10*time.Second-time.Millisecond)) || h.expirations[0].After(time.Now().Add(10*time.Second)) {
					t.Fatal("ten-second deadline not sampled before transport")
				}
			}
		})
	}
}

// Commands are intercepted before transport; this is not actual Redis evidence.
func TestSessionV2RedisRevocationMillisecondDeadline(t *testing.T) {
	client := redis.NewClient(&redis.Options{})
	defer func() { _ = client.Close() }()
	h := &contractHook{}
	client.AddHook(h)
	coordinator, err := adapter.New(client)
	if err != nil {
		t.Fatal(err)
	}
	sid := uuid.New()
	until := time.Now().Add(time.Minute).Truncate(time.Second).Add(789 * time.Millisecond)
	if err := coordinator.MarkRevoked(t.Context(), sid, until); err != nil {
		t.Fatal(err)
	}
	if len(h.commands) != 1 {
		t.Fatal("expected one atomic revocation write")
	}
	args := h.commands[0]
	if len(args) != 5 || strings.ToUpper(fmt.Sprint(args[0])) != "SET" || args[1] != "glowloom:auth:v2:revoked:"+sid.String() || args[2] != "1" || strings.ToUpper(fmt.Sprint(args[3])) != "PXAT" || fmt.Sprint(args[4]) != fmt.Sprint(until.UnixMilli()) {
		t.Fatalf("revocation must preserve absolute millisecond deadline: %v", args)
	}
	if err := coordinator.MarkRevoked(t.Context(), sid, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(h.commands) != 1 {
		t.Fatal("expired revocation hint was written")
	}

	offline := redis.NewClient(&redis.Options{})
	defer func() { _ = offline.Close() }()
	offline.AddHook(offlineHook{})
	unavailable, err := adapter.New(offline)
	if err != nil {
		t.Fatal(err)
	}
	if err := unavailable.MarkRevoked(t.Context(), sid, until); !errors.Is(err, app.ErrRefreshDependency) {
		t.Fatal("revocation write failure lost dependency error")
	}
}
