package creation_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/agenthttp"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
)

func TestCreationHTTPBindsShortLivedAuthorizationToExactRequest(t *testing.T) {
	store, peer := deliveryFixture()
	run := store.delivery.Run
	run.PayloadHash, _ = app.PayloadHash(run.Command)
	peer.receipt.PayloadHash = run.PayloadHash
	secret := strings.Repeat("test-only-", 4)
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		parts := strings.Split(r.Header.Get("X-Lanverse-Creation-Authorization"), ".")
		if len(parts) != 2 {
			t.Error("missing signed grant")
			w.WriteHeader(401)
			return
		}
		signature := hmac.New(sha256.New, []byte(secret))
		_, _ = signature.Write([]byte(parts[0]))
		expected := base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
		if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
			t.Error("invalid signature")
		}
		raw, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			t.Error(err)
			return
		}
		var claims struct {
			Audience, Method, Path string
			BodyHash               string `json:"body_hash"`
			ExpiresAt              int64  `json:"expires_at"`
		}
		if err = json.Unmarshal(raw, &claims); err != nil {
			t.Error(err)
			return
		}
		hash := sha256.Sum256(body)
		if claims.Audience != "lanverse.creation.command" || claims.Method != r.Method || claims.Path != r.URL.EscapedPath() || claims.BodyHash != hex.EncodeToString(hash[:]) || claims.ExpiresAt != now.Add(time.Minute).Unix() {
			t.Error("grant was not bound to exact request and TTL")
		}
		if err = json.NewEncoder(w).Encode(peer.receipt); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	run.Endpoint = server.URL
	client, err := adapter.New(adapter.Config{Secret: secret}, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Accept(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Lookup(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}
func TestCreationHTTPRejectsRedirectsMalformedAndForeignReceipts(t *testing.T) {
	store, peer := deliveryFixture()
	run := store.delivery.Run
	for _, mode := range []string{"redirect", "trailing", "oversized", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			redirected := false
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true; w.WriteHeader(200) }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				case "oversized":
					_, _ = io.WriteString(w, strings.Repeat(" ", 65537))
				case "trailing":
					raw, _ := json.Marshal(peer.receipt)
					_, _ = w.Write(append(raw, []byte("garbage")...))
				case "foreign":
					receipt := peer.receipt
					receipt.PayloadHash = "different"
					_ = json.NewEncoder(w).Encode(receipt)
				}
			}))
			defer server.Close()
			run.Endpoint = server.URL
			client, err := adapter.New(adapter.Config{Secret: strings.Repeat("x", 32)}, nil, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Lookup(context.Background(), run); err == nil {
				t.Fatal("unsafe receipt accepted")
			}
			if redirected {
				t.Fatal("authorization followed redirect")
			}
		})
	}
}

func TestCreationRelocationRoutesOnlyDeclaredOriginAndPreservesRun(t *testing.T) {
	store, peer := deliveryFixture()
	run := store.delivery.Run
	run.Endpoint = "http://127.0.0.1:8788"
	run.PayloadHash, _ = app.PayloadHash(run.Command)
	peer.receipt.PayloadHash = run.PayloadHash
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/execution") {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"detail":"creation_execution_not_started"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(peer.receipt)
	}))
	defer server.Close()
	client, err := adapter.New(adapter.Config{Secret: strings.Repeat("x", 32), RelocatedFrom: run.Endpoint, Endpoint: server.URL}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Accept(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Lookup(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Execution(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if run.Endpoint != "http://127.0.0.1:8788" || calls != 3 {
		t.Fatal("migration rewrote run or missed a request path")
	}
	run.Endpoint = "http://127.0.0.1:1"
	if _, err = client.Lookup(context.Background(), run); err == nil || calls != 3 {
		t.Fatal("undeclared origin was rerouted")
	}
}
