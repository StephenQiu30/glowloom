package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AgentRunRequest struct {
	Skill       string `json:"skill"`
	Stage       string `json:"stage"`
	RequestHash string `json:"request_hash"`
	SnapshotRef string `json:"snapshot_ref"`
}

type AgentProposalItem struct {
	ItemID   string           `json:"item_id"`
	Kind     string           `json:"kind"`
	Value    any              `json:"value"`
	Evidence []map[string]int `json:"evidence"`
}

type AgentRunResponse struct {
	RunID       uuid.UUID           `json:"run_id"`
	Skill       string              `json:"skill"`
	Stage       string              `json:"stage"`
	Status      string              `json:"status"`
	RequestHash string              `json:"request_hash"`
	Items       []AgentProposalItem `json:"items"`
	CreatedAt   time.Time           `json:"created_at"`
	Error       string              `json:"error,omitempty"`
}

type AgentHTTPClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewAgentHTTPClient() *AgentHTTPClient {
	baseURL := os.Getenv("AGENT_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8790"
	}
	return &AgentHTTPClient{baseURL: strings.TrimRight(baseURL, "/"), token: os.Getenv("LANVERSE_AGENT_TOKEN"), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *AgentHTTPClient) Start(ctx context.Context, request AgentRunRequest) (AgentRunResponse, error) {
	return c.do(ctx, http.MethodPost, "/internal/agent-runs", request, "")
}

func (c *AgentHTTPClient) Get(ctx context.Context, id uuid.UUID) (AgentRunResponse, error) {
	return c.do(ctx, http.MethodGet, "/internal/agent-runs/"+id.String(), nil, "")
}

func (c *AgentHTTPClient) Cancel(ctx context.Context, id uuid.UUID) (AgentRunResponse, error) {
	return c.do(ctx, http.MethodPost, "/internal/agent-runs/"+id.String()+"/cancel", nil, "")
}

func (c *AgentHTTPClient) do(ctx context.Context, method, path string, body any, _ string) (AgentRunResponse, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return AgentRunResponse{}, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return AgentRunResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Lanverse-Agent-Token", c.token)
	response, err := c.http.Do(request)
	if err != nil {
		return AgentRunResponse{}, fmt.Errorf("agent request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return AgentRunResponse{}, fmt.Errorf("agent request returned %d", response.StatusCode)
	}
	var result AgentRunResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return AgentRunResponse{}, fmt.Errorf("decode agent result: %w", err)
	}
	return result, nil
}
