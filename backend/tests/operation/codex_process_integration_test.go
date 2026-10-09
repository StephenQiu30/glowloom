package operation_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/codex"
)

// TestCodexProcessHandshake verifies the installed transport without opening a
// thread or submitting inference. The executable must be opted into explicitly.
func TestCodexProcessHandshake(t *testing.T) {
	binary := os.Getenv("LV_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set LV_TEST_CODEX_BINARY to the absolute installed Codex executable")
	}
	launcher, err := codex.NewProcessLauncher(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	conn, err := launcher.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close app-server: %v", err)
		}
	})
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(io.LimitReader(conn, 1<<20))
	call := func(id int, method string, params any, target any) {
		t.Helper()
		if err := encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			t.Fatalf("send %s: %v", method, err)
		}
		for range 32 {
			var response struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := decoder.Decode(&response); err != nil {
				t.Fatalf("receive %s: %v", method, err)
			}
			if response.ID == nil || *response.ID != id {
				continue
			}
			if len(response.Error) != 0 && string(response.Error) != "null" {
				t.Fatalf("app-server rejected %s", method)
			}
			if err := json.Unmarshal(response.Result, target); err != nil {
				t.Fatalf("decode %s result: %v", method, err)
			}
			return
		}
		t.Fatalf("missing %s response", method)
	}
	var initialized struct {
		UserAgent string `json:"userAgent"`
	}
	call(1, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "lanverse-m0-probe", "version": "1"},
		"capabilities": map[string]any{"experimentalApi": true},
	}, &initialized)
	if initialized.UserAgent == "" {
		t.Fatal("initialize returned no user agent")
	}
	if err := encoder.Encode(map[string]any{"method": "initialized"}); err != nil {
		t.Fatalf("send initialized: %v", err)
	}
	var capabilities struct {
		ImageGeneration *bool `json:"imageGeneration"`
	}
	call(2, "modelProvider/capabilities/read", map[string]any{}, &capabilities)
	if capabilities.ImageGeneration == nil {
		t.Fatal("imageGeneration capability is absent")
	}
	t.Logf("installed protocol available; imageGeneration=%t; inference not submitted", *capabilities.ImageGeneration)
	if err := conn.Close(); err != nil {
		t.Fatalf("stop app-server: %v", err)
	}
}
