package application

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// DirectorRecovery is a private recoverable edit, never an authoritative scene.
type DirectorRecovery struct {
	Revision        int64                 `json:"revision"`
	Epoch           uuid.UUID             `json:"epoch"`
	BaseFingerprint string                `json:"base_fingerprint"`
	Director        domain.DirectorConfig `json:"director"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// DirectorRecoveryResult distinguishes no draft from an unreadable draft.
type DirectorRecoveryResult struct {
	Draft *DirectorRecovery `json:"draft"`
}

// SaveDirectorRecoveryInput uses CAS to protect edits retained by another tab.
type SaveDirectorRecoveryInput struct {
	ExpectedRevision int64                 `json:"expected_revision"`
	Epoch            uuid.UUID             `json:"epoch"`
	BaseFingerprint  string                `json:"base_fingerprint"`
	Director         domain.DirectorConfig `json:"director"`
}

// DeleteDirectorRecoveryInput cannot discard a newer draft.
type DeleteDirectorRecoveryInput struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

// DirectorRecoveryStore owns authorized private recovery persistence.
type DirectorRecoveryStore interface {
	ReadDirectorRecovery(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (DirectorRecoveryResult, error)
	SaveDirectorRecovery(context.Context, identity.Principal, uuid.UUID, uuid.UUID, SaveDirectorRecoveryInput) (DirectorRecovery, error)
	DeleteDirectorRecovery(context.Context, identity.Principal, uuid.UUID, uuid.UUID, int64) error
}

func (s *Service) recoveryStore(actor identity.Principal, canvas, node uuid.UUID) (DirectorRecoveryStore, error) {
	if actor.ID == uuid.Nil || canvas == uuid.Nil || node == uuid.Nil {
		return nil, domain.ErrInvalidCommand
	}
	store, ok := s.store.(DirectorRecoveryStore)
	if !ok {
		return nil, ErrNotFound
	}
	return store, nil
}

// ReadDirectorRecovery reads only the caller's unexpired recovery draft.
func (s *Service) ReadDirectorRecovery(ctx context.Context, a identity.Principal, c, n uuid.UUID) (DirectorRecoveryResult, error) {
	store, err := s.recoveryStore(a, c, n)
	if err != nil {
		return DirectorRecoveryResult{}, err
	}
	return store.ReadDirectorRecovery(ctx, a, c, n)
}

// SaveDirectorRecovery validates the same domain scene before private persistence.
func (s *Service) SaveDirectorRecovery(ctx context.Context, a identity.Principal, c, n uuid.UUID, input SaveDirectorRecoveryInput) (DirectorRecovery, error) {
	hash, err := hex.DecodeString(input.BaseFingerprint)
	if err != nil || len(hash) != 32 || strings.ToLower(input.BaseFingerprint) != input.BaseFingerprint || input.Epoch == uuid.Nil || input.ExpectedRevision < 0 || input.ExpectedRevision > 2147483646 || input.Director.Validate() != nil {
		return DirectorRecovery{}, domain.ErrInvalidCommand
	}
	store, err := s.recoveryStore(a, c, n)
	if err != nil {
		return DirectorRecovery{}, err
	}
	return store.SaveDirectorRecovery(ctx, a, c, n, input)
}

// DeleteDirectorRecovery removes only the version explicitly handled by the user.
func (s *Service) DeleteDirectorRecovery(ctx context.Context, a identity.Principal, c, n uuid.UUID, input DeleteDirectorRecoveryInput) error {
	if input.ExpectedRevision < 1 {
		return domain.ErrInvalidCommand
	}
	store, err := s.recoveryStore(a, c, n)
	if err != nil {
		return err
	}
	return store.DeleteDirectorRecovery(ctx, a, c, n, input.ExpectedRevision)
}
