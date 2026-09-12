package workflow_test

import (
	"context"
	"testing"

	workflowexecution "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/execution"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflowdomain "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type recordingHumanGateOwner struct {
	executor string
}

func (owner *recordingHumanGateOwner) ApplyHumanGateDecision(
	_ context.Context,
	_ workflowapp.Actor,
	application workflowdomain.HumanGateOwnerApplication,
) (workflowdomain.HumanGateOwnerResult, error) {
	owner.executor = application.Executor
	return workflowdomain.HumanGateOwnerResult{ReceiptID: "owner-receipt", Operation: "owner-operation"}, nil
}

func TestHumanGateOwnerRouterSendsProductionWorldAndVisualFoundationToBackendOwners(t *testing.T) {
	production, generation := &recordingHumanGateOwner{}, &recordingHumanGateOwner{}
	router, err := workflowexecution.NewHumanGateOwnerRouter(production, generation)
	if err != nil {
		t.Fatal(err)
	}
	for _, executor := range []string{"gate.production_world_review", "gate.visual_foundation_scope"} {
		production.executor = ""
		if _, err = router.ApplyHumanGateDecision(
			context.Background(), workflowapp.Actor{UserID: "actor"},
			workflowdomain.HumanGateOwnerApplication{Executor: executor},
		); err != nil || production.executor != executor || generation.executor != "" {
			t.Fatalf("route %s to Backend owner: production=%q generation=%q err=%v", executor, production.executor, generation.executor, err)
		}
	}
}
