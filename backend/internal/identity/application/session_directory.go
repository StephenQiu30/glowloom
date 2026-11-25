package application

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SessionSummary projects live session metadata without authentication tokens.
type SessionSummary struct {
	ID                                         uuid.UUID
	Current                                    bool
	DeviceSummary                              string
	CreatedAt, LastActiveAt, AbsoluteExpiresAt time.Time
}

// SessionDirectoryStore checks current identity before listing or revoking sessions.
type SessionDirectoryStore interface {
	ListSelfSessions(context.Context, Principal, func() time.Time) ([]SessionSummary, error)
	RevokeSelfSession(context.Context, Principal, uuid.UUID, func() time.Time) error
	RevokeOtherSessions(context.Context, Principal, func() time.Time) error
}

// SessionDirectory lists and revokes only the current account sessions.
type SessionDirectory struct {
	store SessionDirectoryStore
	now   func() time.Time
}

// NewSessionDirectory constructs SessionDirectory with explicit caller-supplied dependencies.
func NewSessionDirectory(store SessionDirectoryStore, now func() time.Time) *SessionDirectory {
	return &SessionDirectory{store: store, now: now}
}

// List returns the actor session metadata with no secret material.
func (s *SessionDirectory) List(ctx context.Context, actor Principal) ([]SessionSummary, error) {
	return s.store.ListSelfSessions(ctx, actor, s.now)
}

// Revoke revokes the selected session only when owned by the actor.
func (s *SessionDirectory) Revoke(ctx context.Context, actor Principal, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrSessionInvalid
	}
	return s.store.RevokeSelfSession(ctx, actor, id, s.now)
}

// RevokeOthers preserves the current session and revokes all other sessions.
func (s *SessionDirectory) RevokeOthers(ctx context.Context, actor Principal) error {
	return s.store.RevokeOtherSessions(ctx, actor, s.now)
}
