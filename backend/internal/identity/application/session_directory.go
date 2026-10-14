package application

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type SessionSummary struct {
	ID                                         uuid.UUID
	Current                                    bool
	DeviceSummary                              string
	CreatedAt, LastActiveAt, AbsoluteExpiresAt time.Time
}
type SessionDirectoryStore interface {
	ListSelfSessions(context.Context, Principal, time.Time) ([]SessionSummary, error)
	RevokeSelfSession(context.Context, Principal, uuid.UUID, time.Time) error
	RevokeOtherSessions(context.Context, Principal, time.Time) error
}
type SessionDirectory struct {
	store SessionDirectoryStore
	now   func() time.Time
}

func NewSessionDirectory(store SessionDirectoryStore, now func() time.Time) *SessionDirectory {
	return &SessionDirectory{store: store, now: now}
}
func (s *SessionDirectory) List(ctx context.Context, actor Principal) ([]SessionSummary, error) {
	return s.store.ListSelfSessions(ctx, actor, s.now().UTC())
}
func (s *SessionDirectory) Revoke(ctx context.Context, actor Principal, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrSessionInvalid
	}
	return s.store.RevokeSelfSession(ctx, actor, id, s.now().UTC())
}
func (s *SessionDirectory) RevokeOthers(ctx context.Context, actor Principal) error {
	return s.store.RevokeOtherSessions(ctx, actor, s.now().UTC())
}
