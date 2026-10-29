package redisconn_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/redisconn"
)

func TestRequestDeadlineBoundsUnresponsiveRedis(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
		_, _ = io.Copy(io.Discard, conn)
	}()
	client, err := redisconn.Open("redis://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = client.Ping(ctx)
	select {
	case conn := <-accepted:
		_ = conn.Close()
	default:
		t.Fatal("Redis command did not reach the loopback transport")
	}
	if err == nil {
		t.Fatal("unresponsive Redis returned success")
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("100ms request deadline was ignored: elapsed %s", elapsed)
	}
}

func TestOpenRejectsMissingURL(t *testing.T) {
	_, err := redisconn.Open(" ")
	if !errors.Is(err, redisconn.ErrURLRequired) {
		t.Fatalf("Open() error = %v, want ErrURLRequired", err)
	}
}

func TestOpenRejectsMalformedURLWithoutExposingPassword(t *testing.T) {
	_, err := redisconn.Open("redis://probe:canary-secret@%invalid/0")
	if !errors.Is(err, redisconn.ErrInvalidURL) {
		t.Fatalf("Open() error = %v, want ErrInvalidURL", err)
	}
	if strings.Contains(err.Error(), "canary-secret") {
		t.Fatalf("Open() error exposed a password: %v", err)
	}
}

func TestPingWithRedis(t *testing.T) {
	url := os.Getenv("GLOWLOOM_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set GLOWLOOM_TEST_REDIS_URL to a disposable Redis database")
	}
	conn, err := redisconn.Open(url)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
