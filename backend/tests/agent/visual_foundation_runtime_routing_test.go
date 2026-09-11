package agent_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	agentclient "github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestVisualFoundationMediaBrokerReadsAndDispatchesExactFrozenMedia(t *testing.T) {
	contents := bytes.Repeat([]byte("visual-reference"), 64)
	invocation := visualFoundationInvocationWithMedia(t, contents)
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := signer.IssueVisualFoundationDispatchAuthorization(invocation, 1)
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := json.Marshal(validVisualFoundationResult(t, invocation, authorization))
	if err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/internal/storygraph/visual-foundation/invocations" ||
			request.Header.Get("X-Lanverse-Dispatch-Authorization") != authorization.Value ||
			request.ContentLength < int64(len(contents)) {
			t.Errorf("unexpected Visual Foundation request: path=%s length=%d", request.URL.Path, request.ContentLength)
		}
		if err := request.ParseMultipartForm(contract.MaxVisualFoundationTotalImageBytes + 1<<20); err != nil {
			t.Errorf("parse Visual Foundation multipart request: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = request.MultipartForm.RemoveAll() }()
		decoded, err := contract.DecodeVisualFoundationInvocation([]byte(request.FormValue("invocation")))
		if err != nil || !reflect.DeepEqual(decoded, invocation) {
			t.Errorf("Visual Foundation invocation drifted: %v", err)
		}
		files := request.MultipartForm.File["media"]
		if len(files) != 1 || files[0].Filename != invocation.Payload.MediaAttachments[0].AttachmentID ||
			files[0].Header.Get("Content-Type") != invocation.Payload.MediaAttachments[0].MediaType {
			t.Errorf("Visual Foundation media identity drifted: %#v", files)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		file, err := files[0].Open()
		if err != nil {
			t.Errorf("open Visual Foundation media: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		received, err := io.ReadAll(file)
		if err != nil || !bytes.Equal(received, contents) {
			t.Errorf("Visual Foundation media bytes drifted: %v", err)
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
	objects := &visualFoundationObjectReader{contents: contents}
	broker, err := agentapp.NewVisualFoundationMediaBroker(
		objects,
		agentclient.New(catalog, nil, server.Client()),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := broker.Invoke(context.Background(), invocation, authorization)
	if err != nil || result.Status != "accepted" || requests.Load() != 1 {
		t.Fatalf("Visual Foundation dispatch: result=%#v requests=%d err=%v", result, requests.Load(), err)
	}
	attachment := invocation.Payload.MediaAttachments[0]
	if len(objects.calls) != 1 || objects.calls[0] != (visualFoundationReadCall{
		objectKey: attachment.ObjectKey, expectedSize: attachment.ByteLength,
		expectedSHA256: attachment.ContentHash, maxBytes: contract.MaxVisualFoundationImageBytes,
	}) {
		t.Fatalf("unexpected Visual Foundation object read: %#v", objects.calls)
	}
}

func TestVisualFoundationMediaBrokerRejectsObjectContentDriftBeforeHTTP(t *testing.T) {
	contents := bytes.Repeat([]byte("visual-reference"), 64)
	invocation := visualFoundationInvocationWithMedia(t, contents)
	authorization := contract.SceneAnalysisDispatchAuthorization{
		Value: "dispatch-token", Hash: visualFoundationHash("dispatch-token"),
		ClaimVersion: 1, ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	t.Cleanup(server.Close)
	catalog, err := contract.NewRuntimeCatalog([]contract.RuntimeRevision{{
		BundleHash: invocation.StageRelease.BundleContentHash,
		BaseURL:    server.URL, ImageDigest: invocation.StageRelease.AgentImageDigest,
	}})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := agentapp.NewVisualFoundationMediaBroker(
		&visualFoundationObjectReader{contents: append([]byte(nil), contents[:len(contents)-1]...)},
		agentclient.New(catalog, nil, server.Client()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = broker.Invoke(context.Background(), invocation, authorization); err == nil || requests.Load() != 0 {
		t.Fatalf("drifted Visual Foundation media reached HTTP: requests=%d err=%v", requests.Load(), err)
	}
}

type visualFoundationReadCall struct {
	objectKey, expectedSHA256 string
	expectedSize, maxBytes    int64
}

type visualFoundationObjectReader struct {
	contents []byte
	calls    []visualFoundationReadCall
}

func (reader *visualFoundationObjectReader) ReadVerified(
	_ context.Context,
	objectKey string,
	expectedSize int64,
	expectedSHA256 string,
	maxBytes int64,
) ([]byte, error) {
	reader.calls = append(reader.calls, visualFoundationReadCall{
		objectKey: objectKey, expectedSize: expectedSize,
		expectedSHA256: expectedSHA256, maxBytes: maxBytes,
	})
	return append([]byte(nil), reader.contents...), nil
}

func visualFoundationInvocationWithMedia(t *testing.T, contents []byte) contract.VisualFoundationInvocation {
	t.Helper()
	invocation := validVisualFoundationInvocation(t)
	digest := sha256.Sum256(contents)
	contentHash := hex.EncodeToString(digest[:])
	invocation.Payload.MediaAttachments[0].ByteLength = int64(len(contents))
	invocation.Payload.MediaAttachments[0].ContentHash = contentHash
	invocation.Payload.StageInput.ReferenceAttachments[0].ContentHash = contentHash
	references, err := json.Marshal(invocation.Payload.StageInput.ReferenceAttachments)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Payload.StageInput.ReferenceAttachmentsHash, err = contract.ProductionCanonicalHash(references)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err = contract.NewVisualFoundationInvocation(
		invocation.InvocationID,
		invocation.AttemptID,
		invocation.StageRelease,
		invocation.Control,
		invocation.Budget,
		invocation.Payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func validVisualFoundationResult(
	t *testing.T,
	invocation contract.VisualFoundationInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) contract.VisualFoundationAttemptResult {
	t.Helper()
	candidate := visualFoundationCandidateJSON(t, invocation.Payload.StageInput)
	outputHash, err := contract.ProductionCanonicalHash(candidate)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := []contract.SceneAnalysisDiagnostic{}
	diagnosticJSON, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(diagnosticJSON)
	if err != nil {
		t.Fatal(err)
	}
	result := contract.VisualFoundationAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease,
		Control: invocation.Control, ClaimVersion: authorization.ClaimVersion,
		DispatchAuthorizationHash: authorization.Hash, Status: "accepted",
		CandidateType: "visual_foundation_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, OutputHash: &outputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: time.Date(2026, 9, 12, 12, 1, 0, 0, time.UTC),
		Executor: contract.VisualFoundationExecutor{
			RuntimeClass: "vision", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "visual-foundation-harness", Model: "codex-cli-default",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	return result
}
