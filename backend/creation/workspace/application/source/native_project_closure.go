package source

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// CloseProjectInput is the original public archive/delete command shape.
type CloseProjectInput struct {
	ProjectID, IdempotencyKey uuid.UUID
	ExpectedRevision          int64
	Action, RequestID         string
}

// ProjectClosureView contains only current Creator-owned policy and generations.
type ProjectClosureView struct {
	Scope     ProjectCreationScope
	Project   domain.Project
	Authority writefence.Authority
}

// ProjectClosureAccess reads current scope/project policy without granting writes.
type ProjectClosureAccess interface {
	ReadProjectClosureView(context.Context, Identity, uuid.UUID, uuid.UUID) (ProjectClosureView, error)
}

// NativeProjectClosure keeps raw credentials at the true Identity source boundary.
type NativeProjectClosure struct {
	identity  SessionAuthorityReader
	access    ProjectClosureAccess
	admission *Admission
	writer    ProjectCreationWriter
	closures  *ProjectClosures
	store     ProjectClosureStore
}

// NewNativeProjectClosure requires actual authentication, source and final writer paths.
func NewNativeProjectClosure(identity SessionAuthorityReader, access ProjectClosureAccess, admission *Admission, writer ProjectCreationWriter, closures *ProjectClosures, store ProjectClosureStore) (*NativeProjectClosure, error) {
	if identity == nil || access == nil || admission == nil || writer == nil || closures == nil || store == nil {
		return nil, writefence.ErrInvalid
	}
	return &NativeProjectClosure{identity, access, admission, writer, closures, store}, nil
}

// ExecuteDetails returns only the original frozen complete response. It never
// fills missing historical receipt fields from today's mutable project state.
func (s *NativeProjectClosure) ExecuteDetails(ctx context.Context, credential Credential, scopeID uuid.UUID, input CloseProjectInput) (ProjectCloseDetails, error) {
	if _, err := s.Execute(ctx, credential, scopeID, input); err != nil {
		return ProjectCloseDetails{}, err
	}
	session, err := s.identity.ReadSourceSessionAuthorities(ctx, credential)
	if err != nil {
		return ProjectCloseDetails{}, err
	}
	id := uuid.NewSHA1(session.Identity.AccountID, []byte("creation.project.close/"+input.IdempotencyKey.String()))
	state, err := s.store.ReadProjectClosure(ctx, id)
	if err != nil {
		return ProjectCloseDetails{}, err
	}
	if state.Details == nil || state.Details.Validate() != nil || state.Spec.ActorID != session.Identity.AccountID || state.Spec.ScopeID != scopeID || state.Spec.ProjectID != input.ProjectID {
		return ProjectCloseDetails{}, ErrProjectClosureUnknown
	}
	return *state.Details, nil
}

// Execute derives authorities from actual Identity/Creator facts, never a caller actor.
func (s *NativeProjectClosure) Execute(ctx context.Context, credential Credential, scopeID uuid.UUID, input CloseProjectInput) (domain.Project, error) {
	requestID, err := uuid.Parse(input.RequestID)
	if s == nil || scopeID == uuid.Nil || input.ProjectID == uuid.Nil || input.IdempotencyKey == uuid.Nil || input.ExpectedRevision < 1 || input.ExpectedRevision >= 2147483647 || (input.Action != "archive" && input.Action != "delete") || err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID {
		return domain.Project{}, writefence.ErrInvalid
	}
	session, err := s.identity.ReadSourceSessionAuthorities(ctx, credential)
	if err != nil {
		return domain.Project{}, err
	}
	if session.Validate() != nil {
		return domain.Project{}, writefence.ErrConflict
	}
	view, err := s.access.ReadProjectClosureView(ctx, session.Identity, scopeID, input.ProjectID)
	if err != nil {
		return domain.Project{}, err
	}
	id := uuid.NewSHA1(session.Identity.AccountID, []byte("creation.project.close/"+input.IdempotencyKey.String()))
	original, err := s.store.ReadProjectClosure(ctx, id)
	var spec ProjectClosureSpec
	if err == nil {
		spec = original.Spec
		if spec.ActorID != session.Identity.AccountID || spec.ScopeID != scopeID || spec.ProjectID != input.ProjectID || spec.OrganizationID != view.Scope.OrganizationID || spec.Action != input.Action || spec.ExpectedRevision != input.ExpectedRevision || spec.IdempotencyKey != input.IdempotencyKey {
			return domain.Project{}, ErrIdempotencyConflict
		}
		if original.Result != nil {
			return *original.Result, nil
		}
		if original.Aborted {
			return domain.Project{}, writefence.ErrFenced
		}
		if spec.SessionID != session.Identity.SessionID {
			return domain.Project{}, writefence.ErrConflict
		}
	} else {
		if !errors.Is(err, writefence.ErrNotFound) {
			return domain.Project{}, err
		}
		if view.Project.IsDelete || view.Project.Status == domain.ProjectStatusCopying || (input.Action=="archive"&&view.Project.Status!=domain.ProjectStatusActive) {
			return domain.Project{}, domain.ErrProjectStateConflict
		}
		if view.Project.Revision != input.ExpectedRevision {
			return domain.Project{}, domain.ErrProjectRevisionConflict
		}
		spec = ProjectClosureSpec{ID: id, ActorID: session.Identity.AccountID, SessionID: session.Identity.SessionID, ScopeID: scopeID, OrganizationID: view.Scope.OrganizationID, ProjectID: input.ProjectID, RequestID: requestID, IdempotencyKey: input.IdempotencyKey, ExpectedRevision: input.ExpectedRevision, ProjectAuthority: view.Authority, CoordinationID: uuid.NewSHA1(id, []byte("creation.project.close.authority")), Action: input.Action}
		spec.ExpectedProjectStatus=view.Project.Status
		if view.Project.CoverAssetID != nil {
			cover := *view.Project.CoverAssetID
			spec.ExpectedCoverAssetID = &cover
		}
		spec.Command = writefence.Command{ID: uuid.NewSHA1(id, []byte("creation.project.close.command")), Action: "creation.project." + input.Action, Authorities: []writefence.Authority{session.Account, session.Session, view.Scope.Authority}}
		spec.Command.SHA256, err = spec.CommandSHA256()
		if err != nil || spec.Validate() != nil {
			return domain.Project{}, writefence.ErrInvalid
		}
	}
	permit, err := s.admission.Admit(ctx, credential, AdmissionRequest{Command: spec.Command, Target: writefence.WriterCreation, ScopeID: scopeID})
	if err != nil {
		return domain.Project{}, err
	}
	if permit.AccountID != spec.ActorID || permit.SessionID != spec.SessionID || permit.PhysicalScopeID != spec.OrganizationID {
		return domain.Project{}, writefence.ErrConflict
	}
	record, err := s.writer.Admit(ctx, spec.Command)
	if err != nil {
		return domain.Project{}, err
	}
	expected, _ := spec.Command.AuthorityDigest()
	actual, err := record.Command.AuthorityDigest()
	if err != nil || actual != expected || record.Command.SHA256 != spec.Command.SHA256 || record.Command.Action != spec.Command.Action || record.Command.ID != spec.Command.ID {
		return domain.Project{}, writefence.ErrConflict
	}
	if record.State != writefence.Admitted && record.State != writefence.Committed {
		return domain.Project{}, writefence.ErrFenced
	}
	return s.closures.Close(ctx, spec)
}
