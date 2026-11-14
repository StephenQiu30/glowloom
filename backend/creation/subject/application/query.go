package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/subject/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// BoundMedia is a server-authorized immutable reference, never an arbitrary asset grant.
type BoundMedia struct {
	SubjectID       uuid.UUID
	VersionID       uuid.UUID
	OwnerScopeID    uuid.UUID
	PhysicalScopeID uuid.UUID
	BindingID       uuid.UUID
	BindingSHA256   string
	ContentSHA256   string
	Name            string
	Description     string
	Role            string
	Position        int
	Media           domain.MediaFact
}

// Query exposes current-space reads independently from command execution.
type Query interface {
	Get(context.Context, identity.Principal, uuid.UUID) (Detail, error)
	List(context.Context, identity.Principal, ListInput) (Page, error)
	ReadVersion(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (domain.Version, error)
	BoundMedia(context.Context, identity.Principal, uuid.UUID) (BoundMedia, error)
}

// RecoveryQuery rechecks current scope before disclosing a permanent command result.
type RecoveryQuery interface {
	Recover(context.Context, identity.Principal, uuid.UUID) (Recovery, error)
}
