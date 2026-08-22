package identity

import "github.com/google/uuid"

type Principal struct {
	UserID       uuid.UUID
	WorkspaceID  uuid.UUID
	MembershipID uuid.UUID
	Role         string
}

type Session struct {
	Token       string    `json:"token"`
	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ExpiresAt   string    `json:"expires_at"`
}
