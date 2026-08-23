package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
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

type membershipAuditState struct {
	Role   RoleCode         `json:"role"`
	Status MembershipStatus `json:"status"`
}

type identityAuditEventRecord struct {
	ID          uuid.UUID      `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID      `gorm:"column:workspace_id;type:uuid"`
	ActorType   string         `gorm:"column:actor_type"`
	ActorID     string         `gorm:"column:actor_id"`
	Action      string         `gorm:"column:action"`
	ObjectType  string         `gorm:"column:object_type"`
	ObjectID    uuid.UUID      `gorm:"column:object_id;type:uuid"`
	BeforeState datatypes.JSON `gorm:"column:before_state;type:jsonb"`
	AfterState  datatypes.JSON `gorm:"column:after_state;type:jsonb"`
	BeforeHash  string         `gorm:"column:before_hash"`
	AfterHash   string         `gorm:"column:after_hash"`
	RequestID   string         `gorm:"column:request_id"`
	Reason      string         `gorm:"column:reason"`
	Result      string         `gorm:"column:result"`
	OccurredAt  time.Time      `gorm:"column:occurred_at"`
}

func (identityAuditEventRecord) TableName() string { return "audit_events" }

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

type workspaceMemberRow struct {
	MembershipID     uuid.UUID        `gorm:"column:membership_id"`
	UserID           uuid.UUID        `gorm:"column:user_id"`
	RoleID           uuid.UUID        `gorm:"column:role_id"`
	Email            string           `gorm:"column:email"`
	DisplayName      string           `gorm:"column:display_name"`
	AccountStatus    AccountStatus    `gorm:"column:account_status"`
	MembershipStatus MembershipStatus `gorm:"column:membership_status"`
	Role             RoleCode         `gorm:"column:role"`
	CreatedAt        time.Time        `gorm:"column:created_at"`
}

type accessAuditRow struct {
	ID                uuid.UUID         `gorm:"column:id"`
	WorkspaceID       uuid.UUID         `gorm:"column:workspace_id"`
	ActorType         string            `gorm:"column:actor_type"`
	ActorID           string            `gorm:"column:actor_id"`
	ActorDisplayName  string            `gorm:"column:actor_display_name"`
	ActorEmail        string            `gorm:"column:actor_email"`
	Action            string            `gorm:"column:action"`
	ObjectType        string            `gorm:"column:object_type"`
	ObjectID          uuid.UUID         `gorm:"column:object_id"`
	ObjectDisplayName string            `gorm:"column:object_display_name"`
	ObjectEmail       string            `gorm:"column:object_email"`
	BeforeState       datatypes.JSON    `gorm:"column:before_state"`
	AfterState        datatypes.JSON    `gorm:"column:after_state"`
	BeforeHash        string            `gorm:"column:before_hash"`
	AfterHash         string            `gorm:"column:after_hash"`
	RequestID         string            `gorm:"column:request_id"`
	Reason            string            `gorm:"column:reason"`
	Result            AccessAuditResult `gorm:"column:result"`
	OccurredAt        time.Time         `gorm:"column:occurred_at"`
}

type workspaceProjectRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id;type:uuid"`
}

func (workspaceProjectRecord) TableName() string { return "projects" }

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
		var admin identityRoleRecord
		if err := tx.Where("code = ? AND scope = ?", RoleAdmin, RoleScopeWorkspace).First(&admin).Error; err != nil {
			return fmt.Errorf("admin role is not seeded: %w", err)
		}
		membership := identityMembershipRecord{ID: membershipID, WorkspaceID: workspaceID, UserID: userID, RoleID: admin.ID, Status: MembershipStatusActive}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("create admin membership: %w", err)
		}
		session := identitySessionRecord{ID: sessionID, FamilyID: familyID, UserID: userID, WorkspaceID: workspaceID, TokenHash: hashToken(refreshToken), ExpiresAt: expiresAt, CreatedAt: now, LastUsedAt: &now}
		if err := tx.Create(&session).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		issue = SessionIssue{SessionID: sessionID, FamilyID: familyID, Identity: AuthIdentity{Account: Account{ID: userID, Email: input.Email.String(), DisplayName: input.DisplayName}, Workspace: Workspace{ID: workspaceID, Name: input.Workspace}, MembershipID: membershipID, Role: RoleAdmin}, RefreshToken: refreshToken, RefreshExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return SessionIssue{}, err
	}
	return issue, nil
}

func (r *IdentityRepository) FindLoginAccount(ctx context.Context, email EmailAddress) (LoginAccount, error) {
	if r.orm == nil {
		return LoginAccount{}, fmt.Errorf("identity repository ORM is not configured")
	}
	var result LoginAccount
	err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			Where("memberships.user_id = ? AND memberships.status = ?", user.ID, MembershipStatusActive).
			Order("CASE WHEN roles.code = 'admin' THEN 0 ELSE 1 END, memberships.created_at ASC").
			Take(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidCredentials
			}
			return fmt.Errorf("load login membership: %w", err)
		}
		if membership.Role == RoleBan {
			return ErrInvalidCredentials
		}
		if err := setWorkspaceConfig(tx, membership.WorkspaceID); err != nil {
			return err
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

func (r *IdentityRepository) RotateRefreshSession(ctx context.Context, refreshToken string) (SessionIssue, error) {
	if r.orm == nil {
		return SessionIssue{}, fmt.Errorf("identity repository ORM is not configured")
	}
	newRefreshToken, err := newOpaqueToken()
	if err != nil {
		return SessionIssue{}, err
	}
	now := time.Now().UTC()
	var issue SessionIssue
	err = r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current identitySessionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hashToken(refreshToken)).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("load refresh session: %w", err)
		}
		if err := setWorkspaceConfig(tx, current.WorkspaceID); err != nil {
			return err
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

func (r *IdentityRepository) RevokeRefreshSession(ctx context.Context, refreshToken string) (uuid.UUID, error) {
	if r.orm == nil {
		return uuid.Nil, fmt.Errorf("identity repository ORM is not configured")
	}
	var sessionID uuid.UUID
	err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session identitySessionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hashToken(refreshToken)).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRefreshInvalid
			}
			return fmt.Errorf("load logout session: %w", err)
		}
		if err := setWorkspaceConfig(tx, session.WorkspaceID); err != nil {
			return err
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

func (r *IdentityRepository) ListWorkspaceMembers(ctx context.Context, workspaceID uuid.UUID, query WorkspaceMemberQuery) (WorkspaceMemberPage, error) {
	if r.orm == nil {
		return WorkspaceMemberPage{}, fmt.Errorf("identity repository ORM is not configured")
	}
	if workspaceID == uuid.Nil {
		return WorkspaceMemberPage{}, httpapi.Validation("Workspace 无效", "提供有效 Workspace 后重试")
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	search := strings.TrimSpace(query.Search)
	var result WorkspaceMemberPage
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		base := tx.Table("iam_memberships AS memberships").
			Joins("JOIN iam_users AS users ON users.id = memberships.user_id").
			Joins("JOIN iam_roles AS roles ON roles.id = memberships.role_id").
			Where("memberships.workspace_id = ?", workspaceID)
		if search != "" {
			pattern := "%" + strings.ToLower(search) + "%"
			base = base.Where("LOWER(users.email) LIKE ? OR LOWER(users.display_name) LIKE ?", pattern, pattern)
		}
		var total int64
		if err := base.Count(&total).Error; err != nil {
			return fmt.Errorf("count workspace members: %w", err)
		}
		var rows []workspaceMemberRow
		if err := base.Select("memberships.id AS membership_id, memberships.user_id, memberships.role_id, users.email, users.display_name, users.status AS account_status, memberships.status AS membership_status, roles.code AS role, memberships.created_at").
			Order("memberships.created_at DESC, memberships.id").
			Offset((page - 1) * pageSize).
			Limit(pageSize).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("list workspace members: %w", err)
		}
		items := make([]WorkspaceMember, 0, len(rows))
		for _, row := range rows {
			items = append(items, workspaceMemberFromRow(row))
		}
		result = WorkspaceMemberPage{Items: items, Total: total, Page: page, PageSize: pageSize}
		return nil
	})
	return result, err
}

func (r *IdentityRepository) ListAccessAudit(ctx context.Context, workspaceID uuid.UUID, query AccessAuditQuery) (AccessAuditPage, error) {
	if r.orm == nil {
		return AccessAuditPage{}, fmt.Errorf("identity repository ORM is not configured")
	}
	if workspaceID == uuid.Nil {
		return AccessAuditPage{}, httpapi.Validation("Workspace 无效", "提供有效 Workspace 后重试")
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var result AccessAuditPage
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		base := tx.Table("audit_events AS events").
			Joins("LEFT JOIN iam_users AS actors ON events.actor_type = 'user' AND actors.id::text = events.actor_id").
			Joins("LEFT JOIN iam_memberships AS objects ON events.object_type = 'iam_membership' AND objects.id = events.object_id AND objects.workspace_id = events.workspace_id").
			Joins("LEFT JOIN iam_users AS object_users ON object_users.id = objects.user_id").
			Where("events.workspace_id = ?", workspaceID)
		if search := strings.TrimSpace(query.Search); search != "" {
			pattern := "%" + strings.ToLower(search) + "%"
			base = base.Where(`LOWER(COALESCE(actors.display_name, '')) LIKE ? OR LOWER(COALESCE(actors.email, '')) LIKE ? OR LOWER(events.actor_id) LIKE ? OR LOWER(events.action) LIKE ? OR LOWER(events.object_type) LIKE ? OR LOWER(events.object_id::text) LIKE ? OR LOWER(COALESCE(object_users.display_name, '')) LIKE ? OR LOWER(COALESCE(object_users.email, '')) LIKE ? OR LOWER(events.reason) LIKE ? OR LOWER(events.request_id) LIKE ?`, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern)
		}
		if actor := strings.TrimSpace(query.Actor); actor != "" {
			pattern := "%" + strings.ToLower(actor) + "%"
			base = base.Where("LOWER(COALESCE(actors.display_name, '')) LIKE ? OR LOWER(COALESCE(actors.email, '')) LIKE ? OR LOWER(events.actor_id) LIKE ?", pattern, pattern, pattern)
		}
		if object := strings.TrimSpace(query.Object); object != "" {
			pattern := "%" + strings.ToLower(object) + "%"
			base = base.Where("LOWER(events.object_type) LIKE ? OR LOWER(events.object_id::text) LIKE ? OR LOWER(COALESCE(object_users.display_name, '')) LIKE ? OR LOWER(COALESCE(object_users.email, '')) LIKE ?", pattern, pattern, pattern, pattern)
		}
		if action := strings.TrimSpace(query.Action); action != "" {
			base = base.Where("events.action = ?", action)
		}
		if query.Result != "" {
			base = base.Where("events.result = ?", query.Result)
		}
		if query.OccurredFrom != nil {
			base = base.Where("events.occurred_at >= ?", query.OccurredFrom.UTC())
		}
		if query.OccurredTo != nil {
			base = base.Where("events.occurred_at <= ?", query.OccurredTo.UTC())
		}
		var total int64
		if err := base.Count(&total).Error; err != nil {
			return fmt.Errorf("count access audit events: %w", err)
		}
		var rows []accessAuditRow
		if err := base.Select(`events.id, events.workspace_id, events.actor_type, events.actor_id,
			COALESCE(actors.display_name, '') AS actor_display_name, COALESCE(actors.email, '') AS actor_email,
			events.action, events.object_type, events.object_id,
			COALESCE(object_users.display_name, '') AS object_display_name, COALESCE(object_users.email, '') AS object_email,
			events.before_state, events.after_state, events.before_hash, events.after_hash,
			events.request_id, events.reason, events.result, events.occurred_at`).
			Order("events.occurred_at DESC, events.id DESC").
			Offset((page - 1) * pageSize).
			Limit(pageSize).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("list access audit events: %w", err)
		}
		items := make([]AccessAuditEvent, 0, len(rows))
		for _, row := range rows {
			beforeState, err := decodeAuditState(row.BeforeState)
			if err != nil {
				return fmt.Errorf("decode access audit baseline %s: %w", row.ID, err)
			}
			afterState, err := decodeAuditState(row.AfterState)
			if err != nil {
				return fmt.Errorf("decode access audit result %s: %w", row.ID, err)
			}
			items = append(items, AccessAuditEvent{
				ID: row.ID, WorkspaceID: row.WorkspaceID, ActorType: row.ActorType, ActorID: row.ActorID,
				ActorDisplayName: row.ActorDisplayName, ActorEmail: row.ActorEmail, Action: row.Action,
				ObjectType: row.ObjectType, ObjectID: row.ObjectID, ObjectDisplayName: row.ObjectDisplayName,
				ObjectEmail: row.ObjectEmail, BeforeState: beforeState, AfterState: afterState,
				BeforeHash: row.BeforeHash, AfterHash: row.AfterHash, RequestID: row.RequestID,
				Reason: row.Reason, Result: row.Result, OccurredAt: row.OccurredAt,
			})
		}
		result = AccessAuditPage{Items: items, Total: total, Page: page, PageSize: pageSize}
		return nil
	})
	return result, err
}

func decodeAuditState(value datatypes.JSON) (map[string]any, error) {
	state := make(map[string]any)
	if err := json.Unmarshal(value, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (r *IdentityRepository) UpdateWorkspaceMember(ctx context.Context, workspaceID, membershipID uuid.UUID, actor Principal, input WorkspaceMemberUpdate) (WorkspaceMember, error) {
	if r.orm == nil {
		return WorkspaceMember{}, fmt.Errorf("identity repository ORM is not configured")
	}
	if workspaceID == uuid.Nil || membershipID == uuid.Nil {
		return WorkspaceMember{}, httpapi.Validation("Workspace 或 Membership 无效", "提供有效 ID 后重试")
	}
	if !actor.Role.IsAdmin() {
		return WorkspaceMember{}, httpapi.Forbidden("只有管理员可以管理成员", "请联系管理员")
	}
	if actor.MembershipID == membershipID {
		return WorkspaceMember{}, httpapi.Conflict("不能修改当前登录管理员", "请由其他管理员执行此操作")
	}
	if input.Role == nil && input.Status == nil {
		return WorkspaceMember{}, httpapi.Validation("至少提供角色或成员状态", "修改 role 或 status 后重试")
	}
	input.Reason = strings.TrimSpace(input.Reason)
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.Reason == "" || len([]rune(input.Reason)) > 500 {
		return WorkspaceMember{}, httpapi.Validation("成员变更理由无效", "填写 1 到 500 个字符的明确理由后重试")
	}
	if actor.UserID == uuid.Nil || input.RequestID == "" || len([]byte(input.RequestID)) > 200 {
		return WorkspaceMember{}, httpapi.Validation("审计主体或请求关联标识无效", "重新登录后重试")
	}
	var result WorkspaceMember
	err := database.WithWorkspaceTransaction(ctx, r.orm, func(tx *gorm.DB) error {
		target, err := loadWorkspaceMemberRow(tx, workspaceID, membershipID, true)
		if err != nil {
			return err
		}
		nextRole := target.Role
		if input.Role != nil {
			nextRole = *input.Role
		}
		nextStatus := target.MembershipStatus
		if input.Status != nil {
			nextStatus = *input.Status
		}
		if !nextRole.IsValid() {
			return httpapi.Validation("成员角色无效", "使用已登记的角色后重试")
		}
		if !nextStatus.IsManageable() {
			return httpapi.Validation("成员状态无效", "使用 active、suspended 或 removed 后重试")
		}
		if target.MembershipStatus == MembershipStatusRemoved && nextStatus != MembershipStatusRemoved {
			return httpapi.Conflict("已移除成员不能直接恢复", "通过重新加入流程恢复成员")
		}
		if target.Role == RoleAdmin && target.MembershipStatus == MembershipStatusActive && (nextRole != RoleAdmin || nextStatus != MembershipStatusActive) {
			var activeAdmins int64
			if err := tx.Table("iam_memberships AS memberships").
				Joins("JOIN iam_roles AS roles ON roles.id = memberships.role_id").
				Where("memberships.workspace_id = ? AND memberships.id <> ? AND memberships.status = ? AND roles.code = ?", workspaceID, target.MembershipID, MembershipStatusActive, RoleAdmin).
				Count(&activeAdmins).Error; err != nil {
				return fmt.Errorf("count active admins: %w", err)
			}
			if activeAdmins < 1 {
				return httpapi.Conflict("Workspace 至少需要一个 active 管理员", "先添加或提升其他管理员后重试")
			}
		}
		updates := map[string]any{}
		if input.Role != nil && nextRole != target.Role {
			scope, ok := roleScopeForCode(nextRole)
			if !ok {
				return httpapi.Validation("成员角色无效", "使用已登记的角色后重试")
			}
			var role identityRoleRecord
			if err := tx.Where("code = ? AND scope = ?", nextRole, scope).Take(&role).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("role is not seeded: %w", err)
				}
				return fmt.Errorf("load member role: %w", err)
			}
			updates["role_id"] = role.ID
		}
		if input.Status != nil && nextStatus != target.MembershipStatus {
			updates["status"] = nextStatus
		}
		if len(updates) > 0 {
			if err := tx.Model(&identityMembershipRecord{}).Where("id = ? AND workspace_id = ?", membershipID, workspaceID).Updates(updates).Error; err != nil {
				return fmt.Errorf("update workspace member: %w", err)
			}
		}
		if (input.Status != nil && nextStatus != MembershipStatusActive && nextStatus != target.MembershipStatus) || (input.Role != nil && nextRole == RoleBan && target.Role != RoleBan) {
			now := time.Now().UTC()
			if err := tx.Model(&identitySessionRecord{}).Where("user_id = ? AND workspace_id = ? AND revoked_at IS NULL", target.UserID, workspaceID).Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error; err != nil {
				return fmt.Errorf("revoke member sessions: %w", err)
			}
		}
		if len(updates) > 0 {
			beforeState, err := json.Marshal(membershipAuditState{Role: target.Role, Status: target.MembershipStatus})
			if err != nil {
				return fmt.Errorf("encode member audit baseline: %w", err)
			}
			afterState, err := json.Marshal(membershipAuditState{Role: nextRole, Status: nextStatus})
			if err != nil {
				return fmt.Errorf("encode member audit result: %w", err)
			}
			event := identityAuditEventRecord{
				ID:          uuid.New(),
				WorkspaceID: workspaceID,
				ActorType:   "user",
				ActorID:     actor.UserID.String(),
				Action:      "iam.membership.updated",
				ObjectType:  "iam_membership",
				ObjectID:    membershipID,
				BeforeState: datatypes.JSON(beforeState),
				AfterState:  datatypes.JSON(afterState),
				BeforeHash:  toolkit.SHA256Hex(beforeState),
				AfterHash:   toolkit.SHA256Hex(afterState),
				RequestID:   input.RequestID,
				Reason:      input.Reason,
				Result:      "succeeded",
				OccurredAt:  time.Now().UTC(),
			}
			if err := tx.Create(&event).Error; err != nil {
				return fmt.Errorf("write member audit event: %w", err)
			}
		}
		updated, err := loadWorkspaceMemberRow(tx, workspaceID, membershipID, false)
		if err != nil {
			return err
		}
		result = workspaceMemberFromRow(updated)
		return nil
	})
	return result, err
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
		Take(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AuthIdentity{}, ErrRefreshInvalid
		}
		return AuthIdentity{}, fmt.Errorf("load session membership: %w", err)
	}
	return AuthIdentity{Account: Account{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName}, Workspace: Workspace{ID: membership.WorkspaceID, Name: membership.WorkspaceName}, MembershipID: membership.MembershipID, Role: membership.Role}, nil
}

func loadWorkspaceMemberRow(tx *gorm.DB, workspaceID, membershipID uuid.UUID, lock bool) (workspaceMemberRow, error) {
	var row workspaceMemberRow
	db := tx.Table("iam_memberships AS memberships").
		Select("memberships.id AS membership_id, memberships.user_id, memberships.role_id, users.email, users.display_name, users.status AS account_status, memberships.status AS membership_status, roles.code AS role, memberships.created_at").
		Joins("JOIN iam_users AS users ON users.id = memberships.user_id").
		Joins("JOIN iam_roles AS roles ON roles.id = memberships.role_id").
		Where("memberships.id = ? AND memberships.workspace_id = ?", membershipID, workspaceID)
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := db.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return workspaceMemberRow{}, httpapi.NotFound("成员")
		}
		return workspaceMemberRow{}, fmt.Errorf("load workspace member: %w", err)
	}
	return row, nil
}

func workspaceMemberFromRow(row workspaceMemberRow) WorkspaceMember {
	return WorkspaceMember{MembershipID: row.MembershipID, UserID: row.UserID, Email: row.Email, DisplayName: row.DisplayName, AccountStatus: row.AccountStatus, MembershipStatus: row.MembershipStatus, Role: row.Role, CreatedAt: row.CreatedAt}
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
				owned, err = r.resourceInWorkspace(tx, "nar_source_revisions", id, workspaceID)
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
