package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
)

func TestRoleHealthStopsWithItsRole(t *testing.T) {
	address := localHealthAddress(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.RunWithHealth(ctx, "probe", address, zap.NewNop(), func(runCtx context.Context) error {
			<-runCtx.Done()
			return nil
		})
	}()
	assertRoleHealth(ctx, t, address)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop role health: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("role health did not stop after cancellation")
	}
	assertRoleHealthStopped(t, address)
}

func TestRoleHealthPropagatesRoleFailure(t *testing.T) {
	address := localHealthAddress(t)
	want := errors.New("role failed")
	err := app.RunWithHealth(t.Context(), "probe", address, zap.NewNop(), func(context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("RunWithHealth error = %v, want role failure", err)
	}
	assertRoleHealthStopped(t, address)
}

func localHealthAddress(t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve health address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release health address: %v", err)
	}
	return address
}

func assertRoleHealth(ctx context.Context, t *testing.T, address string) {
	t.Helper()
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for {
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+address+"/healthz", nil)
		if err != nil {
			t.Fatalf("create health request: %v", err)
		}
		response, err := client.Do(req)
		if err == nil {
			var body struct {
				Status string `json:"status"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json" || decodeErr != nil || body.Status != "ok" {
				t.Fatalf("role health = %d %+v, decode error = %v", response.StatusCode, body, decodeErr)
			}
			return
		}
		if probeCtx.Err() != nil {
			t.Fatalf("role health did not become available at %s: %v", address, probeCtx.Err())
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-probeCtx.Done():
			timer.Stop()
			t.Fatalf("role health did not become available at %s: %v", address, probeCtx.Err())
		case <-timer.C:
		}
	}
}

func assertRoleHealthStopped(t *testing.T, address string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err == nil {
		_ = connection.Close()
		t.Errorf("role health listener at %s is still accepting connections", address)
	}
}
