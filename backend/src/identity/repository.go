package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IdentityRepository struct{ orm *gorm.DB }

func NewIdentityRepository(orm *gorm.DB) *IdentityRepository {
	return &IdentityRepository{orm: orm}
}

type identityUserRecord struct {
	ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	IdentitySubject string    `gorm:"column:identity_subject"`
	DisplayName     string    `gorm:"column:display_name"`
	Status          string
}

func (identityUserRecord) TableName() string { return "iam_users" }

type identityRoleRecord struct {
	ID    uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Code  string
	Scope string
}

func (identityRoleRecord) TableName() string { return "iam_roles" }

type identityMembershipRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id;type:uuid"`
	UserID      uuid.UUID `gorm:"column:user_id;type:uuid"`
	RoleID      uuid.UUID `gorm:"column:role_id;type:uuid"`
	Status      string
}

func (identityMembershipRecord) TableName() string { return "iam_memberships" }

type identitySessionRecord struct {
	ID          uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid"`
	WorkspaceID uuid.UUID  `gorm:"column:workspace_id;type:uuid"`
	TokenHash   string     `gorm:"column:token_hash"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	RevokedAt   *time.Time `gorm:"column:revoked_at"`
}

func (identitySessionRecord) TableName() string { return "iam_sessions" }

type workspaceProjectRecord struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	WorkspaceID uuid.UUID `gorm:"column:workspace_id;type:uuid"`
}

func (workspaceProjectRecord) TableName() string { return "projects" }

type workspaceResourceRecord struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID uuid.UUID `gorm:"column:project_id;type:uuid"`
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (r *IdentityRepository) CreateSession(ctx context.Context, subject string, workspaceID uuid.UUID) (Session, error) {
	if r.orm == nil {
		return Session{}, fmt.Errorf("identity repository ORM is not configured")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("create session token: %w", err)
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(24 * time.Hour)
	var userID uuid.UUID
	if err := r.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user identityUserRecord
		if err := tx.Where("identity_subject = ?", subject).First(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			user = identityUserRecord{ID: uuid.New(), IdentitySubject: subject, DisplayName: subject, Status: "active"}
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("create identity: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("load identity: %w", err)
		} else if err := tx.Model(&user).Updates(map[string]any{"display_name": subject, "status": "active"}).Error; err != nil {
			return fmt.Errorf("activate identity: %w", err)
		}
		userID = user.ID
		var role identityRoleRecord
		if err := tx.Where("code = ?", "owner").First(&role).Error; err != nil {
			return fmt.Errorf("owner role is not seeded: %w", err)
		}
		membership := identityMembershipRecord{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, RoleID: role.ID, Status: "active"}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"role_id", "status"})}).Create(&membership).Error; err != nil {
			return fmt.Errorf("create membership: %w", err)
		}
		if err := tx.Create(&identitySessionRecord{ID: uuid.New(), UserID: userID, WorkspaceID: workspaceID, TokenHash: hashToken(token), ExpiresAt: expires}).Error; err != nil {
			return fmt.Errorf("persist session: %w", err)
		}
		return nil
	}); err != nil {
		return Session{}, err
	}
	return Session{Token: token, UserID: userID, WorkspaceID: workspaceID, ExpiresAt: expires.Format(time.RFC3339)}, nil
}

func (r *IdentityRepository) Authenticate(ctx context.Context, token string, workspaceID uuid.UUID) (Principal, error) {
	if r.orm == nil {
		return Principal{}, fmt.Errorf("identity repository ORM is not configured")
	}
	var session identitySessionRecord
	if err := r.orm.WithContext(ctx).Where("token_hash = ? AND workspace_id = ? AND revoked_at IS NULL AND expires_at > ?", hashToken(token), workspaceID, time.Now().UTC()).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Principal{}, httpapi.NotFound("会话")
		}
		return Principal{}, fmt.Errorf("authenticate session: %w", err)
	}
	var user identityUserRecord
	if err := r.orm.WithContext(ctx).Where("id = ? AND status = ?", session.UserID, "active").First(&user).Error; err != nil {
		return Principal{}, httpapi.NotFound("会话")
	}
	var membership identityMembershipRecord
	if err := r.orm.WithContext(ctx).Where("user_id = ? AND workspace_id = ? AND status = ?", session.UserID, workspaceID, "active").First(&membership).Error; err != nil {
		return Principal{}, httpapi.NotFound("Workspace 成员关系")
	}
	var role identityRoleRecord
	if err := r.orm.WithContext(ctx).Where("id = ?", membership.RoleID).First(&role).Error; err != nil {
		return Principal{}, fmt.Errorf("load membership role: %w", err)
	}
	return Principal{UserID: user.ID, WorkspaceID: workspaceID, MembershipID: membership.ID, Role: role.Code}, nil
}

func (r *IdentityRepository) Revoke(ctx context.Context, token string) error {
	if r.orm == nil {
		return fmt.Errorf("identity repository ORM is not configured")
	}
	now := time.Now().UTC()
	return r.orm.WithContext(ctx).Model(&identitySessionRecord{}).Where("token_hash = ? AND revoked_at IS NULL", hashToken(token)).Update("revoked_at", now).Error
}

func (r *IdentityRepository) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, path string) error {
	if r.orm == nil {
		return fmt.Errorf("identity repository ORM is not configured")
	}
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
			err = r.orm.WithContext(ctx).Where("id = ? AND workspace_id = ?", id, workspaceID).First(&project).Error
			owned = err == nil
		case "content-units":
			owned, err = r.resourceInWorkspace(ctx, "prj_content_units", id, workspaceID)
		case "script-revisions":
			owned, err = r.resourceInWorkspace(ctx, "script_revisions", id, workspaceID)
		case "operations":
			owned, err = r.resourceInWorkspace(ctx, "operations", id, workspaceID)
		case "agent-runs":
			owned, err = r.resourceInWorkspace(ctx, "m06_agent_runs", id, workspaceID)
		case "shots":
			owned, err = r.resourceInWorkspace(ctx, "sht_shots", id, workspaceID)
		case "candidates":
			owned, err = r.resourceInWorkspace(ctx, "media_candidates", id, workspaceID)
		case "generation-plans":
			owned, err = r.resourceInWorkspace(ctx, "gen_plans", id, workspaceID)
		}
		if err != nil {
			return fmt.Errorf("authorize resource: %w", err)
		}
		if !owned {
			return httpapi.NotFound("资源")
		}
	}
	return nil
}

func (r *IdentityRepository) resourceInWorkspace(ctx context.Context, table string, resourceID, workspaceID uuid.UUID) (bool, error) {
	var resource workspaceResourceRecord
	if err := r.orm.WithContext(ctx).Table(table).Where("id = ?", resourceID).First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	var project workspaceProjectRecord
	if err := r.orm.WithContext(ctx).Where("id = ? AND workspace_id = ?", resource.ProjectID, workspaceID).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
