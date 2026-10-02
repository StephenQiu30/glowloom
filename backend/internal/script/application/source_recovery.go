package application

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// SourceWriteIntent exposes only current authorized recovery metadata.
type SourceWriteIntent struct {
	ID                     uuid.UUID  `json:"id"`
	Revision               int64      `json:"revision"`
	Action                 string     `json:"action"`
	Status                 string     `json:"status"`
	ExpectedScriptRevision int64      `json:"expected_script_revision"`
	BaseVersionID          *uuid.UUID `json:"base_version_id,omitempty"`
	ObjectCount            int        `json:"object_count"`
	ConfirmedObjectCount   int        `json:"confirmed_object_count"`
	CancellationRequested  bool       `json:"cancellation_requested"`
	NeedsReconciliation    bool       `json:"needs_reconciliation"`
	ActiveIO               bool       `json:"active_io"`
	CanControl             bool       `json:"can_control"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// SourceWritePage includes only the authenticated current caller's own scope.
type SourceWritePage struct {
	CurrentActorID uuid.UUID           `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID           `json:"current_org_id"`
	Items          []SourceWriteIntent `json:"items"`
	NextAfter      *int64              `json:"next_after,omitempty"`
}

// SourceControlCommand permanently binds one explicit intent control and its CAS.
type SourceControlCommand struct {
	ProjectID        uuid.UUID `json:"project_id"`
	IntentID         uuid.UUID `json:"intent_id"`
	Key              uuid.UUID `json:"key"`
	RequestID        uuid.UUID `json:"request_id"`
	Action           string    `json:"action"`
	ExpectedRevision int64     `json:"expected_revision"`
}

// SourceControlReceipt is the immutable acceptance response, not cleanup success.
type SourceControlReceipt struct {
	IntentID uuid.UUID `json:"intent_id"`
	Revision int64     `json:"revision"`
	Action   string    `json:"action"`
	Accepted bool      `json:"accepted"`
}

// SourceIntentRecord retains private proof for owning cleanup, never HTTP serialization.
type SourceIntentRecord struct {
	View    SourceWriteIntent
	Plan    WritePlan
	OwnerID *uuid.UUID
	IOState string
	Objects []SourceObjectState
}

// SourceObjectState distinguishes absent untouched keys from an uncertain mutation.
type SourceObjectState struct {
	Fact       domain.ObjectFact
	PutStarted bool
	Confirmed  bool
	Removed    bool
}

// SourceRecoveryStore owns permanent controls and exact unpublished-object fences.
type SourceRecoveryStore interface {
	ListSourceWrites(context.Context, identityapp.Principal, uuid.UUID, int64, int) (SourceWritePage, error)
	SourceWrite(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (SourceIntentRecord, error)
	ControlSourceWrite(context.Context, identityapp.Principal, SourceControlCommand, time.Time) (SourceControlReceipt, error)
	ProveSourceStop(context.Context, identityapp.Principal, SourceControlCommand, uuid.UUID, time.Time) error
	AuthorizeSourceRemoval(context.Context, identityapp.Principal, SourceControlCommand, domain.ObjectFact) error
	BeginSourceRemoval(context.Context, identityapp.Principal, SourceControlCommand, domain.ObjectFact, time.Time) error
	ConfirmSourceRemoval(context.Context, identityapp.Principal, SourceControlCommand, domain.ObjectFact, time.Time) error
	FinishSourceControl(context.Context, identityapp.Principal, SourceControlCommand, bool, time.Time) error
}

// SourceIOStore records only this synchronous call's actual I/O ownership and exit.
type SourceIOStore interface {
	ClaimSourceIO(context.Context, identityapp.Principal, WritePlan, uuid.UUID, time.Time) error
	BeginSourceObject(context.Context, identityapp.Principal, WritePlan, uuid.UUID, domain.ObjectFact) error
	ConfirmSourceObject(context.Context, identityapp.Principal, WritePlan, uuid.UUID, domain.ObjectFact) error
	EndSourceIO(context.Context, WritePlan, uuid.UUID, bool, time.Time) error
}

type sourceIO struct {
	cancel context.CancelFunc
	done   chan struct{}
}
type sourceIORegistry struct {
	mu      sync.Mutex
	entries map[uuid.UUID]*sourceIO
}

func (r *sourceIORegistry) begin(ctx context.Context, id uuid.UUID) (context.Context, *sourceIO) {
	ioCtx, cancel := context.WithCancel(ctx)
	entry := &sourceIO{cancel: cancel, done: make(chan struct{})}
	r.mu.Lock()
	r.entries[id] = entry
	r.mu.Unlock()
	return ioCtx, entry
}
func (r *sourceIORegistry) end(id uuid.UUID, entry *sourceIO, recorded bool) {
	entry.cancel()
	close(entry.done)
	if recorded {
		r.mu.Lock()
		delete(r.entries, id)
		r.mu.Unlock()
	}
}
func (r *sourceIORegistry) stop(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	entry := r.entries[id]
	r.mu.Unlock()
	if entry == nil {
		return ErrNeedsReconciliation
	}
	entry.cancel()
	select {
	case <-entry.done:
		return nil
	case <-ctx.Done():
		return errors.Join(ErrNeedsReconciliation, ctx.Err())
	}
}

func (r *sourceIORegistry) forget(id uuid.UUID) { r.mu.Lock(); delete(r.entries, id); r.mu.Unlock() }

// SourceRecovery cancels only its own synchronous I/O and verifies the frozen bytes.
type SourceRecovery struct {
	store   SourceRecoveryStore
	sources *SourceService
	now     func() time.Time
}

// NewSourceRecovery must share the exact SourceService instance owning active I/O.
func NewSourceRecovery(store SourceRecoveryStore, sources *SourceService, now func() time.Time) *SourceRecovery {
	return &SourceRecovery{store: store, sources: sources, now: now}
}

// List allows refresh recovery without retaining private bodies or object paths.
func (s *SourceRecovery) List(ctx context.Context, actor identityapp.Principal, project uuid.UUID, after int64, limit int) (SourceWritePage, error) {
	if s == nil || s.store == nil {
		return SourceWritePage{}, ErrUnavailable
	}
	if after < 0 || after > 100000 || limit < 1 || limit > 100 {
		return SourceWritePage{}, domain.ErrInvalidSource
	}
	return s.store.ListSourceWrites(ctx, actor, project, after, limit)
}

// Get returns current state after current project authorization.
func (s *SourceRecovery) Get(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (SourceWriteIntent, error) {
	if s == nil || s.store == nil {
		return SourceWriteIntent{}, ErrUnavailable
	}
	r, err := s.store.SourceWrite(ctx, actor, project, id)
	return r.View, err
}

// Control first commits the explicit intent, then attempts bounded owned recovery.
// Its permanent response remains accepted even when physical proof stays unavailable.
func (s *SourceRecovery) Control(ctx context.Context, actor identityapp.Principal, input SourceControlCommand) (SourceControlReceipt, error) {
	if s == nil || s.store == nil || s.sources == nil || s.sources.objects == nil || s.sources.io == nil || s.now == nil {
		return SourceControlReceipt{}, ErrUnavailable
	}
	if input.ProjectID == uuid.Nil || input.IntentID == uuid.Nil || input.Key == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 1 || (input.Action != "cancel" && input.Action != "reconcile") {
		return SourceControlReceipt{}, domain.ErrInvalidSource
	}
	receipt, err := s.store.ControlSourceWrite(ctx, actor, input, s.now().UTC())
	if err != nil {
		return SourceControlReceipt{}, err
	}
	// This work has no detached lifetime; a timed-out call retains its durable fence.
	recoverCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = s.recover(recoverCtx, actor, input)
	if err != nil {
		recordErr := s.store.FinishSourceControl(recoverCtx, actor, input, true, s.now().UTC())
		if recordErr != nil {
			return receipt, nil
		}
	}
	return receipt, nil
}

func (s *SourceRecovery) recover(ctx context.Context, actor identityapp.Principal, input SourceControlCommand) error {
	r, err := s.store.SourceWrite(ctx, actor, input.ProjectID, input.IntentID)
	if err != nil {
		return err
	}
	if r.View.Status == "completed" || r.View.Status == "cancelled" {
		return nil
	}
	if r.OwnerID != nil {
		if err := s.sources.io.stop(ctx, *r.OwnerID); err != nil {
			return err
		}
		if err := s.store.ProveSourceStop(ctx, actor, input, *r.OwnerID, s.now().UTC()); err != nil {
			return err
		}
		s.sources.io.forget(*r.OwnerID)
		r, err = s.store.SourceWrite(ctx, actor, input.ProjectID, input.IntentID)
		if err != nil {
			return err
		}
	}
	if r.IOState != "idle" && r.IOState != "ended" {
		return ErrNeedsReconciliation
	}
	if !r.View.CancellationRequested {
		return s.store.FinishSourceControl(ctx, actor, input, false, s.now().UTC())
	}
	for _, object := range r.Objects {
		if object.Removed {
			continue
		}
		if err := s.store.AuthorizeSourceRemoval(ctx, actor, input, object.Fact); err != nil {
			return err
		}
		_, readErr := s.sources.readObject(ctx, object.Fact)
		if readErr != nil && !errors.Is(readErr, ErrObjectMissing) {
			return readErr
		}
		if errors.Is(readErr, ErrObjectMissing) && object.PutStarted && !object.Confirmed {
			return ErrNeedsReconciliation
		}
		if readErr == nil {
			if err := s.store.BeginSourceRemoval(ctx, actor, input, object.Fact, s.now().UTC()); err != nil {
				return err
			}
			removeErr := s.sources.objects.Remove(ctx, object.Fact.Key)
			_, afterErr := s.sources.readObject(ctx, object.Fact)
			if !errors.Is(afterErr, ErrObjectMissing) {
				return errors.Join(ErrNeedsReconciliation, removeErr, afterErr)
			}
		}
		if err := s.store.ConfirmSourceRemoval(ctx, actor, input, object.Fact, s.now().UTC()); err != nil {
			return err
		}
	}
	return s.store.FinishSourceControl(ctx, actor, input, false, s.now().UTC())
}
