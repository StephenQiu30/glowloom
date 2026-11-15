package grpchandoff

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// DeliveryClient consumes only Execution's closed input-acceptance method. It
// cannot create a quote, reserve funds or submit a provider operation.
type DeliveryClient struct{ target Target }

// NewDeliveryClient requires the dedicated Creation-to-Execution capability.
func NewDeliveryClient(target Target) (*DeliveryClient, error) {
	validated, err := NewCancellationClient(target)
	if err != nil {
		return nil, err
	}
	return &DeliveryClient{target: validated.target}, nil
}

// AcceptFrozenInput makes one bounded call. A lost ACK remains the original task's
// responsibility; the adapter never retries or invents another input identity.
func (c *DeliveryClient) AcceptFrozenInput(ctx context.Context, key application.FrozenInputKey) (application.FrozenInputDeliveryReceipt, error) {
	var out application.FrozenInputDeliveryReceipt
	if c == nil {
		return out, writefence.ErrUnavailable
	}
	if _, err := decodeKey(encodeKey(key)); err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	md := metadata.Pairs("authorization", "Bearer "+c.target.Token)
	for _, name := range []string{"traceparent", "tracestate"} {
		if value := carrier[name]; value != "" {
			md.Set(name, value)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	connection, err := grpc.NewClient(c.target.Address, grpc.WithTransportCredentials(c.target.Transport.Clone()), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageBytes), grpc.MaxCallSendMsgSize(maxMessageBytes), grpc.MaxRetryRPCBufferSize(0)))
	if err != nil {
		return out, writefence.ErrUnavailable
	}
	defer func() { _ = connection.Close() }()
	response, err := pb.NewFrozenInputConsumerClient(connection).AcceptFrozenInput(ctx, encodeKey(key))
	if err != nil {
		return out, localError(err)
	}
	if !closed(response) {
		return out, domain.ErrFrozenInputConflict
	}
	out.Key, err = decodeKey(response.Key)
	if err != nil || out.Key != key {
		return application.FrozenInputDeliveryReceipt{}, domain.ErrFrozenInputConflict
	}
	out.RequestID, err = wireUUID(response.RequestId)
	if err != nil {
		return out, err
	}
	out.OperationID, err = wireUUID(response.OperationId)
	if err != nil {
		return out, err
	}
	out.SubmissionID, err = wireUUID(response.SubmissionId)
	if err != nil {
		return out, err
	}
	out.ReceiptID, err = wireUUID(response.ReceiptId)
	if err != nil {
		return out, err
	}
	return out, nil
}
