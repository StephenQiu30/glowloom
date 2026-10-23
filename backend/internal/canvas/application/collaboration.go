package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// CollaborationInput is one immutable, causally ordered atomic batch.
type CollaborationInput struct {
	// StrictBase fences an explicitly reviewed recovery against a newer durable head.
	StrictBase       bool               `json:"strict_base,omitempty"`
	ClientID         uuid.UUID          `json:"client_id,omitempty"`
	OperationVersion int                `json:"operation_version"`
	BaseRevision     int64              `json:"base_revision"`
	Operations       []domain.Operation `json:"operations"`
	UndoOf           string             `json:"undo_of,omitempty"`
}

// CollaborationReceipt is a durable terminal outcome. Rejections have no
// result revision; their key can never subsequently acquire a new meaning.
type CollaborationReceipt struct {
	Status   string `json:"status"`
	OpID     string `json:"op_id"`
	Revision int64  `json:"revision,omitempty"`
	Code     string `json:"code,omitempty"`
}

// CollaborationChange contains authoritative operations and normalized effects.
// Private preimages and original requests are never exposed on this feed.
type CollaborationChange struct {
	Revision   int64              `json:"revision"`
	ActorID    uuid.UUID          `json:"actor_id"`
	OpID       string             `json:"op_id"`
	Operations []domain.Operation `json:"operations"`
	Effects    []domain.Effect    `json:"effects"`
}

// CollaborationStore owns history, authorization and terminal transactionality.
type CollaborationStore interface {
	SubmitCollaboration(context.Context, identity.Principal, uuid.UUID, string, CollaborationInput) (CollaborationReceipt, error)
	ResolveCollaboration(context.Context, identity.Principal, uuid.UUID, string, CollaborationInput) (*CollaborationReceipt, error)
	CollaborationChanges(context.Context, identity.Principal, uuid.UUID, int64) ([]CollaborationChange, error)
}

// CollaborationService exposes the ordered operation protocol.
type CollaborationService struct{ store CollaborationStore }

// NewCollaborationService injects durable collaboration storage.
func NewCollaborationService(store CollaborationStore) *CollaborationService {
	return &CollaborationService{store: store}
}

// Submit accepts or durably rejects an immutable batch.
func (s *CollaborationService) Submit(ctx context.Context, a identity.Principal, id uuid.UUID, key string, input CollaborationInput) (CollaborationReceipt, error) {
	if !validKey(key) || input.ClientID == uuid.Nil {
		return CollaborationReceipt{}, domain.ErrInvalidCommand
	}
	return s.store.SubmitCollaboration(ctx, a, id, key, input)
}

// Resolve queries the terminal outcome of the exact original request.
func (s *CollaborationService) Resolve(ctx context.Context, a identity.Principal, id uuid.UUID, key string, input CollaborationInput) (*CollaborationReceipt, error) {
	if !validKey(key) {
		return nil, domain.ErrInvalidCommand
	}
	return s.store.ResolveCollaboration(ctx, a, id, key, input)
}

// Changes reads contiguous canonical changes after the client baseline.
func (s *CollaborationService) Changes(ctx context.Context, a identity.Principal, id uuid.UUID, after int64) ([]CollaborationChange, error) {
	if after < 1 {
		return nil, domain.ErrInvalidCommand
	}
	return s.store.CollaborationChanges(ctx, a, id, after)
}
