// Package runtimeclient implements the authenticated HTTP boundary to the independent Agent runtime.
package runtimeclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// RuntimeClient wraps the generated client with service authentication and bounded request lifetimes.
type RuntimeClient struct{ client *Client }

// New creates a runtime client with the supplied origin and direction credential.
func New(origin, token string) (*RuntimeClient, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, fmt.Errorf("invalid Agent service configuration")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = 32
	transport.ResponseHeaderTimeout = 15 * time.Second
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	client, err := NewClient(strings.TrimRight(origin, "/"), WithHTTPClient(httpClient), WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Agent-Audience", "agent")
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &RuntimeClient{client: client}, nil
}

// Request dispatches to generated operations while preserving the raw response for streaming and bounded consumers.
func (c *RuntimeClient) Request(ctx context.Context, operation string, params map[string]string, query url.Values, body any, lastEventID string) (*http.Response, error) {
	var required []string
	switch operation {
	case "getAgentSession", "updateAgentSession", "createAgentRun", "listAgentRuns", "getCurrentAgentSubmission":
		required = []string{"sid"}
	case "getAgentSubmission":
		required = []string{"sid", "requestId"}
	case "getAgentRun", "listAgentMessages", "applyAgentDecision", "subscribeAgentEvents":
		required = []string{"rid"}
	case "getAgentMessage":
		required = []string{"rid", "messageId"}
	}
	for _, name := range required {
		if params[name] == "" {
			return nil, fmt.Errorf("missing Agent operation parameter %q", name)
		}
	}
	requestCtx := ctx
	cancel := func() {}
	if operation != "subscribeAgentEvents" {
		requestCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	}
	actor, project := query.Get("actor_id"), query.Get("project_id")
	optional := func(key string) *string {
		v := query.Get(key)
		if v == "" {
			return nil
		}
		return &v
	}
	var limit *int64
	if value := query.Get("limit"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			limit = &n
		}
	}
	// Preserve the caller's query verbatim, including invalid values that the
	// Agent validates and reports as a client error.
	queryEditor := func(_ context.Context, req *http.Request) error {
		req.URL.RawQuery = query.Encode()
		return nil
	}
	var response *http.Response
	var err error
	switch operation {
	case "createAgentSession":
		input, ok := body.(CreateSessionRequest)
		if !ok {
			err = fmt.Errorf("invalid Agent request body for %s", operation)
			break
		}
		response, err = c.client.CreateAgentSession(requestCtx, &CreateAgentSessionParams{XAgentAudience: "agent"}, input)
	case "listAgentSessions":
		response, err = c.client.ListAgentSessions(requestCtx, &ListAgentSessionsParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent", Limit: limit, Cursor: optional("cursor")}, queryEditor)
	case "getAgentSession":
		response, err = c.client.GetAgentSession(requestCtx, params["sid"], &GetAgentSessionParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent"}, queryEditor)
	case "updateAgentSession":
		input, ok := body.(UpdateSessionRequest)
		if !ok {
			err = fmt.Errorf("invalid Agent request body for %s", operation)
			break
		}
		response, err = c.client.UpdateAgentSession(requestCtx, params["sid"], &UpdateAgentSessionParams{XAgentAudience: "agent"}, input)
	case "createAgentRun":
		input, ok := body.(CreateRunRequest)
		if !ok {
			err = fmt.Errorf("invalid Agent request body for %s", operation)
			break
		}
		response, err = c.client.CreateAgentRun(requestCtx, params["sid"], &CreateAgentRunParams{XAgentAudience: "agent"}, input)
	case "listAgentRuns":
		response, err = c.client.ListAgentRuns(requestCtx, params["sid"], &ListAgentRunsParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent", Limit: limit, Cursor: optional("cursor")}, queryEditor)
	case "getCurrentAgentSubmission":
		response, err = c.client.GetCurrentAgentSubmission(requestCtx, params["sid"], &GetCurrentAgentSubmissionParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent"}, queryEditor)
	case "getAgentSubmission":
		response, err = c.client.GetAgentSubmission(requestCtx, params["sid"], params["requestId"], &GetAgentSubmissionParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent"}, queryEditor)
	case "getAgentRun":
		response, err = c.client.GetAgentRun(requestCtx, params["rid"], &GetAgentRunParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent"}, queryEditor)
	case "listAgentMessages":
		response, err = c.client.ListAgentMessages(requestCtx, params["rid"], &ListAgentMessagesParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent", Limit: limit, Before: optional("before"), Through: query.Get("through")}, queryEditor)
	case "getAgentMessage":
		response, err = c.client.GetAgentMessage(requestCtx, params["rid"], params["messageId"], &GetAgentMessageParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent"}, queryEditor)
	case "applyAgentDecision":
		input, ok := body.(ControlRequest)
		if !ok {
			err = fmt.Errorf("invalid Agent request body for %s", operation)
			break
		}
		response, err = c.client.ApplyAgentDecision(requestCtx, params["rid"], &ApplyAgentDecisionParams{XAgentAudience: "agent"}, input)
	case "subscribeAgentEvents":
		var cursor *string
		if lastEventID != "" {
			cursor = &lastEventID
		}
		response, err = c.client.SubscribeAgentEvents(requestCtx, params["rid"], &SubscribeAgentEventsParams{ActorId: actor, ProjectId: project, XAgentAudience: "agent", LastEventID: cursor}, queryEditor)
	default:
		err = fmt.Errorf("unknown Agent operation %q", operation)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = &cancelBody{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { defer b.cancel(); return b.ReadCloser.Close() }
