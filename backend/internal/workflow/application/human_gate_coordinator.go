package application

import (
	"context"
	"errors"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

type HumanGateDecisionReader interface {
	GetHumanGateDecision(context.Context, Actor, string) (domain.HumanGateReviewDecision, error)
}

type HumanGateSignalService interface {
	SignalHumanGate(context.Context, Actor, SignalHumanGateCommand) (domain.SignalIntent, error)
}

type HumanGateCoordinationRepository interface {
	GetHumanGateCoordination(context.Context, string, string) (domain.HumanGateCoordination, error)
}

type HumanGateRepairService interface {
	RerunFromHumanGate(context.Context, Actor, domain.HumanGateReviewDecision) (domain.WorkflowRun, error)
}

type HumanGateCoordinator struct {
	decisions HumanGateDecisionReader
	signals   HumanGateSignalService
	statuses  HumanGateCoordinationRepository
	repairs   HumanGateRepairService
}

func NewHumanGateCoordinator(
	decisions HumanGateDecisionReader,
	signals HumanGateSignalService,
	statuses HumanGateCoordinationRepository,
	repairs HumanGateRepairService,
) *HumanGateCoordinator {
	return &HumanGateCoordinator{decisions: decisions, signals: signals, statuses: statuses, repairs: repairs}
}

func (coordinator *HumanGateCoordinator) GetHumanGate(
	ctx context.Context,
	actor Actor,
	decisionID string,
) (domain.HumanGateCoordination, error) {
	decision, err := coordinator.resolveDecision(ctx, actor, decisionID)
	if err != nil {
		return domain.HumanGateCoordination{}, err
	}
	return coordinator.loadStatus(ctx, decision)
}

func (coordinator *HumanGateCoordinator) ResumeHumanGate(
	ctx context.Context,
	actor Actor,
	decisionID string,
) (domain.HumanGateCoordination, error) {
	decision, err := coordinator.resolveDecision(ctx, actor, decisionID)
	if err != nil {
		return domain.HumanGateCoordination{}, err
	}
	current, err := coordinator.loadStatus(ctx, decision)
	if err != nil {
		return domain.HumanGateCoordination{}, err
	}
	if current.OwnerApplyStatus == "conflict" || current.WorkflowResumeStatus == "conflict" {
		return current, conflict("Human gate coordination is in conflict")
	}
	if current.WorkflowResumeStatus == "completed" {
		return coordinator.ensureRepair(ctx, actor, decision, current)
	}
	_, signalErr := coordinator.signals.SignalHumanGate(ctx, actor, SignalHumanGateCommand{
		WorkspaceID: decision.WorkspaceID, WorkflowRunID: decision.WorkflowRunID, NodeRunID: decision.NodeRunID,
		HumanTaskID: decision.HumanTaskID, ReviewDecisionID: decision.ReviewDecisionID,
		SubjectRevision: decision.SubjectRevision, Decision: decision.Decision,
		DecisionPayloadHash: decision.DecisionPayloadHash,
		IdempotencyKey:      "human-gate-decision:" + decision.ReviewDecisionID,
	})
	updated, statusErr := coordinator.loadStatus(ctx, decision)
	if statusErr != nil {
		return domain.HumanGateCoordination{}, statusErr
	}
	if signalErr != nil {
		return updated, signalErr
	}
	if updated.OwnerApplyStatus == "conflict" || updated.WorkflowResumeStatus == "conflict" {
		return updated, conflict("Human gate coordination is in conflict")
	}
	return coordinator.ensureRepair(ctx, actor, decision, updated)
}

func (coordinator *HumanGateCoordinator) ensureRepair(
	ctx context.Context,
	actor Actor,
	decision domain.HumanGateReviewDecision,
	coordination domain.HumanGateCoordination,
) (domain.HumanGateCoordination, error) {
	if decision.Decision != "changes_requested" ||
		(decision.SubjectType != "structure_identity_gate_input" && decision.SubjectType != "production_world_gate_input") ||
		coordination.WorkflowResumeStatus != "completed" ||
		coordination.RepairWorkflowRunID != "" {
		return coordination, nil
	}
	repairRun, err := coordinator.repairs.RerunFromHumanGate(ctx, actor, decision)
	if errors.Is(err, ErrRepairSourcePending) {
		return coordination, nil
	}
	if err != nil {
		return coordination, err
	}
	coordination.RepairWorkflowRunID = repairRun.ID
	return coordination, nil
}

func (coordinator *HumanGateCoordinator) resolveDecision(
	ctx context.Context,
	actor Actor,
	decisionID string,
) (domain.HumanGateReviewDecision, error) {
	decisionID = strings.TrimSpace(decisionID)
	actor.UserID = strings.TrimSpace(actor.UserID)
	if coordinator == nil || coordinator.decisions == nil || coordinator.signals == nil || coordinator.statuses == nil || coordinator.repairs == nil ||
		actor.UserID == "" || actor.TokenVersion < 1 || decisionID == "" {
		return domain.HumanGateReviewDecision{}, invalid("Invalid human gate coordination request")
	}
	decision, err := coordinator.decisions.GetHumanGateDecision(ctx, actor, decisionID)
	if err != nil {
		return domain.HumanGateReviewDecision{}, normalizeError(err)
	}
	if decision.ReviewDecisionID != decisionID || decision.WorkspaceID == "" || decision.WorkflowRunID == "" ||
		decision.ProjectID == "" || decision.NodeRunID == "" || decision.HumanTaskID == "" || decision.SubjectRevision < 1 ||
		len(decision.SubjectHash) != 64 || len(decision.DecisionPayloadHash) != 64 ||
		!validHumanGateDecision(decision.Decision) {
		return domain.HumanGateReviewDecision{}, errors.New("human gate review decision has drifted")
	}
	hasStructureRepair := decision.ChangeRequest != nil
	hasProductionWorldRepair := decision.ProductionWorldChangeRequest != nil
	if (decision.Decision == "changes_requested") != (hasStructureRepair || hasProductionWorldRepair) ||
		(hasStructureRepair && hasProductionWorldRepair) {
		return domain.HumanGateReviewDecision{}, errors.New("human gate repair decision has drifted")
	}
	if decision.Decision == "changes_requested" &&
		!((decision.SubjectType == "structure_identity_gate_input" && hasStructureRepair) ||
			(decision.SubjectType == "production_world_gate_input" && hasProductionWorldRepair)) {
		return domain.HumanGateReviewDecision{}, errors.New("human gate repair subject has drifted")
	}
	return decision, nil
}

func (coordinator *HumanGateCoordinator) loadStatus(
	ctx context.Context,
	decision domain.HumanGateReviewDecision,
) (domain.HumanGateCoordination, error) {
	status, err := coordinator.statuses.GetHumanGateCoordination(ctx, decision.WorkspaceID, decision.ReviewDecisionID)
	if err != nil {
		return domain.HumanGateCoordination{}, normalizeError(err)
	}
	if status.ReviewDecisionID != decision.ReviewDecisionID || status.DecisionStatus != "recorded" ||
		!validOwnerApplyStatus(status.OwnerApplyStatus) || !validWorkflowResumeStatus(status.WorkflowResumeStatus) {
		return domain.HumanGateCoordination{}, errors.New("human gate coordination status has drifted")
	}
	return status, nil
}

func validHumanGateDecision(value string) bool {
	switch value {
	case "approved", "rejected", "changes_requested", "selected":
		return true
	default:
		return false
	}
}

func validOwnerApplyStatus(value string) bool {
	switch value {
	case "pending", "not_required", "completed", "conflict":
		return true
	default:
		return false
	}
}

func validWorkflowResumeStatus(value string) bool {
	switch value {
	case "pending", "unknown", "completed", "conflict":
		return true
	default:
		return false
	}
}

var _ HumanGateSignalService = (*SignalService)(nil)
