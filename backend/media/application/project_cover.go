package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ProjectCoverRequest binds a display observation to the original closed project.
// It is neither permission to read bytes nor evidence to release a reference.
type ProjectCoverRequest struct {
	CloseID, ProjectID, OrganizationID, ScopeID, AssetID uuid.UUID
	ProjectEpoch                                         int64
	CoordinationID                                       uuid.UUID
}

// Validate requires the original closure identity and its independently installed control.
func (r ProjectCoverRequest) Validate() error {
	if r.CloseID == uuid.Nil || r.ProjectID == uuid.Nil || r.OrganizationID == uuid.Nil || r.ScopeID == uuid.Nil || r.AssetID == uuid.Nil || r.ProjectEpoch < 2 || r.CoordinationID != uuid.NewSHA1(r.CloseID, []byte("creation.project.close.authority")) {
		return writefence.ErrInvalid
	}
	return nil
}

// ProjectCoverSnapshot contains only display facts, without an object route or grant.
type ProjectCoverSnapshot struct {
	Request       ProjectCoverRequest
	Available     bool
	AssetRevision int64
}

// ProjectCoverReader reads Media's local facts after the original closure barrier.
type ProjectCoverReader interface {
	ReadProjectCover(context.Context, ProjectCoverRequest) (ProjectCoverSnapshot, error)
}
