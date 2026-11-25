package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/domain"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrInvalidProjectChange means a public project mutation violates its closed contract.
	ErrInvalidProjectChange = errors.New("invalid project change")
)

// ProjectWorkGuard reads owning modules' durable work while the project is locked.
// The adapter binds this port to its transaction before calling it.
type ProjectWorkGuard interface {
	HasInflightWork(context.Context, identityapp.Principal, uuid.UUID) (bool, error)
}

// ProjectSnapshot is an authorized project's settings and lifecycle facts.
// Defaults remain owned by workspace and are read at the same project revision.
type ProjectSnapshot struct {
	LastModifiedAt    *time.Time `json:",omitempty"`
	LastModifiedBasis *string    `json:",omitempty"`
	Project           domain.Project
	DefaultModels     map[string]string
	CoverUnavailable  bool `json:"CoverUnavailable,omitempty"`
}

// ProjectChangeInput reuses the mutable patch contract for all project changes.
// Lifecycle actions allow only the project's identity, revision, and request ID.
type ProjectChangeInput struct {
	Action         string
	IdempotencyKey uuid.UUID
	Patch          UpdateProjectInput
}

// Validate rejects unknown actions and settings attached to lifecycle requests.
func (i ProjectChangeInput) Validate() error {
	p := i.Patch
	requestID, err := uuid.Parse(p.RequestID)
	if p.ProjectID == uuid.Nil || p.ExpectedRevision < 1 || p.ExpectedRevision >= math.MaxInt32 || i.IdempotencyKey == uuid.Nil || err != nil || requestID == uuid.Nil || requestID.String() != p.RequestID {
		return ErrInvalidProjectChange
	}
	hasPatch := p.Name != nil || p.Description != nil || p.StylePresetID != nil || p.AllowOverseasModels != nil || p.SetCover || p.CoverAssetID != nil
	switch i.Action {
	case "patch":
		if !hasPatch || !validProjectPatch(p) {
			return ErrInvalidProjectChange
		}
	case "archive", "unarchive", "delete", "restore":
		if hasPatch {
			return ErrInvalidProjectChange
		}
	default:
		return ErrInvalidProjectChange
	}
	return nil
}

// ProjectLifecycleStore atomically authorizes reads and durable project changes.
type ProjectLifecycleStore interface {
	ReadProjectSnapshot(context.Context, identityapp.Principal, uuid.UUID) (ProjectSnapshot, error)
	ApplyProjectChange(context.Context, identityapp.Principal, ProjectChangeInput, time.Time) (ProjectSnapshot, error)
}

// ProjectLifecycle exposes complete settings and reversible lifecycle commands.
type ProjectLifecycle struct {
	store ProjectLifecycleStore
	now   func() time.Time
}

// NewProjectLifecycle injects persistence and the clock used for recovery deadlines.
func NewProjectLifecycle(store ProjectLifecycleStore, now func() time.Time) *ProjectLifecycle {
	return &ProjectLifecycle{store: store, now: now}
}

// Get reads an undeleted active or archived project in the current organization.
func (s *ProjectLifecycle) Get(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (ProjectSnapshot, error) {
	if !canManageModelDefaults(actor) {
		return ProjectSnapshot{}, identityapp.ErrForbidden
	}
	if projectID == uuid.Nil {
		return ProjectSnapshot{}, workspacesource.ErrProjectNotFound
	}
	if s == nil || s.store == nil {
		return ProjectSnapshot{}, workspacesource.ErrProjectDependencyUnavailable
	}
	saved, err := s.store.ReadProjectSnapshot(ctx, actor, projectID)
	if err != nil {
		return ProjectSnapshot{}, fmt.Errorf("read project lifecycle: %w", err)
	}
	if !validProjectSnapshot(saved, actor, projectID) || saved.Project.IsDelete {
		return ProjectSnapshot{}, workspacesource.ErrProjectDependencyUnavailable
	}
	return saved, nil
}

// Change commits a guarded mutation or returns its first durable response.
func (s *ProjectLifecycle) Change(ctx context.Context, actor identityapp.Principal, input ProjectChangeInput) (ProjectSnapshot, error) {
	if !canManageModelDefaults(actor) {
		return ProjectSnapshot{}, identityapp.ErrForbidden
	}
	if err := input.Validate(); err != nil {
		return ProjectSnapshot{}, err
	}
	if s == nil || s.store == nil || s.now == nil {
		return ProjectSnapshot{}, workspacesource.ErrProjectDependencyUnavailable
	}
	now := s.now().UTC()
	if now.IsZero() {
		return ProjectSnapshot{}, ErrInvalidProjectChange
	}
	saved, err := s.store.ApplyProjectChange(ctx, actor, input, now)
	if err != nil {
		return ProjectSnapshot{}, fmt.Errorf("change project lifecycle: %w", err)
	}
	if !validProjectSnapshot(saved, actor, input.Patch.ProjectID) {
		return ProjectSnapshot{}, workspacesource.ErrProjectDependencyUnavailable
	}
	return saved, nil
}

// Validate rejects incomplete or inconsistent persisted lifecycle facts.
func (s ProjectSnapshot) Validate() error {
	p := s.Project
	if (s.CoverUnavailable && p.CoverAssetID == nil) || p.Validate() != nil || p.CreateTime.IsZero() || p.UpdateTime.IsZero() || !validDefaultModels(s.DefaultModels) {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	if (p.Status == "archived" && (p.ArchivedAt == nil || p.ArchivedAt.IsZero())) || (p.Status == "active" && p.ArchivedAt != nil) {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	if p.IsDelete {
		if p.DeleteTime == nil || p.PurgeAfter == nil || !p.PurgeAfter.After(*p.DeleteTime) {
			return workspacesource.ErrProjectDependencyUnavailable
		}
	} else if p.DeleteTime != nil || p.PurgeAfter != nil {
		return workspacesource.ErrProjectDependencyUnavailable
	}
	return nil
}

func validProjectSnapshot(s ProjectSnapshot, actor identityapp.Principal, id uuid.UUID) bool {
	return s.Project.ID == id && s.Project.OrgID == actor.OrgID && s.Validate() == nil
}

func validProjectPatch(input UpdateProjectInput) bool {
	if (!input.SetCover && input.CoverAssetID != nil) || (input.CoverAssetID != nil && *input.CoverAssetID == uuid.Nil) {
		return false
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 50 || strings.ContainsRune(name, 0) {
			return false
		}
	}
	return input.Description == nil || (utf8.ValidString(*input.Description) && !strings.ContainsRune(*input.Description, 0))
}

func applyProjectPatch(before domain.Project, input UpdateProjectInput) domain.Project {
	after := before
	if input.SetCover {
		after.CoverAssetID = nil
		if input.CoverAssetID != nil {
			id := *input.CoverAssetID
			after.CoverAssetID = &id
		}
	}
	if input.Name != nil {
		after.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		after.Description = *input.Description
	}
	if input.StylePresetID != nil {
		after.StylePresetID = *input.StylePresetID
	}
	if input.AllowOverseasModels != nil {
		after.AllowOverseasModels = *input.AllowOverseasModels
	}
	return after
}

// PrepareProjectChange performs only domain transitions on transaction-locked facts.
// It does not query work, authorize a database actor, or commit any side effect.
func PrepareProjectChange(actor identityapp.Principal, input ProjectChangeInput, before domain.Project, now time.Time, hasBlockingWork bool) (domain.Project, []identityapp.OutboxEvent, error) {
	if !canManageModelDefaults(actor) {
		return domain.Project{}, nil, identityapp.ErrForbidden
	}
	if err := input.Validate(); err != nil {
		return domain.Project{}, nil, err
	}
	if before.ID != input.Patch.ProjectID || before.OrgID != actor.OrgID || before.Validate() != nil || now.IsZero() {
		return domain.Project{}, nil, ErrInvalidProjectChange
	}
	if before.Revision != input.Patch.ExpectedRevision {
		return domain.Project{}, nil, domain.ErrProjectRevisionConflict
	}
	after := before
	var err error
	switch input.Action {
	case "patch":
		if err = before.CanWrite(); err != nil {
			break
		}
		after = applyProjectPatch(before, input.Patch)
		if sameProjectSettings(before, after) {
			return before, nil, nil
		}
		after.Revision++
	case "archive":
		err = after.Archive(now, hasBlockingWork)
	case "unarchive":
		err = after.Unarchive()
		if err == nil && hasBlockingWork {
			err = domain.ErrProjectHasInflightOperations
		}
	case "delete":
		err = after.Delete(now, hasBlockingWork)
	case "restore":
		err = after.Restore(now)
		if err == nil && hasBlockingWork {
			err = domain.ErrProjectHasInflightOperations
		}
	}
	if err != nil {
		return domain.Project{}, nil, err
	}
	if err := after.Validate(); err != nil {
		return domain.Project{}, nil, fmt.Errorf("validate project change: %w", err)
	}
	if input.Action == "patch" {
		events, err := projectUpdatedEvents(actor, before, after, input.Patch.RequestID, now.UTC())
		return after, events, err
	}
	events, err := projectLifecycleEvents(actor, input.Action, before, after, input.Patch.RequestID, now.UTC())
	return after, events, err
}

func projectLifecycleEvents(actor identityapp.Principal, action string, before, after domain.Project, requestID string, now time.Time) ([]identityapp.OutboxEvent, error) {
	events, err := workspacesource.ProjectLifecycleEvents(actor.ID, actor.OrgID, action, before, after, requestID, now, uuid.New(), uuid.New())
	if err != nil {
		return nil, err
	}
	result := make([]identityapp.OutboxEvent, len(events))
	for i, event := range events {
		result[i] = identityapp.OutboxEvent{ID: event.ID, Topic: event.Topic, PartitionKey: event.PartitionKey, Payload: event.Payload}
	}
	return result, nil
}
