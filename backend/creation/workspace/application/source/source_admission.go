// Package source owns Creation's authenticated source policy and durable resource preparation.
package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Credential failures retain the exact authenticated public rejection category.
var (
	ErrSourceSessionInvalid = errors.New("creation source session invalid")
	ErrSourceAccessExpired  = errors.New("creation source access expired")
	ErrSourceCSRFInvalid    = errors.New("creation source csrf invalid")
	ErrSourceForbidden      = errors.New("creation source forbidden")
)

// Identity is the Identity authority's exact account/session reference. It
// contains no content role, organization, platform privilege or caller Principal.
type Identity struct {
	AccountID, SessionID uuid.UUID
	CredentialEpoch      int64
}

// Credential keeps original credentials private to the admission boundary.
// They are passed to Identity and never persisted by Creation.
type Credential struct{ access, legacy, csrf string }

// LegacyCredential carries an original legacy cookie and CSRF proof.
func LegacyCredential(token, csrf string) Credential {
	return Credential{legacy: token, csrf: csrf}
}

// AccessCredential carries an original access cookie and CSRF proof.
func AccessCredential(token, csrf string) Credential {
	return Credential{access: token, csrf: csrf}
}

// IdentityProof exposes credentials only for the protocol adapter's request.
func (c Credential) IdentityProof() (access, legacy, csrf string) {
	return c.access, c.legacy, c.csrf
}

// String redacts original credentials in ordinary formatting.
func (Credential) String() string { return "[private creation source credential]" }

// GoString also redacts original credentials when printing Go syntax.
func (Credential) GoString() string { return "[private creation source credential]" }

// IdentityAuthority is consumed before any Creation SQL transaction starts.
// Verify alone cannot supply a write permit; Admit must persist the exact command
// under Identity's current account and session policy locks.
type IdentityAuthority interface {
	VerifySourceIdentity(context.Context, Credential) (Identity, error)
	AdmitSourceSession(context.Context, Credential, writefence.Command, writefence.Writer) (Identity, error)
}

// AdmissionRequest binds one existing business command to its resource and
// effect owner. Epochs must already have been installed by their true issuers.
type AdmissionRequest struct {
	Command            writefence.Command
	Target             writefence.Writer
	ScopeID, ProjectID uuid.UUID
}

// Permit is source-side evidence, not a replacement for an effect owner's
// business authorization and final local Guard/Complete transaction.
type Permit struct {
	Command                                                   writefence.Command
	Target                                                    writefence.Writer
	AccountID, SessionID, ScopeID, ProjectID, PhysicalScopeID uuid.UUID
}

// CommandStore commits current Creation policy and command admission in
// the same local transaction. It cannot query Identity or call a remote owner.
type CommandStore interface {
	RegisterSourceCommand(context.Context, Identity, AdmissionRequest) (Permit, error)
}

// Admission preserves the fixed Identity -> Creation -> effect-owner order.
type Admission struct {
	identity IdentityAuthority
	store    CommandStore
}

// NewAdmission injects the private Identity authority and Creation store.
func NewAdmission(identity IdentityAuthority, store CommandStore) *Admission {
	return &Admission{identity: identity, store: store}
}

// Admit authenticates the original proof and persists both source admissions.
// No network operation runs inside the Creation policy transaction.
func (s *Admission) Admit(ctx context.Context, credential Credential, request AdmissionRequest) (Permit, error) {
	if s == nil || s.identity == nil || s.store == nil {
		return Permit{}, writefence.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return Permit{}, err
	}
	if request.Target != writefence.WriterCreation {
		return Permit{}, writefence.ErrInvalid
	}
	id, err := s.identity.VerifySourceIdentity(ctx, credential)
	if err != nil {
		return Permit{}, fmt.Errorf("verify creation source identity: %w", err)
	}
	if err := request.Validate(id); err != nil {
		return Permit{}, err
	}
	admitted, err := s.identity.AdmitSourceSession(ctx, credential, request.Command, writefence.WriterCreation)
	if err != nil {
		return Permit{}, fmt.Errorf("admit creation source session: %w", err)
	}
	if admitted != id {
		return Permit{}, writefence.ErrConflict
	}
	permit, err := s.store.RegisterSourceCommand(ctx, id, request)
	if err != nil {
		return Permit{}, fmt.Errorf("admit creation source policy: %w", err)
	}
	if !sourcePermitMatches(permit, id, request) {
		return Permit{}, writefence.ErrConflict
	}
	return permit, nil
}

// Validate rejects unbound authority sets and actions belonging to another owner.
// Agent grants have an additional budget/authorization contract and cannot enter
// through this direct user-command path.
func (r AdmissionRequest) Validate(id Identity) error {
	if id.AccountID == uuid.Nil || id.SessionID == uuid.Nil || id.CredentialEpoch < 1 || r.ScopeID == uuid.Nil {
		return writefence.ErrInvalid
	}
	if _, err := r.Command.AuthorityDigest(); err != nil {
		return err
	}
	if !sourceActionTarget(r.Command.Action, r.Target, r.ProjectID != uuid.Nil) {
		return writefence.ErrInvalid
	}
	var account, session, scope, project bool
	for _, a := range r.Command.Authorities {
		switch {
		case a.Issuer == writefence.Identity && a.Kind == writefence.Account && a.ID == id.AccountID:
			account = true
		case a.Issuer == writefence.Identity && a.Kind == writefence.Session && a.ID == id.SessionID:
			session = true
		case a.Issuer == writefence.Creation && a.Kind == writefence.Scope && a.ID == r.ScopeID:
			scope = true
		case a.Issuer == writefence.Creation && a.Kind == writefence.Project && r.ProjectID != uuid.Nil && a.ID == r.ProjectID:
			project = true
		default:
			return writefence.ErrInvalid
		}
	}
	if !account || !session || !scope || project != (r.ProjectID != uuid.Nil) {
		return writefence.ErrInvalid
	}
	return nil
}

func sourceActionTarget(action string, target writefence.Writer, project bool) bool {
	switch action {
	case "creation.project.create", "creation.independent_canvas.create", "creation.project.archive", "creation.project.delete":
		return target == writefence.WriterCreation && !project
	case "creation.project.patch", "creation.canvas.edit", "creation.script.edit", "creation.bible.edit", "creation.subject.edit", "creation.prompt.edit":
		return target == writefence.WriterCreation && project
	case "creation.frozen_input.freeze", "creation.frozen_input.cancel":
		return target == writefence.WriterExecution && project
	case "operation.quote.confirm", "operation.batch.confirm", "execution.frozen_input.accept":
		return target == writefence.WriterExecution && project
	case "media.upload.publish", "media.reference_pin.acquire":
		return target == writefence.WriterMedia && project
	default:
		return false
	}
}
