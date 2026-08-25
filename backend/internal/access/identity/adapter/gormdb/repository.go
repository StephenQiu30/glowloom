package gormdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/access/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/access/identity/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
)

type Store struct{ database *gorm.DB }
type repository struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (store *Store) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	return platformdatabase.WithinTransaction(ctx, store.database, func(transaction *gorm.DB) error {
		return operation(&repository{database: transaction})
	})
}

func (repo *repository) FindVerificationByEmail(ctx context.Context, email string, lock bool) (domain.RegistrationVerification, error) {
	query := repo.database.WithContext(ctx).Where("email_normalized = ?", email)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.RegistrationVerification
	if err := query.First(&record).Error; err != nil {
		return domain.RegistrationVerification{}, normalizeNotFound(err)
	}
	return verificationDomain(record), nil
}

func (repo *repository) FindVerificationByTicketDigest(ctx context.Context, digest string, lock bool) (domain.RegistrationVerification, error) {
	query := repo.database.WithContext(ctx).Where("ticket_digest = ?", digest)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.RegistrationVerification
	if err := query.First(&record).Error; err != nil {
		return domain.RegistrationVerification{}, normalizeNotFound(err)
	}
	return verificationDomain(record), nil
}

func (repo *repository) SaveVerification(ctx context.Context, verification domain.RegistrationVerification) error {
	record, err := verificationRecord(verification)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Save(&record).Error
}

func (repo *repository) CreateVerification(ctx context.Context, verification domain.RegistrationVerification) error {
	record, err := verificationRecord(verification)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Create(&record).Error
}

func (repo *repository) CreateAccount(ctx context.Context, user domain.User, workspace domain.Workspace, membership domain.Membership) error {
	userRecord, err := userRecord(user)
	if err != nil {
		return err
	}
	workspaceRecord, err := workspaceRecord(workspace)
	if err != nil {
		return err
	}
	membershipRecord, err := membershipRecord(membership)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&userRecord).Error; err != nil {
		return normalizeConflict(err, "Account already exists")
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&workspaceRecord).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&membershipRecord).Error
}

func (repo *repository) FindUserByEmail(ctx context.Context, email string, lock bool) (domain.User, error) {
	query := repo.database.WithContext(ctx).Where("email_normalized = ?", email)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.UserAccount
	if err := query.First(&record).Error; err != nil {
		return domain.User{}, normalizeNotFound(err)
	}
	return userDomain(record), nil
}

func (repo *repository) FindUserByID(ctx context.Context, id string, lock bool) (domain.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return domain.User{}, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).Where("id = ?", parsed)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.UserAccount
	if err = query.First(&record).Error; err != nil {
		return domain.User{}, normalizeNotFound(err)
	}
	return userDomain(record), nil
}

func (repo *repository) SaveUser(ctx context.Context, user domain.User) error {
	record, err := userRecord(user)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).
		Model(&model.UserAccount{}).
		Where("id = ?", record.ID).
		Select("email_normalized", "password_hash", "token_version", "display_name", "avatar_url", "status", "last_login_at", "updated_at").
		Updates(&record).Error
}

func (repo *repository) PrimaryWorkspace(ctx context.Context, userID string) (domain.Workspace, domain.Membership, error) {
	parsed, err := uuid.Parse(userID)
	if err != nil {
		return domain.Workspace{}, domain.Membership{}, application.ErrNotFound
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).
		Where("user_id = ? AND status = ?", parsed, "active").
		Order("joined_at").Order("id").First(&membership).Error; err != nil {
		return domain.Workspace{}, domain.Membership{}, normalizeNotFound(err)
	}
	var workspace model.Workspace
	if err = repo.database.WithContext(ctx).First(&workspace, "id = ?", membership.WorkspaceID).Error; err != nil {
		return domain.Workspace{}, domain.Membership{}, normalizeNotFound(err)
	}
	return workspaceDomain(workspace), membershipDomain(membership), nil
}

func (repo *repository) CreateSession(ctx context.Context, session domain.AuthSession) error {
	record, err := sessionRecord(session)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func (repo *repository) FindSessionByDigest(ctx context.Context, digest string, lock bool) (domain.AuthSession, error) {
	query := repo.database.WithContext(ctx).Where("token_digest = ?", digest)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var record model.AuthSession
	if err := query.First(&record).Error; err != nil {
		return domain.AuthSession{}, normalizeNotFound(err)
	}
	return sessionDomain(record), nil
}

func (repo *repository) SaveSession(ctx context.Context, session domain.AuthSession) error {
	record, err := sessionRecord(session)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Save(&record).Error
}

func (repo *repository) RevokeUserSessions(ctx context.Context, userID string, revokedAt time.Time) error {
	parsed, err := uuid.Parse(userID)
	if err != nil {
		return application.ErrNotFound
	}
	return repo.database.WithContext(ctx).
		Model(&model.AuthSession{}).
		Where("user_id = ? AND revoked_at IS NULL", parsed).
		Updates(map[string]any{"revoked_at": revokedAt, "updated_at": revokedAt}).Error
}

func (repo *repository) ListWorkspaces(ctx context.Context, userID string, includeArchived bool) ([]application.WorkspaceMembership, error) {
	parsed, err := uuid.Parse(userID)
	if err != nil {
		return nil, application.ErrNotFound
	}
	query := repo.database.WithContext(ctx).
		Preload("Workspace").
		Where("user_id = ? AND idn_memberships.status = ?", parsed, "active").
		Order("joined_at").Order("id")
	if !includeArchived {
		query = query.Joins("JOIN idn_workspaces ON idn_workspaces.id = idn_memberships.workspace_id").Where("idn_workspaces.status = ?", "active")
	}
	var records []model.Membership
	if err = query.Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]application.WorkspaceMembership, len(records))
	for index, record := range records {
		result[index] = application.WorkspaceMembership{Workspace: workspaceDomain(record.Workspace), Membership: membershipDomain(record)}
	}
	return result, nil
}

func (repo *repository) FindWorkspaceForUser(ctx context.Context, userID, workspaceID string, lock bool) (domain.Workspace, domain.Membership, error) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return domain.Workspace{}, domain.Membership{}, application.ErrNotFound
	}
	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		return domain.Workspace{}, domain.Membership{}, application.ErrNotFound
	}
	var membership model.Membership
	if err = repo.database.WithContext(ctx).
		Where("workspace_id = ? AND user_id = ? AND status = ?", workspaceUUID, userUUID, "active").
		First(&membership).Error; err != nil {
		return domain.Workspace{}, domain.Membership{}, normalizeNotFound(err)
	}
	query := repo.database.WithContext(ctx).Where("id = ?", workspaceUUID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var workspace model.Workspace
	if err = query.First(&workspace).Error; err != nil {
		return domain.Workspace{}, domain.Membership{}, normalizeNotFound(err)
	}
	return workspaceDomain(workspace), membershipDomain(membership), nil
}

func (repo *repository) CreateWorkspace(ctx context.Context, workspace domain.Workspace, membership domain.Membership) error {
	workspaceModel, err := workspaceRecord(workspace)
	if err != nil {
		return err
	}
	membershipModel, err := membershipRecord(membership)
	if err != nil {
		return err
	}
	if err = repo.database.WithContext(ctx).Omit(clause.Associations).Create(&workspaceModel).Error; err != nil {
		return err
	}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&membershipModel).Error
}

func (repo *repository) SaveWorkspace(ctx context.Context, workspace domain.Workspace) error {
	record, err := workspaceRecord(workspace)
	if err != nil {
		return err
	}
	return repo.database.WithContext(ctx).
		Model(&model.Workspace{}).Where("id = ?", record.ID).
		Select("name", "status", "revision", "archived_at", "updated_at").Updates(&record).Error
}

func (repo *repository) AppendAudit(ctx context.Context, event application.AuditEvent) error {
	workspaceID, err := uuid.Parse(event.WorkspaceID)
	if err != nil {
		return err
	}
	actorID, err := uuid.Parse(event.ActorID)
	if err != nil {
		return err
	}
	targetID, err := uuid.Parse(event.TargetID)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return err
	}
	record := model.AuditEvent{ID: uuid.New(), WorkspaceID: workspaceID, ActorID: actorID, Action: event.Action, TargetType: event.TargetType, TargetID: targetID, Result: "succeeded", TraceID: event.TraceID, Metadata: datatypes.JSON(metadata), OccurredAt: event.OccurredAt}
	return repo.database.WithContext(ctx).Omit(clause.Associations).Create(&record).Error
}

func normalizeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func normalizeConflict(err error, message string) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return &application.Error{Code: application.CodeConflict, Message: message, Status: 409}
	}
	return err
}

func userRecord(value domain.User) (model.UserAccount, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.UserAccount{}, fmt.Errorf("parse user id: %w", err)
	}
	return model.UserAccount{ID: id, EmailNormalized: value.Email, PasswordHash: value.PasswordHash, TokenVersion: value.TokenVersion, DisplayName: value.DisplayName, AvatarURL: value.AvatarURL, Status: value.Status, LastLoginAt: value.LastLoginAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func userDomain(value model.UserAccount) domain.User {
	return domain.User{ID: value.ID.String(), Email: value.EmailNormalized, PasswordHash: value.PasswordHash, TokenVersion: value.TokenVersion, DisplayName: value.DisplayName, AvatarURL: value.AvatarURL, Status: value.Status, LastLoginAt: value.LastLoginAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func workspaceRecord(value domain.Workspace) (model.Workspace, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.Workspace{}, fmt.Errorf("parse workspace id: %w", err)
	}
	return model.Workspace{ID: id, Name: value.Name, Status: value.Status, Revision: value.Revision, ArchivedAt: value.ArchivedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func workspaceDomain(value model.Workspace) domain.Workspace {
	return domain.Workspace{ID: value.ID.String(), Name: value.Name, Status: value.Status, Revision: value.Revision, ArchivedAt: value.ArchivedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func membershipRecord(value domain.Membership) (model.Membership, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.Membership{}, err
	}
	workspaceID, err := uuid.Parse(value.WorkspaceID)
	if err != nil {
		return model.Membership{}, err
	}
	userID, err := uuid.Parse(value.UserID)
	if err != nil {
		return model.Membership{}, err
	}
	return model.Membership{ID: id, WorkspaceID: workspaceID, UserID: userID, Role: value.Role, Status: value.Status, JoinedAt: value.JoinedAt, RemovedAt: value.RemovedAt}, nil
}

func membershipDomain(value model.Membership) domain.Membership {
	return domain.Membership{ID: value.ID.String(), WorkspaceID: value.WorkspaceID.String(), UserID: value.UserID.String(), Role: value.Role, Status: value.Status, JoinedAt: value.JoinedAt, RemovedAt: value.RemovedAt}
}

func verificationRecord(value domain.RegistrationVerification) (model.RegistrationVerification, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.RegistrationVerification{}, err
	}
	return model.RegistrationVerification{ID: id, EmailNormalized: value.Email, CodeDigest: value.CodeDigest, AttemptCount: value.AttemptCount, Status: value.Status, ExpiresAt: value.ExpiresAt, TicketDigest: value.TicketDigest, TicketExpiresAt: value.TicketExpiresAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func verificationDomain(value model.RegistrationVerification) domain.RegistrationVerification {
	return domain.RegistrationVerification{ID: value.ID.String(), Email: value.EmailNormalized, CodeDigest: value.CodeDigest, AttemptCount: value.AttemptCount, Status: value.Status, ExpiresAt: value.ExpiresAt, TicketDigest: value.TicketDigest, TicketExpiresAt: value.TicketExpiresAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func sessionRecord(value domain.AuthSession) (model.AuthSession, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return model.AuthSession{}, err
	}
	userID, err := uuid.Parse(value.UserID)
	if err != nil {
		return model.AuthSession{}, err
	}
	return model.AuthSession{ID: id, UserID: userID, TokenDigest: value.TokenDigest, TokenVersion: value.TokenVersion, ExpiresAt: value.ExpiresAt, RevokedAt: value.RevokedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}, nil
}

func sessionDomain(value model.AuthSession) domain.AuthSession {
	return domain.AuthSession{ID: value.ID.String(), UserID: value.UserID.String(), TokenDigest: value.TokenDigest, TokenVersion: value.TokenVersion, ExpiresAt: value.ExpiresAt, RevokedAt: value.RevokedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}
