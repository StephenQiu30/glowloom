package agent_test

import (
	"errors"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestPlanCandidateRegistrationInvalidatesApprovalAndValidatesDAG(t *testing.T) {
	r := controlRun()
	now := r.CreatedAt
	if err := r.Apply(domain.ControlInput{Action: "approve", CandidateID: r.Candidate.ID, ManifestHash: r.Candidate.Hash, MaxMicros: 30, ExpiresAt: now.Add(time.Hour)}, now); err != nil {
		t.Fatal(err)
	}
	old := r.RuntimeIdentity()
	plan := domain.PlanManifest{Schema: "glowloom.plan/v1", Revision: 1, GoalHash: r.GoalHash, ModelReleaseID: r.Goal.Model.ReleaseID, Inputs: r.Goal.Inputs, Steps: []domain.PlanStep{{ID: "parse", Agent: "script-parser/v1", Required: true, Actions: []domain.ActionGrant{{Name: "agent.delegate"}}}}}
	candidate, err := r.RegisterPlan(uuid.New(), plan, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Approved || r.Status != "awaiting_execution_approval" {
		t.Fatal("old planning approval retained", r.Status)
	}
	if err := r.Authorize(old, "model.generate", now); !errors.Is(err, domain.ErrFenced) {
		t.Fatal("old approved identity accepted", err)
	}
	if err := r.Apply(domain.ControlInput{Action: "approve", CandidateID: candidate.ID, ManifestHash: candidate.Hash, MaxMicros: 70, ExpiresAt: now.Add(time.Hour)}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckGrant(domain.ActionRequest{Action: "agent.delegate", StepID: "parse"}, now); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckGrant(domain.ActionRequest{Action: "generation.submit", StepID: "parse"}, now); !errors.Is(err, domain.ErrPermission) {
		t.Fatal("unlisted step action", err)
	}
	plan.Revision = 2
	plan.Steps[0].DependsOn = []string{"missing"}
	if _, err := r.RegisterPlan(uuid.New(), plan, now); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("missing dependency accepted", err)
	}
	plan.Steps[0].DependsOn = []string{"parse"}
	if _, err := r.RegisterPlan(uuid.New(), plan, now); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("cycle accepted", err)
	}
}
