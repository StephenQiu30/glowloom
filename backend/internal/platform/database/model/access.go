package model

import (
	"time"

	"github.com/google/uuid"
)

type UserAccount struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey"`
	EmailNormalized string     `gorm:"type:varchar(320);not null;unique"`
	PasswordHash    string     `gorm:"type:text;not null"`
	TokenVersion    int        `gorm:"not null;check:ck_idn_user_token_version,token_version >= 1"`
	DisplayName     string     `gorm:"type:varchar(80);not null"`
	AvatarURL       *string    `gorm:"type:text"`
	Status          string     `gorm:"type:varchar(20);not null;index:ix_idn_user_accounts_status;check:ck_idn_user_status,status IN ('active','deactivated')"`
	LastLoginAt     *time.Time `gorm:"type:timestamptz"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null"`
}

func (UserAccount) TableName() string { return "idn_user_accounts" }

type Workspace struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Name       string     `gorm:"type:varchar(120);not null"`
	Status     string     `gorm:"type:varchar(20);not null;index:ix_idn_workspaces_status;check:ck_idn_workspace_status,status IN ('active','archived')"`
	Revision   int        `gorm:"not null;check:ck_idn_workspace_revision,revision >= 1"`
	ArchivedAt *time.Time `gorm:"type:timestamptz"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt  time.Time  `gorm:"type:timestamptz;not null"`
}

func (Workspace) TableName() string { return "idn_workspaces" }

type Membership struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey"`
	WorkspaceID uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_idn_membership_workspace_user,priority:1"`
	UserID      uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uq_idn_membership_workspace_user,priority:2"`
	Role        string      `gorm:"type:varchar(20);not null;check:ck_idn_membership_role,role IN ('owner','editor','viewer')"`
	Status      string      `gorm:"type:varchar(20);not null;check:ck_idn_membership_status,status IN ('active','removed')"`
	JoinedAt    time.Time   `gorm:"type:timestamptz;not null"`
	RemovedAt   *time.Time  `gorm:"type:timestamptz"`
	Workspace   Workspace   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	User        UserAccount `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (Membership) TableName() string { return "idn_memberships" }

type RegistrationVerification struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey"`
	EmailNormalized string     `gorm:"type:varchar(320);not null;unique"`
	CodeDigest      string     `gorm:"type:char(64);not null"`
	AttemptCount    int        `gorm:"not null;check:ck_idn_registration_attempt_count,attempt_count >= 0"`
	Status          string     `gorm:"type:varchar(20);not null;check:ck_idn_registration_status,status IN ('pending','confirmed','consumed')"`
	ExpiresAt       time.Time  `gorm:"type:timestamptz;not null;index:ix_idn_registration_expires"`
	TicketDigest    *string    `gorm:"type:char(64);unique"`
	TicketExpiresAt *time.Time `gorm:"type:timestamptz"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null"`
}

func (RegistrationVerification) TableName() string { return "idn_registration_verifications" }

type AuthSession struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey"`
	UserID       uuid.UUID   `gorm:"type:uuid;not null;index:ix_idn_auth_sessions_user"`
	TokenDigest  string      `gorm:"type:char(64);not null;unique"`
	TokenVersion int         `gorm:"not null;check:ck_idn_auth_session_token_version,token_version >= 1"`
	ExpiresAt    time.Time   `gorm:"type:timestamptz;not null;index:ix_idn_auth_sessions_expires"`
	RevokedAt    *time.Time  `gorm:"type:timestamptz"`
	CreatedAt    time.Time   `gorm:"type:timestamptz;not null"`
	UpdatedAt    time.Time   `gorm:"type:timestamptz;not null"`
	User         UserAccount `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (AuthSession) TableName() string { return "idn_auth_sessions" }
