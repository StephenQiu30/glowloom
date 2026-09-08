package creation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	creation "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	"github.com/google/uuid"
)

type resumeStore struct {
	creation.ProposalStore
	run domain.Run
}

func (s resumeStore) AuthorizedRun(context.Context, creation.Actor, string, bool) (domain.Run, error) {
	return s.run, nil
}

type resumeRuntime struct {
	snapshot domain.ExecutionSnapshot
	calls    int
}

func (r *resumeRuntime) Execution(context.Context, domain.Run) (domain.ExecutionSnapshot, error) {
	return r.snapshot, nil
}
func (r *resumeRuntime) Draft(context.Context, domain.Run, string) (domain.DraftEnvelope, error) {
	return domain.DraftEnvelope{}, errors.New("not used")
}
func (r *resumeRuntime) Resume(_ context.Context, run domain.Run) (domain.ResumeReceipt, error) {
	r.calls++
	return domain.ResumeReceipt{Schema: "creation-resume-production", CommandID: run.Command.RunID, RunID: run.Command.RunID, Status: "resume_requested"}, nil
}

func TestCreationResumePreservesCommandIdentityAndRequiresRecoverableFailure(t *testing.T) {
	actor := creation.Actor{UserID: uuid.NewString(), TokenVersion: 2}
	run := domain.Run{Command: domain.Command{RunID: uuid.NewString(), ActorID: actor.UserID, Source: domain.Source{RevisionID: uuid.NewString()}}, Revision: 3, TokenVersion: 2, PayloadHash: strings.Repeat("a", 64)}
	runtime := &resumeRuntime{snapshot: domain.ExecutionSnapshot{RunID: run.Command.RunID, CommandID: run.Command.RunID, PayloadHash: run.PayloadHash, SourceRevisionID: run.Command.Source.RevisionID, Status: "blocked", CanResume: true}}
	service := creation.NewProposalService(resumeStore{run: run}, runtime, creation.Config{})
	for _, mode := range []string{"revision", "actor", "token", "unknown", "running", "rejected", "completed", "blocked_not_resumable", "binding"} {
		t.Run(mode, func(t *testing.T) {
			candidate := actor
			expected := run.Revision
			snapshot := runtime.snapshot
			switch mode {
			case "revision":
				expected++
			case "actor":
				candidate.UserID = uuid.NewString()
			case "token":
				candidate.TokenVersion++
			case "blocked_not_resumable":
				runtime.snapshot.CanResume = false
			case "binding":
				runtime.snapshot.PayloadHash = strings.Repeat("b", 64)
			default:
				runtime.snapshot.Status = mode
			}
			if _, err := service.Resume(context.Background(), candidate, run.Command.RunID, expected); err == nil {
				t.Fatal("unsafe or stale resume accepted")
			}
			if runtime.calls != 0 {
				t.Fatal("resume dispatched before validation")
			}
			runtime.snapshot = snapshot
		})
	}
	receipt, err := service.Resume(context.Background(), actor, run.Command.RunID, run.Revision)
	if err != nil || receipt.RunID != run.Command.RunID || runtime.calls != 1 {
		t.Fatalf("resume changed command or failed: %+v %v", receipt, err)
	}
}
