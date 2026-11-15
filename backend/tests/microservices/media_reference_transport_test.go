package microservices_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/mediapins"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediareference"
	mediarpc "github.com/StephenQiu30/glowloom-server/backend/media/adapter/grpcreference"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

func receiptWireKey(pin mediadomain.ReferencePin) *pb.ReferencePinReceiptKey {
	key := pinReceiptKey(pin)
	return &pb.ReferencePinReceiptKey{PinId: key.PinID.String(), AcquireCommandId: key.AcquireCommandID.String(),
		ConsumerCommandId: key.ConsumerCommandID.String(), ConsumerCommandSha256: key.ConsumerCommandSHA256,
		ConsumerResourceId: key.ConsumerResourceID.String(), ScopeId: key.ScopeID.String(), ProjectId: key.ProjectID.String()}
}

func TestMediaReferenceReceiptPrivateTransportRejectsForgedAuthority(t *testing.T) {
	f, store, pin := frozenPinReceiptFixture(t)
	var calls atomic.Int32
	var traced atomic.Bool
	checked := pinReceiptRead(func(ctx context.Context, key mediaapp.ReferencePinReceiptKey) (mediadomain.ReferencePin, error) {
		calls.Add(1)
		incoming, _ := metadata.FromIncomingContext(ctx)
		for key := range incoming {
			if key != "traceparent" && key != "tracestate" {
				t.Errorf("private caller metadata reached Media persistence: %s", key)
				return mediadomain.ReferencePin{}, mediadomain.ErrReferencePinProof
			}
		}
		if len(incoming.Get("traceparent")) == 1 && trace.SpanContextFromContext(ctx).IsValid() {
			traced.Store(true)
		}
		return store.ReadReferencePinReceipt(ctx, key)
	})
	serverTransport, clientTransport, _ := ownerPeerTLS(t, "creation")
	server, err := mediarpc.NewServer(checked, mediaReceiptToken, "creation", serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	address := serveFrozenRPC(t, server)
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(clientTransport.Clone()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	reader := pb.NewCreationReferencePinsClient(connection)
	for _, name := range []string{"token", "duplicate_auth", "unbounded", "excessive_deadline", "unknown_field", "uuid_encoding", "partial_proof"} {
		t.Run(name, func(t *testing.T) {
			key := proto.Clone(receiptWireKey(pin)).(*pb.ReferencePinReceiptKey)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			auth := metadata.Pairs("authorization", "Bearer "+mediaReceiptToken)
			want := codes.InvalidArgument
			switch name {
			case "token":
				auth = metadata.Pairs("authorization", "Bearer "+strings.Repeat("wrong-fixture", 4))
				want = codes.Unauthenticated
			case "duplicate_auth":
				auth.Append("authorization", "Bearer "+mediaReceiptToken)
				want = codes.Unauthenticated
			case "unbounded":
				ctx = t.Context()
			case "excessive_deadline":
				long, stop := context.WithTimeout(t.Context(), time.Minute)
				defer stop()
				ctx = long
			case "unknown_field":
				key.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 1})
			case "uuid_encoding":
				key.PinId = "{" + key.PinId + "}"
			case "partial_proof":
				key.ConsumerCommandSha256 = ""
			}
			before := calls.Load()
			if _, err := reader.ReadAcquisition(metadata.NewOutgoingContext(ctx, auth), key); status.Code(err) != want {
				t.Fatalf("forged private request code=%s want=%s error=%v", status.Code(err), want, err)
			}
			if calls.Load() != before {
				t.Fatal("rejected private request reached owner persistence")
			}
		})
	}
	client, err := mediapins.New(mediapins.Target{Address: address, Token: mediaReceiptToken, Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(t.Context(), span)
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "forged", "cookie", "synthetic-user-cookie", "x-user-id", uuid.NewString(), "x-role", "platform_admin"))
	if _, err := client.VerifyFrozenInputPins(ctx, f.request); err != nil || !traced.Load() {
		t.Fatal("dedicated service identity or W3C propagation lost", err, traced.Load())
	}
}

func TestMediaReferenceReceiptConsumerRejectsChangedFactsAndReleasedState(t *testing.T) {
	f, _, original := frozenPinReceiptFixture(t)
	for _, name := range []string{"asset", "revision", "kind", "review", "released", "consumer_command", "acquire"} {
		t.Run(name, func(t *testing.T) {
			changed := original
			switch name {
			case "asset":
				changed.Identity.AssetID = uuid.New()
			case "revision":
				changed.Identity.AssetRevision++
			case "kind":
				changed.Kind = "video"
			case "review":
				changed.ModerationDetail = []byte(`{"number":9007199254740992,"scale":1.23}`)
			case "released":
				changed.State = "released"
			case "consumer_command":
				changed.Identity.ConsumerCommandID = uuid.New()
			case "acquire":
				changed.AcquireCommandID = uuid.New()
			}
			serverTransport, clientTransport, _ := ownerPeerTLS(t, "creation")
			server, err := mediarpc.NewServer(pinReceiptRead(func(context.Context, mediaapp.ReferencePinReceiptKey) (mediadomain.ReferencePin, error) {
				return changed, nil
			}), mediaReceiptToken, "creation", serverTransport)
			if err != nil {
				t.Fatal(err)
			}
			client, err := mediapins.New(mediapins.Target{Address: serveFrozenRPC(t, server), Token: mediaReceiptToken, Transport: clientTransport})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if _, err := client.VerifyFrozenInputPins(t.Context(), f.request); err == nil {
				t.Fatal("changed or released acquisition was accepted")
			}
		})
	}
	if _, err := (*mediapins.Client)(nil).VerifyFrozenInputPins(t.Context(), f.request); !errors.Is(err, promptdomain.ErrReferenceUnavailable) {
		t.Fatal("missing Media proof dependency did not fail closed", err)
	}
}
