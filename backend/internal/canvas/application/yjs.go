package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrYjsContention asks callers to re-read the durable fence.
var ErrYjsContention = errors.New("collaboration baseline changed")

// ErrYjsNotEnabled requires a revision-fenced document initialization.
var ErrYjsNotEnabled = errors.New("collaboration migration required")

// YjsBinding binds an isolated computation to a durable document baseline.
type YjsBinding struct {
	DocumentID uuid.UUID `json:"documentId"`
	Epoch      uuid.UUID `json:"epoch"`
	WriterTerm Revision  `json:"writerTerm"`
	CommitSeq  Revision  `json:"commitSeq"`
}

// YjsState is read atomically with its business projection.
type YjsState struct {
	RestorationNodes []NodeDTO `json:"-"`
	YjsBinding
	State     []byte      `json:"state"`
	GraphHash string      `json:"graphHash"`
	Document  DocumentDTO `json:"document"`
}

// YjsUpdate is immutable once dispatched; its identity survives reconnects.
type YjsUpdate struct {
	Epoch       uuid.UUID `json:"epoch"`
	SessionID   uuid.UUID `json:"sessionId"`
	UpdateID    uuid.UUID `json:"updateId"`
	Update      []byte    `json:"update"`
	PayloadHash string    `json:"payloadHash"`
	ReviewSeq   *Revision `json:"reviewSeq,omitempty"`
}

// YjsRestoration is a runtime-decoded intent; the store must prove the original deletion.
type YjsRestoration struct {
	ID              uuid.UUID   `json:"id"`
	Epoch           uuid.UUID   `json:"epoch"`
	SourceCommitSeq Revision    `json:"sourceCommitSeq"`
	Nodes           []uuid.UUID `json:"nodes"`
	Connections     []uuid.UUID `json:"connections"`
}

// YjsCandidate is a trusted runtime result, never a browser-supplied projection.
type YjsCandidate struct {
	AppliedUpdate []byte          `json:"appliedUpdate,omitempty"`
	Transition    *YjsRestoration `json:"transition,omitempty"`
	YjsBinding
	State       []byte      `json:"state"`
	StateHash   string      `json:"stateHash"`
	GraphHash   string      `json:"graphHash"`
	UpdateID    uuid.UUID   `json:"updateId,omitempty"`
	PayloadHash string      `json:"payloadHash,omitempty"`
	Effect      string      `json:"effect,omitempty"`
	Projection  DocumentDTO `json:"projection"`
}

// YjsReceipt is committed with the binary state, graph, and publication event.
type YjsReceipt struct {
	Epoch           uuid.UUID `json:"epoch"`
	CommitSeq       Revision  `json:"commitSeq"`
	ContentRevision Revision  `json:"contentRevision"`
	GraphHash       string    `json:"graphHash"`
	UpdateID        uuid.UUID `json:"updateId"`
	PayloadHash     string    `json:"payloadHash"`
	Effect          string    `json:"effect,omitempty"`
	Code            string    `json:"code,omitempty"`
	Status          string    `json:"status"`
	Update          []byte    `json:"update,omitempty"`
}

// YjsRuntime computes isolated candidates without database access.
type YjsRuntime interface {
	Initialize(context.Context, YjsState) (YjsCandidate, error)
	Prepare(context.Context, YjsState, YjsUpdate) (YjsCandidate, error)
}

// YjsStore owns authoritative permission checks and atomic acceptance.
type YjsStore interface {
	LoadYjs(context.Context, identity.Principal, uuid.UUID) (YjsState, error)
	InitializeYjs(context.Context, identity.Principal, YjsState, YjsCandidate) (YjsState, error)
	CommitYjs(context.Context, identity.Principal, YjsState, YjsUpdate, *YjsCandidate, string) (YjsReceipt, error)
	ResolveYjs(context.Context, identity.Principal, uuid.UUID, YjsUpdate) (*YjsReceipt, error)
}

// YjsRejection may be persisted only after rechecking the candidate baseline.
type YjsRejection struct{ Code string }

func (e *YjsRejection) Error() string { return e.Code }

// YjsService coordinates prepare outside locks and commit under the durable fence.
type YjsService struct {
	store   YjsStore
	runtime YjsRuntime
}

// NewYjsService injects authority and isolated computation.
func NewYjsService(store YjsStore, runtime YjsRuntime) *YjsService {
	return &YjsService{store: store, runtime: runtime}
}

// Load returns an authorized atomic state and projection.
func (s *YjsService) Load(ctx context.Context, a identity.Principal, id uuid.UUID) (YjsState, error) {
	return s.store.LoadYjs(ctx, a, id)
}

// Initialize converts a revision-fenced document exactly once.
func (s *YjsService) Initialize(ctx context.Context, a identity.Principal, id uuid.UUID, revision Revision) (YjsState, error) {
	base, err := s.store.LoadYjs(ctx, a, id)
	if err != nil {
		return YjsState{}, err
	}
	if len(base.State) > 0 {
		return base, nil
	}
	if base.Document.Revision != revision {
		return YjsState{}, ErrYjsContention
	}
	if !base.Document.Permissions.Write {
		return YjsState{}, domain.ErrGraphReadOnly
	}
	body, err := json.Marshal(base.Document)
	if err != nil {
		return YjsState{}, err
	}
	if len(body) > domain.MaxDocumentBytes {
		return YjsState{}, &domain.OperationError{Code: "canvas_capacity_exceeded"}
	}
	base.Epoch = uuid.New()
	base.WriterTerm = 1
	candidate, err := s.runtime.Initialize(ctx, base)
	if err != nil {
		return YjsState{}, err
	}
	if err = validateYjsCandidate(base, candidate); err != nil {
		return YjsState{}, err
	}
	return s.store.InitializeYjs(ctx, a, base, candidate)
}
func validYjsUpdate(update YjsUpdate) error {
	digest := sha256.Sum256(update.Update)
	if update.Epoch == uuid.Nil || update.SessionID == uuid.Nil || update.UpdateID == uuid.Nil || (update.ReviewSeq != nil && *update.ReviewSeq < 0) || len(update.Update) == 0 || len(update.Update) > 768<<10 || hex.EncodeToString(digest[:]) != update.PayloadHash {
		return domain.ErrInvalidCommand
	}
	return nil
}
func validateYjsCandidate(base YjsState, candidate YjsCandidate) error {
	digest := sha256.Sum256(candidate.State)
	if candidate.YjsBinding != base.YjsBinding || len(candidate.State) == 0 || len(candidate.State) > 8<<20 || hex.EncodeToString(digest[:]) != candidate.StateHash || len(candidate.GraphHash) != 64 {
		return fmt.Errorf("invalid runtime candidate binding")
	}
	return nil
}

// Resolve checks the immutable original identity under current read authority.
func (s *YjsService) Resolve(ctx context.Context, a identity.Principal, id uuid.UUID, update YjsUpdate) (*YjsReceipt, error) {
	if err := validYjsUpdate(update); err != nil {
		return nil, err
	}
	return s.store.ResolveYjs(ctx, a, id, update)
}

// Submit prepares outside the database lock and atomically accepts or rejects.
func (s *YjsService) Submit(ctx context.Context, a identity.Principal, id uuid.UUID, update YjsUpdate) (YjsReceipt, error) {
	if err := validYjsUpdate(update); err != nil {
		return YjsReceipt{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return YjsReceipt{}, err
		}
		receipt, err := s.store.ResolveYjs(ctx, a, id, update)
		if err != nil {
			return YjsReceipt{}, err
		}
		if receipt != nil {
			return *receipt, nil
		}
		base, err := s.store.LoadYjs(ctx, a, id)
		if err != nil {
			return YjsReceipt{}, err
		}
		if len(base.State) == 0 {
			return YjsReceipt{}, ErrYjsNotEnabled
		}
		code := ""
		var candidate *YjsCandidate
		switch {
		case base.Epoch != update.Epoch:
			code = "EPOCH_CHANGED"
		case update.ReviewSeq != nil && *update.ReviewSeq != base.CommitSeq:
			code = "REVIEW_STALE"
		default:
			prepared, prepareErr := s.runtime.Prepare(ctx, base, update)
			var rejected *YjsRejection
			switch {
			case errors.As(prepareErr, &rejected):
				code = rejected.Code
			case prepareErr != nil:
				return YjsReceipt{}, prepareErr
			default:
				if err := validateYjsCandidate(base, prepared); err != nil {
					return YjsReceipt{}, err
				}
				if prepared.UpdateID != update.UpdateID || prepared.PayloadHash != update.PayloadHash {
					return YjsReceipt{}, fmt.Errorf("runtime changed request identity")
				}
				candidate = &prepared
			}
		}
		result, err := s.store.CommitYjs(ctx, a, base, update, candidate, code)
		if errors.Is(err, ErrYjsContention) {
			continue
		}
		return result, err
	}
	return YjsReceipt{}, ErrYjsContention
}
