package microservices_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	outputrpc "github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/mediaoutput"
	executionapp "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/outputpublication"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// failedFinalStatus fixtures a peer that sends a structurally complete ACK message
// followed by a non-OK final RPC status. It has no durable archive receipt.
type failedFinalStatus struct {
	pb.UnimplementedOperationOutputsServer
	ack contract.OperationOutputReceipt
}

func (s *failedFinalStatus) PublishOperationOutput(stream grpc.ClientStreamingServer[pb.PublishOperationOutputFrame, pb.OperationOutputReceipt]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	req, err := contract.DecodeRequest(first.GetPublication())
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid test header")
	}
	var body bytes.Buffer
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if frame.GetPublication() != nil || len(frame.GetChunk()) == 0 {
			return status.Error(codes.InvalidArgument, "invalid test frame")
		}
		_, _ = body.Write(frame.GetChunk())
	}
	sum := sha256.Sum256(body.Bytes())
	if int64(body.Len()) != req.Output.ByteSize || hex.EncodeToString(sum[:]) != req.Output.SHA256 {
		return status.Error(codes.InvalidArgument, "invalid complete bytes")
	}
	ack := s.ack
	ack.CommandID = req.Command.ID
	ack.RequestSHA256 = req.Command.SHA256
	ack.PayloadSHA256 = req.Output.Digest()
	ack.Output = req.Output
	if err := stream.SendAndClose(contract.EncodeReceipt(ack)); err != nil {
		return err
	}
	return status.Error(codes.Unavailable, "synthetic failure after complete ACK message")
}
func (*failedFinalStatus) ResolveOperationOutput(context.Context, *pb.OutputPublication) (*pb.OperationOutputReceipt, error) {
	return nil, status.Error(codes.NotFound, "original archive responsibility unresolved")
}

func TestExecutionOutputCompleteACKMessageRequiresOKFinalRPCStatus(t *testing.T) {
	f := newExecutionOutputFixture(t)
	body := []byte("verified complete bytes")
	sum := sha256.Sum256(body)
	manifest, err := f.store.ReadOutputManifest(t.Context(), f.req.Publication.Output.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Artifacts[0].Size = int64(len(body))
	manifest.Artifacts[0].SHA256 = hex.EncodeToString(sum[:])
	raw, _ := json.Marshal(manifest)
	if err := f.admin.Exec(`UPDATE operation.provider_result SET result=?::jsonb WHERE operation_id=?`, string(raw), manifest.Identity.OperationID).Error; err != nil {
		t.Fatal(err)
	}
	f.req.Publication.Output.ByteSize = int64(len(body))
	f.req.Publication.Output.SHA256 = hex.EncodeToString(sum[:])
	f.req.Publication.Output.Provider.ManifestSHA256 = outputTestHash(t, manifest)
	f.req.Publication.Command.SHA256 = f.req.Publication.Output.Digest()
	f.req.Command.SHA256 = f.req.Digest()
	serverTransport, clientTransport, _ := ownerPeerTLS(t, "execution")
	server := grpc.NewServer(grpc.Creds(serverTransport))
	pb.RegisterOperationOutputsServer(server, &failedFinalStatus{ack: f.ack})
	client, err := outputrpc.New(outputrpc.Target{Address: serveFrozenRPC(t, server), Token: strings.Repeat("f", 40), Transport: clientTransport})
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := executionapp.NewOutputDelivery(f.store, client)
	if err != nil {
		t.Fatal(err)
	}
	err = delivery.Deliver(t.Context(), f.req, func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil })
	if !errors.Is(err, writefence.ErrUnavailable) || !errors.Is(err, contract.ErrOutputPublicationUnknown) {
		t.Fatalf("complete message hid final RPC failure: %v", err)
	}
	var facts struct {
		Status           string
		Cost             int64
		Outputs, Settles int
	}
	if err := f.execution.Raw(`SELECT status,(SELECT sum(cost_micros) FROM operation.provider_call WHERE operation_id=o.id) AS cost,(SELECT count(*) FROM operation.operation_output WHERE operation_id=o.id) AS outputs,(SELECT count(*) FROM billing.ledger_entry WHERE operation_id=o.id AND entry_type='settle') AS settles FROM operation.operation o WHERE id=?`, manifest.Identity.OperationID).Scan(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if facts.Status != "ingesting" || facts.Cost != 1234 || facts.Outputs != 0 || facts.Settles != 0 {
		t.Fatalf("non-OK final status changed original liabilities: %+v", facts)
	}
}
