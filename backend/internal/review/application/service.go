package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/review/domain"
)

var ErrNotFound = errors.New("review resource not found")

type Error struct {
	Code, Message string
	Status        int
}

func (value *Error) Error() string { return value.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type Repository interface {
	FindTaskByNode(context.Context, string, string) (domain.HumanTask, error)
	EnsureTask(context.Context, domain.HumanTask) (domain.HumanTask, error)
	Claim(context.Context, Actor, ClaimCommand, string, time.Time, time.Time) (domain.ClaimResult, error)
	Decide(context.Context, Actor, DecideCommand, domain.ReviewDecision, time.Time) (domain.DecisionResult, error)
}

type Config struct {
	Now        func() time.Time
	NewID      func() string
	ClaimLease time.Duration
}

type Service struct {
	repository Repository
	config     Config
}

type OpenCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	SubjectType, SubjectID                           string
	SubjectRevision                                  int
	CandidateIDs                                     []string
	RubricVersion                                    string
}

type ClaimCommand struct {
	TaskID, IdempotencyKey string
	ExpectedRevision       int
}

type DecideCommand struct {
	TaskID, ClaimToken, Decision, SelectedCandidateID, IdempotencyKey string
	ExpectedTaskRevision, ExpectedSubjectRevision                     int
}

func NewService(repository Repository, config Config) *Service {
	return &Service{repository: repository, config: config}
}

func (service *Service) Open(ctx context.Context, command OpenCommand) (domain.HumanTask, error) {
	command = normalizeOpen(command)
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil ||
		!validOpen(command) {
		return domain.HumanTask{}, invalid("Invalid human task input")
	}
	if existing, err := service.repository.FindTaskByNode(ctx, command.WorkspaceID, command.NodeRunID); err == nil {
		desired := taskFromCommand(existing.ID, command, existing.CreatedAt)
		if !domain.SameTaskBinding(existing, desired) {
			return domain.HumanTask{}, conflict("Human task binding has drifted")
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return domain.HumanTask{}, err
	}
	now := service.config.Now().UTC()
	desired := taskFromCommand(service.config.NewID(), command, now)
	return service.repository.EnsureTask(ctx, desired)
}

func (service *Service) Claim(ctx context.Context, actor Actor, command ClaimCommand) (domain.ClaimResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.TaskID = strings.TrimSpace(command.TaskID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil ||
		service.config.ClaimLease <= 0 || actor.UserID == "" || command.TaskID == "" || command.ExpectedRevision < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.ClaimResult{}, invalid("Invalid human task claim")
	}
	now := service.config.Now().UTC()
	claimToken := strings.TrimSpace(service.config.NewID())
	if claimToken == "" {
		return domain.ClaimResult{}, errors.New("human task claim token is empty")
	}
	return service.repository.Claim(ctx, actor, command, claimToken, now.Add(service.config.ClaimLease), now)
}

func (service *Service) Decide(ctx context.Context, actor Actor, command DecideCommand) (domain.DecisionResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.TaskID = strings.TrimSpace(command.TaskID)
	command.ClaimToken = strings.TrimSpace(command.ClaimToken)
	command.Decision = strings.ToLower(strings.TrimSpace(command.Decision))
	command.SelectedCandidateID = strings.TrimSpace(command.SelectedCandidateID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.UserID == "" || command.TaskID == "" || command.ClaimToken == "" || command.ExpectedTaskRevision < 1 ||
		command.ExpectedSubjectRevision < 1 || !validDecision(command.Decision, command.SelectedCandidateID) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.DecisionResult{}, invalid("Invalid review decision")
	}
	now := service.config.Now().UTC()
	decision := domain.ReviewDecision{
		ID: service.config.NewID(), HumanTaskID: command.TaskID, Decision: command.Decision,
		SubjectRevision: command.ExpectedSubjectRevision, CreatedBy: actor.UserID, CreatedAt: now,
	}
	if command.SelectedCandidateID != "" {
		decision.SelectedCandidateID = &command.SelectedCandidateID
	}
	return service.repository.Decide(ctx, actor, command, decision, now)
}

func normalizeOpen(command OpenCommand) OpenCommand {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.WorkflowRunID = strings.TrimSpace(command.WorkflowRunID)
	command.NodeRunID = strings.TrimSpace(command.NodeRunID)
	command.SubjectType = strings.TrimSpace(command.SubjectType)
	command.SubjectID = strings.TrimSpace(command.SubjectID)
	command.RubricVersion = strings.TrimSpace(command.RubricVersion)
	command.CandidateIDs = append([]string(nil), command.CandidateIDs...)
	for index := range command.CandidateIDs {
		command.CandidateIDs[index] = strings.TrimSpace(command.CandidateIDs[index])
	}
	slices.Sort(command.CandidateIDs)
	command.CandidateIDs = slices.Compact(command.CandidateIDs)
	return command
}

func validOpen(command OpenCommand) bool {
	if command.WorkspaceID == "" || command.ProjectID == "" || command.WorkflowRunID == "" || command.NodeRunID == "" ||
		command.SubjectType == "" || command.SubjectID == "" || command.SubjectRevision < 1 || command.RubricVersion == "" ||
		len(command.CandidateIDs) > 100 {
		return false
	}
	for _, candidate := range command.CandidateIDs {
		if candidate == "" {
			return false
		}
	}
	return true
}

func validDecision(decision, selectedCandidateID string) bool {
	switch decision {
	case "approved", "rejected", "changes_requested":
		return selectedCandidateID == ""
	case "selected":
		return selectedCandidateID != ""
	default:
		return false
	}
}

func taskFromCommand(id string, command OpenCommand, now time.Time) domain.HumanTask {
	return domain.HumanTask{
		ID: id, WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		SubjectType: command.SubjectType, SubjectID: command.SubjectID, SubjectRevision: command.SubjectRevision,
		CandidateIDs: command.CandidateIDs, RubricVersion: command.RubricVersion,
		Status: "OPEN", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}
