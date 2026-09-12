package application

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
)

var ErrReferenceCallStateConflict = errors.New("Reference Provider call state conflicts with expected revision")

type ReferenceCallDispatchUsage struct{ Unresolved, Daily int64 }

type ReferenceCallDispatchRepository interface {
	ReferenceExecutionRepository
	FindReferenceCallState(context.Context, string, string, string, string) (domain.ReferenceCallState, error)
	UpdateReferenceCallState(context.Context, string, string, string, domain.ReferenceCallState, domain.ReferenceCallState) error
	ReferenceCallDispatchUsage(context.Context, string, time.Time, time.Time) (ReferenceCallDispatchUsage, error)
}

type ReferenceCallDispatchTransactions interface {
	WithinReferenceCallDispatch(context.Context, func(ReferenceCallDispatchRepository) error) error
}

type ClaimReferenceCallCommand struct {
	WorkspaceID      string                       `json:"workspace_id"`
	ProjectID        string                       `json:"project_id"`
	ExecutionRef     domain.GenerationRevisionRef `json:"execution_ref"`
	CallKey          string                       `json:"call_key"`
	ExpectedRevision int64                        `json:"expected_revision"`
}

type ExpireReferenceCallCommand struct {
	WorkspaceID     string                       `json:"workspace_id"`
	ProjectID       string                       `json:"project_id"`
	ExecutionRef    domain.GenerationRevisionRef `json:"execution_ref"`
	CallKey         string                       `json:"call_key"`
	SubmissionToken string                       `json:"submission_token"`
}

type ReferenceCallDispatchResult struct {
	State          domain.ReferenceCallState `json:"state"`
	ShouldDispatch bool                      `json:"should_dispatch"`
}

type ReferenceCallDispatchService struct {
	transactions ReferenceCallDispatchTransactions
	registry     *MediaFactoryRegistry
	now          func() time.Time
	newID        func() string
}

func NewReferenceCallDispatchService(transactions ReferenceCallDispatchTransactions, registry *MediaFactoryRegistry, now func() time.Time, newID func() string) (*ReferenceCallDispatchService, error) {
	if transactions == nil || registry == nil || now == nil || newID == nil {
		return nil, errors.New("Reference call dispatch dependencies are required")
	}
	return &ReferenceCallDispatchService{transactions, registry, now, newID}, nil
}

func validReferenceCallScope(actor Actor, workspace, project string, execution domain.GenerationRevisionRef, key string) bool {
	return validUUID(actor.UserID) && actor.TokenVersion > 0 && validUUID(workspace) && validUUID(project) && execution.Valid() && execution.Revision == 1 && intentHashPattern.MatchString(key)
}

func (service *ReferenceCallDispatchService) Claim(ctx context.Context, actor Actor, command ClaimReferenceCallCommand) (ReferenceCallDispatchResult, error) {
	if !validReferenceCallScope(actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey) || command.ExpectedRevision != 1 {
		return ReferenceCallDispatchResult{}, invalid("Invalid Reference call dispatch command")
	}
	var result ReferenceCallDispatchResult
	err := service.transactions.WithinReferenceCallDispatch(ctx, func(repo ReferenceCallDispatchRepository) error {
		execution, state, err := readReferenceCall(ctx, repo, actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey)
		if err != nil {
			return err
		}
		// No replay, restart or new caller obtains a second send right. A stale
		// business Head does not require another Submit to observe this fact.
		if state.Status != domain.ProviderCallPending {
			result.State = state
			return nil
		}
		if state.Revision != command.ExpectedRevision {
			return ErrReferenceCallStateConflict
		}
		receipt, err := repo.FindReferenceExecutionReceipt(ctx, execution.WorkspaceID, execution.ID)
		if err != nil {
			return err
		}
		prepared := PrepareInitialReferenceExecutionCommand{WorkspaceID: execution.WorkspaceID, ProjectID: execution.ProjectID, TargetRef: execution.ReadSet.TargetRef, AuthorizationRef: execution.ReadSet.AuthorizationRef, IdempotencyKey: receipt.IdempotencyKey}
		inputs, err := readReferenceExecutionInputs(ctx, repo, service.registry, actor, prepared, readFrozenReferenceExecutionProvider)
		if err != nil {
			return err
		}
		preparer := Actor{UserID: execution.CreatedBy, TokenVersion: execution.MembershipTokenVersion}
		hash, err := referenceExecutionPreparationInputHash(preparer, prepared, inputs.readSet)
		if err != nil {
			return err
		}
		if hash != receipt.InputHash {
			return platformcommand.ErrInputMismatch
		}
		if _, err = validateReferenceExecutionPublication(ctx, repo, preparer, prepared, inputs, receipt, execution); err != nil {
			return err
		}
		limits := domain.DefaultReferenceGenerationLimits()
		policy, err := limits.Ref()
		if err != nil {
			return err
		}
		if policy != execution.ReadSet.OperationalPolicyRef {
			return conflict("Reference call operational policy has drifted")
		}
		now := service.now().UTC().Truncate(time.Microsecond)
		if now.IsZero() || now.Before(execution.CreatedAt) {
			return invalid("Reference dispatch time predates execution")
		}
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		usage, err := repo.ReferenceCallDispatchUsage(ctx, command.WorkspaceID, start, start.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		if usage.Unresolved < 0 || usage.Daily < 0 || usage.Unresolved >= int64(limits.MaxConcurrentCalls) || usage.Daily >= int64(limits.MaxDailyCalls) {
			return conflict("Reference call operational limit reached")
		}
		claimed, send, err := domain.ClaimReferenceCall(state, domain.ReferenceCallDispatch{SubmissionToken: service.newID(), DispatchedBy: actor.UserID, MembershipTokenVersion: actor.TokenVersion, DispatchedAt: now, DeadlineAt: now.Add(time.Duration(limits.SubmitTimeoutSeconds) * time.Second)})
		if err != nil {
			return err
		}
		if !send {
			return ErrReferenceCallStateConflict
		}
		if err = repo.UpdateReferenceCallState(ctx, command.WorkspaceID, command.ProjectID, execution.ID, state, claimed); err != nil {
			return err
		}
		result = ReferenceCallDispatchResult{State: claimed, ShouldDispatch: true}
		return nil
	})
	if err != nil {
		return ReferenceCallDispatchResult{}, err
	}
	return result, nil
}

func (service *ReferenceCallDispatchService) Expire(ctx context.Context, actor Actor, command ExpireReferenceCallCommand) (domain.ReferenceCallState, error) {
	if !validReferenceCallScope(actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey) || !validUUID(command.SubmissionToken) {
		return domain.ReferenceCallState{}, invalid("Invalid Reference call expiry command")
	}
	var result domain.ReferenceCallState
	err := service.transactions.WithinReferenceCallDispatch(ctx, func(repo ReferenceCallDispatchRepository) error {
		_, state, err := readReferenceCall(ctx, repo, actor, command.WorkspaceID, command.ProjectID, command.ExecutionRef, command.CallKey)
		if err != nil {
			return err
		}
		if state.Dispatch == nil || state.Dispatch.SubmissionToken != command.SubmissionToken {
			return conflict("Reference call submission token does not match")
		}
		expired, changed, err := domain.ExpireReferenceCall(state, service.now())
		if err != nil {
			return err
		}
		if changed {
			if err = repo.UpdateReferenceCallState(ctx, command.WorkspaceID, command.ProjectID, command.ExecutionRef.ID, state, expired); err != nil {
				return err
			}
		}
		result = expired
		return nil
	})
	if err != nil {
		return domain.ReferenceCallState{}, err
	}
	return result, nil
}

func readReferenceCall(ctx context.Context, repo ReferenceCallDispatchRepository, actor Actor, workspace, project string, ref domain.GenerationRevisionRef, key string) (domain.ReferenceExecution, domain.ReferenceCallState, error) {
	if err := repo.LockProviderWorkspace(ctx, workspace); err != nil {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, err
	}
	if err := repo.AuthorizeReferenceGenerationProject(ctx, actor, workspace, project); err != nil {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, err
	}
	execution, err := repo.FindReferenceExecution(ctx, workspace, project, ref.ID)
	if err != nil {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, err
	}
	if execution.Revision != ref.Revision || execution.ContentHash != ref.ContentHash {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, conflict("Reference dispatch execution identity has drifted")
	}
	job, _, err := repo.FindReferenceProviderJob(ctx, workspace, project, ref.ID)
	if err != nil {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, err
	}
	if job.ExecutionRef != ref || !slices.Contains(job.CallKeys, key) {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, conflict("Reference call does not belong to the frozen job")
	}
	state, err := repo.FindReferenceCallState(ctx, workspace, project, ref.ID, key)
	if err != nil {
		return domain.ReferenceExecution{}, domain.ReferenceCallState{}, err
	}
	return execution, state, nil
}
