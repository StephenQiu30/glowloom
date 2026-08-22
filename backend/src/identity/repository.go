package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

type IdentityRepository struct {
	orm        *gorm.DB
	refreshTTL time.Duration
}

func NewIdentityRepository(orm *gorm.DB, refreshTTLs ...time.Duration) *IdentityRepository {
	refreshTTL := 30 * 24 * time.Hour
	if len(refreshTTLs) > 0 && refreshTTLs[0] > 0 {
		refreshTTL = refreshTTLs[0]
	}
	return &IdentityRepository{orm: orm, refreshTTL: refreshTTL}
}

type identityUserRecord struct {
	ID              uuid.UUID     `gorm:"column:id;type:uuid;primaryKey"`
	IdentitySubject string        `gorm:"column:identity_subject"`
	Email           string        `gorm:"column:email"`
	PasswordHash    string        `gorm:"column:password_hash"`
	DisplayName     string        `gorm:"column:display_name"`
	Status          AccountStatus `gorm:"column:status"`
	EmailVerifiedAt *time.Time    `gorm:"column:email_verified_at"`
	CreatedAt       time.Time     `gorm:"column:created_at"`
}

func (identityUserRecord) TableName() string { return "iam_users" }

type identityRoleRecord struct {
	ID    uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Code  RoleCode  `gorm:"column:code"`
	Scope RoleScope `gorm:"column:scope"`
}

func (identityRoleRecord) TableName() string { return "iam_roles" }

type identityMembershipRecord struct {
	ID          uuid.UUID        `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID        `gorm:"column:workspace_id;type:uuid"`
	UserID      uuid.UUID        `gorm:"column:user_id;type:uuid"`
	RoleID      uuid.UUID        `gorm:"column:role_id;type:uuid"`
	Status      MembershipStatus `gorm:"column:status"`
}

func (identityMembershipRecord) TableName() string { return "iam_memberships" }

type identitySessionRecord struct {
	ID          uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	FamilyID    uuid.UUID  `gorm:"column:family_id;type:uuid"`
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid"`
	WorkspaceID uuid.UUID  `gorm:"column:workspace_id;type:uuid"`
	TokenHash   string     `gorm:"column:token_hash"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	RevokedAt   *time.Time `gorm:"column:revoked_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at"`
}

func (identitySessionRecord) TableName() string { return "iam_sessions" }

type workspaceRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Name      string    `gorm:"column:name"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (workspaceRecord) TableName() string { return "workspaces" }

type workspaceMembershipRow struct {
	MembershipID  uuid.UUID `gorm:"column:membership_id"`
	WorkspaceID   uuid.UUID `gorm:"column:workspace_id"`
	WorkspaceName string    `gorm:"column:workspace_name"`
	Role          RoleCode  `gorm:"column:role"`
}

type workspaceProjectRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id;type:uuid"`
}

type workspaceResourceRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"column:project_id;type:uuid"`
}

func (r *IdentityRepository) RegisterAccount(ctx context.Context, input PersistedRegisterInput) (SessionIssue, error) {
	if r.orm == nil {
		return SessionIssue{}, fmt.Errorf("identity repository ORM is not configured")
	}
	refreshToken, err := newOpaqueToken()
	if err != nil {
		return SessionIssue{}, err
	}
	now := time.Now().UTC()
	workspaceID, userID, membershipID, sessionID, familyID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	expiresAt := now.Add(r.refreshTTL)
	var issue SessionIssue
	err = r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing identityUserRecord
		if err := tx.Where("email = ?", input.Email.String()).First(&existing).Error; err == nil {
			return ErrEmailRegistered
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("check account email: %w", err)
		}
		workspace := workspaceRecord{ID: workspaceID, Name: input.Workspace, CreatedAt: now}
		if err := tx.Create(&workspace).Error; err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
		if err := setWorkspaceConfig(tx, workspaceID); err != nil {
			return err
		}
		user := identityUserRecord{ID: userID, IdentitySubject: identitySubjectEmailPrefix + input.Email.String(), Email: input.Email.String(), PasswordHash: input.PasswordHash, DisplayName: input.DisplayName, Status: AccountStatusActive, CreatedAt: now}
		if err := tx.Create(&user).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrEmailRegistered
			}
			return fmt.Errorf("create account: %w", err)
		}
		var owner identityRoleRecord
		if err := tx.Where("code = ? AND scope = ?", RoleOwner, RoleScopeWorkspace).First(&owner).Error; err != nil {
			return fmt.Errorf("owner role is not seeded: %w", err)
		}
		membership := identityMembershipRecord{ID: membershipID, WorkspaceID: workspaceID, UserID: userID, RoleID: owner.ID, Status: MembershipStatusActive}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("create owner membership: %w", err)
		}
		session := identitySessionRecord{ID: sessionID, FamilyID: familyID, UserID: userID, WorkspaceID: workspaceID, TokenHash: hashToken(refreshToken), ExpiresAt: expiresAt, CreatedAt: now, LastUsedAt: &now}
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		issue = SessionIssue{SessionID: sessionID, FamilyID: familyID, Identity: AuthIdentity{Account: Account{ID: userID, Email: input.Email.String(), DisplayName: input.DisplayName}, Workspace: Workspace{ID: workspaceID, Name: input.Workspace}, MembershipID: membershipID, Role: RoleOwner}, RefreshToken: refreshToken, RefreshExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return SessionIssue{}, err
	}
	return issue, nil
}

func (r *IdentityRepository) FindLoginAccount(ctx context.Context, email EmailAddress, workspaceID uuid.UUID) (LoginAccount, error) {
	if r.orm == nil {
		return LoginAccount{}, fmt.Errorf("identity repository ORM is not configured")
	}
	var result LoginAccount
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		var user identityUserRecord
		if err := tx.Where("email = ? AND status = ?", email.String(), AccountStatusActive).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidCredentials
			}
			return fmt.Errorf("load login account: %w", err)
		}
		var membership workspaceMembershipRow
		if err := tx.Table("iam_memberships AS memberships").
			Select("memberships.id AS membership_id, memberships.workspace_id, workspaces.name AS workspace_name, roles.code AS role").
			Joins("JOIN workspaces ON workspaces.id = memberships.workspace_id").
			Joins("JOIN iam_roles AS roles ON roles.id = memberships.role_id").
			Where("memberships.user_id = ? AND memberships.workspace_id = ? AND memberships.status = ?", user.ID, workspaceID, MembershipStatusActive).
			First(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidCredentials
			}
			return fmt.Errorf("load login membership: %w", err)
		}
		result = LoginAccount{Identity: AuthIdentity{Account: Account{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName}, Workspace: Workspace{ID: membership.WorkspaceID, Name: membership.WorkspaceName}, MembershipID: membership.MembershipID, Role: membership.Role}, PasswordHash: user.PasswordHash}
		return nil
	})
	return result, err
}

func (r *IdentityRepository) CreateSession(ctx context.Context, identity AuthIdentity) (SessionIssue, error) {
	if r.orm == nil {
		return SessionIssue{}, fmt.Errorf("identity repository ORM is not configured")
	}
	refreshToken, err := newOpaqueToken()
	if err != nil {
		return SessionIssue{}, err
	}
	now := time.Now().UTC()
	session := identitySessionRecord{ID: uuid.New(), FamilyID: uuid.New(), UserID: identity.Account.ID, WorkspaceID: identity.Workspace.ID, TokenHash: hashToken(refreshToken), ExpiresAt: now.Add(r.refreshTTL), CreatedAt: now, LastUsedAt: &now}
	if err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		return nil
	}); err != nil {
		return SessionIssue{}, err
	}
	return SessionIssue{SessionID: session.ID, FamilyID: session.FamilyID, Identity: identity, RefreshToken: refreshToken, RefreshExpiresAt: session.ExpiresAt}, nil
}

func (r *IdentityRepository) RotateRefreshSession(ctx context.Context, workspaceID uuid.UUID, refreshToken string) (SessionIssue, error) {
	if r.orm == nil {
		return SessionIssue{}, fmt.Errorf("identity repository ORM is not configured")
	}
	if workspaceID == uuid.Nil {
		return SessionIssue{}, ErrRefreshInvalid
	}
	newRefreshToken, err := newOpaqueToken()
	if err != nil {
		return SessionIssue{}, err
	}
	now := time.Now().UTC()
	var issue SessionIssue
	err = database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		var current identitySessionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ? AND workspace_id = ?", hashToken(refreshToken), workspaceID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("load refresh session: %w", err)
		}
		if current.RevokedAt != nil {
			if err := tx.Model(&identitySessionRecord{}).Where("family_id = ? AND revoked_at IS NULL", current.FamilyID).Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error; err != nil {
				return fmt.Errorf("revoke refresh family after replay: %w", err)
			}
			return &RefreshReplayError{SessionID: current.ID, FamilyID: current.FamilyID}
		}
		if !current.ExpiresAt.After(now) {
			return ErrRefreshInvalid
		}
		identity, err := r.identityForSession(tx, current.UserID, current.WorkspaceID, current.ID)
		if err != nil {
			return err
		}
		if err := tx.Model(&current).Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error; err != nil {
			return fmt.Errorf("consume refresh session: %w", err)
		}
		next := identitySessionRecord{ID: uuid.New(), FamilyID: current.FamilyID, UserID: current.UserID, WorkspaceID: current.WorkspaceID, TokenHash: hashToken(newRefreshToken), ExpiresAt: now.Add(r.refreshTTL), CreatedAt: now, LastUsedAt: &now}
		if err := tx.Create(&next).Error; err != nil {
			return fmt.Errorf("create rotated refresh session: %w", err)
		}
		issue = SessionIssue{SessionID: next.ID, FamilyID: next.FamilyID, PreviousSessionID: current.ID, Identity: identity, RefreshToken: newRefreshToken, RefreshExpiresAt: next.ExpiresAt}
		return nil
	})
	if err != nil {
		return SessionIssue{}, err
	}
	return issue, nil
}

func (r *IdentityRepository) RevokeRefreshSession(ctx context.Context, workspaceID uuid.UUID, refreshToken string) (uuid.UUID, error) {
	if r.orm == nil {
		return uuid.Nil, fmt.Errorf("identity repository ORM is not configured")
	}
	var sessionID uuid.UUID
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		var session identitySessionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ? AND workspace_id = ?", hashToken(refreshToken), workspaceID).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("load logout session: %w", err)
		}
		sessionID = session.ID
		if session.RevokedAt != nil {
			return nil
		}
		now := time.Now().UTC()
		return tx.Model(&session).Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error
	})
	return sessionID, err
}

func (r *IdentityRepository) Authenticate(ctx context.Context, userID, sessionID, workspaceID uuid.UUID) (Principal, error) {
	if r.orm == nil {
		return Principal{}, fmt.Errorf("identity repository ORM is not configured")
	}
	var principal Principal
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		var session identitySessionRecord
		if err := tx.Where("id = ? AND user_id = ? AND workspace_id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, userID, workspaceID, time.Now().UTC()).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpapi.NotFound("会话")
			}
			return fmt.Errorf("authenticate session: %w", err)
		}
		identity, err := r.identityForSession(tx, userID, workspaceID, session.ID)
		if err != nil {
			return err
		}
		principal = Principal{UserID: identity.Account.ID, WorkspaceID: identity.Workspace.ID, MembershipID: identity.MembershipID, SessionID: session.ID, Role: identity.Role}
		return nil
	})
	return principal, err
}

func (r *IdentityRepository) identityForSession(tx *gorm.DB, userID, workspaceID, sessionID uuid.UUID) (AuthIdentity, error) {
	var user identityUserRecord
	if err := tx.Where("id = ? AND status = ?", userID, AccountStatusActive).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AuthIdentity{}, ErrRefreshInvalid
		}
		return AuthIdentity{}, fmt.Errorf("load session account: %w", err)
	}
	var membership workspaceMembershipRow
	if err := tx.Table("iam_memberships AS memberships").
		Select("memberships.id AS membership_id, memberships.workspace_id, workspaces.name AS workspace_name, roles.code AS role").
		Joins("JOIN workspaces ON workspaces.id = memberships.workspace_id").
		Joins("JOIN iam_roles AS roles ON roles.id = memberships.role_id").
		Where("memberships.user_id = ? AND memberships.workspace_id = ? AND memberships.status = ?", userID, workspaceID, MembershipStatusActive).
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AuthIdentity{}, ErrRefreshInvalid
		}
		return AuthIdentity{}, fmt.Errorf("load session membership: %w", err)
	}
	return AuthIdentity{Account: Account{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName}, Workspace: Workspace{ID: membership.WorkspaceID, Name: membership.WorkspaceName}, MembershipID: membership.MembershipID, Role: membership.Role}, nil
}

func (r *IdentityRepository) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, path string) error {
	if r.orm == nil {
		return fmt.Errorf("identity repository ORM is not configured")
	}
	return database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		for index, part := range parts {
			if index+1 >= len(parts) {
				continue
			}
			id, err := uuid.Parse(parts[index+1])
			if err != nil {
				continue
			}
			var owned bool
			switch part {
			case "workspaces":
				owned = id == workspaceID
			case "projects":
				var project workspaceProjectRecord
				err = tx.Where("id = ? AND workspace_id = ?", id, workspaceID).First(&project).Error
				owned = err == nil
			case "content-units":
				owned, err = r.resourceInWorkspace(tx, "prj_content_units", id, workspaceID)
			case "script-revisions":
				owned, err = r.resourceInWorkspace(tx, "script_revisions", id, workspaceID)
			case "operations":
				owned, err = r.resourceInWorkspace(tx, "operations", id, workspaceID)
			case "agent-runs":
				owned, err = r.resourceInWorkspace(tx, "m06_agent_runs", id, workspaceID)
			case "shots":
				owned, err = r.resourceInWorkspace(tx, "sht_shots", id, workspaceID)
			case "candidates":
				owned, err = r.resourceInWorkspace(tx, "media_candidates", id, workspaceID)
			case "generation-plans":
				owned, err = r.resourceInWorkspace(tx, "gen_plans", id, workspaceID)
			}
			if err != nil {
				return fmt.Errorf("authorize resource: %w", err)
			}
			if !owned && part != "" {
				switch part {
				case "workspaces", "projects", "content-units", "script-revisions", "operations", "agent-runs", "shots", "candidates", "generation-plans":
					return httpapi.NotFound("资源")
				}
			}
		}
		return nil
	})
}

func (r *IdentityRepository) resourceInWorkspace(tx *gorm.DB, table string, resourceID, workspaceID uuid.UUID) (bool, error) {
	var resource workspaceResourceRecord
	if err := tx.Table(table).Where("id = ?", resourceID).First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var project workspaceProjectRecord
	if err := tx.Where("id = ? AND workspace_id = ?", resource.ProjectID, workspaceID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func setWorkspaceConfig(tx *gorm.DB, workspaceID uuid.UUID) error {
	if err := tx.Exec("SELECT set_config(?, ?, true)", "app.workspace_id", workspaceID.String()).Error; err != nil {
		return fmt.Errorf("set workspace database context: %w", err)
	}
	return nil
}

func newOpaqueToken() (string, error) {
	return toolkit.RandomHexToken(32)
}

func hashToken(token string) string {
	return toolkit.SHA256String(token)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
