package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	agentclient "github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestReferenceBriefHTTPClientDispatchesExactFrozenInvocation(t *testing.T) {
	invocation := validReferenceBriefInvocation(t)
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := signer.IssueReferenceBriefDispatchAuthorization(invocation, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&referenceBriefExecutionRuntime{t: t}).InvokeReferenceBrief(
		context.Background(), invocation, authorization,
	)
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/internal/storygraph/reference-brief/invocations" ||
			request.Header.Get("X-Lanverse-Dispatch-Authorization") != authorization.Value ||
			request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Reference Brief request: path=%s headers=%v", request.URL.Path, request.Header)
		}
		var decoded contract.ReferenceBriefInvocation
		decoder := json.NewDecoder(request.Body)
		if decodeErr := decoder.Decode(&decoded); decodeErr != nil || !reflect.DeepEqual(decoded, invocation) {
			t.Errorf("Reference Brief invocation drifted: %v", decodeErr)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(resultJSON)
	}))
	t.Cleanup(server.Close)
	catalog, err := contract.NewRuntimeCatalog([]contract.RuntimeRevision{{
		BundleHash: invocation.StageRelease.BundleContentHash,
		BaseURL:    server.URL, ImageDigest: invocation.StageRelease.AgentImageDigest,
	}})
	if err != nil {
		t.Fatal(err)
	}
	client := agentclient.New(catalog, nil, server.Client())
	got, err := client.InvokeReferenceBrief(context.Background(), invocation, authorization)
	if err != nil || got.ResultHash != result.ResultHash || requests.Load() != 1 {
		t.Fatalf("Reference Brief dispatch: result=%#v requests=%d err=%v", got, requests.Load(), err)
	}
}
