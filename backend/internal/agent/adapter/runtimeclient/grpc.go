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

func (g *grpcTransport) request(ctx context.Context, operation string, params map[string]string, query url.Values, body any, lastEventID string) (*http.Response, error) {
	if operation == "subscribeAgentEvents" {
		return g.streamEvents(ctx, params["rid"], query, lastEventID)
	}
	method, path, err := operationRoute(operation, params)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	qMap := make(map[string]string, len(query))
	for k, vals := range query {
		if len(vals) > 0 {
			qMap[k] = vals[0]
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := g.dial()
	if err != nil {
		return nil, fmt.Errorf("connect Agent gRPC: %w", err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := agentpb.NewAgentRuntimeClient(conn).Invoke(g.outgoingContext(callCtx), &agentpb.InvokeRequest{
		Operation: operation,
		Method:    method,
		Path:      path,
		Params:    params,
		Query:     qMap,
		BodyJson:  payload,
	})
	if err != nil {
		return nil, fmt.Errorf("agent gRPC %s: %w", operation, err)
	}
	header := http.Header{}
	if ct := resp.GetContentType(); ct != "" {
		header.Set("Content-Type", ct)
	}
	return &http.Response{
		StatusCode: int(resp.GetStatusCode()),
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(resp.GetBodyJson())),
	}, nil
}

func (g *grpcTransport) streamEvents(ctx context.Context, runID string, query url.Values, lastEventID string) (*http.Response, error) {
	conn, err := g.dial()
	if err != nil {
		return nil, fmt.Errorf("connect Agent gRPC stream: %w", err)
	}
	streamCtx, cancel := context.WithCancel(ctx)
	qMap := make(map[string]string, len(query))
	for k, vals := range query {
		if len(vals) > 0 {
			qMap[k] = vals[0]
		}
	}
	stream, err := agentpb.NewAgentRuntimeClient(conn).StreamRunEvents(g.outgoingContext(streamCtx), &agentpb.StreamRunEventsRequest{
		RunId:       runID,
		LastEventId: lastEventID,
		Query:       qMap,
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

func operationRoute(operation string, params map[string]string) (string, string, error) {
	switch operation {
	case "createAgentSession":
		return http.MethodPost, "/internal/agent/sessions", nil
	case "listAgentSessions":
		return http.MethodGet, "/internal/agent/sessions", nil
	case "getAgentSession":
		return http.MethodGet, "/internal/agent/sessions/" + url.PathEscape(params["sid"]), nil
	case "updateAgentSession":
		return http.MethodPatch, "/internal/agent/sessions/" + url.PathEscape(params["sid"]), nil
	case "createAgentRun":
		return http.MethodPost, "/internal/agent/sessions/" + url.PathEscape(params["sid"]) + "/runs", nil
	case "listAgentRuns":
		return http.MethodGet, "/internal/agent/sessions/" + url.PathEscape(params["sid"]) + "/runs", nil
	case "getCurrentAgentSubmission":
		return http.MethodGet, "/internal/agent/sessions/" + url.PathEscape(params["sid"]) + "/submissions/current", nil
	case "getAgentSubmission":
		return http.MethodGet, "/internal/agent/sessions/" + url.PathEscape(params["sid"]) + "/submissions/" + url.PathEscape(params["requestId"]), nil
	case "getAgentRun":
		return http.MethodGet, "/internal/agent/runs/" + url.PathEscape(params["rid"]), nil
	case "listAgentMessages":
		return http.MethodGet, "/internal/agent/runs/" + url.PathEscape(params["rid"]) + "/messages", nil
	case "getAgentMessage":
		return http.MethodGet, "/internal/agent/runs/" + url.PathEscape(params["rid"]) + "/messages/" + url.PathEscape(params["messageId"]), nil
	case "applyAgentDecision":
		return http.MethodPost, "/internal/agent/runs/" + url.PathEscape(params["rid"]) + "/controls", nil
	default:
		return "", "", fmt.Errorf("unknown Agent operation %q", operation)
	}
}
