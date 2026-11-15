package microservices_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestCreationSourceProofRPCResolvesOnlyExactOriginalConsumerIdentity(t *testing.T) {
	for _, shape := range []string{"", "wrong-target", "wrong-command", "wrong-identity", "nil-proof", "unknown-proof", "unknown-identity", "unknown-command", "lost-ack"} {
		t.Run(shape, func(t *testing.T) {
			id, request := sourceAdmissionFixture()
			request.Command.Action = "execution.frozen_input.accept"
			request.Target = writefence.WriterExecution
			var calls atomic.Int32
			client := startSourceAuthority(t, &sourceAuthorityServer{resolve: func(ctx context.Context, r *pb.ResolveAdmissionRequest) (*pb.ResolveAdmissionResponse, error) {
				calls.Add(1)
				md, _ := metadata.FromIncomingContext(ctx)
				if r.Caller != coordination.Writer_WRITER_CREATION || r.Target != coordination.Writer_WRITER_EXECUTION || len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer "+strings.Repeat("c", 32) {
					t.Error("proof reader capability or original target changed")
				}
				response := &pb.ResolveAdmissionResponse{Proof: &pb.AdmissionProof{Identity: sourceWireIdentity(id), Command: proto.Clone(r.Command).(*coordination.Command), Target: r.Target}}
				switch shape {
				case "wrong-target":
					response.Proof.Target = coordination.Writer_WRITER_CREATION
				case "wrong-command":
					response.Proof.Command.Sha256 = strings.Repeat("b", 64)
				case "wrong-identity":
					response.Proof.Identity.SessionId = response.Proof.Identity.AccountId
				case "nil-proof":
					response.Proof = nil
				case "unknown-proof":
					response.Proof.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "unknown-identity":
					response.Proof.Identity.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "unknown-command":
					response.Proof.Command.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
				case "lost-ack":
					return nil, status.Error(codes.Unavailable, "proof unavailable")
				}
				return response, nil
			}})
			resolved, err := client.ResolveSourceAdmission(t.Context(), request.Command, request.Target)
			if shape == "" {
				if err != nil || resolved != id {
					t.Fatalf("original proof: %+v %v", resolved, err)
				}
			} else if err == nil {
				t.Fatal("inexact proof granted resource admission")
			}
			if calls.Load() != 1 {
				t.Fatalf("proof RPC retried: %d", calls.Load())
			}
		})
	}
}
