package microservices_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type creationSourceProof struct {
	identity workspacesource.Identity
	err      error
	calls    *[]string
	command  writefence.Command
	target   writefence.Writer
}

func (p creationSourceProof) ResolveSourceAdmission(_ context.Context, command writefence.Command, target writefence.Writer) (workspacesource.Identity, error) {
	*p.calls = append(*p.calls, "resolve-original-admission")
	if p.target != target || p.command.ID != command.ID || p.command.SHA256 != command.SHA256 || p.command.Action != command.Action {
		return workspacesource.Identity{}, writefence.ErrConflict
	}
	return p.identity, p.err
}

func TestCreationSourceProofRequiresOriginalTargetBeforeResourcePolicy(t *testing.T) {
	for _, shape := range []string{"", "missing-proof", "wrong-target", "foreign-identity", "consumer-selection-changed", "source-command-as-consumer"} {
		t.Run(shape, func(t *testing.T) {
			id, request := sourceAdmissionFixture()
			request.Command.Action = "execution.frozen_input.accept"
			request.Target = writefence.WriterExecution
			var calls []string
			proof := creationSourceProof{identity: id, calls: &calls, command: request.Command, target: request.Target}
			switch shape {
			case "missing-proof":
				proof.err = writefence.ErrNotFound
			case "wrong-target":
				proof.target = writefence.WriterCreation
			case "foreign-identity":
				proof.identity.SessionID = proof.identity.AccountID
			case "consumer-selection-changed":
				request.Command.SHA256 = strings.Repeat("b", 64)
			case "source-command-as-consumer":
				request.Command.Action = "creation.frozen_input.freeze"
			}
			service := workspacesource.NewProofAdmission(proof, creationSourceStore{calls: &calls})
			_, err := service.Admit(t.Context(), request)
			if shape == "" {
				if err != nil || strings.Join(calls, ",") != "resolve-original-admission,creation-admit" {
					t.Fatalf("same-target source chain: %v %v", calls, err)
				}
			} else {
				if err == nil {
					t.Fatal("source policy accepted missing or different original consumer proof")
				}
				for _, call := range calls {
					if call == "creation-admit" {
						t.Fatalf("invalid proof reached policy transaction: %v", calls)
					}
				}
			}
		})
	}
}

func TestCreationReferencePinSourceRequiresOriginalMediaAdmission(t *testing.T) {
	id, request := sourceAdmissionFixture()
	request.Command.Action = "media.reference_pin.acquire"
	request.Target = writefence.WriterMedia
	var calls []string
	proof := creationSourceProof{identity: id, calls: &calls, command: request.Command, target: request.Target}
	permit, err := workspacesource.NewProofAdmission(proof, creationSourceStore{calls: &calls}).Admit(t.Context(), request)
	if err != nil || permit.Target != writefence.WriterMedia || strings.Join(calls, ",") != "resolve-original-admission,creation-admit" {
		t.Fatalf("original Media acquisition did not reach current Creation policy: %+v %v %v", permit, calls, err)
	}
	request.Target = writefence.WriterExecution
	if _, err := workspacesource.NewProofAdmission(proof, creationSourceStore{calls: &calls}).Admit(t.Context(), request); err == nil {
		t.Fatal("acquisition retargeted to Execution")
	}
}

func TestCreationSourceCredentialPathCannotAdmitExecutionOrMedia(t *testing.T) {
	id, request := sourceAdmissionFixture()
	request.Command.Action = "execution.frozen_input.accept"
	request.Target = writefence.WriterExecution
	var calls []string
	service := workspacesource.NewAdmission(creationIdentityAuthority{identity: id, calls: &calls}, creationSourceStore{calls: &calls})
	_, err := service.Admit(t.Context(), workspacesource.AccessCredential("credential", "csrf"), request)
	if !errors.Is(err, writefence.ErrInvalid) || len(calls) != 0 {
		t.Fatalf("Creation admitted Identity for a foreign writer: %v %v", calls, err)
	}
}
