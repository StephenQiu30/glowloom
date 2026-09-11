package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const maxResultBytes = 16 << 20

type GrantIssuer interface {
	Issue(contract.StageInvocation, int, int64) (string, error)
}

func (client *HTTP) InvokeVisualFoundation(
	ctx context.Context,
	invocation contract.VisualFoundationInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	media []agentapp.VisualFoundationMedia,
) (contract.VisualFoundationAttemptResult, error) {
	if err := invocation.Validate(); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	if err := authorization.Validate(); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	if len(media) != len(invocation.Payload.MediaAttachments) {
		return contract.VisualFoundationAttemptResult{}, fmt.Errorf("Visual Foundation media set is incomplete")
	}
	runtime, err := client.runtimes.Resolve(invocation.StageRelease.BundleContentHash)
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	if runtime.ImageDigest != invocation.StageRelease.AgentImageDigest {
		return contract.VisualFoundationAttemptResult{}, contract.ErrSkillBundleUnavailable
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	encoded, err := json.Marshal(invocation)
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	invocationPart, err := writer.CreateFormField("invocation")
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	if _, err = invocationPart.Write(encoded); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	for index, value := range media {
		attachment := invocation.Payload.MediaAttachments[index]
		digest := sha256.Sum256(value.Contents)
		if value.AttachmentID != attachment.AttachmentID || value.MediaType != attachment.MediaType ||
			int64(len(value.Contents)) != attachment.ByteLength ||
			hex.EncodeToString(digest[:]) != attachment.ContentHash {
			return contract.VisualFoundationAttemptResult{}, fmt.Errorf("Visual Foundation media content drifted")
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
			"name": "media", "filename": value.AttachmentID,
		}))
		header.Set("Content-Type", value.MediaType)
		part, createErr := writer.CreatePart(header)
		if createErr != nil {
			return contract.VisualFoundationAttemptResult{}, createErr
		}
		if _, createErr = part.Write(value.Contents); createErr != nil {
			return contract.VisualFoundationAttemptResult{}, createErr
		}
	}
	if err = writer.Close(); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	endpoint := strings.TrimRight(runtime.BaseURL, "/") + "/internal/storygraph/visual-foundation/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Lanverse-Dispatch-Authorization", authorization.Value)
	response, err := client.client.Do(request)
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, fmt.Errorf("Visual Foundation outcome unknown: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return contract.VisualFoundationAttemptResult{}, fmt.Errorf("Visual Foundation returned HTTP %d", response.StatusCode)
	}
	resultBytes, err := io.ReadAll(io.LimitReader(response.Body, maxResultBytes+1))
	if err != nil || len(resultBytes) > maxResultBytes {
		return contract.VisualFoundationAttemptResult{}, errorsOrLimit(err)
	}
	result, err := contract.DecodeVisualFoundationAttemptResult(resultBytes)
	if err != nil {
		return contract.VisualFoundationAttemptResult{}, fmt.Errorf("decode Visual Foundation result: %w", err)
	}
	if err = result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	return result, nil
}

func (client *HTTP) InvokeSceneAnalysis(
	ctx context.Context,
	invocation contract.SceneAnalysisInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.SceneAnalysisAttemptResult, error) {
	if err := invocation.Validate(); err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	if err := authorization.Validate(); err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	runtime, err := client.runtimes.Resolve(invocation.StageRelease.BundleContentHash)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	if runtime.ImageDigest != invocation.StageRelease.AgentImageDigest {
		return contract.SceneAnalysisAttemptResult{}, contract.ErrSkillBundleUnavailable
	}
	body, err := json.Marshal(invocation)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	endpoint := strings.TrimRight(runtime.BaseURL, "/") + "/internal/storygraph/scene-analysis/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Lanverse-Dispatch-Authorization", authorization.Value)
	response, err := client.client.Do(request)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, fmt.Errorf("scene Analysis outcome unknown: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return contract.SceneAnalysisAttemptResult{}, fmt.Errorf("scene Analysis returned HTTP %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResultBytes+1))
	if err != nil || len(encoded) > maxResultBytes {
		return contract.SceneAnalysisAttemptResult{}, errorsOrLimit(err)
	}
	result, err := contract.DecodeSceneAnalysisAttemptResult(encoded)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, fmt.Errorf("decode Scene Analysis result: %w", err)
	}
	if err = result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash); err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	return result, nil
}

type HTTP struct {
	runtimes contract.RuntimeCatalog
	client   *http.Client
	grants   GrantIssuer
}

func New(runtimes contract.RuntimeCatalog, grants GrantIssuer, httpClient *http.Client) *HTTP {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &HTTP{runtimes: runtimes, client: httpClient, grants: grants}
}

func (client *HTTP) Invoke(ctx context.Context, invocation contract.StageInvocation, attempt int, fencingToken int64) (contract.StageResult, error) {
	if err := invocation.Validate(); err != nil {
		return contract.StageResult{}, err
	}
	runtime, err := client.runtimes.Resolve(invocation.ExecutionPolicy.SkillBundleHash)
	if err != nil {
		return contract.StageResult{}, err
	}
	body, err := json.Marshal(invocation)
	if err != nil {
		return contract.StageResult{}, err
	}
	grantValue, err := client.grants.Issue(invocation, attempt, fencingToken)
	if err != nil {
		return contract.StageResult{}, err
	}
	endpoint := strings.TrimRight(runtime.BaseURL, "/") + "/internal/storygraph/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return contract.StageResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Lanverse-Execution-Grant", grantValue)
	response, err := client.client.Do(request)
	if err != nil {
		return contract.StageResult{}, fmt.Errorf("agent invocation outcome unknown: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return contract.StageResult{}, fmt.Errorf("agent invocation returned HTTP %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResultBytes+1))
	if err != nil || len(encoded) > maxResultBytes {
		return contract.StageResult{}, errorsOrLimit(err)
	}
	result, err := contract.DecodeStageResult(encoded)
	if err != nil {
		return contract.StageResult{}, fmt.Errorf("decode agent result: %w", err)
	}
	if err = result.ValidateFor(invocation); err != nil {
		return contract.StageResult{}, err
	}
	return result, nil
}

func errorsOrLimit(err error) error {
	if err != nil {
		return fmt.Errorf("read agent result: %w", err)
	}
	return fmt.Errorf("agent result exceeds %d bytes", maxResultBytes)
}
