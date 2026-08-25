package application

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/identity/domain"
)

var ErrNotFound = errors.New("record not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (e *Error) Error() string { return e.Message }

const (
	CodeInvalidRequest  = "invalid_request"
	CodeUnauthenticated = "unauthenticated"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeConflict        = "resource_conflict"
	CodeVersionConflict = "version_conflict"
	CodeStateConflict   = "state_conflict"
)

type Actor struct {
	UserID       string
	TokenVersion int
}

type Repository interface {
	FindVerificationByEmail(context.Context, string, bool) (domain.RegistrationVerification, error)
	FindVerificationByTicketDigest(context.Context, string, bool) (domain.RegistrationVerification, error)
	SaveVerification(context.Context, domain.RegistrationVerification) error
	CreateVerification(context.Context, domain.RegistrationVerification) error

	CreateAccount(context.Context, domain.User, domain.Workspace, domain.Membership) error
	FindUserByEmail(context.Context, string, bool) (domain.User, error)
	FindUserByID(context.Context, string, bool) (domain.User, error)
	SaveUser(context.Context, domain.User) error
	PrimaryWorkspace(context.Context, string) (domain.Workspace, domain.Membership, error)

	CreateSession(context.Context, domain.AuthSession) error
	FindSessionByDigest(context.Context, string, bool) (domain.AuthSession, error)
	SaveSession(context.Context, domain.AuthSession) error
	RevokeUserSessions(context.Context, string, time.Time) error

	ListWorkspaces(context.Context, string, bool) ([]WorkspaceMembership, error)
	FindWorkspaceForUser(context.Context, string, string, bool) (domain.Workspace, domain.Membership, error)
	CreateWorkspace(context.Context, domain.Workspace, domain.Membership) error
	SaveWorkspace(context.Context, domain.Workspace) error

	AppendAudit(context.Context, AuditEvent) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) bool
}

type TokenIssuer interface {
	Issue(string, int) (string, error)
}

type VerificationSender interface {
	Send(context.Context, string, string) (bool, error)
}

type WorkspaceMembership struct {
	Workspace  domain.Workspace
	Membership domain.Membership
}

type AuditEvent struct {
	WorkspaceID, ActorID, Action, TargetType, TargetID, TraceID string
	Metadata                                                    map[string]any
	OccurredAt                                                  time.Time
}

type UserView struct {
	ID, Email, DisplayName string
	AvatarURL              *string
}

type WorkspaceView struct {
	ID, Name, Status, Role string
	Revision               int
}

type MeView struct {
	User      UserView
	Workspace WorkspaceView
}

type AuthResult struct {
	Me           MeView
	AccessToken  string
	ExpiresIn    int
	RefreshToken string
}

type VerificationAccepted struct {
	EmailSent         bool
	RetryAfterSeconds int
}

type VerificationConfirmed struct {
	RegistrationTicket string
	ExpiresIn          int
}

type RegisterCommand struct{ Ticket, Password, DisplayName, TraceID string }
type LoginCommand struct{ Email, Password, TraceID string }
type ProfileCommand struct {
	DisplayName, AvatarURL       *string
	DisplayNameSet, AvatarURLSet bool
	TraceID                      string
}
type WorkspaceUpdateCommand struct {
	Name             string
	ExpectedRevision int
	TraceID          string
}
type WorkspaceStateCommand struct {
	ExpectedRevision int
	TraceID          string
}
