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
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const maxVisionReviewEnvelopeBytes = 1 << 20

// InvokeVisionReview transports one complete frozen group. Authorization issuance,
// current Owner checks and durable attempt ownership belong to the calling application.
func (client *HTTP) InvokeVisionReview(
	ctx context.Context,
	invocation contract.VisionReviewInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	images [][]byte,
) (contract.VisionReviewAttemptResult, error) {
	if err := ctx.Err(); err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	if err := invocation.Validate(); err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	if err := authorization.Validate(); err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	if !authorization.ExpiresAt.After(time.Now()) {
		return contract.VisionReviewAttemptResult{}, fmt.Errorf("Vision Review dispatch authorization expired")
	}
	attachments := invocation.Payload.StageInput.Attachments
	if len(images) != len(attachments) {
		return contract.VisionReviewAttemptResult{}, fmt.Errorf("Vision Review media set is incomplete")
	}
	for index, contents := range images {
		attachment := attachments[index]
		digest := sha256.Sum256(contents)
		if int64(len(contents)) != attachment.ByteLength || hex.EncodeToString(digest[:]) != attachment.Slot.SHA256 {
			return contract.VisionReviewAttemptResult{}, fmt.Errorf("Vision Review media content drifted")
		}
	}
	runtime, err := client.runtimes.Resolve(invocation.StageRelease.BundleContentHash)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	if runtime.ImageDigest != invocation.StageRelease.AgentImageDigest {
		return contract.VisionReviewAttemptResult{}, contract.ErrSkillBundleUnavailable
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	if len(encoded) > maxVisionReviewEnvelopeBytes {
		return contract.VisionReviewAttemptResult{}, fmt.Errorf("Vision Review invocation exceeds byte budget")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("invocation", string(encoded)); err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	for index, contents := range images {
		attachment := attachments[index]
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
			"name": "media", "filename": attachment.Slot.SlotKey,
		}))
		header.Set("Content-Type", attachment.MediaType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return contract.VisionReviewAttemptResult{}, err
		}
		if _, err := part.Write(contents); err != nil {
			return contract.VisionReviewAttemptResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(invocation.Budget.MaxExecutionSeconds)*time.Second+30*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(runtime.BaseURL, "/") + "/internal/storygraph/vision-review/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	request.GetBody = nil
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Lanverse-Dispatch-Authorization", authorization.Value)
	transport := *client.client
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := transport.Do(request)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, fmt.Errorf("Vision Review outcome unknown: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return contract.VisionReviewAttemptResult{}, fmt.Errorf("Vision Review returned HTTP %d", response.StatusCode)
	}
	resultBytes, err := io.ReadAll(io.LimitReader(response.Body, maxVisionReviewEnvelopeBytes+1))
	if err != nil || len(resultBytes) > maxVisionReviewEnvelopeBytes {
		return contract.VisionReviewAttemptResult{}, errorsOrLimit(err)
	}
	result, err := contract.DecodeVisionReviewAttemptResult(resultBytes)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, fmt.Errorf("decode Vision Review result: %w", err)
	}
	if err := result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash); err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	return result, nil
}
