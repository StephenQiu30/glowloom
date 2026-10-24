package application

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// YjsVersion identifies an immutable, authorized historical graph.
type YjsVersion struct {
	ID        uuid.UUID `json:"id"`
	Epoch     uuid.UUID `json:"epoch"`
	Revision  Revision  `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	Reason    string    `json:"reason"`
}

// YjsRestore is an immutable, revision-fenced control command.
type YjsRestore struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"sessionId"`
	VersionID uuid.UUID `json:"versionId"`
	Epoch     uuid.UUID `json:"epoch"`
	CommitSeq Revision  `json:"commitSeq"`
}

// YjsRestoreResult records the original outcome even after later edits.
type YjsRestoreResult struct {
	Epoch    uuid.UUID `json:"epoch"`
	Revision Revision  `json:"revision"`
}

// YjsHistoryStore owns historical authorization and atomic epoch switching.
type YjsHistoryStore interface {
	YjsVersions(context.Context, identity.Principal, uuid.UUID, Revision) ([]YjsVersion, error)
	YjsVersionState(context.Context, identity.Principal, uuid.UUID, uuid.UUID, bool) (YjsState, error)
	ResolveYjsRestore(context.Context, identity.Principal, uuid.UUID, YjsRestore) (*YjsRestoreResult, error)
	CommitYjsRestore(context.Context, identity.Principal, YjsState, YjsRestore, YjsCandidate) (YjsRestoreResult, error)
}

func (s *YjsService) historyStore() (YjsHistoryStore, error) {
	store, ok := s.store.(YjsHistoryStore)
	if !ok {
		return nil, fmt.Errorf("history unavailable")
	}
	return store, nil
}

// Versions reads metadata on demand; zero before starts at the newest page.
func (s *YjsService) Versions(ctx context.Context, a identity.Principal, id uuid.UUID, before Revision) ([]YjsVersion, error) {
	store, err := s.historyStore()
	if err != nil {
		return nil, err
	}
	return store.YjsVersions(ctx, a, id, before)
}

// Version returns a complete read-only historical snapshot.
func (s *YjsService) Version(ctx context.Context, a identity.Principal, id, version uuid.UUID) (DocumentDTO, error) {
	store, err := s.historyStore()
	if err != nil {
		return DocumentDTO{}, err
	}
	state, err := store.YjsVersionState(ctx, a, id, version, false)
	state.Document.Permissions.Write = false
	return state.Document, err
}

// RestoreVersion preserves old receipts and creates a new epoch, never rewinds a pointer.
func (s *YjsService) RestoreVersion(ctx context.Context, a identity.Principal, id uuid.UUID, input YjsRestore) (YjsRestoreResult, error) {
	if input.ID == uuid.Nil || input.SessionID == uuid.Nil || input.VersionID == uuid.Nil || input.Epoch == uuid.Nil || input.CommitSeq < 0 {
		return YjsRestoreResult{}, domain.ErrInvalidCommand
	}
	store, err := s.historyStore()
	if err != nil {
		return YjsRestoreResult{}, err
	}
	replay, err := store.ResolveYjsRestore(ctx, a, id, input)
	if err != nil {
		return YjsRestoreResult{}, err
	}
	if replay != nil {
		return *replay, nil
	}
	base, err := s.Load(ctx, a, id)
	if err != nil {
		return YjsRestoreResult{}, err
	}
	if base.Epoch != input.Epoch || base.CommitSeq != input.CommitSeq {
		return YjsRestoreResult{}, ErrYjsContention
	}
	if base.WriterTerm == math.MaxInt64 {
		return YjsRestoreResult{}, domain.ErrInvalidCommand
	}
	target, err := store.YjsVersionState(ctx, a, id, input.VersionID, true)
	if err != nil {
		return YjsRestoreResult{}, err
	}
	target.Epoch = uuid.New()
	target.WriterTerm = base.WriterTerm + 1
	target.CommitSeq = 0
	candidate, err := s.runtime.Initialize(ctx, target)
	if err != nil {
		return YjsRestoreResult{}, err
	}
	if err = validateYjsCandidate(target, candidate); err != nil {
		return YjsRestoreResult{}, err
	}
	return store.CommitYjsRestore(ctx, a, base, input, candidate)
}

// ResolveRestore permits original-outcome lookup after losing write access.
func (s *YjsService) ResolveRestore(ctx context.Context, a identity.Principal, id uuid.UUID, input YjsRestore) (*YjsRestoreResult, error) {
	store, err := s.historyStore()
	if err != nil {
		return nil, err
	}
	return store.ResolveYjsRestore(ctx, a, id, input)
}
