package creation_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpapi "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

type frozenSourceStore struct {
	app.Repository
	run       domain.Run
	actor     app.Actor
	project   string
	revoked   bool
	workspace string
}

func (s *frozenSourceStore) WithinTransaction(_ context.Context, fn func(app.Repository) error) error {
	return fn(s)
}
func (s *frozenSourceStore) Get(context.Context, string) (domain.Run, error) { return s.run, nil }
func (s *frozenSourceStore) Authorize(_ context.Context, actor app.Actor, project string, write bool) (string, error) {
	s.actor, s.project = actor, project
	if s.revoked || !write {
		return "", app.Problem("forbidden", 403)
	}
	return s.workspace, nil
}

type frozenSourceReader struct {
	calls   int
	actor   app.Actor
	project string
	source  domain.Source
}

func (s *frozenSourceReader) ReadFrozenSource(_ context.Context, actor app.Actor, project string, source domain.Source) (domain.FrozenSource, error) {
	s.calls++
	s.actor, s.project, s.source = actor, project, source
	return domain.FrozenSource{RevisionID: source.RevisionID, ContentHash: source.ContentHash, Text: "甲😀\n乙"}, nil
}
func frozenRun(t *testing.T) domain.Run {
	t.Helper()
	id := uuid.NewString()
	command := domain.Command{Schema: "creation-command-production", CommandID: id, RunID: id, WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), ActorID: uuid.NewString(), Source: domain.Source{DocumentID: uuid.NewString(), RevisionID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64), SpanIndexID: uuid.NewString()}, FlowType: domain.FlowType, WorkflowID: "lanverse:creation:" + id}
	hash, err := app.PayloadHash(command)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Run{Command: command, PayloadHash: hash, TokenVersion: 7}
}
func TestFrozenSourceUsesStoredActorAndSourceAndRechecksRevocation(t *testing.T) {
	run := frozenRun(t)
	store := &frozenSourceStore{run: run, workspace: run.Command.WorkspaceID}
	reader := &frozenSourceReader{}
	service := app.NewSourceService(store, reader)
	var first domain.FrozenSource
	for range 2 {
		got, err := service.ReadSource(context.Background(), run.Command.RunID, run.PayloadHash)
		if err != nil {
			t.Fatal(err)
		}
		if first.Text != "" && first != got {
			t.Fatal("repeated read changed frozen source")
		}
		first = got
	}
	if reader.actor.UserID != run.Command.ActorID || reader.actor.TokenVersion != 7 || reader.project != run.Command.ProjectID || reader.source != run.Command.Source {
		t.Fatal("caller replaced stored identity")
	}
	for _, which := range []string{"payload", "workspace", "permission"} {
		t.Run(which, func(t *testing.T) {
			store.workspace = run.Command.WorkspaceID
			store.revoked = false
			hash := run.PayloadHash
			switch which {
			case "payload":
				hash = strings.Repeat("f", 64)
			case "workspace":
				store.workspace = uuid.NewString()
			case "permission":
				store.revoked = true
			}
			before := reader.calls
			if _, err := service.ReadSource(context.Background(), run.Command.RunID, hash); err == nil || reader.calls != before {
				t.Fatalf("unauthorized source call: %v", err)
			}
		})
	}
}

type sourceHTTPStub struct{ calls int }

func (s *sourceHTTPStub) ReadSource(context.Context, string, string) (domain.FrozenSource, error) {
	s.calls++
	return domain.FrozenSource{RevisionID: "revision", ContentHash: strings.Repeat("a", 64), Text: "甲😀\n乙"}, nil
}

const sourceTestSecret = "source-bridge-test-only-secret-32-bytes"

func signSource(t *testing.T, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(sourceTestSecret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func TestFrozenSourceHTTPBindsAuthenticationToExactRequest(t *testing.T) {
	now := time.Unix(1800000000, 0)
	path := "/internal/creation/runs/" + uuid.NewString() + "/source"
	body := `{"payload_hash":"` + strings.Repeat("a", 64) + `"}`
	for _, name := range []string{"valid", "signature", "audience", "expired", "future", "path", "method", "body", "duplicate header", "query", "extra field", "duplicate field", "trailing object", "oversized"} {
		t.Run(name, func(t *testing.T) {
			stub := &sourceHTTPStub{}
			handler, err := httpapi.NewSourceHandler(stub, sourceTestSecret, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			handler.Register(mux)
			requestBody := body
			requestPath := path
			if name == "extra field" {
				requestBody = strings.TrimSuffix(body, "}") + `,"project_id":"foreign"}`
			}
			if name == "duplicate field" {
				requestBody = strings.TrimSuffix(body, "}") + `,"payload_hash":"` + strings.Repeat("b", 64) + `"}`
			}
			if name == "trailing object" {
				requestBody += "{}"
			}
			if name == "oversized" {
				requestBody = strings.Repeat("x", 4097)
			}
			hash := sha256.Sum256([]byte(requestBody))
			claims := map[string]any{"audience": "lanverse.creation.platform.source", "method": "POST", "path": path, "body_hash": hex.EncodeToString(hash[:]), "expires_at": now.Unix() + 60}
			switch name {
			case "audience":
				claims["audience"] = "lanverse.creation.command"
			case "expired":
				claims["expires_at"] = now.Unix()
			case "future":
				claims["expires_at"] = now.Unix() + 61
			case "path":
				claims["path"] = path + "/wrong"
			case "method":
				claims["method"] = "GET"
			case "body":
				claims["body_hash"] = strings.Repeat("f", 64)
			case "query":
				requestPath += "?source_url=https://foreign.invalid"
			}
			token := signSource(t, claims)
			if name == "signature" {
				token += "x"
			}
			request := httptest.NewRequest("POST", requestPath, strings.NewReader(requestBody))
			request.Header.Set("X-Lanverse-Creation-Authorization", token)
			if name == "duplicate header" {
				request.Header.Add("X-Lanverse-Creation-Authorization", token)
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if name == "valid" {
				if response.Code != 200 || stub.calls != 1 || !strings.Contains(response.Body.String(), "甲😀") {
					t.Fatalf("valid source: %d %s", response.Code, response.Body.String())
				}
				return
			}
			if response.Code < 400 || stub.calls != 0 {
				t.Fatalf("invalid request reached source: %d calls=%d", response.Code, stub.calls)
			}
		})
	}
}
