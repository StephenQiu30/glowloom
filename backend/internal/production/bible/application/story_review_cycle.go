package application

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type StoryReviewCommand struct {
	Actor                                            Actor
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	CandidateRevisionID, CandidateRevisionHash       string
	MaxRepairRounds                                  int
}

type StoryReviewState struct {
	Status                                     string
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevisionNo                        int64
	FailureCode                                string
}

type StoryReviewInvocationState struct {
	InvocationID                               string
	Status, FailureCode, ResultHash            string
	CandidateRevisionID, CandidateRevisionHash string
	Candidate                                  json.RawMessage
}

type StoryReviewSeed struct {
	CurrentCandidateRevisionID, CurrentCandidateRevisionHash string
	CurrentCandidateRevisionNo                               int64
	CurrentStageInstanceKey                                  string
	CurrentCandidate                                         json.RawMessage
	SourceRef                                                agentcontract.StageSourceRef
	CurrentUpstream                                          agentcontract.StageUpstreamCandidateRef
	LatestManifest                                           *domain.StoryReviewManifest
	Review, Repair                                           *StoryReviewInvocationState
	RepairsUsed                                              int
}

type StoryReviewPreparation struct {
	Command    StoryReviewCommand
	CreatedAt  time.Time
	Manifest   domain.StoryReviewManifest
	Invocation domain.Invocation
}

type StoryRepairPreparation struct {
	Command    StoryReviewCommand
	CreatedAt  time.Time
	Manifest   domain.StoryReviewManifest
	Invocation domain.Invocation
}

type StoryReviewRepository interface {
	LoadStoryReview(context.Context, StoryReviewCommand) (StoryReviewSeed, error)
	EnsureStoryReviewInvocation(context.Context, StoryReviewPreparation) error
	EnsureStoryRepairInvocation(context.Context, StoryRepairPreparation) error
}

type StoryReviewService struct {
	repository StoryReviewRepository
	repairer   *StoryCandidateRepairService
	config     Config
}

func NewStoryReviewService(
	repository StoryReviewRepository,
	repairer *StoryCandidateRepairService,
	config Config,
) *StoryReviewService {
	return &StoryReviewService{repository: repository, repairer: repairer, config: config}
}

func (service *StoryReviewService) EnsureStoryReview(
	ctx context.Context,
	command StoryReviewCommand,
) (StoryReviewState, error) {
	if service == nil || service.repository == nil || service.repairer == nil ||
		service.config.Now == nil || service.config.NewID == nil {
		return StoryReviewState{}, errors.New("Story review service is unavailable")
	}
	command.Actor.UserID = strings.TrimSpace(command.Actor.UserID)
	for _, identifier := range []string{
		command.Actor.UserID, command.WorkspaceID, command.ProjectID, command.WorkflowRunID,
		command.NodeRunID, command.CandidateRevisionID,
	} {
		if strings.TrimSpace(identifier) == "" {
			return StoryReviewState{}, errors.New("invalid Story review workflow identity")
		}
	}
	for _, identifier := range []string{
		command.Actor.UserID, command.WorkspaceID, command.ProjectID, command.WorkflowRunID,
		command.NodeRunID, command.CandidateRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return StoryReviewState{}, errors.New("invalid Story review workflow identity")
		}
	}
	if command.Actor.TokenVersion < 1 || command.MaxRepairRounds < 1 || command.MaxRepairRounds > 3 ||
		!storyAnalysisHashPattern.MatchString(command.CandidateRevisionHash) {
		return StoryReviewState{}, errors.New("invalid Story review input")
	}

	seed, err := service.repository.LoadStoryReview(ctx, command)
	if err != nil {
		return StoryReviewState{}, err
	}
	if seed.LatestManifest == nil || seed.LatestManifest.RootInputHash != seed.CurrentCandidateRevisionHash {
		manifestID := service.config.NewID()
		version := int64(1)
		var parent *string
		if seed.LatestManifest != nil {
			manifestID = seed.LatestManifest.ManifestID
			version = seed.LatestManifest.Version + 1
			value := seed.LatestManifest.ManifestHash
			parent = &value
		}
		manifest, buildErr := domain.BuildStoryReviewManifest(domain.StoryReviewManifestInput{
			ManifestID: manifestID, Version: version, ParentManifestHash: parent,
			WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
			TargetCandidateRevisionID:   seed.CurrentCandidateRevisionID,
			TargetCandidateRevisionHash: seed.CurrentCandidateRevisionHash,
		})
		if buildErr != nil {
			return StoryReviewState{}, buildErr
		}
		invocation, buildErr := buildStoryReviewInvocation(manifest, seed, command.ProjectID, service.config.NewID(), service.config.Now().UTC())
		if buildErr != nil {
			return StoryReviewState{}, buildErr
		}
		if err = service.repository.EnsureStoryReviewInvocation(ctx, StoryReviewPreparation{
			Command: command, CreatedAt: service.config.Now().UTC(), Manifest: manifest, Invocation: invocation,
		}); err != nil {
			return StoryReviewState{}, err
		}
		return StoryReviewState{Status: "pending"}, nil
	}
	if seed.Review == nil {
		return StoryReviewState{}, errors.New("Story review manifest has no review invocation")
	}
	switch seed.Review.Status {
	case "queued", "running", "unknown":
		return StoryReviewState{Status: "pending"}, nil
	case "failed":
		return needsStoryReview(seed.Review.FailureCode), nil
	case "succeeded":
	default:
		return StoryReviewState{}, errors.New("Story review invocation returned an invalid status")
	}

	var reviewInput agentcontract.StoryGraphReviewStageInput
	requestCandidate, err := domain.DecodeStoryReconciliationCandidate(
		seed.CurrentCandidate,
		storyCandidateEvidence(seed.CurrentCandidate),
	)
	if err != nil {
		return StoryReviewState{}, err
	}
	gate, err := EvaluateStoryReconciliationGate(
		seed.CurrentCandidateRevisionID,
		seed.CurrentCandidateRevisionHash,
		requestCandidate,
	)
	if err != nil {
		return StoryReviewState{}, err
	}
	reviewInput = agentcontract.StoryGraphReviewStageInput{
		ReviewedStage:               domain.ReconcileStoryStage,
		TargetCandidateRevisionID:   seed.CurrentCandidateRevisionID,
		TargetCandidateRevisionHash: seed.CurrentCandidateRevisionHash,
		CandidateItemStart:          0, CandidateItemEnd: domain.StoryReconciliationCandidateItemCount(requestCandidate),
		TargetCandidate: seed.CurrentCandidate, DeterministicGate: gate,
	}
	review, err := agentcontract.DecodeStoryGraphReviewCandidate(seed.Review.Candidate)
	if err != nil || agentcontract.ValidateStoryGraphReviewCandidate(reviewInput, review) != nil {
		return StoryReviewState{}, errors.New("persisted Story review candidate is invalid")
	}
	if len(gate.Blockers) > 0 {
		return needsStoryReview("deterministic_gate_blocked"), nil
	}
	issues := append([]agentcontract.StoryGraphReviewIssue(nil), review.ReviewIssues...)
	slices.SortFunc(issues, func(left, right agentcontract.StoryGraphReviewIssue) int {
		return strings.Compare(left.IssueKey, right.IssueKey)
	})
	var blocking *agentcontract.StoryGraphReviewIssue
	for index := range issues {
		if issues[index].Severity == "blocking" {
			blocking = &issues[index]
			break
		}
	}
	if blocking == nil {
		return StoryReviewState{
			Status: "ready", CandidateRevisionID: seed.CurrentCandidateRevisionID,
			CandidateRevisionHash: seed.CurrentCandidateRevisionHash,
			CandidateRevisionNo:   seed.CurrentCandidateRevisionNo,
		}, nil
	}
	if seed.RepairsUsed >= command.MaxRepairRounds {
		return needsStoryReview("repair_budget_exhausted"), nil
	}
	if blocking.SubjectKey == nil || strings.TrimSpace(*blocking.SubjectKey) == "" {
		return needsStoryReview("repair_boundary_unavailable"), nil
	}
	if seed.Repair == nil {
		target, targetErr := StoryCandidateRepairAllowedTarget(seed.CurrentCandidate, *blocking.SubjectKey)
		if targetErr != nil {
			return needsStoryReview("repair_boundary_unavailable"), nil
		}
		invocation, buildErr := buildStoryRepairInvocation(
			*seed.LatestManifest,
			seed,
			command.ProjectID,
			*blocking,
			target,
			seed.RepairsUsed+1,
			command.MaxRepairRounds,
			service.config.NewID(),
			service.config.Now().UTC(),
		)
		if buildErr != nil {
			return StoryReviewState{}, buildErr
		}
		if err = service.repository.EnsureStoryRepairInvocation(ctx, StoryRepairPreparation{
			Command: command, CreatedAt: service.config.Now().UTC(), Manifest: *seed.LatestManifest, Invocation: invocation,
		}); err != nil {
			return StoryReviewState{}, err
		}
		return StoryReviewState{Status: "pending"}, nil
	}
	switch seed.Repair.Status {
	case "queued", "running", "unknown":
		return StoryReviewState{Status: "pending"}, nil
	case "failed":
		return needsStoryReview(seed.Repair.FailureCode), nil
	case "succeeded":
		_, err = service.repairer.Apply(ctx, command.Actor, StoryCandidateRepairCommand{
			WorkspaceID: command.WorkspaceID, StageInstanceKey: seed.CurrentStageInstanceKey,
			ExpectedRevisionID:            seed.CurrentCandidateRevisionID,
			ExpectedCandidateRevisionHash: seed.CurrentCandidateRevisionHash,
			ExpectedHeadRevision:          seed.CurrentCandidateRevisionNo,
			RepairInvocationID:            seed.Repair.InvocationID,
			IdempotencyKey:                "story-review-repair:" + command.NodeRunID + ":" + seed.Repair.InvocationID,
		})
		if err != nil {
			return StoryReviewState{}, err
		}
		return StoryReviewState{Status: "pending"}, nil
	default:
		return StoryReviewState{}, errors.New("Story repair invocation returned an invalid status")
	}
}

func buildStoryReviewInvocation(
	manifest domain.StoryReviewManifest,
	seed StoryReviewSeed,
	projectID, invocationID string,
	createdAt time.Time,
) (domain.Invocation, error) {
	candidate, err := domain.DecodeStoryReconciliationCandidate(seed.CurrentCandidate, storyCandidateEvidence(seed.CurrentCandidate))
	if err != nil {
		return domain.Invocation{}, err
	}
	gate, err := EvaluateStoryReconciliationGate(seed.CurrentCandidateRevisionID, seed.CurrentCandidateRevisionHash, candidate)
	if err != nil {
		return domain.Invocation{}, err
	}
	inputJSON, err := json.Marshal(agentcontract.StoryGraphReviewStageInput{
		ReviewedStage:               domain.ReconcileStoryStage,
		TargetCandidateRevisionID:   seed.CurrentCandidateRevisionID,
		TargetCandidateRevisionHash: seed.CurrentCandidateRevisionHash,
		CandidateItemStart:          0, CandidateItemEnd: domain.StoryReconciliationCandidateItemCount(candidate),
		TargetCandidate: seed.CurrentCandidate, DeterministicGate: gate,
	})
	if err != nil {
		return domain.Invocation{}, err
	}
	request, err := agentcontract.NewStageInvocation(invocationID, agentcontract.StoryGraphDefinition().ExecutionPolicy(), agentcontract.StageInvocationPayload{
		Stage: domain.ReviewStoryGraphStage, ShardKey: "story-review",
		WorkspaceID: manifest.WorkspaceID, ProjectID: projectID,
		SourceRefs:         []agentcontract.StageSourceRef{seed.SourceRef},
		UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{seed.CurrentUpstream},
		ShardManifestRef:   agentcontract.ShardManifestRef{ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash},
		Shard:              agentcontract.InvocationShard{Kind: "story_review", Key: "story-review", TreePath: "review"},
		StageInput:         inputJSON,
	})
	if err != nil {
		return domain.Invocation{}, err
	}
	return storyReviewDomainInvocation(request, manifest, "story_review_shard", createdAt)
}

func buildStoryRepairInvocation(
	manifest domain.StoryReviewManifest,
	seed StoryReviewSeed,
	projectID string,
	issue agentcontract.StoryGraphReviewIssue,
	target agentcontract.StoryGraphRepairAllowedTarget,
	repairRound, maxRepairRounds int,
	invocationID string,
	createdAt time.Time,
) (domain.Invocation, error) {
	inputJSON, err := json.Marshal(agentcontract.StoryGraphRepairStageInput{
		TargetCandidateRevisionID:   seed.CurrentCandidateRevisionID,
		TargetCandidateRevisionHash: seed.CurrentCandidateRevisionHash,
		ReviewCandidateRevisionID:   seed.Review.CandidateRevisionID,
		ReviewCandidateRevisionHash: seed.Review.CandidateRevisionHash,
		TargetIssue:                 issue, AllowedTargets: []agentcontract.StoryGraphRepairAllowedTarget{target},
		ReadOnlyAdjacency: []agentcontract.StoryGraphRepairReadOnlyFragment{},
		RepairRound:       repairRound, MaxRepairRounds: maxRepairRounds,
	})
	if err != nil {
		return domain.Invocation{}, err
	}
	request, err := agentcontract.NewStageInvocation(invocationID, agentcontract.StoryGraphDefinition().ExecutionPolicy(), agentcontract.StageInvocationPayload{
		Stage: "repair_candidate", ShardKey: "story-repair",
		WorkspaceID: manifest.WorkspaceID, ProjectID: projectID,
		SourceRefs: []agentcontract.StageSourceRef{seed.SourceRef},
		UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{
			seed.CurrentUpstream,
			{
				Stage: domain.ReviewStoryGraphStage, ShardKey: "story-review",
				CandidateRevisionID:   seed.Review.CandidateRevisionID,
				CandidateRevisionHash: seed.Review.CandidateRevisionHash,
				SourceInvocationID:    seed.Review.InvocationID, SourceResultHash: seed.Review.ResultHash,
			},
		},
		ShardManifestRef: agentcontract.ShardManifestRef{ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash},
		Shard:            agentcontract.InvocationShard{Kind: "candidate_repair", Key: "story-repair", TreePath: "repair"},
		StageInput:       inputJSON,
	})
	if err != nil {
		return domain.Invocation{}, err
	}
	return storyReviewDomainInvocation(request, manifest, "story_repair_shard", createdAt)
}

func storyReviewDomainInvocation(
	request agentcontract.StageInvocation,
	manifest domain.StoryReviewManifest,
	requestType string,
	createdAt time.Time,
) (domain.Invocation, error) {
	policyJSON, err := json.Marshal(request.ExecutionPolicy)
	if err != nil {
		return domain.Invocation{}, err
	}
	payloadJSON, err := json.Marshal(request.Payload)
	if err != nil {
		return domain.Invocation{}, err
	}
	stageKey, err := request.StageInstanceKey()
	if err != nil {
		return domain.Invocation{}, err
	}
	return domain.Invocation{
		ID: request.InvocationID, WorkspaceID: manifest.WorkspaceID,
		RequestType: requestType, RequestID: request.InvocationID,
		WorkflowRunID: manifest.WorkflowRunID, NodeRunID: manifest.NodeRunID,
		ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
		Kind: "storygraph_stage", Stage: request.Payload.Stage, ShardKey: request.Payload.ShardKey,
		InputHash: request.InputHash, StageInstanceKey: stageKey, ManifestHash: manifest.ManifestHash,
		ExecutionPolicy: policyJSON, Payload: payloadJSON, Status: "queued", CreatedAt: createdAt,
	}, nil
}

func storyCandidateEvidence(raw json.RawMessage) []domain.Evidence {
	var candidate domain.StoryReconciliationCandidate
	if json.Unmarshal(raw, &candidate) != nil {
		return nil
	}
	return domain.StoryReconciliationCandidateEvidence(candidate)
}

func needsStoryReview(code string) StoryReviewState {
	if strings.TrimSpace(code) == "" {
		code = "story_review_failed"
	}
	return StoryReviewState{Status: "needs_review", FailureCode: code}
}
