package agent_test

import (
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/google/uuid"
)

func controlRun() domain.Run {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	return domain.NewRun(uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), domain.GoalManifest{
		Goal: "解析一集剧本", Model: domain.DialogueModel{ReleaseID: uuid.New(), ContextWindow: 8192, MaxTokens: 512, Input: []string{"text"}, Params: []byte(`{}`)},
		Inputs:               []domain.InputRef{{Kind: "script", ID: uuid.MustParse("739ac222-aa2e-48f2-b6cb-c6cb44375715"), VersionID: uuid.MustParse("962aadf8-26d8-4759-9407-30f0c732668f"), Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		PlanningBudgetMicros: 30, RootBudgetMicros: 100,
	}, now)
}
func TestControlApprovalAndBarrier(t *testing.T) {
	r := controlRun()
	now := r.CreatedAt
	identity := r.RuntimeIdentity()
	if err := r.Authorize(identity, "model.generate", now); !errors.Is(err, domain.ErrApprovalRequired) {
		t.Fatalf("first submission authorized a model: %v", err)
	}
	if err := r.Apply(domain.ControlInput{Action: "approve", CandidateID: uuid.New(), ManifestHash: r.Candidate.Hash, MaxMicros: 30, ExpiresAt: now.Add(time.Hour)}, now); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("unregistered candidate approved: %v", err)
	}
	if err := r.Apply(domain.ControlInput{Action: "approve", CandidateID: r.Candidate.ID, ManifestHash: r.Candidate.Hash, MaxMicros: 30, ExpiresAt: now.Add(time.Hour)}, now); err != nil {
		t.Fatal(err)
	}
	identity = r.RuntimeIdentity()
	if err := r.Authorize(identity, "model.generate", now); err != nil {
		t.Fatal(err)
	}
	old := identity
	if err := r.Apply(domain.ControlInput{Action: "pause"}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Authorize(old, "model.generate", now); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("old revision passed pause: %v", err)
	}
	if err := r.Authorize(r.RuntimeIdentity(), "model.generate", now); !errors.Is(err, domain.ErrRunClosed) {
		t.Fatalf("paused run admitted: %v", err)
	}
	if err := r.Apply(domain.ControlInput{Action: "resume"}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Authorize(old, "model.generate", now); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("old revision reopened: %v", err)
	}
	if err := r.Apply(domain.ControlInput{Action: "stop"}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(domain.ControlInput{Action: "resume"}, now); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stopped run resumed: %v", err)
	}
	if err := r.Authorize(r.RuntimeIdentity(), "session.snapshot", now); err != nil {
		t.Fatalf("stop hid receipts: %v", err)
	}
}
func TestRunAdmissionPlanningOnlyReadsFrozenSources(t *testing.T) {
	r := controlRun()
	now := r.CreatedAt
	if err := r.Apply(domain.ControlInput{Action: "approve", CandidateID: r.Candidate.ID, ManifestHash: r.Candidate.Hash, MaxMicros: 30, ExpiresAt: now.Add(time.Hour)}, now); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"generation.submit", "skill.run", "agent.delegate", "script.parse.propose"} {
		if err := r.CheckGrant(domain.ActionRequest{Action: action}, now); !errors.Is(err, domain.ErrPermission) {
			t.Errorf("planning allowed %s: %v", action, err)
		}
	}
	if err := r.CheckGrant(domain.ActionRequest{Action: "script.read", Input: r.Goal.Inputs[0]}, now); err != nil {
		t.Fatal(err)
	}
	input := r.Goal.Inputs[0]
	input.VersionID = uuid.New()
	if err := r.CheckGrant(domain.ActionRequest{Action: "script.read", Input: input}, now); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("read changed version: %v", err)
	}
	if err := r.Reserve(domain.ActionRequest{Action: "model.generate", MaxMicros: 20}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Reserve(domain.ActionRequest{Action: "model.compaction", MaxMicros: 11}, now); !errors.Is(err, domain.ErrBudget) {
		t.Fatalf("overspent planning cap: %v", err)
	}
	if r.ReservedMicros != 20 {
		t.Fatalf("reserved after rejection=%d", r.ReservedMicros)
	}
	if err := r.Apply(domain.ControlInput{Action: "pause"}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(domain.ControlInput{Action: "resume"}, now.Add(2*time.Hour)); !errors.Is(err, domain.ErrApprovalRequired) {
		t.Fatalf("expired approval resumed: %v", err)
	}
}
