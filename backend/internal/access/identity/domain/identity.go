package domain

import "time"

type User struct {
	ID, Email, PasswordHash, DisplayName, Status string
	TokenVersion                                 int
	AvatarURL                                    *string
	LastLoginAt                                  *time.Time
	CreatedAt, UpdatedAt                         time.Time
}

type Workspace struct {
	ID, Name, Status     string
	Revision             int
	ArchivedAt           *time.Time
	CreatedAt, UpdatedAt time.Time
}

type Membership struct {
	ID, WorkspaceID, UserID, Role, Status string
	JoinedAt                              time.Time
	RemovedAt                             *time.Time
}

type RegistrationVerification struct {
	ID, Email, CodeDigest, Status string
	AttemptCount                  int
	ExpiresAt                     time.Time
	TicketDigest                  *string
	TicketExpiresAt               *time.Time
	CreatedAt, UpdatedAt          time.Time
}

type AuthSession struct {
	ID, UserID, TokenDigest string
	TokenVersion            int
	ExpiresAt               time.Time
	RevokedAt               *time.Time
	CreatedAt, UpdatedAt    time.Time
}
