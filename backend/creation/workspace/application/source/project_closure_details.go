package source

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ProjectCoverRequest reads one exact cover after the original close barrier.
// It never grants asset access, creates a pin or exposes an object location.
type ProjectCoverRequest struct {
	CloseID, ProjectID, OrganizationID, ScopeID, AssetID uuid.UUID
	ProjectEpoch                                         int64
	CoordinationID                                       uuid.UUID
}

// Validate requires the exact resource and installed closing generation.
func (r ProjectCoverRequest) Validate() error {
	if r.CloseID == uuid.Nil || r.ProjectID == uuid.Nil || r.OrganizationID == uuid.Nil || r.ScopeID == uuid.Nil || r.AssetID == uuid.Nil || r.ProjectEpoch < 1 || r.CoordinationID != uuid.NewSHA1(r.CloseID, []byte("creation.project.close.authority")) {
		return writefence.ErrInvalid
	}
	return nil
}

// ProjectCoverObservation is an immutable read fact, not a write authority.
type ProjectCoverObservation struct {
	Request       ProjectCoverRequest
	Available     bool
	AssetRevision int64
}

// ProjectCoverReader consumes Media's current exact closed-project cover fact.
type ProjectCoverReader interface {
	ReadProjectCover(context.Context, ProjectCoverRequest) (ProjectCoverObservation, error)
}

// ProjectCloseDetails freezes the original complete public response context.
type ProjectCloseDetails struct {
	Project           domain.Project
	DefaultModels     map[string]string
	LastModifiedAt    *time.Time
	LastModifiedBasis *string
	CoverUnavailable  bool
	Cover             *ProjectCoverObservation `json:",omitempty"`
}

// Validate rejects missing frozen fields and inconsistent cover evidence.
func (d ProjectCloseDetails) Validate() error {
	if d.Project.Validate() != nil || d.DefaultModels == nil || d.LastModifiedAt == nil || d.LastModifiedAt.IsZero() || d.LastModifiedBasis == nil || *d.LastModifiedBasis == "" {
		return writefence.ErrConflict
	}
	if d.Cover != nil && (d.Cover.AssetRevision < 0 || (d.Cover.Available && d.Cover.AssetRevision < 1)) {
		return writefence.ErrConflict
	}
	if d.Project.CoverAssetID == nil {
		if d.Cover != nil || d.CoverUnavailable {
			return writefence.ErrConflict
		}
	} else {
		if d.Cover == nil || d.Cover.Request.Validate() != nil || d.Cover.Request.AssetID != *d.Project.CoverAssetID || d.Cover.Request.ProjectID != d.Project.ID || d.CoverUnavailable == d.Cover.Available {
			return writefence.ErrConflict
		}
	}
	return nil
}

// CoverRequest binds the expected cover to this closure's project authority.
func (s ProjectClosureSpec) CoverRequest() ProjectCoverRequest {
	request := ProjectCoverRequest{CloseID: s.ID, ProjectID: s.ProjectID, OrganizationID: s.OrganizationID, ScopeID: s.ScopeID, ProjectEpoch: s.Control().Epoch, CoordinationID: s.CoordinationID}
	if s.ExpectedCoverAssetID != nil {
		request.AssetID = *s.ExpectedCoverAssetID
	}
	return request
}

type projectClosureCoverContext struct{}

// ProjectClosureCover exposes only the use case's verified exact Media read fact.
func ProjectClosureCover(ctx context.Context) (ProjectCoverObservation, bool) {
	value, ok := ctx.Value(projectClosureCoverContext{}).(ProjectCoverObservation)
	return value, ok
}
