package application

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrInvalidProviderDeletion indicates a missing confirmation or request identity.
var ErrInvalidProviderDeletion = errors.New("invalid provider deletion")

// ProviderDeletionDraft identifies an unpublished model in the confirmation.
type ProviderDeletionDraft struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Revision int64     `json:"revision"`
}

// ProviderDeletionPreview describes the current scope of a possible deletion.
type ProviderDeletionPreview struct {
	Revision      int64                   `json:"revision"`
	BlockedReason string                  `json:"blocked_reason,omitempty"`
	Drafts        []ProviderDeletionDraft `json:"drafts"`
	Token         string                  `json:"token,omitempty"`
}

// DeleteProviderInput binds the write to its preview and original request.
type DeleteProviderInput struct {
	Token     string
	RequestID uuid.UUID
}

// DeletedProvider is the safe receipt of a committed deletion.
type DeletedProvider struct {
	ID uuid.UUID `json:"id"`
}

// ProviderDeletionStore checks dependencies and applies a guarded deletion.
type ProviderDeletionStore interface {
	PreviewProviderDeletion(context.Context, identityapp.Principal, uuid.UUID) (ProviderDeletionPreview, error)
	DeleteProvider(context.Context, identityapp.Principal, uuid.UUID, DeleteProviderInput) (DeletedProvider, error)
}

// ProviderDeletion previews and deletes unused service connections.
type ProviderDeletion struct{ store ProviderDeletionStore }

func NewProviderDeletion(store ProviderDeletionStore) *ProviderDeletion {
	return &ProviderDeletion{store: store}
}

func (s *ProviderDeletion) Preview(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (ProviderDeletionPreview, error) {
	if !IsPlatformAdmin(actor) {
		return ProviderDeletionPreview{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil {
		return ProviderDeletionPreview{}, ErrInvalidProviderDeletion
	}
	return s.store.PreviewProviderDeletion(ctx, actor, id)
}

func (s *ProviderDeletion) Delete(ctx context.Context, actor identityapp.Principal, id uuid.UUID, input DeleteProviderInput) (DeletedProvider, error) {
	if !IsPlatformAdmin(actor) {
		return DeletedProvider{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || input.RequestID == uuid.Nil || len(input.Token) != 64 {
		return DeletedProvider{}, ErrInvalidProviderDeletion
	}
	if _, err := hex.DecodeString(input.Token); err != nil {
		return DeletedProvider{}, ErrInvalidProviderDeletion
	}
	return s.store.DeleteProvider(ctx, actor, id, input)
}
