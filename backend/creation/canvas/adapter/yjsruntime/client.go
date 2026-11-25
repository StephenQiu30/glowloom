// Package yjsruntime calls the private, isolated JavaScript candidate runtime over gRPC (preferred) or HTTP.
package yjsruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	canvaspb "github.com/StephenQiu30/glowloom-server/backend/internal/gen/canvas"
)

const maxRuntimeMessageBytes = 16 << 20

// Client has no credentials or addresses supplied by canvas clients.
type Client struct {
	base, token string
	http        *http.Client
}

// New constructs a private runtime client with bounded requests.
// Targets with "grpc://" scheme (or bare host:port) use gRPC; "http(s)://" uses HTTP.
func New(base, token string) *Client {
	return &Client{
		base:  strings.TrimRight(strings.TrimSpace(base), "/"),
		token: token,
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Initialize converts an authorized durable snapshot without committing it.
func (c *Client) Initialize(ctx context.Context, state application.YjsState) (application.YjsCandidate, error) {
	return c.call(ctx, "initialize", state, nil)
}

// Prepare computes an isolated candidate bound to the current durable state.
func (c *Client) Prepare(ctx context.Context, state application.YjsState, update application.YjsUpdate) (application.YjsCandidate, error) {
	return c.call(ctx, "prepare", struct {
		application.YjsState
		Update           []byte                `json:"update,omitempty"`
		UpdateID         string                `json:"updateId"`
		PayloadHash      string                `json:"payloadHash"`
		RestorationNodes []application.NodeDTO `json:"restorationNodes"`
	}{state, update.Update, update.UpdateID.String(), update.PayloadHash, state.RestorationNodes}, update.Update)
}

// Rename produces a title-only delta through the same isolated admission runtime.
func (c *Client) Rename(ctx context.Context, state application.YjsState, key uuid.UUID, name string) (application.YjsCandidate, error) {
	return c.call(ctx, "rename", struct {
		application.YjsState
		UpdateID uuid.UUID `json:"updateId"`
		Name     string    `json:"name"`
	}{state, key, name}, nil)
}

// Commands computes an exact-prefix semantic command through the shared model.
func (c *Client) Commands(ctx context.Context, state application.YjsState, key uuid.UUID, projection application.DocumentDTO) (application.YjsCandidate, error) {
	return c.call(ctx, "commands", struct {
		application.YjsState
		UpdateID   uuid.UUID               `json:"updateId"`
		Projection application.DocumentDTO `json:"projection"`
	}{state, key, projection}, nil)
}

func (c *Client) usesGRPC() bool {
	return strings.HasPrefix(c.base, "grpc://") ||
		(!strings.HasPrefix(c.base, "http://") && !strings.HasPrefix(c.base, "https://") && c.base != "")
}

func (c *Client) call(ctx context.Context, action string, input any, updateBytes []byte) (application.YjsCandidate, error) {
	var result application.YjsCandidate
	if c.base == "" || len(c.token) < 32 {
		return result, fmt.Errorf("collaboration runtime not configured")
	}
	if c.usesGRPC() {
		return c.callGRPC(ctx, action, input, updateBytes)
	}
	return c.callHTTP(ctx, action, input)
}

func (c *Client) callGRPC(ctx context.Context, action string, input any, updateBytes []byte) (application.YjsCandidate, error) {
	var result application.YjsCandidate
	body, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	target := strings.TrimPrefix(c.base, "grpc://")
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	callCtx = metadata.NewOutgoingContext(callCtx, metadata.Pairs("authorization", "Bearer "+c.token))

	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxRuntimeMessageBytes),
			grpc.MaxCallSendMsgSize(maxRuntimeMessageBytes),
		),
	)
	if err != nil {
		return result, fmt.Errorf("connect collaboration runtime: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := canvaspb.NewCanvasRuntimeClient(conn)
	req := &canvaspb.ComputeRequest{PayloadJson: body, UpdateBytes: updateBytes}

	var resp *canvaspb.ComputeResponse
	switch action {
	case "initialize":
		resp, err = client.Initialize(callCtx, req)
	case "prepare":
		resp, err = client.Prepare(callCtx, req)
	case "rename":
		resp, err = client.Rename(callCtx, req)
	case "commands":
		resp, err = client.Commands(callCtx, req)
	default:
		return result, fmt.Errorf("unsupported collaboration action %q", action)
	}
	if err != nil {
		return result, fmt.Errorf("collaboration runtime unavailable: %w", err)
	}
	if resp.GetDefinitive() && resp.GetErrorCode() != "" {
		return result, &application.YjsRejection{Code: resp.GetErrorCode()}
	}
	if len(resp.GetPayloadJson()) > maxRuntimeMessageBytes {
		return result, fmt.Errorf("invalid runtime response size")
	}
	decoder := json.NewDecoder(bytes.NewReader(resp.GetPayloadJson()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode runtime candidate: %w", err)
	}
	return result, nil
}

func (c *Client) callHTTP(ctx context.Context, action string, input any) (application.YjsCandidate, error) {
	var result application.YjsCandidate
	body, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/"+action, bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("create runtime request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return result, fmt.Errorf("collaboration runtime unavailable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRuntimeMessageBytes+1))
	if err != nil || len(data) > maxRuntimeMessageBytes {
		return result, fmt.Errorf("invalid runtime response size")
	}
	if response.StatusCode != http.StatusOK {
		var problem struct {
			Code       string `json:"code"`
			Definitive bool   `json:"definitive"`
		}
		if response.StatusCode == http.StatusUnprocessableEntity && json.Unmarshal(data, &problem) == nil && problem.Definitive && problem.Code != "" {
			return result, &application.YjsRejection{Code: problem.Code}
		}
		return result, fmt.Errorf("collaboration runtime response %d", response.StatusCode)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode runtime candidate: %w", err)
	}
	return result, nil
}
