package microservices_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	fencerpc "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/grpctransport"
)

func TestWriteFenceRPCRequiresClosedWriterSetAndDedicatedCredentials(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[writefence.Writer]fencerpc.Target)
	}{
		{"missing writer", func(targets map[writefence.Writer]fencerpc.Target) { delete(targets, writefence.WriterCreation) }},
		{"extra writer", func(targets map[writefence.Writer]fencerpc.Target) {
			targets["caller-writer"] = targets[writefence.WriterMedia]
		}},
		{"shared audience token", func(targets map[writefence.Writer]fencerpc.Target) {
			value := targets[writefence.WriterMedia]
			value.Token = targets[writefence.WriterExecution].Token
			targets[writefence.WriterMedia] = value
		}},
		{"nil transport", func(targets map[writefence.Writer]fencerpc.Target) {
			value := targets[writefence.WriterMedia]
			value.Transport = nil
			targets[writefence.WriterMedia] = value
		}},
		{"URI is not an address", func(targets map[writefence.Writer]fencerpc.Target) {
			value := targets[writefence.WriterMedia]
			value.Address = "http://127.0.0.1:9091"
			targets[writefence.WriterMedia] = value
		}},
		{"zero port", func(targets map[writefence.Writer]fencerpc.Target) {
			value := targets[writefence.WriterMedia]
			value.Address = "127.0.0.1:0"
			targets[writefence.WriterMedia] = value
		}},
		{"token whitespace", func(targets map[writefence.Writer]fencerpc.Target) {
			value := targets[writefence.WriterMedia]
			value.Token += " "
			targets[writefence.WriterMedia] = value
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			targets := fenceTargets("identity:9092")
			test.mutate(targets)
			if _, err := fencerpc.NewClient(writefence.Identity, targets); !errors.Is(err, writefence.ErrInvalid) {
				t.Fatalf("invalid deployment accepted: %v", err)
			}
		})
	}
	if _, err := fencerpc.NewClient(writefence.Identity, fenceTargets("identity:9092")); err != nil {
		t.Fatalf("ordinary private hostname rejected: %v", err)
	}
	if _, err := fencerpc.NewClient("caller-issuer", fenceTargets("identity:9092")); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("unknown issuer accepted: %v", err)
	}
}

func TestWriteFenceRPCPreservesClosedFailureReasons(t *testing.T) {
	for _, want := range []error{writefence.ErrInvalid, writefence.ErrFenced, writefence.ErrConflict, writefence.ErrStale, writefence.ErrNotFound, writefence.ErrCommitted, writefence.ErrFrozen, writefence.ErrUnavailable} {
		t.Run(want.Error(), func(t *testing.T) {
			var calls atomic.Int32
			service, err := fencerpc.NewServer(writefence.WriterMedia, fenceRPCStore{install: func(context.Context, writefence.Control) error {
				calls.Add(1)
				return want
			}}, identityFenceToken(writefence.WriterMedia), creationFenceToken(writefence.WriterMedia))
			if err != nil {
				t.Fatal(err)
			}
			client, err := fencerpc.NewClient(writefence.Identity, fenceTargets(startFenceRPC(t, service, nil)))
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Install(t.Context(), writefence.WriterMedia, writefence.Control{Authority: fenceCommand().Authorities[0], CoordinationID: uuid.New(), Blocked: true}); !errors.Is(err, want) {
				t.Fatalf("reason mismatch: got %v, want %v", err, want)
			}
			if calls.Load() != 1 {
				t.Fatalf("rejected control was replayed %d times", calls.Load())
			}
		})
	}
}

type malformedFenceResult struct {
	pb.UnimplementedWriterFenceServer
	mode string
}

func (s malformedFenceResult) Resolve(_ context.Context, request *pb.ResolveRequest) (*pb.ResolveResponse, error) {
	result := &pb.ResolveResponse{Command: request.GetCommand(), State: pb.CommandState_COMMAND_STATE_COMMITTED, ReceiptJson: []byte(`{"result":"retained"}`)}
	switch s.mode {
	case "another ID":
		id := uuid.New()
		result.Command.Id = append([]byte(nil), id[:]...)
	case "another epoch":
		result.Command.Authorities[0].Epoch++
	case "another input":
		result.Command.Sha256 = strings.Repeat("b", 64)
	case "another action":
		result.Command.Action = "another.action"
	case "unknown state":
		result.State = pb.CommandState(99)
	case "missing command":
		result.Command = nil
	case "array receipt":
		result.ReceiptJson = []byte(`[]`)
	case "null receipt":
		result.ReceiptJson = []byte(`null`)
	case "uncommitted receipt":
		result.State = pb.CommandState_COMMAND_STATE_ADMITTED
	case "oversized receipt":
		result.ReceiptJson = []byte(`{"result":"` + strings.Repeat("x", writefence.MaxReceiptBytes) + `"}`)
	}
	return result, nil
}

func TestWriteFenceRPCRejectsMismatchedRecoveryResults(t *testing.T) {
	for _, mode := range []string{"another ID", "another epoch", "another input", "another action", "unknown state", "missing command", "array receipt", "null receipt", "uncommitted receipt", "oversized receipt"} {
		t.Run(mode, func(t *testing.T) {
			client, err := fencerpc.NewClient(writefence.Identity, fenceTargets(startFenceRPC(t, malformedFenceResult{mode: mode}, nil)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Resolve(t.Context(), writefence.WriterMedia, fenceCommand()); !errors.Is(err, writefence.ErrUnavailable) {
				t.Fatalf("malformed recovery became a trusted receipt: %v", err)
			}
		})
	}
}

func TestWriteFenceRPCDeploymentObjectsCannotBeMutatedThroughCallerMap(t *testing.T) {
	service, err := fencerpc.NewServer(writefence.WriterMedia, fenceRPCStore{}, identityFenceToken(writefence.WriterMedia), creationFenceToken(writefence.WriterMedia))
	if err != nil {
		t.Fatal(err)
	}
	targets := fenceTargets(startFenceRPC(t, service, nil))
	client, err := fencerpc.NewClient(writefence.Identity, targets)
	if err != nil {
		t.Fatal(err)
	}
	targets[writefence.WriterMedia] = fencerpc.Target{Address: "127.0.0.1:1", Token: "caller-controlled-token-at-least-32-bytes", Transport: insecure.NewCredentials()}
	if err := client.Install(t.Context(), writefence.WriterMedia, writefence.Control{Authority: fenceCommand().Authorities[0], CoordinationID: uuid.New(), Blocked: true}); err != nil {
		t.Fatalf("caller map replaced the configured target: %v", err)
	}
}
