package creation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/agenthttp"
	httpapi "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

type traceAccess struct {
	run domain.Run
	err error
}

func (s traceAccess) AuthorizedRun(context.Context, app.Actor, string, bool) (domain.Run, error) {
	return s.run, s.err
}

type tracePeer struct {
	value   domain.ManifestSnapshot
	history domain.AttemptHistory
	calls   int
}

func (p *tracePeer) Manifest(context.Context, domain.Run) (domain.ManifestSnapshot, error) {
	p.calls++
	return p.value, nil
}
func (p *tracePeer) Attempts(context.Context, domain.Run, string) (domain.AttemptHistory, error) {
	p.calls++
	return p.history, nil
}
func traceFixture(t *testing.T) (domain.Run, domain.ManifestSnapshot) {
	t.Helper()
	raw, e := os.ReadFile("testdata/command.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Command     domain.Command `json:"command"`
		PayloadHash string         `json:"payload_hash"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile("testdata/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var v domain.ManifestSnapshot
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	return domain.Run{Command: f.Command, PayloadHash: f.PayloadHash}, v
}
func TestManifestQueryValidatesPythonFixtureAndCurrentAccess(t *testing.T) {
	run, value := traceFixture(t)
	peer := &tracePeer{value: value}
	s := app.NewTraceService(traceAccess{run: run}, peer)
	if _, e := s.Manifest(context.Background(), app.Actor{}, run.Command.RunID); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"run", "source", "hash", "gate", "version", "availability"} {
		t.Run(mode, func(t *testing.T) {
			_, bad := traceFixture(t)
			switch mode {
			case "run":
				bad.RunID = uuid.NewString()
			case "source":
				bad.Manifest.SourceRevisionID = uuid.NewString()
			case "hash":
				hash := strings.Repeat("b", 64)
				bad.ManifestHash = &hash
			case "gate":
				bad.Manifest.Template.Stages[0].ReviewRequired = false
			case "version":
				bad.Manifest.Version = 2
			case "availability":
				bad.Availability = "not_frozen"
			}
			peer.value = bad
			if _, e := s.Manifest(context.Background(), app.Actor{}, run.Command.RunID); e == nil {
				t.Fatal("drift accepted")
			}
		})
	}
	peer.calls = 0
	s = app.NewTraceService(traceAccess{err: app.Problem("forbidden", 403)}, peer)
	if _, e := s.Manifest(context.Background(), app.Actor{}, run.Command.RunID); e == nil || peer.calls != 0 {
		t.Fatal("requested before authorization")
	}
	if _, e := s.Manifest(context.Background(), app.Actor{}, "invalid"); e == nil || peer.calls != 0 {
		t.Fatal("invalid identity queried")
	}
}
func TestAttemptsRejectCrossScopeAndInvalidTerminalRecords(t *testing.T) {
	run, _ := traceFixture(t)
	step, id := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	deadline, lease := now.Add(time.Minute), now.Add(2*time.Minute)
	h := domain.AttemptHistory{Schema: "creation-attempt-history-production", CommandID: run.Command.CommandID, RunID: run.Command.RunID, StepID: step, StepKey: "map_manuscript", CurrentAttemptID: &id, HistoryOrigin: "recorded", Attempts: []domain.Attempt{{AttemptID: id, AttemptNo: 1, Fence: 1, InputHash: strings.Repeat("a", 64), State: "running", StartedAt: now, ExecutionDeadline: deadline, LeaseExpiresAt: lease, UsageStatus: "unknown"}}}
	peer := &tracePeer{history: h}
	s := app.NewTraceService(traceAccess{run: run}, peer)
	if _, e := s.Attempts(context.Background(), app.Actor{}, run.Command.RunID, step); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"foreign-step", "foreign-run", "terminal", "fence", "time", "current", "legacy", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(h)
			var bad domain.AttemptHistory
			_ = json.Unmarshal(raw, &bad)
			switch mode {
			case "foreign-step":
				bad.StepID = uuid.NewString()
			case "foreign-run":
				bad.RunID = uuid.NewString()
			case "terminal":
				bad.Attempts[0].State = "succeeded"
			case "fence":
				bad.Attempts[0].Fence = 0
			case "time":
				bad.Attempts[0].ExecutionDeadline = now.Add(-time.Minute)
			case "current":
				bad.CurrentAttemptID = nil
			case "legacy":
				bad.HistoryOrigin = "unavailable"
			case "duplicate":
				bad.Attempts = append(bad.Attempts, bad.Attempts[0])
			}
			peer.history = bad
			if _, e := s.Attempts(context.Background(), app.Actor{}, run.Command.RunID, step); e == nil {
				t.Fatal("invalid attempt accepted")
			}
		})
	}
}
func TestTraceClientRejectsAmbiguousJSONAndMapsUnavailable(t *testing.T) {
	run, _ := traceFixture(t)
	raw, e := os.ReadFile("testdata/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"valid", "duplicate", "trailing", "unavailable", "unknown-field", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/internal/creation/commands/"+run.Command.RunID+"/manifest" || r.Header.Get("X-Lanverse-Creation-Authorization") == "" {
					t.Error("unsigned or wrong route")
				}
				switch mode {
				case "unavailable":
					w.WriteHeader(404)
					_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
				case "duplicate":
					_, _ = w.Write([]byte(`{"run_id":"foreign",` + string(raw[1:])))
				case "trailing":
					_, _ = w.Write(append(raw, []byte(` {}`)...))
				case "unknown-field":
					_, _ = w.Write([]byte(`{"prompt":"private",` + string(raw[1:])))
				case "redirect":
					w.Header().Set("Location", "http://127.0.0.1:1")
					w.WriteHeader(307)
				default:
					_, _ = w.Write(raw)
				}
			}))
			defer server.Close()
			run.Endpoint = server.URL
			client, e := adapter.New(strings.Repeat("test", 8), nil, time.Now)
			if e != nil {
				t.Fatal(e)
			}
			_, e = client.Manifest(context.Background(), run)
			if mode == "valid" {
				if e != nil {
					t.Fatal(e)
				}
			} else if e == nil {
				t.Fatal("unsafe upstream accepted")
			}
			if mode == "unavailable" {
				var p *app.Error
				if !errors.As(e, &p) || p.Status != 503 {
					t.Fatalf("missing capability = %v", e)
				}
			}
		})
	}
}

func TestUnknownExecution404CannotInventQueuedState(t *testing.T) {
	run, _ := traceFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
	}))
	defer server.Close()
	run.Endpoint = server.URL
	client, err := adapter.New(strings.Repeat("x", 32), nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.Execution(context.Background(), run)
	if err == nil || value.Status == "queued" {
		t.Fatal("missing route presented as queued execution")
	}
}

func TestTraceHTTPReadOnlyContract(t *testing.T) {
	run, value := traceFixture(t)
	peer := &tracePeer{value: value}
	service := app.NewTraceService(traceAccess{run: run}, peer)
	mux := http.NewServeMux()
	httpapi.NewTraceHandler(service, proposalHTTPAuth{}).Register(mux)
	path := "/api/creation-runs/" + run.Command.RunID + "/manifest"
	for _, mode := range []string{"valid", "query", "body", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			target, body := path, ""
			switch mode {
			case "query":
				target += "?refresh=true"
			case "body":
				body = "{}"
			case "invalid":
				target = "/api/creation-runs/bad/manifest"
			}
			before := peer.calls
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", target, strings.NewReader(body)))
			want := 422
			if mode == "valid" {
				want = 200
			}
			if response.Code != want || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("response=%d %s", response.Code, response.Body.String())
			}
			if mode != "valid" && peer.calls != before {
				t.Fatal("invalid query requested agent")
			}
		})
	}
}

type traceTransport func(*http.Request) (*http.Response, error)

func (f traceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type traceFailedBody struct{ err error }

func (b traceFailedBody) Read([]byte) (int, error) { return 0, b.err }
func (b traceFailedBody) Close() error             { return nil }

func TestTraceTransportPreservesCancellationAndBodyTimeout(t *testing.T) {
	run, _ := traceFixture(t)
	run.Endpoint = "http://agent.test"
	for _, body := range []bool{false, true} {
		cause := context.Canceled
		if body {
			cause = context.DeadlineExceeded
		}
		client, err := adapter.New(strings.Repeat("x", 32), &http.Client{Transport: traceTransport(func(r *http.Request) (*http.Response, error) {
			if body {
				return &http.Response{StatusCode: 200, Body: traceFailedBody{cause}, Header: make(http.Header)}, nil
			}
			return nil, cause
		})}, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Manifest(context.Background(), run)
		var problem *app.Error
		want := 502
		if body {
			want = 504
		}
		if !errors.Is(err, cause) || !errors.As(err, &problem) || problem.Status != want {
			t.Fatalf("lost transport cause/status: %v", err)
		}
	}
}
