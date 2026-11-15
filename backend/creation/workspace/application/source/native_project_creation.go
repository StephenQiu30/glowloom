package source

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// SessionAuthorities is a current read snapshot from Identity, not an
// admission. Credential epoch is deliberately independent from both head epochs.
type SessionAuthorities struct {
	Identity Identity
	Account  writefence.Authority
	Session  writefence.Authority
}

// Validate binds both generations to the actual authenticated account/session.
func (s SessionAuthorities) Validate() error {
	if s.Identity.AccountID == uuid.Nil || s.Identity.SessionID == uuid.Nil || s.Identity.CredentialEpoch < 1 || s.Account.Validate() != nil || s.Session.Validate() != nil || s.Account.Issuer != writefence.Identity || s.Account.Kind != writefence.Account || s.Account.ID != s.Identity.AccountID || s.Session.Issuer != writefence.Identity || s.Session.Kind != writefence.Session || s.Session.ID != s.Identity.SessionID {
		return writefence.ErrInvalid
	}
	return nil
}

// SessionAuthorityReader validates the original current credential and
// carried CSRF proof without touching activity or registering any write command.
type SessionAuthorityReader interface {
	ReadSourceSessionAuthorities(context.Context, Credential) (SessionAuthorities, error)
}

// ProjectCreationScope contains only actual local scope policy and current head.
type ProjectCreationScope struct {
	ScopeID        uuid.UUID
	OrganizationID uuid.UUID
	Revision       int64
	Authority      writefence.Authority
}

// ProjectCreationLookup retains the original accepted specification or response.
// A response may predate the owner split; it never creates a synthetic preparation.
type ProjectCreationLookup struct {
	Spec       *ProjectPreparationSpec
	Activation *CreatedProject
}

// ProjectCreationSource reads actual policy and an original public idempotency
// receipt. Both methods lock current local policy; no Identity tables are read.
type ProjectCreationSource interface {
	ReadProjectCreationScope(context.Context, uuid.UUID, uuid.UUID) (ProjectCreationScope, error)
	LookupProjectCreation(context.Context, Identity, ProjectCreationScope, uuid.UUID, string) (ProjectCreationLookup, error)
}

// ProjectCreationWriter admits only the already source-authorized local command.
type ProjectCreationWriter interface {
	Admit(context.Context, writefence.Command) (writefence.Record, error)
}

// NativeProjectCreation uses real Identity -> Creation admission, hidden durable
// source facts and the zero-credit participant. Its public result is active only.
type NativeProjectCreation struct {
	identity     SessionAuthorityReader
	source       ProjectCreationSource
	admission    *Admission
	writer       ProjectCreationWriter
	preparations *ProjectPreparations
}

// NewNativeProjectCreation requires every actual authority/participant boundary.
func NewNativeProjectCreation(identity SessionAuthorityReader, source ProjectCreationSource, admission *Admission, writer ProjectCreationWriter, preparations *ProjectPreparations) (*NativeProjectCreation, error) {
	if identity == nil || source == nil || admission == nil || writer == nil || preparations == nil {
		return nil, ErrProjectPreparationConflict
	}
	return &NativeProjectCreation{identity, source, admission, writer, preparations}, nil
}

// Execute requires the public transport's original Cookie/Origin/CSRF policy.
// It additionally validates the carried CSRF through Identity even on a replay.
// Neither an actor, a role nor caller-selected authority epochs are accepted.
func (s *NativeProjectCreation) Execute(ctx context.Context, credential Credential, scopeID, key uuid.UUID, input CreateProjectInput) (CreatedProject, error) {
	requestID, err := uuid.Parse(input.RequestID)
	if s == nil || scopeID == uuid.Nil || key == uuid.Nil || err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID {
		return CreatedProject{}, ErrInvalidCreateProject
	}
	session, err := s.identity.ReadSourceSessionAuthorities(ctx, credential)
	if err != nil {
		return CreatedProject{}, err
	}
	if session.Validate() != nil {
		return CreatedProject{}, writefence.ErrConflict
	}
	scope, err := s.source.ReadProjectCreationScope(ctx, session.Identity.AccountID, scopeID)
	if err != nil {
		return CreatedProject{}, err
	}
	if scope.ScopeID != scopeID || scope.OrganizationID == uuid.Nil || scope.Revision < 1 || scope.Authority.Validate() != nil || scope.Authority.Issuer != writefence.Creation || scope.Authority.Kind != writefence.Scope || scope.Authority.ID != scopeID {
		return CreatedProject{}, writefence.ErrConflict
	}
	fingerprint, err := ProjectCreationSHA256(scope.OrganizationID, scopeID, input)
	if err != nil {
		return CreatedProject{}, err
	}
	original, err := s.source.LookupProjectCreation(ctx, session.Identity, scope, key, fingerprint)
	if err != nil && !errors.Is(err, ErrProjectPreparationUnknown) {
		return CreatedProject{}, err
	}
	if err == nil && original.Activation != nil {
		return *original.Activation, nil
	}
	var spec ProjectPreparationSpec
	if err == nil {
		if original.Spec == nil || original.Spec.Validate() != nil || original.Spec.ActorID != session.Identity.AccountID || original.Spec.SessionID != session.Identity.SessionID || original.Spec.ScopeID != scopeID || original.Spec.Project.OrgID != scope.OrganizationID || original.Spec.IdempotencyKey != key || original.Spec.InputSHA256 != fingerprint {
			return CreatedProject{}, ErrProjectPreparationConflict
		}
		spec = *original.Spec
	} else {
		spec, err = BuildProjectPreparation(session, scope, key, input)
		if err != nil {
			return CreatedProject{}, err
		}
	}
	request := AdmissionRequest{Command: spec.Command, Target: writefence.WriterCreation, ScopeID: scopeID}
	permit, err := s.admission.Admit(ctx, credential, request)
	if err != nil {
		return CreatedProject{}, err
	}
	if permit.AccountID != spec.ActorID || permit.SessionID != spec.SessionID || permit.PhysicalScopeID != spec.Project.OrgID {
		return CreatedProject{}, writefence.ErrConflict
	}
	record, err := s.writer.Admit(ctx, spec.Command)
	if err != nil {
		return CreatedProject{}, err
	}
	binding, bindingErr := record.Command.AuthorityDigest()
	expectedBinding, _ := spec.Command.AuthorityDigest()
	if bindingErr != nil || binding != expectedBinding || record.Command.ID != spec.Command.ID || record.Command.SHA256 != spec.Command.SHA256 || record.Command.Action != spec.Command.Action {
		return CreatedProject{}, writefence.ErrConflict
	}
	if record.State != writefence.Admitted && record.State != writefence.Committed {
		return CreatedProject{}, writefence.ErrFenced
	}
	return s.preparations.Prepare(ctx, spec)
}

// BuildProjectPreparation reuses the original public fingerprint and project
// validation. Stable IDs are scoped to account + public idempotency key, never
// guessed identity credential epochs or a caller-provided project identifier.
func BuildProjectPreparation(session SessionAuthorities, scope ProjectCreationScope, key uuid.UUID, input CreateProjectInput) (ProjectPreparationSpec, error) {
	requestID, err := uuid.Parse(input.RequestID)
	if session.Validate() != nil || scope.ScopeID == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.Revision < 1 || scope.Authority.Validate() != nil || scope.Authority.Issuer != writefence.Creation || scope.Authority.Kind != writefence.Scope || scope.Authority.ID != scope.ScopeID || key == uuid.Nil || err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID || !utf8.ValidString(input.Name) || !utf8.ValidString(input.Description) {
		return ProjectPreparationSpec{}, ErrInvalidCreateProject
	}
	fingerprint, err := ProjectCreationSHA256(scope.OrganizationID, scope.ScopeID, input)
	if err != nil {
		return ProjectPreparationSpec{}, err
	}
	id := uuid.NewSHA1(session.Identity.AccountID, []byte("creation.project.preparation/"+key.String()))
	var styleKey *string
	if input.StyleKey != nil {
		value := *input.StyleKey
		styleKey = &value
	}
	style := input.StyleType
	if style == "" {
		style = "realistic"
	}
	project := domain.Project{ID: uuid.NewSHA1(id, []byte("creation.project")), CanvasID: uuid.NewSHA1(id, []byte("creation.canvas")), OrgID: scope.OrganizationID, OwnerID: session.Identity.AccountID, IndependentCanvas: input.IndependentCanvas, StyleKey: styleKey, Name: strings.TrimSpace(input.Name), Description: input.Description, AspectRatio: input.AspectRatio, StyleType: style, StyleSubtype: input.StyleSubtype, StylePresetID: input.StylePresetID, Resolution: "1080p", Status: domain.ProjectStatusCopying, Revision: 1}
	if project.Validate() != nil {
		return ProjectPreparationSpec{}, ErrInvalidCreateProject
	}
	spec := ProjectPreparationSpec{PreparationID: id, ActorID: session.Identity.AccountID, SessionID: session.Identity.SessionID, IdempotencyKey: key, RequestID: requestID, ScopeID: scope.ScopeID, ScopeRevision: scope.Revision, InputSHA256: fingerprint, Project: project, Command: writefence.Command{ID: uuid.NewSHA1(id, []byte("creation.project.create")), Action: "creation.project.create", Authorities: []writefence.Authority{session.Account, session.Session, scope.Authority}}}
	if input.IndependentCanvas {
		spec.Command.Action = "creation.independent_canvas.create"
	}
	spec.Command.SHA256, err = spec.CommandSHA256()
	if err != nil || spec.Validate() != nil {
		return ProjectPreparationSpec{}, ErrInvalidCreateProject
	}
	return spec, nil
}
