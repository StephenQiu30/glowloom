package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

var (
	// ErrInvalidProjectChange means a public project mutation violates its closed contract.
	ErrInvalidProjectChange = errors.New("invalid project change")
	// ErrProjectDependencyUnavailable means required lifecycle evidence cannot be read.
	ErrProjectDependencyUnavailable = errors.New("project lifecycle dependency unavailable")
)

// ProjectWorkGuard reads owning modules' durable work while the project is locked.
// The adapter binds this port to its transaction before calling it.
type ProjectWorkGuard interface {
	HasInflightWork(context.Context, identityapp.Principal, uuid.UUID) (bool, error)
}

// ProjectSnapshot is an authorized project's settings and lifecycle facts.
// Defaults remain owned by workspace and are read at the same project revision.
type ProjectSnapshot struct {
	Project          domain.Project
	DefaultModels    map[string]string
	CoverUnavailable bool `json:"CoverUnavailable,omitempty"`
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
		return ProjectSnapshot{}, ErrProjectNotFound
	}
	if s == nil || s.store == nil {
		return ProjectSnapshot{}, ErrProjectDependencyUnavailable
	}
	saved, err := s.store.ReadProjectSnapshot(ctx, actor, projectID)
	if err != nil {
		return ProjectSnapshot{}, fmt.Errorf("read project lifecycle: %w", err)
	}
	if !validProjectSnapshot(saved, actor, projectID) || saved.Project.IsDelete {
		return ProjectSnapshot{}, ErrProjectDependencyUnavailable
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
		return ProjectSnapshot{}, ErrProjectDependencyUnavailable
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
		return ProjectSnapshot{}, ErrProjectDependencyUnavailable
	}
	return saved, nil
}

// Validate rejects incomplete or inconsistent persisted lifecycle facts.
func (s ProjectSnapshot) Validate() error {
	p := s.Project
	if (s.CoverUnavailable && p.CoverAssetID == nil) || p.Validate() != nil || p.CreateTime.IsZero() || p.UpdateTime.IsZero() || !validDefaultModels(s.DefaultModels) {
		return ErrProjectDependencyUnavailable
	}
	if (p.Status == "archived" && (p.ArchivedAt == nil || p.ArchivedAt.IsZero())) || (p.Status == "active" && p.ArchivedAt != nil) {
		return ErrProjectDependencyUnavailable
	}
	if p.IsDelete {
		if p.DeleteTime == nil || p.PurgeAfter == nil || !p.PurgeAfter.After(*p.DeleteTime) {
			return ErrProjectDependencyUnavailable
		}
	} else if p.DeleteTime != nil || p.PurgeAfter != nil {
		return ErrProjectDependencyUnavailable
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
	changes := map[string]string{"archive": "archived", "unarchive": "unarchived", "delete": "deleted", "restore": "restored"}
	change := changes[action]
	changedID, auditID := uuid.New(), uuid.New()
	changed, err := json.Marshal(map[string]any{"event_id": changedID, "event_type": projectChangedTopic, "occurred_at": now, "org_id": actor.OrgID, "project_id": after.ID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "project", "id": after.ID, "revision": after.Revision}, "data": map[string]any{"change": change}})
	if err != nil {
		return nil, fmt.Errorf("encode project lifecycle event: %w", err)
	}
	audit, err := json.Marshal(map[string]any{"event_id": auditID, "event_type": projectAuditTopic, "occurred_at": now, "org_id": actor.OrgID, "project_id": after.ID, "actor": map[string]any{"kind": "user", "id": actor.ID}, "aggregate": map[string]any{"type": "audit", "id": auditID}, "data": map[string]any{"action": "project." + change, "object": map[string]any{"type": "project", "id": after.ID}, "request_id": requestID, "before": projectLifecycleSummary(before), "after": projectLifecycleSummary(after)}})
	if err != nil {
		return nil, fmt.Errorf("encode project lifecycle audit: %w", err)
	}
	return []identityapp.OutboxEvent{{ID: changedID, Topic: projectChangedTopic, PartitionKey: after.ID.String(), Payload: changed}, {ID: auditID, Topic: projectAuditTopic, PartitionKey: after.ID.String(), Payload: audit}}, nil
}

func projectLifecycleSummary(p domain.Project) map[string]any {
	return map[string]any{"revision": p.Revision, "status": p.Status, "is_delete": p.IsDelete, "archived_at": p.ArchivedAt, "delete_time": p.DeleteTime, "purge_after": p.PurgeAfter}
}
