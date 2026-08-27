package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

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
	GetDecision(context.Context, Actor, string) (domain.DecisionResult, error)
	EnsureTask(context.Context, domain.HumanTask) (domain.HumanTask, error)
	Claim(context.Context, Actor, ClaimCommand, string, time.Time, time.Time) (domain.ClaimResult, error)
	Renew(context.Context, Actor, RenewCommand, time.Time, time.Time) (domain.ClaimResult, error)
	Release(context.Context, Actor, ReleaseCommand, time.Time) (domain.HumanTask, error)
	ExpireClaims(context.Context, int, time.Time) (int, error)
	Decide(context.Context, Actor, DecideCommand, domain.ReviewDecision, time.Time) (domain.DecisionResult, error)
}

type QueryRepository interface {
	ListTasks(context.Context, Actor, ListTasksQuery, time.Time) (domain.HumanTaskPage, error)
	GetTask(context.Context, Actor, string, time.Time) (domain.HumanTaskDetail, error)
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
	SubjectHash                                      string
	CandidateIDs                                     []string
	RubricVersion                                    string
	AllowedDecisions                                 []string
}

type ListTasksQuery struct {
	ProjectID, Status, SubjectType, After string
	Limit                                 int
}

type ClaimCommand struct {
	TaskID, IdempotencyKey string
	ExpectedRevision       int
}

type RenewCommand struct {
	TaskID, ClaimToken, IdempotencyKey string
	ExpectedRevision                   int
}

type ReleaseCommand struct {
	TaskID, ClaimToken, IdempotencyKey string
	ExpectedRevision                   int
}

type DecideCommand struct {
	TaskID, ClaimToken, Decision, SelectedCandidateID, IdempotencyKey string
	ExpectedSubjectHash                                               string
	ExpectedTaskRevision, ExpectedSubjectRevision                     int
}

func NewService(repository Repository, config Config) *Service {
	return &Service{repository: repository, config: config}
}

func (service *Service) ListTasks(ctx context.Context, actor Actor, query ListTasksQuery) (domain.HumanTaskPage, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	query.Status = strings.TrimSpace(query.Status)
	query.SubjectType = strings.TrimSpace(query.SubjectType)
	query.After = strings.TrimSpace(query.After)
	repository, supported := service.repository.(QueryRepository)
	if service == nil || service.config.Now == nil || !supported || actor.UserID == "" || actor.TokenVersion < 1 ||
		query.ProjectID == "" || query.Limit < 1 || query.Limit > 100 || !validTaskStatusFilter(query.Status) {
		return domain.HumanTaskPage{}, invalid("Invalid human task query")
	}
	if _, err := uuid.Parse(query.ProjectID); err != nil {
		return domain.HumanTaskPage{}, ErrNotFound
	}
	if query.After != "" {
		if _, err := uuid.Parse(query.After); err != nil {
			return domain.HumanTaskPage{}, invalid("Invalid human task cursor")
		}
	}
	return repository.ListTasks(ctx, actor, query, service.config.Now().UTC())
}

func (service *Service) GetTask(ctx context.Context, actor Actor, taskID string) (domain.HumanTaskDetail, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	taskID = strings.TrimSpace(taskID)
	repository, supported := service.repository.(QueryRepository)
	if service == nil || service.config.Now == nil || !supported || actor.UserID == "" || actor.TokenVersion < 1 {
		return domain.HumanTaskDetail{}, invalid("Invalid human task query")
	}
	if _, err := uuid.Parse(taskID); err != nil {
		return domain.HumanTaskDetail{}, ErrNotFound
	}
	return repository.GetTask(ctx, actor, taskID, service.config.Now().UTC())
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

func (service *Service) Renew(ctx context.Context, actor Actor, command RenewCommand) (domain.ClaimResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.TaskID = strings.TrimSpace(command.TaskID)
	command.ClaimToken = strings.TrimSpace(command.ClaimToken)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.ClaimLease <= 0 ||
		actor.UserID == "" || command.TaskID == "" || command.ClaimToken == "" || command.ExpectedRevision < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.ClaimResult{}, invalid("Invalid human task claim renewal")
	}
	now := service.config.Now().UTC()
	return service.repository.Renew(ctx, actor, command, now.Add(service.config.ClaimLease), now)
}

func (service *Service) Release(ctx context.Context, actor Actor, command ReleaseCommand) (domain.HumanTask, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.TaskID = strings.TrimSpace(command.TaskID)
	command.ClaimToken = strings.TrimSpace(command.ClaimToken)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.repository == nil || service.config.Now == nil || actor.UserID == "" ||
		command.TaskID == "" || command.ClaimToken == "" || command.ExpectedRevision < 1 ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.HumanTask{}, invalid("Invalid human task claim release")
	}
	return service.repository.Release(ctx, actor, command, service.config.Now().UTC())
}

func (service *Service) ExpireClaims(ctx context.Context, limit int) (int, error) {
	if service == nil || service.repository == nil || service.config.Now == nil || limit < 1 || limit > 500 {
		return 0, invalid("Invalid human task expiry sweep")
	}
	return service.repository.ExpireClaims(ctx, limit, service.config.Now().UTC())
}

func (service *Service) Decide(ctx context.Context, actor Actor, command DecideCommand) (domain.DecisionResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	command.TaskID = strings.TrimSpace(command.TaskID)
	command.ClaimToken = strings.TrimSpace(command.ClaimToken)
	command.Decision = strings.ToLower(strings.TrimSpace(command.Decision))
	command.SelectedCandidateID = strings.TrimSpace(command.SelectedCandidateID)
	command.ExpectedSubjectHash = strings.ToLower(strings.TrimSpace(command.ExpectedSubjectHash))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil ||
		actor.UserID == "" || command.TaskID == "" || command.ClaimToken == "" || command.ExpectedTaskRevision < 1 ||
		command.ExpectedSubjectRevision < 1 || !validDecision(command.Decision, command.SelectedCandidateID) ||
		!validHash(command.ExpectedSubjectHash) ||
		command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return domain.DecisionResult{}, invalid("Invalid review decision")
	}
	if command.SelectedCandidateID != "" {
		if _, err := uuid.Parse(command.SelectedCandidateID); err != nil {
			return domain.DecisionResult{}, invalid("Invalid selected candidate")
		}
	}
	now := service.config.Now().UTC()
	decision := domain.ReviewDecision{
		ID: service.config.NewID(), HumanTaskID: command.TaskID, Decision: command.Decision,
		SubjectRevision: command.ExpectedSubjectRevision, SubjectHash: command.ExpectedSubjectHash,
		CreatedBy: actor.UserID, CreatedAt: now,
	}
	if command.SelectedCandidateID != "" {
		decision.SelectedCandidateID = &command.SelectedCandidateID
	}
	return service.repository.Decide(ctx, actor, command, decision, now)
}

func (service *Service) GetDecision(ctx context.Context, actor Actor, decisionID string) (domain.DecisionResult, error) {
	actor.UserID = strings.TrimSpace(actor.UserID)
	decisionID = strings.TrimSpace(decisionID)
	if service == nil || service.repository == nil || actor.UserID == "" || actor.TokenVersion < 1 {
		return domain.DecisionResult{}, invalid("Invalid review decision query")
	}
	if _, err := uuid.Parse(actor.UserID); err != nil {
		return domain.DecisionResult{}, invalid("Invalid review decision query")
	}
	if _, err := uuid.Parse(decisionID); err != nil {
		return domain.DecisionResult{}, ErrNotFound
	}
	return service.repository.GetDecision(ctx, actor, decisionID)
}

func normalizeOpen(command OpenCommand) OpenCommand {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.WorkflowRunID = strings.TrimSpace(command.WorkflowRunID)
	command.NodeRunID = strings.TrimSpace(command.NodeRunID)
	command.SubjectType = strings.TrimSpace(command.SubjectType)
	command.SubjectID = strings.TrimSpace(command.SubjectID)
	command.SubjectHash = strings.ToLower(strings.TrimSpace(command.SubjectHash))
	command.RubricVersion = strings.TrimSpace(command.RubricVersion)
	command.CandidateIDs = append([]string(nil), command.CandidateIDs...)
	for index := range command.CandidateIDs {
		command.CandidateIDs[index] = strings.TrimSpace(command.CandidateIDs[index])
	}
	slices.Sort(command.CandidateIDs)
	command.CandidateIDs = slices.Compact(command.CandidateIDs)
	command.AllowedDecisions = append([]string(nil), command.AllowedDecisions...)
	for index := range command.AllowedDecisions {
		command.AllowedDecisions[index] = strings.ToLower(strings.TrimSpace(command.AllowedDecisions[index]))
	}
	slices.Sort(command.AllowedDecisions)
	command.AllowedDecisions = slices.Compact(command.AllowedDecisions)
	return command
}

func validOpen(command OpenCommand) bool {
	if command.WorkspaceID == "" || command.ProjectID == "" || command.WorkflowRunID == "" || command.NodeRunID == "" ||
		command.SubjectType == "" || command.SubjectID == "" || command.SubjectRevision < 1 || !validHash(command.SubjectHash) ||
		command.RubricVersion == "" || len(command.CandidateIDs) > 100 || len(command.AllowedDecisions) == 0 ||
		len(command.AllowedDecisions) > 4 {
		return false
	}
	for _, candidate := range command.CandidateIDs {
		if candidate == "" {
			return false
		}
	}
	for _, decision := range command.AllowedDecisions {
		if !validAllowedDecision(decision) ||
			(decision == "selected" && len(command.CandidateIDs) == 0) {
			return false
		}
	}
	return true
}

func validAllowedDecision(value string) bool {
	switch value {
	case "approved", "rejected", "changes_requested", "selected":
		return true
	default:
		return false
	}
}

func validTaskStatusFilter(value string) bool {
	switch value {
	case "", "active", "OPEN", "CLAIMED", "COMPLETED", "CANCELLED", "STALE":
		return true
	default:
		return false
	}
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
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
		SubjectHash: command.SubjectHash, CandidateIDs: command.CandidateIDs, RubricVersion: command.RubricVersion,
		AllowedDecisions: command.AllowedDecisions,
		Status:           "OPEN", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func invalid(message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 422}
}

func conflict(message string) error {
	return &Error{Code: "resource_conflict", Message: message, Status: 409}
}
