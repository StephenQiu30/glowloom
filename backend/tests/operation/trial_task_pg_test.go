package operation_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	trial "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type runTrial func(context.Context, application.TrialInput) (application.TrialProgress, error)

func (f runTrial) Run(ctx context.Context, i application.TrialInput) (application.TrialProgress, error) {
	return f(ctx, i)
}

func TestTrialTaskNeverRepeatsUncertainSubmission(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_TASK_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated PG")
	}
	db, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error("close trial task database:", err)
		}
	})
	ctx := context.Background()
	s := trial.NewSessions(db).WithOutputAdmission(trialTaskOutputAdmission)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
	input := application.TrialInput{Actor: actor, Request: application.TrialRequest{RequestID: uuid.New(), Source: "cloud", ProviderID: uuid.New(), Kind: "video", Model: "fixture", Prompt: "test"}}
	state, err := s.Start(ctx, input)
	if err != nil || state.State != "running" {
		t.Fatalf("start %v %v", state, err)
	}
	_, err = s.Start(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	job, err := taskruntime.NewStore(db).ForBusiness(application.TrialID(actor, input.Request.RequestID)+"/1").Claim(ctx, []string{trial.Cloud}, "test", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim %v %v", job, err)
	}
	calls := 0
	handle := s.Handler(runTrial(func(context.Context, application.TrialInput) (application.TrialProgress, error) {
		calls++
		return application.TrialProgress{}, context.DeadlineExceeded
	}))
	for range 2 {
		if _, err := handle(ctx, *job); err != nil {
			t.Fatal(err)
		}
	}
	state, err = s.State(ctx, application.TrialID(actor, input.Request.RequestID))
	if err != nil || calls != 1 || state.State != "unknown" {
		t.Fatalf("dispatches %d state %+v err %v", calls, state, err)
	}
}

func trialDB(t *testing.T) (*gorm.DB, *trial.Sessions) {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_TASK_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated PG")
	}
	db, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error("close trial task database:", err)
		}
	})
	return db, trial.NewSessions(db).WithOutputAdmission(trialTaskOutputAdmission)
}
func trialJob(t *testing.T, db *gorm.DB, kind, id string, generation int) *taskruntime.Job {
	t.Helper()
	job, err := taskruntime.NewStore(db).ForBusiness(fmt.Sprintf("%s/%d", id, generation)).Claim(t.Context(), []string{kind}, "fixture", time.Minute)
	if job == nil || err != nil {
		t.Fatalf("trial claim: %v %v", job, err)
	}
	return job
}
func TestTrialVideoQueriesOriginalIdentityAndRecoversWithoutSubmitting(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint("query_error_", failure), func(t *testing.T) {
			db, s := trialDB(t)
			ctx := t.Context()
			actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
			input := application.TrialInput{Actor: actor, Request: application.TrialRequest{RequestID: uuid.New(), Source: "cloud", Kind: "video", ProviderID: uuid.New(), Model: "fixture", Prompt: "render"}}
			id := application.TrialID(actor, input.Request.RequestID)
			if _, err := s.Start(ctx, input); err != nil {
				t.Fatal(err)
			}
			job := trialJob(t, db, trial.Cloud, id, 1)
			key, cfg := uuid.New(), uuid.New()
			calls := []string{}
			handle := s.Handler(runTrial(func(_ context.Context, in application.TrialInput) (application.TrialProgress, error) {
				calls = append(calls, in.Action)
				if in.Action == "submit" {
					return application.TrialProgress{State: "pending", TaskID: "original", ConfigVersionID: cfg, CredentialID: key}, nil
				}
				if in.TaskID != "original" || in.ConfigVersionID != cfg || in.CredentialID != key {
					t.Fatalf("lost original identity: %+v", in)
				}
				if failure && in.Action == "query" {
					return application.TrialProgress{}, errors.New("query connection lost")
				}
				return application.TrialProgress{State: "succeeded", TaskID: "original", ConfigVersionID: cfg, CredentialID: key}, nil
			}))
			next, err := handle(ctx, *job)
			if err != nil || next.IsZero() {
				t.Fatalf("physical wait: %v %v", next, err)
			}
			next, err = handle(ctx, *job)
			if err != nil || !next.IsZero() {
				t.Fatalf("terminal query: %v %v", next, err)
			}
			state, err := s.State(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if failure {
				if state.State != "unknown" || state.Turns[0].TaskID != "original" {
					t.Fatalf("lost unknown receipt: %+v", state)
				}
				if err = s.Recover(ctx, id); err != nil {
					t.Fatal(err)
				}
				if _, err = handle(ctx, *trialJob(t, db, trial.Cloud, id, 2)); err != nil {
					t.Fatal(err)
				}
				state, err = s.State(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
			}
			if state.State != "succeeded" || calls[0] != "submit" || calls[1] != "query" || (failure && calls[2] != "recover") {
				t.Fatalf("unexpected progress: %v %+v", calls, state)
			}
		})
	}
}
func TestTrialChatKeepsOriginalThreadAndDeduplicatesTurns(t *testing.T) {
	db, s := trialDB(t)
	ctx := t.Context()
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
	req := application.TrialRequest{RequestID: uuid.New(), Source: "codex", Kind: "chat", Prompt: "hello", Model: "frozen"}
	if _, err := s.Start(ctx, application.TrialInput{Request: req, Actor: actor}); err != nil {
		t.Fatal(err)
	}
	id := application.TrialID(actor, req.RequestID)
	var calls []application.TrialInput
	handle := s.Handler(runTrial(func(_ context.Context, in application.TrialInput) (application.TrialProgress, error) {
		calls = append(calls, in)
		return application.TrialProgress{State: "succeeded", ThreadID: "original-thread", Text: "answer"}, nil
	}))
	if _, err := handle(ctx, *trialJob(t, db, trial.Codex, id, 1)); err != nil {
		t.Fatal(err)
	}
	msg := application.TrialMessage{RequestID: uuid.New(), Prompt: "continue"}
	if err := s.Turn(ctx, id, msg); err != nil {
		t.Fatal(err)
	}
	if err := s.Turn(ctx, id, msg); err != nil {
		t.Fatal(err)
	}
	if _, err := handle(ctx, *trialJob(t, db, trial.Codex, id, 2)); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[1].ThreadID != "original-thread" || calls[1].Request.Model != "frozen" || calls[1].Request.RequestID != msg.RequestID {
		t.Fatalf("chat replay: %+v", calls)
	}
	msg.Prompt = "changed"
	if !errors.Is(s.Turn(ctx, id, msg), trial.ErrConflict) {
		t.Fatal("changed replay accepted")
	}
	if err := s.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	state, err := s.State(ctx, id)
	if err != nil || state.State != "closed" {
		t.Fatalf("chat did not close %+v %v", state, err)
	}
}

func TestTrialCancellationPreventsQueuedDispatch(t *testing.T) {
	db, s := trialDB(t)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New()}
	input := application.TrialInput{Actor: actor, Request: application.TrialRequest{RequestID: uuid.New(), Source: "cloud", ProviderID: uuid.New(), Kind: "video", Model: "fixture", Prompt: "cancel before send"}}
	id := application.TrialID(actor, input.Request.RequestID)
	if _, err := s.Start(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	job := trialJob(t, db, trial.Cloud, id, 1)
	if err := s.Cancel(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	calls := 0
	handle := s.Handler(runTrial(func(context.Context, application.TrialInput) (application.TrialProgress, error) {
		calls++
		return application.TrialProgress{State: "pending", TaskID: "unexpected"}, nil
	}))
	next, err := handle(t.Context(), *job)
	if err != nil || !next.IsZero() || calls != 0 {
		t.Fatalf("cancelled queued work dispatched: calls=%d next=%v err=%v", calls, next, err)
	}
	state, err := s.State(t.Context(), id)
	if err != nil || state.State != "unknown" || state.Turns[0].ErrorCode != "trial_cancel_unconfirmed" {
		t.Fatalf("cancel changed upstream certainty: %+v %v", state, err)
	}
}

// Trial task protocol tests persist an explicit physical result identity; they
// exercise durable scheduling rather than provider or storage byte delivery.
func trialTaskOutputAdmission(_ context.Context, _ *gorm.DB, input application.TrialInput) (mediadomain.ObjectRef, error) {
	return mediadomain.ObjectRef{ProfileID: "trial-task-fixture", Bucket: "trial-task-fixture", Key: application.TrialObjectKey(input, 1)}, nil
}
