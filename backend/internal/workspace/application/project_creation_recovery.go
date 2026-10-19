package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

var (
	// ErrCreationTargetUnavailable marks a receipt whose project is in the recycle bin.
	ErrCreationTargetUnavailable = errors.New("creation target unavailable")
	// ErrMainCanvasUnavailable marks a broken stable main-canvas association.
	ErrMainCanvasUnavailable = errors.New("main canvas unavailable")
)

// ProjectCreationRecovery reports committed identity, never the original mutable snapshot.
type ProjectCreationRecovery struct {
	State         string
	ProjectID     *uuid.UUID
	CanvasID      *uuid.UUID
	ProjectStatus *string
}

// ProjectCreationRecoveryStore reads the actor receipt and current authorized target.
type ProjectCreationRecoveryStore interface {
	ReadCreationRecovery(context.Context, identity.Principal, uuid.UUID) (ProjectCreationRecovery, error)
}

// ProjectCreationRecoveryQuery validates readonly creation recovery results.
type ProjectCreationRecoveryQuery struct{ store ProjectCreationRecoveryStore }

// NewProjectCreationRecoveryQuery constructs the readonly recovery query.
func NewProjectCreationRecoveryQuery(store ProjectCreationRecoveryStore) *ProjectCreationRecoveryQuery {
	return &ProjectCreationRecoveryQuery{store: store}
}

// Resolve reads a committed creation result without creating or replaying a mutation.
func (q *ProjectCreationRecoveryQuery) Resolve(ctx context.Context, actor identity.Principal, key uuid.UUID) (ProjectCreationRecovery, error) {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.ScopeID == uuid.Nil || actor.SessionID == uuid.Nil || actor.CredentialEpoch < 1 {
		return ProjectCreationRecovery{}, identity.ErrForbidden
	}
	if key == uuid.Nil {
		return ProjectCreationRecovery{}, ErrInvalidCreateProject
	}
	if q == nil || q.store == nil {
		return ProjectCreationRecovery{}, ErrProjectDependencyUnavailable
	}
	result, err := q.store.ReadCreationRecovery(ctx, actor, key)
	if err != nil {
		return ProjectCreationRecovery{}, fmt.Errorf("resolve project creation: %w", err)
	}
	valid := result.State == "unrecorded" && result.ProjectID == nil && result.CanvasID == nil && result.ProjectStatus == nil
	if result.State == "created" && result.ProjectID != nil && *result.ProjectID != uuid.Nil && result.CanvasID != nil && *result.CanvasID != uuid.Nil && result.ProjectStatus != nil {
		valid = *result.ProjectStatus == "active" || *result.ProjectStatus == "archived" || *result.ProjectStatus == "copying"
	}
	if !valid {
		return ProjectCreationRecovery{}, ErrProjectDependencyUnavailable
	}
	return result, nil
}
