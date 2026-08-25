package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/project/domain"
)

type Capability string

const (
	ContentRead     Capability = "content:read"
	ContentWrite    Capability = "content:write"
	BudgetManage    Capability = "budget:manage"
	WorkspaceManage Capability = "workspace:manage"
)

type Actor struct {
	UserID       string
	TokenVersion int
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
type Repository interface {
	Authorize(context.Context, Actor, string, Capability) error
	Create(context.Context, domain.Project) error
	Get(context.Context, string, bool) (domain.Project, error)
	Save(context.Context, domain.Project) error
	Delete(context.Context, string) error
	List(context.Context, ListQuery) ([]domain.Project, int, error)
	Dependencies(context.Context, string) (DependencySummary, error)
	AppendAudit(context.Context, AuditEvent) error
	FindReceipt(context.Context, string, string, string) (platformcommand.Receipt, error)
	FindReceiptByResource(context.Context, string, string, string, string) (platformcommand.Receipt, error)
	CreateReceipt(context.Context, platformcommand.Receipt) error
}

var ErrNotFound = errors.New("project not found")

type Error struct {
	Code       string
	Message    string
	Status     int
	NextAction string
	Details    map[string]any
}

func (e *Error) Error() string { return e.Message }

const (
	CodeInvalidRequest      = "invalid_request"
	CodeUnauthenticated     = "unauthenticated"
	CodeForbidden           = "forbidden"
	CodeNotFound            = "not_found"
	CodeVersionConflict     = "version_conflict"
	CodeStateConflict       = "state_conflict"
	CodeIdempotencyConflict = "idempotency_conflict"
)

func IsCode(err error, code string) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

type CreateCommand struct {
	WorkspaceID, Name     string
	Description           *string
	AspectRatio, Language string
	VisualStyle           *string
	TargetDurationMS      int
	IdempotencyKey        string
}
type UpdateCommand struct {
	Name, Description, AspectRatio, Language, VisualStyle *string
	DescriptionSet, VisualStyleSet                        bool
	TargetDurationMS                                      *int
	ExpectedRevision                                      int
	IdempotencyKey                                        string
}
type BudgetCommand struct {
	Amount, Currency string
	ExpectedRevision int
	IdempotencyKey   string
}
type StateCommand struct {
	ExpectedRevision int
	IdempotencyKey   string
}
type ListQuery struct {
	WorkspaceID         string
	IncludeArchived     bool
	Search, Sort, Order string
	Limit, Offset       int
}
type AuditEvent struct {
	WorkspaceID, ActorID, Action, TargetID string
	Revision                               int
	OccurredAt                             time.Time
	Metadata                               map[string]any
}
type DependencySummary struct{ Episodes, ScriptVersions, StoryboardShots, StoryboardSpecVersions, Assets, AssetVersions, Tasks int }
type DeleteBlocker struct {
	Code         string `json:"code"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Summary      string `json:"summary"`
}
type DeletePreflight struct {
	Allowed  bool            `json:"allowed"`
	Blockers []DeleteBlocker `json:"blockers"`
}

type Service struct {
	transactions TransactionManager
	now          func() time.Time
	newID        func() string
}

func NewService(transactions TransactionManager, now func() time.Time, newID func() string) *Service {
	return &Service{transactions: transactions, now: now, newID: newID}
}

func (service *Service) Create(ctx context.Context, actor Actor, command CreateCommand) (domain.Project, error) {
	command.Name = strings.TrimSpace(command.Name)
	if command.WorkspaceID == "" || command.Name == "" || len(command.Name) > 120 {
		return domain.Project{}, invalid("Invalid project")
	}
	if command.AspectRatio == "" {
		command.AspectRatio = "9:16"
	}
	if !oneOf(command.AspectRatio, "9:16", "16:9", "1:1") {
		return domain.Project{}, invalid("Invalid aspect ratio")
	}
	if command.Language == "" {
		command.Language = "zh-CN"
	}
	if command.TargetDurationMS == 0 {
		command.TargetDurationMS = 90000
	}
	if command.TargetDurationMS < 1000 || command.TargetDurationMS > 7200000 {
		return domain.Project{}, invalid("Invalid target duration")
	}
	if err := validateIdempotencyKey(command.IdempotencyKey); err != nil {
		return domain.Project{}, err
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return domain.Project{}, err
	}
	now := service.now()
	var project domain.Project
	err = service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.Authorize(ctx, actor, command.WorkspaceID, ContentWrite); err != nil {
			return err
		}
		receipt, receiptErr := repository.FindReceipt(ctx, command.WorkspaceID, "project.create", command.IdempotencyKey)
		if receiptErr == nil {
			project, receiptErr = replayProject(receipt, inputHash)
			return receiptErr
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		project = domain.Project{ID: service.newID(), WorkspaceID: command.WorkspaceID, Name: command.Name, Description: command.Description, AspectRatio: command.AspectRatio, Language: command.Language, VisualStyle: command.VisualStyle, TargetDurationMS: command.TargetDurationMS, BudgetLimit: "0.000000", Currency: "CNY", Status: domain.StatusActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := repository.Create(ctx, project); err != nil {
			return err
		}
		if err := repository.AppendAudit(ctx, audit(project, actor, "project.created", now)); err != nil {
			return err
		}
		return service.saveReceipt(ctx, repository, actor, project, "project.create", command.IdempotencyKey, inputHash, project, now)
	})
	return project, err
}

func (service *Service) List(ctx context.Context, actor Actor, query ListQuery) ([]domain.Project, int, error) {
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, invalid("Invalid pagination")
	}
	if query.Sort == "" {
		query.Sort = "updated_at"
	}
	if query.Order == "" {
		query.Order = "desc"
	}
	if !oneOf(query.Sort, "name", "created_at", "updated_at") || !oneOf(query.Order, "asc", "desc") {
		return nil, 0, invalid("Invalid sorting")
	}
	var items []domain.Project
	var total int
	err := service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.Authorize(ctx, actor, query.WorkspaceID, ContentRead); err != nil {
			return err
		}
		var err error
		items, total, err = repository.List(ctx, query)
		return err
	})
	return items, total, err
}

func (service *Service) Get(ctx context.Context, actor Actor, id string) (domain.Project, error) {
	return service.loadAuthorized(ctx, actor, id, ContentRead, false, false)
}

func (service *Service) Update(ctx context.Context, actor Actor, id string, command UpdateCommand) (domain.Project, error) {
	if command.ExpectedRevision < 1 {
		return domain.Project{}, invalid("Invalid expected revision")
	}
	if command.Name == nil && command.Description == nil && !command.DescriptionSet && command.AspectRatio == nil && command.Language == nil && command.VisualStyle == nil && !command.VisualStyleSet && command.TargetDurationMS == nil {
		return domain.Project{}, invalid("No project changes supplied")
	}
	if err := validateIdempotencyKey(command.IdempotencyKey); err != nil {
		return domain.Project{}, err
	}
	inputHash, err := platformcommand.InputHash(struct {
		ID      string
		Command UpdateCommand
	}{id, command})
	if err != nil {
		return domain.Project{}, err
	}
	var result domain.Project
	err = service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		project, err := repository.Get(ctx, id, true)
		if err != nil {
			return normalizeNotFound(err)
		}
		if err = repository.Authorize(ctx, actor, project.WorkspaceID, ContentWrite); err != nil {
			return err
		}
		receipt, receiptErr := repository.FindReceipt(ctx, project.WorkspaceID, "project.update", command.IdempotencyKey)
		if receiptErr == nil {
			result, receiptErr = replayProject(receipt, inputHash)
			return receiptErr
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if project.Status == domain.StatusArchived {
			return state("Project is archived", "restore_project")
		}
		if err = revision(project, command.ExpectedRevision); err != nil {
			return err
		}
		if command.Name != nil {
			value := strings.TrimSpace(*command.Name)
			if value == "" || len(value) > 120 {
				return invalid("Invalid project name")
			}
			project.Name = value
		}
		if command.DescriptionSet {
			project.Description = command.Description
		}
		if command.AspectRatio != nil {
			if !oneOf(*command.AspectRatio, "9:16", "16:9", "1:1") {
				return invalid("Invalid aspect ratio")
			}
			project.AspectRatio = *command.AspectRatio
		}
		if command.Language != nil {
			project.Language = *command.Language
		}
		if command.VisualStyleSet {
			project.VisualStyle = command.VisualStyle
		}
		if command.TargetDurationMS != nil {
			if *command.TargetDurationMS < 1000 || *command.TargetDurationMS > 7200000 {
				return invalid("Invalid target duration")
			}
			project.TargetDurationMS = *command.TargetDurationMS
		}
		project.Revision++
		project.UpdatedAt = service.now()
		if err = repository.Save(ctx, project); err != nil {
			return err
		}
		if err = repository.AppendAudit(ctx, audit(project, actor, "project.updated", project.UpdatedAt)); err != nil {
			return err
		}
		result = project
		return service.saveReceipt(ctx, repository, actor, project, "project.update", command.IdempotencyKey, inputHash, project, project.UpdatedAt)
	})
	return result, err
}

func (service *Service) UpdateBudget(ctx context.Context, actor Actor, id string, command BudgetCommand) (domain.Project, error) {
	if command.ExpectedRevision < 1 || command.Amount == "" || len(command.Currency) != 3 {
		return domain.Project{}, invalid("Invalid budget")
	}
	if err := validateIdempotencyKey(command.IdempotencyKey); err != nil {
		return domain.Project{}, err
	}
	inputHash, err := platformcommand.InputHash(struct {
		ID      string
		Command BudgetCommand
	}{id, command})
	if err != nil {
		return domain.Project{}, err
	}
	var result domain.Project
	err = service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		project, err := repository.Get(ctx, id, true)
		if err != nil {
			return normalizeNotFound(err)
		}
		if err = repository.Authorize(ctx, actor, project.WorkspaceID, BudgetManage); err != nil {
			return err
		}
		receipt, receiptErr := repository.FindReceipt(ctx, project.WorkspaceID, "project.budget.update", command.IdempotencyKey)
		if receiptErr == nil {
			result, receiptErr = replayProject(receipt, inputHash)
			return receiptErr
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if project.Status == domain.StatusArchived {
			return state("Project is archived", "restore_project")
		}
		if err = revision(project, command.ExpectedRevision); err != nil {
			return err
		}
		project.BudgetLimit = command.Amount
		project.Currency = command.Currency
		project.Revision++
		project.UpdatedAt = service.now()
		if err = repository.Save(ctx, project); err != nil {
			return err
		}
		if err = repository.AppendAudit(ctx, audit(project, actor, "project.budget_updated", project.UpdatedAt)); err != nil {
			return err
		}
		result = project
		return service.saveReceipt(ctx, repository, actor, project, "project.budget.update", command.IdempotencyKey, inputHash, project, project.UpdatedAt)
	})
	return result, err
}

func (service *Service) SetArchived(ctx context.Context, actor Actor, id string, command StateCommand, archived bool) (domain.Project, error) {
	if err := validateIdempotencyKey(command.IdempotencyKey); err != nil {
		return domain.Project{}, err
	}
	operation := "project.restore"
	if archived {
		operation = "project.archive"
	}
	inputHash, err := platformcommand.InputHash(struct {
		ID      string
		Command StateCommand
	}{id, command})
	if err != nil {
		return domain.Project{}, err
	}
	var result domain.Project
	err = service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		project, err := repository.Get(ctx, id, true)
		if err != nil {
			return normalizeNotFound(err)
		}
		if err = repository.Authorize(ctx, actor, project.WorkspaceID, WorkspaceManage); err != nil {
			return err
		}
		receipt, receiptErr := repository.FindReceipt(ctx, project.WorkspaceID, operation, command.IdempotencyKey)
		if receiptErr == nil {
			result, receiptErr = replayProject(receipt, inputHash)
			return receiptErr
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		if err = revision(project, command.ExpectedRevision); err != nil {
			return err
		}
		expected := domain.StatusArchived
		if archived {
			expected = domain.StatusActive
		}
		if project.Status != expected {
			return state("Project state conflict", "")
		}
		now := service.now()
		if archived {
			project.Status = domain.StatusArchived
			project.ArchivedAt = &now
			project.ArchivedBy = &actor.UserID
		} else {
			project.Status = domain.StatusActive
			project.ArchivedAt = nil
			project.ArchivedBy = nil
		}
		project.Revision++
		project.UpdatedAt = now
		if err = repository.Save(ctx, project); err != nil {
			return err
		}
		action := "project.restored"
		if archived {
			action = "project.archived"
		}
		if err = repository.AppendAudit(ctx, audit(project, actor, action, now)); err != nil {
			return err
		}
		result = project
		return service.saveReceipt(ctx, repository, actor, project, operation, command.IdempotencyKey, inputHash, project, now)
	})
	return result, err
}

func (service *Service) DeletePreflight(ctx context.Context, actor Actor, id string) (DeletePreflight, error) {
	project, err := service.loadAuthorized(ctx, actor, id, WorkspaceManage, false, true)
	if err != nil {
		return DeletePreflight{}, err
	}
	var dependencies DependencySummary
	err = service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		var err error
		dependencies, err = repository.Dependencies(ctx, id)
		return err
	})
	if err != nil {
		return DeletePreflight{}, err
	}
	blockers := dependencyBlockers(project, dependencies)
	return DeletePreflight{Allowed: len(blockers) == 0, Blockers: blockers}, nil
}
func (service *Service) Delete(ctx context.Context, actor Actor, id string, expectedRevision int, idempotencyKey string) error {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return err
	}
	inputHash, err := platformcommand.InputHash(struct {
		ID               string
		ExpectedRevision int
	}{id, expectedRevision})
	if err != nil {
		return err
	}
	return service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		receipt, receiptErr := repository.FindReceiptByResource(ctx, id, actor.UserID, "project.delete", idempotencyKey)
		if receiptErr == nil {
			_, receiptErr = platformcommand.Replay[bool](receipt, inputHash)
			return normalizeReceiptError(receiptErr)
		}
		if !errors.Is(receiptErr, platformcommand.ErrReceiptNotFound) {
			return receiptErr
		}
		project, err := repository.Get(ctx, id, true)
		if err != nil {
			return normalizeNotFound(err)
		}
		if err = repository.Authorize(ctx, actor, project.WorkspaceID, WorkspaceManage); err != nil {
			return err
		}
		if err = revision(project, expectedRevision); err != nil {
			return err
		}
		dependencies, err := repository.Dependencies(ctx, id)
		if err != nil {
			return err
		}
		if len(dependencyBlockers(project, dependencies)) > 0 {
			return state("Project has dependent resources", "review_delete_blockers")
		}
		if err = repository.AppendAudit(ctx, audit(project, actor, "project.deleted", service.now())); err != nil {
			return err
		}
		if err = repository.Delete(ctx, id); err != nil {
			return err
		}
		return service.saveReceipt(ctx, repository, actor, project, "project.delete", idempotencyKey, inputHash, true, service.now())
	})
}

func (service *Service) loadAuthorized(ctx context.Context, actor Actor, id string, capability Capability, forUpdate, allowArchived bool) (domain.Project, error) {
	var project domain.Project
	err := service.transactions.WithinTransaction(ctx, func(repository Repository) error {
		var err error
		project, err = repository.Get(ctx, id, forUpdate)
		if err != nil {
			return normalizeNotFound(err)
		}
		if err = repository.Authorize(ctx, actor, project.WorkspaceID, capability); err != nil {
			return err
		}
		if !allowArchived && project.Status == domain.StatusArchived && capability != ContentRead {
			return state("Project is archived", "restore_project")
		}
		return nil
	})
	return project, err
}
func dependencyBlockers(project domain.Project, d DependencySummary) []DeleteBlocker {
	pairs := []struct {
		n             int
		code, summary string
	}{{d.Episodes, "HAS_EPISODES", "项目包含 %d 个单集"}, {d.ScriptVersions, "HAS_SCRIPT_VERSIONS", "项目关联 %d 个剧本版本"}, {d.StoryboardShots, "HAS_STORYBOARD_SHOTS", "项目关联 %d 个分镜镜头"}, {d.Assets, "HAS_ASSETS", "项目已有 %d 个资产"}, {d.Tasks, "HAS_TASKS", "项目关联 %d 个任务"}}
	result := make([]DeleteBlocker, 0, len(pairs))
	for _, pair := range pairs {
		if pair.n > 0 {
			result = append(result, DeleteBlocker{Code: pair.code, ResourceType: "project", ResourceID: project.ID, Summary: fmt.Sprintf(pair.summary, pair.n)})
		}
	}
	return result
}
func audit(project domain.Project, actor Actor, action string, at time.Time) AuditEvent {
	return AuditEvent{WorkspaceID: project.WorkspaceID, ActorID: actor.UserID, Action: action, TargetID: project.ID, Revision: project.Revision, OccurredAt: at, Metadata: map[string]any{"revision": project.Revision, "status": project.Status}}
}
func revision(project domain.Project, expected int) error {
	if project.Revision != expected {
		return &Error{Code: CodeVersionConflict, Message: "Project has changed", Status: 409, Details: map[string]any{"current_revision": project.Revision}}
	}
	return nil
}
func normalizeNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return &Error{Code: CodeNotFound, Message: "Project not found", Status: 404}
	}
	return err
}
func invalid(message string) error {
	return &Error{Code: CodeInvalidRequest, Message: message, Status: 422}
}
func state(message, next string) error {
	return &Error{Code: CodeStateConflict, Message: message, Status: 409, NextAction: next}
}
func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (service *Service) saveReceipt(ctx context.Context, repository Repository, actor Actor, project domain.Project, operation, key, inputHash string, result any, createdAt time.Time) error {
	encoded, err := platformcommand.Result(result)
	if err != nil {
		return err
	}
	return repository.CreateReceipt(ctx, platformcommand.Receipt{ID: service.newID(), WorkspaceID: project.WorkspaceID, Operation: operation, IdempotencyKey: key, InputHash: inputHash, ResourceID: project.ID, Result: encoded, CreatedBy: actor.UserID, CreatedAt: createdAt})
}

func replayProject(receipt platformcommand.Receipt, inputHash string) (domain.Project, error) {
	project, err := platformcommand.Replay[domain.Project](receipt, inputHash)
	return project, normalizeReceiptError(err)
}

func normalizeReceiptError(err error) error {
	if errors.Is(err, platformcommand.ErrInputMismatch) {
		return &Error{Code: CodeIdempotencyConflict, Message: "Idempotency key was already used with different input", Status: 409}
	}
	return err
}

func validateIdempotencyKey(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 200 {
		return invalid("Invalid idempotency key")
	}
	return nil
}
