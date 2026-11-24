// Package application owns the new AI admission, dispatch and money lifecycle.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// Dispatch is issued only after request, hold, route and attempt commit together.
type Dispatch struct {
	PollTask string
	Call     domain.Call
	Attempt  uuid.UUID
	Route    domain.Route
	Record   domain.Record
	Send     bool
}

// Store is the only owner of AI funds and irreversible send claims.
type Store interface {
	Model(context.Context, string, string) (domain.Model, error)
	Admit(context.Context, domain.Call) (Dispatch, error)
	SaveReceipt(context.Context, uuid.UUID, uuid.UUID, domain.Receipt) error
	SavePending(context.Context, Dispatch, domain.Receipt) (domain.Record, error)
	Settle(context.Context, uuid.UUID) (domain.Record, error)
	Unknown(context.Context, uuid.UUID, uuid.UUID) error
	Backup(context.Context, Dispatch) (Dispatch, error)
	Read(context.Context, uuid.UUID, uuid.UUID) (domain.Record, error)
	ReleaseNotSent(context.Context, Dispatch) (domain.Record, error)
}

// Provider handles one native request. ErrNotSent is permitted only with proof.
type Provider interface {
	Send(context.Context, Dispatch, func(json.RawMessage) error) (domain.Receipt, error)
}

// Service coordinates persistence, one supplier attempt and recoverable settlement.
type Service struct {
	store    Store
	provider Provider
}

// New composes the single admission, send and settlement lifecycle.
func New(store Store, provider Provider) *Service { return &Service{store: store, provider: provider} }

// Execute never retries an unknown result, even when the client reconnects.
func (s *Service) Execute(ctx context.Context, call domain.Call, emit func(json.RawMessage) error) (domain.Record, error) {
	if s.store == nil || s.provider == nil {
		return domain.Record{}, domain.ErrUnavailable
	}
	d, err := s.store.Admit(ctx, call)
	if err != nil {
		return domain.Record{}, err
	}
	if !d.Send {
		if d.Record.State == "received" {
			return s.store.Settle(ctx, d.Record.ID)
		}
		return d.Record, nil
	}
	return s.ExecuteDispatch(ctx, d, emit)
}

// ExecuteDispatch runs an already committed one-time dispatch claim.
func (s *Service) ExecuteDispatch(ctx context.Context, d Dispatch, emit func(json.RawMessage) error) (domain.Record, error) {
	call := d.Call
	var receipt domain.Receipt
	var err error
	if s.provider == nil {
		return s.store.ReleaseNotSent(ctx, d)
	}
	deadline := call.Deadline
	if latest := time.Now().Add(180 * time.Second); deadline.After(latest) {
		deadline = latest
	}
	sendCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	receipt, err = s.provider.Send(sendCtx, d, emit)
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer stop()
	if errors.Is(err, ErrNotSent) {
		next, backupErr := s.store.Backup(saveCtx, d)
		stop()
		if backupErr == nil && next.Send {
			d = next
			receipt, err = s.provider.Send(sendCtx, d, emit)
		}
		saveCtx, stop = context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer stop()
		if backupErr != nil || errors.Is(err, ErrNotSent) {
			record, releaseErr := s.store.ReleaseNotSent(saveCtx, d)
			if releaseErr != nil {
				return record, releaseErr
			}
			return record, domain.ErrUnavailable
		}
	}
	if err != nil {
		_ = s.store.Unknown(saveCtx, d.Call.ID, d.Attempt)
		return domain.Record{ID: call.ID, State: "unknown"}, domain.ErrUnknown
	}
	if receipt.Pending {
		return s.store.SavePending(saveCtx, d, receipt)
	}
	if err = s.store.SaveReceipt(saveCtx, d.Call.ID, d.Attempt, receipt); err != nil {
		_ = s.store.Unknown(saveCtx, d.Call.ID, d.Attempt)
		return domain.Record{ID: call.ID, State: "unknown"}, err
	}
	return s.store.Settle(saveCtx, d.Call.ID)
}

// ErrNotSent carries the transport's affirmative evidence that no connection existed.
var ErrNotSent = notSentError{}

type notSentError struct{}

func (notSentError) Error() string { return "AI provider request was not sent" }
