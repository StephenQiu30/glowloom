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

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	agenthttp "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/agenthttp"
	httpapi "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/httpapi"
	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

type proposalHTTPAuth struct{}

func (proposalHTTPAuth) Authenticate(*http.Request) (authentication.Claims, error) {
	return authentication.Claims{}, nil
}

type gateHTTPService struct {
	httpapi.ProposalService
	calls int
}

func (s *gateHTTPService) Resolve(_ context.Context, _, gate string, _ app.ResolveCommand) (domain.GateResult, error) {
	s.calls++
	return domain.GateResult{Gate: gate, Status: "pending", Receipts: []domain.AdoptionReceipt{}}, nil
}

func (s *gateHTTPService) Resume(_ context.Context, _ app.Actor, runID string, revision int64) (domain.ResumeReceipt, error) {
	if revision != 3 {
		return domain.ResumeReceipt{}, app.Problem("revision_conflict", 409)
	}
	return domain.ResumeReceipt{Schema: "creation-resume-production", CommandID: runID, RunID: runID, Status: "resume_requested"}, nil
}

func TestCreationPublicResumeReportsAsynchronousAcceptance(t *testing.T) {
	handler, err := httpapi.NewProposalHandler(&gateHTTPService{}, proposalHTTPAuth{}, sourceTestSecret, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/creation-runs/44444444-4444-4444-8444-444444444444/resume", strings.NewReader(`{"expected_revision":3}`)))
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"status":"resume_requested"`) {
		t.Fatalf("resume did not distinguish request acceptance: %d %s", response.Code, response.Body.String())
	}
}

func TestCreationGateRequiresExactMethodPathBodyAudienceAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	service := &gateHTTPService{}
	handler, err := httpapi.NewProposalHandler(service, proposalHTTPAuth{}, sourceTestSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	path := "/internal/creation/runs/44444444-4444-4444-8444-444444444444/gates/map_manuscript/resolve"
	body := `{"payload_hash":"` + strings.Repeat("a", 64) + `","drafts":[]}`
	hash := sha256.Sum256([]byte(body))
	for _, mode := range []string{"valid", "source-audience", "method", "path", "body", "expired", "future", "duplicate-header"} {
		t.Run(mode, func(t *testing.T) {
			claims := map[string]any{"audience": "lanverse.creation.platform", "method": "POST", "path": path, "body_hash": hex.EncodeToString(hash[:]), "expires_at": now.Add(time.Minute).Unix()}
			switch mode {
			case "source-audience":
				claims["audience"] = "lanverse.creation.platform.source"
			case "method":
				claims["method"] = "GET"
			case "path":
				claims["path"] = path + "other"
			case "body":
				claims["body_hash"] = strings.Repeat("b", 64)
			case "expired":
				claims["expires_at"] = now.Unix()
			case "future":
				claims["expires_at"] = now.Add(61 * time.Second).Unix()
			}
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			token := signSource(t, claims)
			request.Header.Set("X-Lanverse-Creation-Authorization", token)
			if mode == "duplicate-header" {
				request.Header.Add("X-Lanverse-Creation-Authorization", token)
			}
			response := httptest.NewRecorder()
			before := service.calls
			mux.ServeHTTP(response, request)
			if mode == "valid" {
				if response.Code != 200 || service.calls != before+1 || !strings.Contains(response.Body.String(), `"status":"pending"`) {
					t.Fatalf("valid gate: %d %s", response.Code, response.Body.String())
				}
			} else if response.Code != 401 || service.calls != before {
				t.Fatalf("invalid authorization invoked gate: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCreationRuntimeClientSignsResumeAndTreatsNotStartedAsQueued(t *testing.T) {
	run := frozenRun(t)
	now := time.Now().UTC()
	calls := 0
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		parts := strings.Split(r.Header.Get("X-Lanverse-Creation-Authorization"), ".")
		if len(parts) != 2 {
			t.Error("missing signature")
			w.WriteHeader(401)
			return
		}
		payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
		signature, _ := base64.RawURLEncoding.DecodeString(parts[1])
		mac := hmac.New(sha256.New, []byte(sourceTestSecret))
		_, _ = mac.Write([]byte(parts[0]))
		var claims struct {
			Audience, Method, Path string
			BodyHash               string `json:"body_hash"`
			ExpiresAt              int64  `json:"expires_at"`
		}
		_ = json.Unmarshal(payload, &claims)
		hash := sha256.Sum256(body)
		if !hmac.Equal(signature, mac.Sum(nil)) || claims.Audience != "lanverse.creation.command" || claims.Method != r.Method || claims.Path != r.URL.EscapedPath() || claims.BodyHash != hex.EncodeToString(hash[:]) || claims.ExpiresAt != now.Add(time.Minute).Unix() {
			t.Error("runtime signature lost binding")
			w.WriteHeader(401)
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"detail":"creation_execution_not_started"}`)
			return
		}
		var input map[string]string
		if json.Unmarshal(body, &input) != nil || len(input) != 1 || input["payload_hash"] != run.PayloadHash {
			t.Error("resume input changed frozen command")
			w.WriteHeader(422)
			return
		}
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(domain.ResumeReceipt{Schema: "creation-resume-production", CommandID: run.Command.RunID, RunID: run.Command.RunID, Status: "resume_requested"})
	}))
	defer peer.Close()
	run.Endpoint = peer.URL
	client, err := agenthttp.New(agenthttp.Config{Secret: sourceTestSecret}, peer.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	execution, err := client.Execution(context.Background(), run)
	if err != nil || execution.Status != "queued" || execution.RunID != run.Command.RunID {
		t.Fatalf("not-started execution=%+v err=%v", execution, err)
	}
	receipt, err := client.Resume(context.Background(), run)
	if err != nil || receipt.RunID != run.Command.RunID || receipt.Status != "resume_requested" || calls != 2 {
		t.Fatalf("resume=%+v err=%v", receipt, err)
	}
}

func TestCreationExecutionDecodesPythonOutputBindings(t *testing.T) {
	run := frozenRun(t)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"schema":"creation-execution-production","status":"waiting_review","stage":"map_manuscript","last_error":null,"outputs":[{"step_id":"11111111-1111-4111-8111-111111111111","step_key":"map_manuscript","output_role":"candidate","item_key":"primary","draft_id":"22222222-2222-4222-8222-222222222222","candidate_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","result_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`)
	}))
	defer peer.Close()
	run.Endpoint = peer.URL
	client, err := agenthttp.New(agenthttp.Config{Secret: sourceTestSecret}, peer.Client(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.Execution(context.Background(), run)
	if err != nil || len(snapshot.Outputs) != 1 {
		t.Fatalf("real output binding rejected: %+v %v", snapshot, err)
	}
}
