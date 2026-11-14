package medianormalization_test

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
)

func TestNormalizationClientPropagatesTraceWithoutForwardingCallerIdentity(t *testing.T) {
	parent := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(t.Context(), parent)
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer caller-must-not-leak", "x-user-id", "caller-must-not-leak", "baggage", "private=caller-must-not-leak"))
	client := rawProcessor(t, func(stream grpc.BidiStreamingServer[pb.NormalizeRequest, pb.NormalizeResponse]) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		if auth := md.Get("authorization"); len(auth) != 1 || auth[0] != "Bearer "+serviceToken {
			return status.Error(codes.PermissionDenied, "caller identity leaked")
		}
		for key, values := range md {
			if key == "x-user-id" || key == "baggage" || strings.Contains(strings.Join(values, ""), "caller-must-not-leak") {
				return status.Error(codes.PermissionDenied, "unapproved metadata leaked")
			}
		}
		carrier := propagation.MapCarrier{"traceparent": strings.Join(md.Get("traceparent"), ""), "tracestate": strings.Join(md.Get("tracestate"), "")}
		incoming := propagation.TraceContext{}.Extract(stream.Context(), carrier)
		if got := trace.SpanContextFromContext(incoming); got.TraceID() != parent.TraceID() || got.SpanID() != parent.SpanID() {
			return status.Error(codes.FailedPrecondition, "trace correlation lost")
		}
		if err := drainSource(stream); err != nil {
			return err
		}
		data := mp4Signature()
		if err := stream.Send(canonicalFrame(data)); err != nil {
			return err
		}
		return stream.Send(&pb.NormalizeResponse{Payload: &pb.NormalizeResponse_Chunk{Chunk: data}})
	})
	result, err := client.Normalize(ctx, clientSource(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.File.Close() }()
}
