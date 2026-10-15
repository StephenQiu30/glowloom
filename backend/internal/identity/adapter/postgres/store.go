// Package postgres persists organization-scoped identity accounts.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// Store keeps identity queries scoped to one organization.
type Store struct {
	db              *gorm.DB
	invitationTeams TeamGovernanceFactory
	noticeProtector application.ProofProtector
}

// NewStore injects the database handle for identity accounts.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

type userRow struct {
	ID                 uuid.UUID
	OrgID              uuid.UUID
	LoginName          string
	EmailKey           string
	EmailVerifiedAt    *time.Time
	EmailVersion       int64
	PlatformRole       string
	PlatformEpoch      int64
	AvatarAssetID      *uuid.UUID
	TermsVersion       string
	TermsAcceptedAt    *time.Time
	Theme              string
	PromptSuffix       string
	DeletedAt          *time.Time
	DisplayName        string
	Role               string
	Status             string
	PasswordHash       string
	MustChangePassword bool
	PasswordChangedAt  *time.Time
	FailedLoginCount   int
	LockedUntil        *time.Time
	SessionEpoch       int64
	LastLoginAt        *time.Time
	Revision           int64
	CreateTime         time.Time
	UpdateTime         time.Time
}

func (r userRow) domainUser() domain.User {
	user := domain.User{
		ID: r.ID, OrgID: r.OrgID, LoginName: r.LoginName, DisplayName: r.DisplayName,
		Email: r.EmailKey, EmailVersion: r.EmailVersion, PlatformRole: r.PlatformRole, PlatformEpoch: r.PlatformEpoch, AvatarAssetID: r.AvatarAssetID, TermsVersion: r.TermsVersion, Theme: r.Theme, PromptSuffix: r.PromptSuffix,
		Role: domain.Role(r.Role), Status: domain.Status(r.Status), PasswordHash: r.PasswordHash,
		MustChangePassword: r.MustChangePassword, FailedLoginCount: r.FailedLoginCount,
		SessionEpoch: r.SessionEpoch, Revision: r.Revision, CreateTime: r.CreateTime, UpdateTime: r.UpdateTime,
	}
	if r.EmailVerifiedAt != nil {
		user.EmailVerifiedAt = *r.EmailVerifiedAt
	}
	if r.TermsAcceptedAt != nil {
		user.TermsAcceptedAt = *r.TermsAcceptedAt
	}
	if r.DeletedAt != nil {
		user.DeletedAt = *r.DeletedAt
	}
	if r.PasswordChangedAt != nil {
		user.PasswordChangedAt = *r.PasswordChangedAt
	}
	if r.LockedUntil != nil {
		user.LockedUntil = *r.LockedUntil
	}
	if r.LastLoginAt != nil {
		user.LastLoginAt = *r.LastLoginAt
	}
	return user
}
