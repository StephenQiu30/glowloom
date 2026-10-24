// Package yjsruntime calls the private, isolated JavaScript candidate runtime.
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

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
)

// Client has no credentials or addresses supplied by canvas clients.
type Client struct {
	base, token string
	http        *http.Client
}

// New constructs a private runtime client with bounded requests.
func New(base, token string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Initialize converts an authorized durable snapshot without committing it.
func (c *Client) Initialize(ctx context.Context, state application.YjsState) (application.YjsCandidate, error) {
	return c.call(ctx, "initialize", state)
}

// Prepare computes an isolated candidate bound to the current durable state.
func (c *Client) Prepare(ctx context.Context, state application.YjsState, update application.YjsUpdate) (application.YjsCandidate, error) {
	return c.call(ctx, "prepare", struct {
		application.YjsState
		Update           []byte                `json:"update"`
		UpdateID         string                `json:"updateId"`
		PayloadHash      string                `json:"payloadHash"`
		RestorationNodes []application.NodeDTO `json:"restorationNodes"`
	}{state, update.Update, update.UpdateID.String(), update.PayloadHash, state.RestorationNodes})
}
func (c *Client) call(ctx context.Context, action string, input any) (application.YjsCandidate, error) {
	var result application.YjsCandidate
	if c.base == "" || len(c.token) < 32 {
		return result, fmt.Errorf("collaboration runtime not configured")
	}
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
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
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
