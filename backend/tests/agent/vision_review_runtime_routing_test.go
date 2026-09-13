package agent_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentclient "github.com/StephenQiu30/lanverse/backend/internal/agent/client"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func visionInvocationWithImages(t *testing.T) (contract.VisionReviewInvocation, [][]byte) {
	t.Helper()
	invocation := validVisionReviewInvocation(t)
	input := invocation.Payload.StageInput
	images := make([][]byte, len(input.Attachments))
	for index := range input.Attachments {
		picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
		picture.Set(0, 0, color.RGBA{R: uint8(index * 70), G: 30, B: 40, A: 255})
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, picture); err != nil {
			t.Fatal(err)
		}
		images[index] = buffer.Bytes()
		digest := sha256.Sum256(images[index])
		input.Subject.Slots[index].SHA256 = hex.EncodeToString(digest[:])
		input.Attachments[index].Slot = input.Subject.Slots[index]
		input.Attachments[index].ByteLength = int64(len(images[index]))
		input.Attachments[index].PixelWidth = 2
		input.Attachments[index].PixelHeight = 2
	}
	var err error
	invocation.Payload.StageInput, err = contract.BuildVisionReviewInput(input)
	if err != nil {
		t.Fatal(err)
	}
	invocation.InputHash, err = invocation.ComputeInputHash()
	if err != nil {
		t.Fatal(err)
	}
	return invocation, images
}

func TestVisionReviewHTTPClientSendsOneExactPrivateMultipartGroup(t *testing.T) {
	invocation, images := visionInvocationWithImages(t)
	signer, err := grant.NewSigner("synthetic-vision-client-secret-not-a-credential", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := signer.IssueVisionReviewDispatchAuthorization(invocation, 1)
	if err != nil {
		t.Fatal(err)
	}
	fixture := readVisionReviewWireFixture(t)
	result, err := contract.DecodeVisionReviewAttemptResult(fixture.AcceptedResult)
	if err != nil {
		t.Fatal(err)
	}
	candidate, _, err := contract.DecodeVisionReviewCandidate(result.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Subject = invocation.Payload.StageInput.Subject
	result.Candidate = mustJSON(t, candidate)
	outputHash, err := contract.ProductionCanonicalHash(result.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	result.OutputHash = &outputHash
	result.InputHash = invocation.InputHash
	result.DispatchAuthorizationHash = authorization.Hash
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(invocation, 1, authorization.Hash); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	responseMode := "accepted"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/internal/storygraph/vision-review/invocations" || r.Method != http.MethodPost ||
			r.Header.Get("X-Lanverse-Dispatch-Authorization") != authorization.Value || r.ContentLength <= 0 {
			t.Error("Vision Review request identity is invalid")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		decoded, err := contract.DecodeVisionReviewInvocation([]byte(r.FormValue("invocation")))
		if err != nil || !reflect.DeepEqual(decoded, invocation) {
			t.Errorf("invocation drift: %v", err)
		}
		files := r.MultipartForm.File["media"]
		if len(files) != len(images) {
			t.Error("incomplete image group")
			return
		}
		for index, file := range files {
			stream, err := file.Open()
			if err != nil {
				t.Error(err)
				return
			}
			contents, readErr := io.ReadAll(stream)
			_ = stream.Close()
			if readErr != nil || file.Filename != invocation.Payload.StageInput.Attachments[index].Slot.SlotKey ||
				file.Header.Get("Content-Type") != "image/png" || !bytes.Equal(contents, images[index]) {
				t.Error("image identity drift")
			}
		}
		switch responseMode {
		case "oversized":
			_, _ = io.WriteString(w, strings.Repeat("x", (1<<20)+1))
		case "stale":
			_, _ = w.Write(fixture.AcceptedResult)
		case "failure":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "redirect":
			w.Header().Set("Location", "/unexpected")
			w.WriteHeader(http.StatusTemporaryRedirect)
		default:
			_, _ = w.Write(mustJSON(t, result))
		}
	}))
	t.Cleanup(server.Close)
	catalog, err := contract.NewRuntimeCatalog([]contract.RuntimeRevision{{
		BundleHash: invocation.StageRelease.BundleContentHash, BaseURL: server.URL,
		ImageDigest: invocation.StageRelease.AgentImageDigest,
	}})
	if err != nil {
		t.Fatal(err)
	}
	client := agentclient.New(catalog, nil, server.Client())
	got, err := client.InvokeVisionReview(context.Background(), invocation, authorization, images)
	if err != nil || got.ResultHash != result.ResultHash || requests.Load() != 1 {
		t.Fatalf("vision dispatch failed: requests=%d err=%v", requests.Load(), err)
	}
	for _, mode := range []string{"oversized", "stale", "failure", "redirect"} {
		responseMode = mode
		before := requests.Load()
		if _, err := client.InvokeVisionReview(context.Background(), invocation, authorization, images); err == nil {
			t.Errorf("%s response accepted", mode)
		}
		if requests.Load() != before+1 {
			t.Error("HTTP request was automatically retried")
		}
	}
	for _, mutation := range []string{"missing", "bytes", "expired", "cancelled", "image"} {
		t.Run(mutation, func(t *testing.T) {
			before := requests.Load()
			media := append([][]byte(nil), images...)
			auth := authorization
			frozen := invocation
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mutation {
			case "missing":
				media = media[:1]
			case "bytes":
				media[0] = []byte("drifted")
			case "expired":
				auth.ExpiresAt = time.Now().Add(-time.Minute)
			case "cancelled":
				cancel()
			case "image":
				frozen.StageRelease.AgentImageDigest = "sha256:" + strings.Repeat("8", 64)
				frozen.InputHash, _ = frozen.ComputeInputHash()
			}
			if _, err := client.InvokeVisionReview(ctx, frozen, auth, media); err == nil || requests.Load() != before {
				t.Errorf("invalid input dispatched: %v", err)
			}
		})
	}
	if len(images[0]) == 0 || images[0][0] != 0x89 {
		t.Error("client cleared caller-owned image bytes")
	}
}
