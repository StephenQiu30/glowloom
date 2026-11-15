package serviceauth

import (
	"context"
	"crypto/subtle"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// MTLSUnary authenticates one dedicated service certificate and bearer audience.
// The listener must independently require and verify client certificates. Both
// checks are mandatory, even on loopback; only W3C tracing metadata reaches work.
func MTLSUnary(token, callerDNS string) (grpc.UnaryServerInterceptor, error) {
	if !ValidToken(token) || callerDNS == "" || strings.TrimSpace(callerDNS) != callerDNS {
		return nil, status.Error(codes.InvalidArgument, "service identity configuration required")
	}
	return func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		p, ok := peer.FromContext(ctx)
		if !ok || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		tls, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tls.State.VerifiedChains) == 0 || len(tls.State.PeerCertificates) == 0 || tls.State.PeerCertificates[0].VerifyHostname(callerDNS) != nil {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		deadline, bounded := ctx.Deadline()
		if !bounded || time.Until(deadline) > 30*time.Second {
			return nil, status.Error(codes.InvalidArgument, "bounded deadline required")
		}
		if err := ctx.Err(); err != nil {
			return nil, status.FromContextError(err).Err()
		}
		carrier := propagation.MapCarrier{}
		safe := metadata.MD{}
		for _, key := range []string{"traceparent", "tracestate"} {
			if v := metadata.ValueFromIncomingContext(ctx, key); len(v) == 1 {
				carrier[key] = v[0]
				safe.Set(key, v[0])
			}
		}
		ctx = propagation.TraceContext{}.Extract(ctx, carrier)
		return next(metadata.NewIncomingContext(ctx, safe), request)
	}, nil
}
