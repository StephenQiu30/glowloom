package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/access/identity/domain"
)

const (
	verificationTTL   = 10 * time.Minute
	ticketTTL         = 10 * time.Minute
	verificationRetry = 60 * time.Second
	maxCodeAttempts   = 5
)

type Config struct {
	AccessTokenTTL   time.Duration
	SessionTTL       time.Duration
	DigestSecret     string
	Now              func() time.Time
	NewID            func() string
	NewSecret        func() (string, error)
	VerificationCode func() string
}

type Service struct {
	transactions      TransactionManager
	hasher            PasswordHasher
	issuer            TokenIssuer
	sender            VerificationSender
	config            Config
	dummyPasswordHash string
}

func NewService(transactions TransactionManager, hasher PasswordHasher, issuer TokenIssuer, sender VerificationSender, config Config) (*Service, error) {
	dummyHash, err := hasher.Hash("not-a-real-lanverse-password")
	if err != nil {
		return nil, fmt.Errorf("prepare password verifier: %w", err)
	}
	return &Service{transactions: transactions, hasher: hasher, issuer: issuer, sender: sender, config: config, dummyPasswordHash: dummyHash}, nil
}

func (service *Service) RequestVerification(ctx context.Context, email string) (VerificationAccepted, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return VerificationAccepted{}, invalid("Invalid email")
	}
	code := service.config.VerificationCode()
	if len(code) != 6 {
		return VerificationAccepted{}, errors.New("verification code generator returned an invalid code")
	}
	now := service.config.Now().UTC()
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		verification, findErr := repo.FindVerificationByEmail(ctx, normalized, true)
		switch {
		case findErr == nil:
			verification.CodeDigest = service.digest("registration-code", normalized+":"+code)
			verification.AttemptCount = 0
			verification.Status = "pending"
			verification.ExpiresAt = now.Add(verificationTTL)
			verification.TicketDigest = nil
			verification.TicketExpiresAt = nil
			verification.UpdatedAt = now
			return repo.SaveVerification(ctx, verification)
		case errors.Is(findErr, ErrNotFound):
			return repo.CreateVerification(ctx, domain.RegistrationVerification{
				ID: service.config.NewID(), Email: normalized,
				CodeDigest: service.digest("registration-code", normalized+":"+code),
				Status:     "pending", ExpiresAt: now.Add(verificationTTL), CreatedAt: now, UpdatedAt: now,
			})
		default:
			return findErr
		}
	})
	if err != nil {
		return VerificationAccepted{}, err
	}
	emailSent, err := service.sender.Send(ctx, normalized, code)
	if err != nil {
		return VerificationAccepted{}, &Error{Code: "dependency_unavailable", Message: "Registration email service is unavailable", Status: 503, NextAction: "retry"}
	}
	return VerificationAccepted{EmailSent: emailSent, RetryAfterSeconds: int(verificationRetry.Seconds())}, nil
}

func (service *Service) ConfirmVerification(ctx context.Context, email, code string) (VerificationConfirmed, error) {
	normalized, err := normalizeEmail(email)
	if err != nil || len(code) != 6 {
		return VerificationConfirmed{}, invalid("Invalid verification code")
	}
	ticket, err := service.config.NewSecret()
	if err != nil {
		return VerificationConfirmed{}, err
	}
	now := service.config.Now().UTC()
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		verification, findErr := repo.FindVerificationByEmail(ctx, normalized, true)
		if findErr != nil {
			return invalid("Invalid verification code")
		}
		if verification.Status != "pending" || !verification.ExpiresAt.After(now) || verification.AttemptCount >= maxCodeAttempts {
			return invalid("Invalid verification code")
		}
		verification.AttemptCount++
		verification.UpdatedAt = now
		if !hmac.Equal([]byte(verification.CodeDigest), []byte(service.digest("registration-code", normalized+":"+code))) {
			return repo.SaveVerification(ctx, verification)
		}
		ticketDigest := service.digest("registration-ticket", ticket)
		ticketExpiresAt := now.Add(ticketTTL)
		verification.Status = "confirmed"
		verification.TicketDigest = &ticketDigest
		verification.TicketExpiresAt = &ticketExpiresAt
		return repo.SaveVerification(ctx, verification)
	})
	if err != nil {
		return VerificationConfirmed{}, err
	}
	// A mismatched code only increments the attempt counter; verify the resulting ticket state.
	var confirmed bool
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		verification, findErr := repo.FindVerificationByTicketDigest(ctx, service.digest("registration-ticket", ticket), false)
		confirmed = findErr == nil && verification.Status == "confirmed"
		if findErr != nil && !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		return nil
	})
	if err != nil {
		return VerificationConfirmed{}, err
	}
	if !confirmed {
		return VerificationConfirmed{}, invalid("Invalid verification code")
	}
	return VerificationConfirmed{RegistrationTicket: ticket, ExpiresIn: int(ticketTTL.Seconds())}, nil
}

func (service *Service) Register(ctx context.Context, command RegisterCommand) (AuthResult, error) {
	displayName := strings.TrimSpace(command.DisplayName)
	if displayName == "" || len(displayName) > 80 || len(command.Password) < 12 || len(command.Password) > 128 {
		return AuthResult{}, invalid("Invalid account profile")
	}
	passwordHash, err := service.hasher.Hash(command.Password)
	if err != nil {
		return AuthResult{}, err
	}
	refreshToken, err := service.config.NewSecret()
	if err != nil {
		return AuthResult{}, err
	}
	now := service.config.Now().UTC()
	var user domain.User
	var workspace domain.Workspace
	var membership domain.Membership
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		verification, findErr := repo.FindVerificationByTicketDigest(ctx, service.digest("registration-ticket", command.Ticket), true)
		if findErr != nil || verification.Status != "confirmed" || verification.TicketExpiresAt == nil || !verification.TicketExpiresAt.After(now) {
			return invalid("Registration ticket is invalid")
		}
		if _, findErr = repo.FindUserByEmail(ctx, verification.Email, false); findErr == nil {
			return &Error{Code: CodeConflict, Message: "Account already exists", Status: 409}
		} else if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		user = domain.User{ID: service.config.NewID(), Email: verification.Email, PasswordHash: passwordHash, TokenVersion: 1, DisplayName: displayName, Status: "active", LastLoginAt: &now, CreatedAt: now, UpdatedAt: now}
		workspace = domain.Workspace{ID: service.config.NewID(), Name: displayName + "的工作空间", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}
		membership = domain.Membership{ID: service.config.NewID(), WorkspaceID: workspace.ID, UserID: user.ID, Role: "owner", Status: "active", JoinedAt: now}
		if createErr := repo.CreateAccount(ctx, user, workspace, membership); createErr != nil {
			return createErr
		}
		verification.Status = "consumed"
		verification.UpdatedAt = now
		if saveErr := repo.SaveVerification(ctx, verification); saveErr != nil {
			return saveErr
		}
		if createErr := service.createSession(ctx, repo, user, refreshToken, now); createErr != nil {
			return createErr
		}
		return repo.AppendAudit(ctx, audit(workspace.ID, user.ID, "identity.registered", "user_account", user.ID, command.TraceID, now, map[string]any{"token_version": 1, "workspace_revision": 1}))
	})
	if err != nil {
		return AuthResult{}, err
	}
	return service.authResult(user, workspace, membership, refreshToken)
}

func (service *Service) Login(ctx context.Context, command LoginCommand) (AuthResult, error) {
	email, err := normalizeEmail(command.Email)
	if err != nil {
		return AuthResult{}, unauthenticated()
	}
	refreshToken, err := service.config.NewSecret()
	if err != nil {
		return AuthResult{}, err
	}
	now := service.config.Now().UTC()
	var user domain.User
	var workspace domain.Workspace
	var membership domain.Membership
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		found, findErr := repo.FindUserByEmail(ctx, email, true)
		candidateHash := service.dummyPasswordHash
		if findErr == nil {
			candidateHash = found.PasswordHash
		}
		valid := service.hasher.Verify(candidateHash, command.Password)
		if findErr != nil || !valid || found.Status != "active" {
			return unauthenticated()
		}
		user = found
		workspace, membership, findErr = repo.PrimaryWorkspace(ctx, user.ID)
		if findErr != nil {
			return errors.New("account workspace is unavailable")
		}
		user.LastLoginAt = &now
		user.UpdatedAt = now
		if saveErr := repo.SaveUser(ctx, user); saveErr != nil {
			return saveErr
		}
		if createErr := service.createSession(ctx, repo, user, refreshToken, now); createErr != nil {
			return createErr
		}
		return repo.AppendAudit(ctx, audit(workspace.ID, user.ID, "identity.login_succeeded", "user_account", user.ID, command.TraceID, now, map[string]any{"token_version": user.TokenVersion}))
	})
	if err != nil {
		return AuthResult{}, err
	}
	return service.authResult(user, workspace, membership, refreshToken)
}

func (service *Service) Refresh(ctx context.Context, currentToken string) (AuthResult, error) {
	if len(currentToken) < 32 {
		return AuthResult{}, unauthenticated()
	}
	nextToken, err := service.config.NewSecret()
	if err != nil {
		return AuthResult{}, err
	}
	now := service.config.Now().UTC()
	var user domain.User
	var workspace domain.Workspace
	var membership domain.Membership
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		session, findErr := repo.FindSessionByDigest(ctx, service.digest("refresh-session", currentToken), true)
		if findErr != nil || session.RevokedAt != nil || !session.ExpiresAt.After(now) {
			return unauthenticated()
		}
		user, findErr = repo.FindUserByID(ctx, session.UserID, false)
		if findErr != nil || user.Status != "active" || user.TokenVersion != session.TokenVersion {
			return unauthenticated()
		}
		workspace, membership, findErr = repo.PrimaryWorkspace(ctx, user.ID)
		if findErr != nil {
			return errors.New("account workspace is unavailable")
		}
		session.RevokedAt = &now
		session.UpdatedAt = now
		if saveErr := repo.SaveSession(ctx, session); saveErr != nil {
			return saveErr
		}
		return service.createSession(ctx, repo, user, nextToken, now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	return service.authResult(user, workspace, membership, nextToken)
}

func (service *Service) Me(ctx context.Context, actor Actor) (MeView, error) {
	var user domain.User
	var workspace domain.Workspace
	var membership domain.Membership
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		user, err = service.authenticated(ctx, repo, actor, false)
		if err != nil {
			return err
		}
		workspace, membership, err = repo.PrimaryWorkspace(ctx, user.ID)
		return err
	})
	if err != nil {
		return MeView{}, err
	}
	return presentMe(user, workspace, membership), nil
}

func (service *Service) UpdateProfile(ctx context.Context, actor Actor, command ProfileCommand) (MeView, error) {
	if !command.DisplayNameSet && !command.AvatarURLSet {
		return MeView{}, invalid("No profile changes supplied")
	}
	now := service.config.Now().UTC()
	var user domain.User
	var workspace domain.Workspace
	var membership domain.Membership
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		user, err = service.authenticated(ctx, repo, actor, true)
		if err != nil {
			return err
		}
		workspace, membership, err = repo.PrimaryWorkspace(ctx, user.ID)
		if err != nil {
			return err
		}
		changed := make([]string, 0, 2)
		if command.DisplayNameSet {
			if command.DisplayName == nil || strings.TrimSpace(*command.DisplayName) == "" || len(strings.TrimSpace(*command.DisplayName)) > 80 {
				return invalid("Invalid display name")
			}
			user.DisplayName = strings.TrimSpace(*command.DisplayName)
			changed = append(changed, "display_name")
		}
		if command.AvatarURLSet {
			user.AvatarURL = command.AvatarURL
			changed = append(changed, "avatar_url")
		}
		user.UpdatedAt = now
		if err = repo.SaveUser(ctx, user); err != nil {
			return err
		}
		return repo.AppendAudit(ctx, audit(workspace.ID, user.ID, "identity.profile_updated", "user_account", user.ID, command.TraceID, now, map[string]any{"changed_fields": changed}))
	})
	if err != nil {
		return MeView{}, err
	}
	return presentMe(user, workspace, membership), nil
}

func (service *Service) Logout(ctx context.Context, actor Actor, traceID string) error {
	return service.revokeAccountSessions(ctx, actor, traceID, "identity.logged_out", false, nil, "")
}

func (service *Service) ChangePassword(ctx context.Context, actor Actor, currentPassword, newPassword, traceID string) error {
	if len(newPassword) < 12 || len(newPassword) > 128 {
		return invalid("Password must contain between 12 and 128 characters")
	}
	hash, err := service.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	return service.revokeAccountSessions(ctx, actor, traceID, "identity.password_changed", false, &hash, currentPassword)
}

func (service *Service) Deactivate(ctx context.Context, actor Actor, traceID string) error {
	return service.revokeAccountSessions(ctx, actor, traceID, "identity.account_deactivated", true, nil, "")
}

func (service *Service) revokeAccountSessions(ctx context.Context, actor Actor, traceID, action string, deactivate bool, passwordHash *string, currentPassword string) error {
	now := service.config.Now().UTC()
	return service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		user, err := service.authenticated(ctx, repo, actor, true)
		if err != nil {
			return err
		}
		workspace, _, err := repo.PrimaryWorkspace(ctx, user.ID)
		if err != nil {
			return err
		}
		if passwordHash != nil {
			if !service.hasher.Verify(user.PasswordHash, currentPassword) {
				return unauthenticated()
			}
			user.PasswordHash = *passwordHash
		}
		previousVersion := user.TokenVersion
		user.TokenVersion++
		if deactivate {
			user.Status = "deactivated"
		}
		user.UpdatedAt = now
		if err = repo.SaveUser(ctx, user); err != nil {
			return err
		}
		if err = repo.RevokeUserSessions(ctx, user.ID, now); err != nil {
			return err
		}
		return repo.AppendAudit(ctx, audit(workspace.ID, user.ID, action, "user_account", user.ID, traceID, now, map[string]any{"previous_token_version": previousVersion, "token_version": user.TokenVersion, "status": user.Status}))
	})
}

func (service *Service) ListWorkspaces(ctx context.Context, actor Actor, includeArchived bool) ([]WorkspaceView, error) {
	var result []WorkspaceMembership
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		user, err := service.authenticated(ctx, repo, actor, false)
		if err != nil {
			return err
		}
		result, err = repo.ListWorkspaces(ctx, user.ID, includeArchived)
		return err
	})
	if err != nil {
		return nil, err
	}
	views := make([]WorkspaceView, len(result))
	for index, item := range result {
		views[index] = presentWorkspace(item.Workspace, item.Membership)
	}
	return views, nil
}

func (service *Service) GetWorkspace(ctx context.Context, actor Actor, workspaceID string) (WorkspaceView, error) {
	var workspace domain.Workspace
	var membership domain.Membership
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		user, err := service.authenticated(ctx, repo, actor, false)
		if err != nil {
			return err
		}
		workspace, membership, err = repo.FindWorkspaceForUser(ctx, user.ID, workspaceID, false)
		if errors.Is(err, ErrNotFound) {
			return notFound("Workspace not found")
		}
		return err
	})
	if err != nil {
		return WorkspaceView{}, err
	}
	return presentWorkspace(workspace, membership), nil
}

func (service *Service) CreateWorkspace(ctx context.Context, actor Actor, name, traceID string) (WorkspaceView, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return WorkspaceView{}, invalid("Invalid workspace name")
	}
	now := service.config.Now().UTC()
	var workspace domain.Workspace
	var membership domain.Membership
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		user, err := service.authenticated(ctx, repo, actor, false)
		if err != nil {
			return err
		}
		workspace = domain.Workspace{ID: service.config.NewID(), Name: name, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}
		membership = domain.Membership{ID: service.config.NewID(), WorkspaceID: workspace.ID, UserID: user.ID, Role: "owner", Status: "active", JoinedAt: now}
		if err = repo.CreateWorkspace(ctx, workspace, membership); err != nil {
			return err
		}
		return repo.AppendAudit(ctx, audit(workspace.ID, user.ID, "workspace.created", "workspace", workspace.ID, traceID, now, map[string]any{"revision": 1, "status": "active"}))
	})
	if err != nil {
		return WorkspaceView{}, err
	}
	return presentWorkspace(workspace, membership), nil
}

func (service *Service) UpdateWorkspace(ctx context.Context, actor Actor, workspaceID string, command WorkspaceUpdateCommand) (WorkspaceView, error) {
	name := strings.TrimSpace(command.Name)
	if name == "" || len(name) > 120 {
		return WorkspaceView{}, invalid("Invalid workspace name")
	}
	return service.changeWorkspace(ctx, actor, workspaceID, command.ExpectedRevision, command.TraceID, func(workspace *domain.Workspace, now time.Time) (string, error) {
		if workspace.Status != "active" {
			return "", stateConflict("Workspace is archived")
		}
		workspace.Name = name
		return "workspace.updated", nil
	})
}

func (service *Service) SetWorkspaceArchived(ctx context.Context, actor Actor, workspaceID string, command WorkspaceStateCommand, archived bool) (WorkspaceView, error) {
	return service.changeWorkspace(ctx, actor, workspaceID, command.ExpectedRevision, command.TraceID, func(workspace *domain.Workspace, now time.Time) (string, error) {
		expected := "active"
		target := "archived"
		action := "workspace.archived"
		if !archived {
			expected, target, action = "archived", "active", "workspace.restored"
		}
		if workspace.Status != expected {
			return "", stateConflict("Workspace state does not allow this action")
		}
		workspace.Status = target
		if archived {
			workspace.ArchivedAt = &now
		} else {
			workspace.ArchivedAt = nil
		}
		return action, nil
	})
}

func (service *Service) changeWorkspace(ctx context.Context, actor Actor, workspaceID string, expectedRevision int, traceID string, change func(*domain.Workspace, time.Time) (string, error)) (WorkspaceView, error) {
	now := service.config.Now().UTC()
	var workspace domain.Workspace
	var membership domain.Membership
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		user, err := service.authenticated(ctx, repo, actor, false)
		if err != nil {
			return err
		}
		workspace, membership, err = repo.FindWorkspaceForUser(ctx, user.ID, workspaceID, true)
		if errors.Is(err, ErrNotFound) {
			return notFound("Workspace not found")
		}
		if err != nil {
			return err
		}
		if membership.Role != "owner" {
			return &Error{Code: CodeForbidden, Message: "Action is not allowed", Status: 403}
		}
		if workspace.Revision != expectedRevision {
			return &Error{Code: CodeVersionConflict, Message: "Workspace has changed", Status: 409, Details: map[string]any{"current_revision": workspace.Revision}}
		}
		action, err := change(&workspace, now)
		if err != nil {
			return err
		}
		workspace.Revision++
		workspace.UpdatedAt = now
		if err = repo.SaveWorkspace(ctx, workspace); err != nil {
			return err
		}
		return repo.AppendAudit(ctx, audit(workspace.ID, user.ID, action, "workspace", workspace.ID, traceID, now, map[string]any{"revision": workspace.Revision, "status": workspace.Status}))
	})
	if err != nil {
		return WorkspaceView{}, err
	}
	return presentWorkspace(workspace, membership), nil
}

func (service *Service) authenticated(ctx context.Context, repo Repository, actor Actor, lock bool) (domain.User, error) {
	user, err := repo.FindUserByID(ctx, actor.UserID, lock)
	if err != nil || user.Status != "active" || user.TokenVersion != actor.TokenVersion {
		return domain.User{}, unauthenticated()
	}
	return user, nil
}

func (service *Service) createSession(ctx context.Context, repo Repository, user domain.User, rawToken string, now time.Time) error {
	return repo.CreateSession(ctx, domain.AuthSession{ID: service.config.NewID(), UserID: user.ID, TokenDigest: service.digest("refresh-session", rawToken), TokenVersion: user.TokenVersion, ExpiresAt: now.Add(service.config.SessionTTL), CreatedAt: now, UpdatedAt: now})
}

func (service *Service) authResult(user domain.User, workspace domain.Workspace, membership domain.Membership, refreshToken string) (AuthResult, error) {
	accessToken, err := service.issuer.Issue(user.ID, user.TokenVersion)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Me: presentMe(user, workspace, membership), AccessToken: accessToken, ExpiresIn: int(service.config.AccessTokenTTL.Seconds()), RefreshToken: refreshToken}, nil
}

func (service *Service) digest(scope, value string) string {
	mac := hmac.New(sha256.New, []byte(service.config.DigestSecret))
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized || len(normalized) > 320 {
		return "", errors.New("invalid email")
	}
	return normalized, nil
}

func presentMe(user domain.User, workspace domain.Workspace, membership domain.Membership) MeView {
	return MeView{User: UserView{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, AvatarURL: user.AvatarURL}, Workspace: presentWorkspace(workspace, membership)}
}

func presentWorkspace(workspace domain.Workspace, membership domain.Membership) WorkspaceView {
	return WorkspaceView{ID: workspace.ID, Name: workspace.Name, Status: workspace.Status, Role: membership.Role, Revision: workspace.Revision}
}

func audit(workspaceID, actorID, action, targetType, targetID, traceID string, occurredAt time.Time, metadata map[string]any) AuditEvent {
	return AuditEvent{WorkspaceID: workspaceID, ActorID: actorID, Action: action, TargetType: targetType, TargetID: targetID, TraceID: traceID, Metadata: metadata, OccurredAt: occurredAt}
}

func invalid(message string) error {
	return &Error{Code: CodeInvalidRequest, Message: message, Status: 422}
}
func unauthenticated() error {
	return &Error{Code: CodeUnauthenticated, Message: "Invalid credentials", Status: 401, NextAction: "login"}
}
func notFound(message string) error { return &Error{Code: CodeNotFound, Message: message, Status: 404} }
func stateConflict(message string) error {
	return &Error{Code: CodeStateConflict, Message: message, Status: 409}
}
