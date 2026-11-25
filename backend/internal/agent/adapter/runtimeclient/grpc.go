package runtimeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	agentpb "github.com/StephenQiu30/glowloom-server/backend/internal/gen/agent"
)

const maxAgentGRPCBytes = 8 << 20

type grpcTransport struct {
	target string
	token  string
}

func newGRPCTransport(origin, token string) (*grpcTransport, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "grpc" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, fmt.Errorf("invalid Agent gRPC configuration")
	}
	return &grpcTransport{target: u.Host, token: token}, nil
}

func (g *grpcTransport) dial() (*grpc.ClientConn, error) {
	return grpc.NewClient(
		g.target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxAgentGRPCBytes),
			grpc.MaxCallSendMsgSize(maxAgentGRPCBytes),
		),
	)
}

func (g *grpcTransport) outgoingContext(ctx context.Context) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"authorization", "Bearer "+g.token,
		"x-agent-audience", "agent",
	))
}

func identityFromQuery(query url.Values) *agentpb.Identity {
	return &agentpb.Identity{
		ActorId:   query.Get("actor_id"),
		OrgId:     query.Get("org_id"),
		ScopeId:   query.Get("scope_id"),
		ProjectId: query.Get("project_id"),
	}
}

func parseLimit(query url.Values) int32 {
	if raw := query.Get("limit"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 32); err == nil {
			return int32(n)
		}
	}
	return 0
}

func jsonResponse(statusCode int32, payload []byte) *http.Response {
	code := int(statusCode)
	if code == 0 {
		code = http.StatusOK
	}
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: code,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}
}

func (g *grpcTransport) request(ctx context.Context, operation string, params map[string]string, query url.Values, body any, lastEventID string) (*http.Response, error) {
	if operation == "subscribeAgentEvents" {
		return g.streamEvents(ctx, params["rid"], query, lastEventID)
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := g.dial()
	if err != nil {
		return nil, fmt.Errorf("connect Agent gRPC: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentpb.NewAgentRuntimeClient(conn)
	outCtx := g.outgoingContext(callCtx)
	id := identityFromQuery(query)

	switch operation {
	case "createAgentSession":
		in, ok := body.(CreateSessionRequest)
		if !ok {
			return nil, fmt.Errorf("invalid Agent request body for %s", operation)
		}
		req := &agentpb.CreateSessionRequest{
			Identity: &agentpb.Identity{
				ActorId:   in.ActorId,
				OrgId:     in.OrgId,
				ScopeId:   in.ScopeId,
				ProjectId: in.ProjectId,
			},
			RequestId: in.RequestId,
			Title:     in.Title,
		}
		if in.CanvasId != nil {
			req.CanvasId = *in.CanvasId
		}
		resp, callErr := client.CreateSession(outCtx, req)
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "listAgentSessions":
		resp, callErr := client.ListSessions(outCtx, &agentpb.ListSessionsRequest{
			Identity: id,
			Limit:    parseLimit(query),
			Cursor:   query.Get("cursor"),
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "getAgentSession":
		resp, callErr := client.GetSession(outCtx, &agentpb.GetSessionRequest{
			Identity:  id,
			SessionId: params["sid"],
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "updateAgentSession":
		in, ok := body.(UpdateSessionRequest)
		if !ok {
			return nil, fmt.Errorf("invalid Agent request body for %s", operation)
		}
		req := &agentpb.UpdateSessionRequest{
			Identity: &agentpb.Identity{
				ActorId:   in.ActorId,
				OrgId:     in.OrgId,
				ScopeId:   in.ScopeId,
				ProjectId: in.ProjectId,
			},
			SessionId: params["sid"],
			Title:     in.Title,
		}
		switch string(in.Action) {
		case "pin":
			v := true
			req.Pinned = &v
		case "unpin":
			v := false
			req.Pinned = &v
		case "archive":
			v := true
			req.Archived = &v
		case "unarchive":
			v := false
			req.Archived = &v
		}
		resp, callErr := client.UpdateSession(outCtx, req)
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "createAgentRun":
		in, ok := body.(CreateRunRequest)
		if !ok {
			return nil, fmt.Errorf("invalid Agent request body for %s", operation)
		}
		manifestRaw, marshalErr := json.Marshal(in.Manifest)
		if marshalErr != nil {
			return nil, marshalErr
		}
		inputRaw, marshalErr := json.Marshal(in.Input)
		if marshalErr != nil {
			return nil, marshalErr
		}
		req := &agentpb.StartRunRequest{
			Identity: &agentpb.Identity{
				ActorId:   in.ActorId,
				OrgId:     in.OrgId,
				ScopeId:   in.ScopeId,
				ProjectId: in.ProjectId,
			},
			SessionId:                 params["sid"],
			RequestId:                 in.RequestId,
			Goal:                      in.Goal,
			ManifestJson:              manifestRaw,
			InputJson:                 inputRaw,
			ExpectedExecutionRevision: in.ExpectedExecutionRevision,
			ContentHash:               in.ContentHash,
		}
		if in.RetryOfRunId != nil {
			req.RetryOfRunId = *in.RetryOfRunId
		}
		resp, callErr := client.StartRun(outCtx, req)
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "listAgentRuns":
		resp, callErr := client.ListRuns(outCtx, &agentpb.ListRunsRequest{
			Identity:  id,
			SessionId: params["sid"],
			Limit:     parseLimit(query),
			Cursor:    query.Get("cursor"),
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "getAgentRun":
		resp, callErr := client.GetRun(outCtx, &agentpb.GetRunRequest{
			Identity: id,
			RunId:    params["rid"],
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "getCurrentAgentSubmission":
		resp, callErr := client.GetCurrentSubmission(outCtx, &agentpb.GetCurrentSubmissionRequest{
			Identity:  id,
			SessionId: params["sid"],
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "getAgentSubmission":
		resp, callErr := client.GetSubmission(outCtx, &agentpb.GetSubmissionRequest{
			Identity:  id,
			SessionId: params["sid"],
			RequestId: params["requestId"],
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "listAgentMessages":
		resp, callErr := client.ListMessages(outCtx, &agentpb.ListMessagesRequest{
			Identity: id,
			RunId:    params["rid"],
			Limit:    parseLimit(query),
			Before:   query.Get("before"),
			Through:  query.Get("through"),
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "getAgentMessage":
		resp, callErr := client.GetMessage(outCtx, &agentpb.GetMessageRequest{
			Identity:  id,
			RunId:     params["rid"],
			MessageId: params["messageId"],
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	case "applyAgentDecision":
		in, ok := body.(ControlRequest)
		if !ok {
			return nil, fmt.Errorf("invalid Agent request body for %s", operation)
		}
		payloadRaw, marshalErr := json.Marshal(in)
		if marshalErr != nil {
			return nil, marshalErr
		}
		resp, callErr := client.ApplyDecision(outCtx, &agentpb.ApplyDecisionRequest{
			Identity: &agentpb.Identity{
				ActorId:   in.ActorId,
				ProjectId: in.ProjectId,
			},
			RunId:                 params["rid"],
			DecisionId:            in.DecisionId,
			AuthorizationRevision: in.AuthorizationRevision,
			Action:                string(in.Action),
			PayloadJson:           payloadRaw,
		})
		if callErr != nil {
			return nil, fmt.Errorf("agent gRPC %s: %w", operation, callErr)
		}
		return jsonResponse(resp.GetStatusCode(), resp.GetPayloadJson()), nil

	default:
		return nil, fmt.Errorf("unknown Agent operation %q", operation)
	}
}

func (g *grpcTransport) streamEvents(ctx context.Context, runID string, query url.Values, lastEventID string) (*http.Response, error) {
	conn, err := g.dial()
	if err != nil {
		return nil, fmt.Errorf("connect Agent gRPC stream: %w", err)
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := agentpb.NewAgentRuntimeClient(conn).StreamRunEvents(g.outgoingContext(streamCtx), &agentpb.StreamRunEventsRequest{
		Identity:    identityFromQuery(query),
		RunId:       runID,
		LastEventId: lastEventID,
	})
	if err != nil {
		cancel()
		_ = conn.Close()
		return nil, fmt.Errorf("start Agent gRPC event stream: %w", err)
	}
	pr, pw := io.Pipe()
	go func() {
		defer func() {
			cancel()
			_ = conn.Close()
			_ = pw.Close()
		}()
		for {
			frame, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				return
			}
			if recvErr != nil {
				_ = pw.CloseWithError(recvErr)
				return
			}
			var buf bytes.Buffer
			if id := frame.GetId(); id != "" {
				buf.WriteString("id: " + id + "\n")
			}
			if ev := frame.GetEvent(); ev != "" {
				buf.WriteString("event: " + ev + "\n")
			}
			for _, line := range strings.Split(string(frame.GetDataJson()), "\n") {
				buf.WriteString("data: " + line + "\n")
			}
			buf.WriteString("\n")
			if _, writeErr := pw.Write(buf.Bytes()); writeErr != nil {
				return
			}
		}
	}()
	header := http.Header{}
	header.Set("Content-Type", "text/event-stream")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       &grpcStreamBody{ReadCloser: pr, cancel: cancel},
	}, nil
}

type grpcStreamBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *grpcStreamBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}
