package agent_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentclient "github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestSceneAnalysisRuntimeRoutesOnlyExactBundleAndImage(t *testing.T) {
	fixture := loadStoryGraphSceneAnalysisWireFixture(t)
	invocation, err := contract.DecodeSceneAnalysisInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path != "/internal/storygraph/scene-analysis/invocations" ||
			request.Header.Get("X-Lanverse-Dispatch-Authorization") != "dispatch-token" {
			t.Errorf("unexpected Scene Analysis runtime request: path=%s headers=%v", request.URL.Path, request.Header)
		}
		response.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	authorizationValue := "dispatch-token"
	authorizationDigest := sha256.Sum256([]byte(authorizationValue))
	authorization := contract.SceneAnalysisDispatchAuthorization{
		Value: authorizationValue, Hash: hex.EncodeToString(authorizationDigest[:]),
		ClaimVersion: 1, ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	exactCatalog, err := contract.NewRuntimeCatalog([]contract.RuntimeRevision{{
		BundleHash: invocation.StageRelease.BundleContentHash, BaseURL: server.URL,
		ImageDigest: invocation.StageRelease.AgentImageDigest,
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agentclient.New(exactCatalog, nil, server.Client()).InvokeSceneAnalysis(
		context.Background(), invocation, authorization,
	)
	if err == nil || !strings.Contains(err.Error(), "returned HTTP 503") || requests != 1 {
		t.Fatalf("exact runtime route: requests=%d err=%v", requests, err)
	}

	mismatchedCatalog, err := contract.NewRuntimeCatalog([]contract.RuntimeRevision{{
		BundleHash: invocation.StageRelease.BundleContentHash, BaseURL: server.URL,
		ImageDigest: "sha256:" + strings.Repeat("9", 64),
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agentclient.New(mismatchedCatalog, nil, server.Client()).InvokeSceneAnalysis(
		context.Background(), invocation, authorization,
	)
	if !errors.Is(err, contract.ErrSkillBundleUnavailable) || requests != 1 {
		t.Fatalf("mismatched runtime route reached HTTP: requests=%d err=%v", requests, err)
	}
}
