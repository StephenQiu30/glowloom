package agenthttp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"

	app "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
)

func (c *Client) Execution(ctx context.Context, run domain.Run) (domain.ExecutionSnapshot, error) {
	var value domain.ExecutionSnapshot
	err := c.readProposal(ctx, run, "/internal/creation/commands/"+run.Command.RunID+"/execution", &value)
	var problem *app.Error
	if errors.As(err, &problem) && problem.Code == "agent_execution_not_started" {
		return domain.ExecutionSnapshot{Schema: "creation-execution-production", CommandID: run.Command.RunID, RunID: run.Command.RunID, PayloadHash: run.PayloadHash, SourceRevisionID: run.Command.Source.RevisionID, Status: "queued", Steps: []json.RawMessage{}, Outputs: []domain.DraftRef{}}, nil
	}
	return value, err
}
func (c *Client) Draft(ctx context.Context, run domain.Run, id string) (domain.DraftEnvelope, error) {
	var value domain.DraftEnvelope
	err := c.readProposal(ctx, run, "/internal/creation/commands/"+run.Command.RunID+"/drafts/"+id, &value)
	return value, err
}
func (c *Client) readProposal(ctx context.Context, run domain.Run, path string, target any) error {
	return c.proposalRequest(ctx, run, http.MethodGet, path, nil, target)
}
func (c *Client) Resume(ctx context.Context, run domain.Run) (domain.ResumeReceipt, error) {
	body, err := json.Marshal(struct {
		PayloadHash string `json:"payload_hash"`
	}{run.PayloadHash})
	if err != nil {
		return domain.ResumeReceipt{}, err
	}
	var receipt domain.ResumeReceipt
	err = c.proposalRequest(ctx, run, http.MethodPost, "/internal/creation/commands/"+run.Command.RunID+"/resume", body, &receipt)
	return receipt, err
}
func (c *Client) proposalRequest(ctx context.Context, run domain.Run, method, path string, body []byte, target any) error {
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(run.Endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	payload, err := json.Marshal(authorization{Audience: "lanverse.creation.command", Method: method, Path: request.URL.EscapedPath(), BodyHash: hex.EncodeToString(hash[:]), ExpiresAt: c.now().Add(time.Minute).Unix()})
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := hmac.New(sha256.New, c.secret)
	_, _ = signature.Write([]byte(encoded))
	request.Header.Set("X-Lanverse-Creation-Authorization", encoded+"."+base64.RawURLEncoding.EncodeToString(signature.Sum(nil)))
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return transportError(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4097))
		if readErr != nil {
			return transportError(readErr)
		}
		var failure struct {
			Detail string `json:"detail"`
		}
		if len(raw) > 4096 || canonical.Decode(raw, &failure) != nil {
			return app.Problem("agent_response_invalid", 502)
		}
		if response.StatusCode == http.StatusNotFound {
			switch failure.Detail {
			case "creation_execution_not_started":
				return app.Problem("agent_execution_not_started", 404)
			case "creation_command_not_found", "creation_step_not_found", "creation_draft_not_found":
				return app.ErrNotFound
			default:
				return app.Problem("creation_trace_unavailable", 503)
			}
		}
		if response.StatusCode == http.StatusConflict {
			if method == http.MethodPost && strings.HasSuffix(path, "/resume") {
				return app.Problem("creation_not_resumable", 409)
			}
			return app.Problem("agent_data_conflict", 409)
		}
		if response.StatusCode == http.StatusServiceUnavailable {
			return app.Problem("creation_trace_unavailable", 503)
		}
		return app.Problem("agent_proposal_read_failed", 502)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024+1))
	if err != nil {
		return transportError(err)
	}
	if len(raw) > 16*1024*1024 {
		return app.Problem("agent_proposal_too_large", 502)
	}
	if err = canonical.Decode(raw, target); err != nil {
		return fmt.Errorf("agent response: %w: %w", app.Problem("agent_proposal_invalid", 502), err)
	}
	return nil
}

func transportError(err error) error {
	status, code := 502, "agent_transport_failed"
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		status, code = 504, "agent_timeout"
	}
	return fmt.Errorf("creation agent transport: %w: %w", app.Problem(code, status), err)
}
