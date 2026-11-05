package subject_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	subjecttask "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/task"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"
)

type recoveringVoicePublisher struct {
	fixture *voicePGFixture
	calls   atomic.Int32
}

func (p *recoveringVoicePublisher) Publish(ctx context.Context, work subject.VoiceWork, receipt subject.VoiceReceipt) (subject.VoiceJob, error) {
	if p.calls.Add(1) <= 3 {
		return work.Job, errors.New("temporary publication interruption")
	}
	return p.fixture.Publish(ctx, work, receipt)
}

type countedVoiceSynthesizer struct {
	fixture *voicePGFixture
	calls   atomic.Int32
}

func (p *countedVoiceSynthesizer) Synthesize(ctx context.Context, input subject.VoiceSynthesisInput, key string) (subject.VoiceSynthesisResult, error) {
	p.calls.Add(1)
	return p.fixture.Synthesize(ctx, input, key)
}

func TestVoiceTaskWaitsForCapacityAfterThirdAttempt(t *testing.T) {
	f := newVoicePG(t)
	blocker, err := f.store.Create(t.Context(), f.actor, uuid.New(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	blockingWork, claimed, err := f.store.Claim(t.Context(), blocker.ID, uuid.New())
	if err != nil || !claimed {
		t.Fatal("claim capacity blocker", claimed, err)
	}
	if err := f.owner.Exec(`UPDATE catalog.provider SET concurrency_limit=1 WHERE id=?`, blockingWork.ProviderID).Error; err != nil {
		t.Fatal(err)
	}
	input := f.input
	input.DraftIntentID = uuid.New()
	job, err := f.store.Create(t.Context(), f.actor, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	synth := &countedVoiceSynthesizer{fixture: f}
	worker := subject.NewVoiceWorker(f.store, voiceTestKeys{}, synth, f, f)
	id := taskfixture.StartWork(t, f.runtime, subjecttask.Voice, map[string]any{"job_id": job.ID}, subjecttask.VoiceHandler(worker))
	deadline := time.Now().Add(30 * time.Second)
	for {
		var row struct {
			Attempt            int
			Idle, Done, Failed bool
		}
		if err := f.owner.Raw(`SELECT attempt,owner IS NULL AS idle,completed_at IS NOT NULL AS done,failed_at IS NOT NULL AS failed FROM infra.task_work WHERE kind=? AND business_id=?`, subjecttask.Voice, id).Scan(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.Attempt >= 3 && row.Idle {
			if row.Done || row.Failed {
				t.Fatalf("capacity waiting closed the admitted task: %+v", row)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task did not reach the third capacity wait")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if synth.calls.Load() != 0 {
		t.Fatal("capacity waiting dispatched a request")
	}
	if _, err := f.store.MarkFailed(t.Context(), blockingWork, "fixture_blocker_finished"); err != nil {
		t.Fatal(err)
	}
	taskfixture.WaitWork(t, f.runtime, subjecttask.Voice, id, 30*time.Second)
	final, err := f.store.Get(t.Context(), f.actor, job.ID)
	if err != nil || final.Status != "succeeded" || synth.calls.Load() != 1 {
		t.Fatal("capacity release did not resume the original job", final.Status, synth.calls.Load(), err)
	}
}

func TestVoiceTaskRetriesPublicationAfterThreeInterruptions(t *testing.T) {
	f := newVoicePG(t)
	job, err := f.store.Create(t.Context(), f.actor, uuid.New(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	synth := &countedVoiceSynthesizer{fixture: f}
	publisher := &recoveringVoicePublisher{fixture: f}
	worker := subject.NewVoiceWorker(f.store, voiceTestKeys{}, synth, f, publisher)
	id := taskfixture.StartWork(t, f.runtime, subjecttask.Voice, map[string]any{"job_id": job.ID}, subjecttask.VoiceHandler(worker))
	taskfixture.WaitWork(t, f.runtime, subjecttask.Voice, id, 30*time.Second)
	final, err := f.store.Get(t.Context(), f.actor, job.ID)
	if err != nil || final.Status != "succeeded" || publisher.calls.Load() != 4 || synth.calls.Load() != 1 {
		t.Fatal("publication recovery closed before success or repeated dispatch", final.Status, publisher.calls.Load(), synth.calls.Load(), err)
	}
}

// Paid dispatch is a local fixture. PostgreSQL ownership, immutable request,
// real MP3 processing and private media adoption use the production Go worker.
func TestVoiceTaskPostgresPublicationAndUnknownDispatch(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "publish", true: "unknown"}[unknown], func(t *testing.T) {
			f := newVoicePG(t)
			f.unknown = unknown
			job, err := f.store.Create(t.Context(), f.actor, uuid.New(), f.input)
			if err != nil {
				t.Fatal(err)
			}
			worker := subject.NewVoiceWorker(f.store, voiceTestKeys{}, f, f, f)
			taskfixture.RunWork(t, f.runtime, subjecttask.Voice, map[string]any{"job_id": job.ID}, subjecttask.VoiceHandler(worker))
			final, err := f.store.Get(t.Context(), f.actor, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "succeeded"
			if unknown {
				want = "unknown"
			}
			if final.Status != want || f.sends != 1 {
				t.Fatalf("terminal voice facts=%+v sends=%d", final, f.sends)
			}
			// Re-reading/replaying the original domain request cannot dispatch again.
			if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: job.ID}); err != nil {
				t.Fatal(err)
			}
			if f.sends != 1 {
				t.Fatal("same request dispatched twice", f.sends)
			}
		})
	}
}
