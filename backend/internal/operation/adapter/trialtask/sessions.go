// Package trialtask persists development trials and executes their original task references.
package trialtask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Trial task kinds distinguish cloud provider execution from local Codex execution.
const (
	Cloud = "trial.cloud"
	Codex = "trial.codex"
)

// Trial errors distinguish missing sessions from conflicting saved requests.
var (
	ErrMissing  = errors.New("trial not found")
	ErrConflict = errors.New("trial request conflict")
)

// OutputAdmission pins a physical trial output in the caller's admission transaction.
type OutputAdmission func(context.Context, *gorm.DB, application.TrialInput) (mediadomain.ObjectRef, error)

// Sessions persists trial intent, execution state and task references.
type Sessions struct {
	db     *gorm.DB
	output OutputAdmission
}

// WithOutputAdmission injects the transaction-bound physical output admission function.
func (s *Sessions) WithOutputAdmission(f OutputAdmission) *Sessions { s.output = f; return s }

// NewSessions injects the database that owns durable trial sessions.
func NewSessions(db *gorm.DB) *Sessions { return &Sessions{db: db} }

type session struct {
	ID         string
	Input      application.TrialInput
	State      application.TrialState
	Generation int64
	Phase      string
	Polls      int
	UpdatedAt  time.Time
}
type trialRef struct {
	ID         string
	Generation int64
}

func read(tx *gorm.DB, id string, lock bool) (session, error) {
	var row struct {
		Input, State []byte
		Generation   int64
		Phase        string
		Polls        int
		UpdatedAt    time.Time
	}
	query := "SELECT input,state,generation,phase,polls,updated_at FROM operation.development_trial WHERE id=?"
	if lock {
		query += " FOR UPDATE"
	}
	result := tx.Raw(query, id).Scan(&row)
	if result.Error != nil {
		return session{}, result.Error
	}
	if result.RowsAffected != 1 {
		return session{}, ErrMissing
	}
	out := session{ID: id, Generation: row.Generation, Phase: row.Phase, Polls: row.Polls, UpdatedAt: row.UpdatedAt}
	if err := json.Unmarshal(row.Input, &out.Input); err != nil {
		return out, err
	}
	return out, json.Unmarshal(row.State, &out.State)
}
func (row *session) save(tx *gorm.DB) error {
	input, err := json.Marshal(row.Input)
	if err != nil {
		return err
	}
	state, err := json.Marshal(row.State)
	if err != nil {
		return err
	}
	return tx.Exec("UPDATE operation.development_trial SET input=?::jsonb,state=?::jsonb,generation=?,phase=?,polls=?,updated_at=clock_timestamp() WHERE id=?", string(input), string(state), row.Generation, row.Phase, row.Polls, row.ID).Error
}
func (row *session) enqueue(ctx context.Context, tx *gorm.DB) error {
	kind := Cloud
	if row.Input.Request.Source == "codex" {
		kind = Codex
	}
	return taskruntime.EnqueueReference(ctx, tx, kind, fmt.Sprintf("%s/%d", row.ID, row.Generation), time.Now(), trialRef{row.ID, row.Generation})
}

// Start admits a trial and its initial task reference.
func (s *Sessions) Start(ctx context.Context, input application.TrialInput) (application.TrialState, error) {
	req := input.Request
	row := session{ID: application.TrialID(input.Actor, req.RequestID), Input: input, Phase: "ready", Generation: 1, State: application.TrialState{Fingerprint: req.Fingerprint(), Source: req.Source, ProviderID: req.ProviderID, Kind: req.Kind, Model: req.Model, State: "running", Turns: []application.TrialTurn{{RequestID: req.RequestID, Prompt: req.Prompt, TrialProgress: application.TrialProgress{State: "running"}}}}}
	if err := req.Validate(); err != nil {
		return row.State, err
	}
	stateRaw, err := json.Marshal(row.State)
	if err != nil {
		return row.State, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, readErr := read(tx, row.ID, false)
		if readErr == nil {
			if existing.State.Fingerprint != row.State.Fingerprint {
				return ErrConflict
			}
			row = existing
			return nil
		}
		if !errors.Is(readErr, ErrMissing) {
			return readErr
		}
		// Only the durable original intent selects its output destination. Replay
		// returns before consulting an active route that may have changed.
		input.OutputRef = nil
		if req.Kind != "chat" && req.Kind != "probe" {
			if s.output == nil {
				return application.ErrInvalidTrial
			}
			ref, e := s.output(ctx, tx, input)
			if e != nil {
				return e
			}
			if ref.Validate() != nil || ref.Key != application.TrialObjectKey(input, 1) {
				return application.ErrInvalidTrial
			}
			input.OutputRef = &ref
		}
		row.Input = input
		inputRaw, e := json.Marshal(input)
		if e != nil {
			return e
		}
		r := tx.Exec("INSERT INTO operation.development_trial(id,input,state,phase) VALUES(?,?::jsonb,?::jsonb,'ready') ON CONFLICT(id) DO NOTHING", row.ID, string(inputRaw), string(stateRaw))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 1 {
			return row.enqueue(ctx, tx)
		}
		existing, err := read(tx, row.ID, false)
		if err != nil {
			return err
		}
		if existing.State.Fingerprint != row.State.Fingerprint {
			return ErrConflict
		}
		row = existing
		return nil
	})
	return row.State, err
}

// State reads the saved trial state by its durable identity.
func (s *Sessions) State(ctx context.Context, id string) (application.TrialState, error) {
	row, err := read(s.db.WithContext(ctx), id, false)
	if err == nil && row.State.Kind == "chat" && row.State.State == "succeeded" && time.Since(row.UpdatedAt) >= 24*time.Hour {
		row.State.State = "closed"
	}
	return row.State, err
}

// Turn records a message for the existing trial session.
func (s *Sessions) Turn(ctx context.Context, id string, msg application.TrialMessage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := read(tx, id, true)
		if err != nil {
			return err
		}
		for _, turn := range row.State.Turns {
			if turn.RequestID == msg.RequestID {
				if turn.Prompt != msg.Prompt {
					return ErrConflict
				}
				return nil
			}
		}
		if row.State.Kind != "chat" || row.State.State != "succeeded" || len(row.State.Turns) >= 32 || time.Since(row.UpdatedAt) >= 24*time.Hour {
			return ErrConflict
		}
		row.Input.Request.RequestID = msg.RequestID
		row.Input.Request.Prompt = msg.Prompt
		row.Input.Action = "submit"
		if err := row.Input.Request.Validate(); err != nil {
			return err
		}
		row.Input.ThreadID = row.State.Turns[len(row.State.Turns)-1].ThreadID
		row.State.Turns = append(row.State.Turns, application.TrialTurn{RequestID: msg.RequestID, Prompt: msg.Prompt, TrialProgress: application.TrialProgress{State: "running"}})
		row.Phase = "ready"
		row.State.State = "running"
		row.Generation++
		row.Polls = 0
		if err := row.save(tx); err != nil {
			return err
		}
		return row.enqueue(ctx, tx)
	})
}

// Cancel records cancellation of an existing trial session.
func (s *Sessions) Cancel(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := read(tx, id, true)
		if err != nil {
			return err
		}
		if row.Phase == "done" {
			if row.State.Kind == "chat" && row.State.State == "succeeded" {
				row.State.State = "closed"
				return row.save(tx)
			}
			return nil
		}
		kind := Cloud
		if row.Input.Request.Source == "codex" {
			kind = Codex
		}
		if err := taskruntime.RequestStop(ctx, tx, kind, trialRef{ID: id, Generation: row.Generation}); err != nil {
			return err
		}
		row.Phase = "done"
		row.State.State = "unknown"
		turn := &row.State.Turns[len(row.State.Turns)-1]
		turn.State = "unknown"
		turn.ErrorCode = "trial_cancel_unconfirmed"
		return row.save(tx)
	})
}

// Recover schedules recovery of an existing trial session.
func (s *Sessions) Recover(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := read(tx, id, true)
		if err != nil {
			return err
		}
		if row.Input.Action == "recover" && row.Phase != "done" {
			return nil
		}
		if row.State.State != "unknown" || len(row.State.Turns) == 0 || row.State.Kind != "image" && row.State.Kind != "video" {
			return ErrConflict
		}
		turn := row.State.Turns[len(row.State.Turns)-1]
		row.Input.Action = "recover"
		row.Input.TaskID = turn.TaskID
		row.Input.ConfigVersionID = turn.ConfigVersionID
		row.Input.CredentialID = turn.CredentialID
		row.Input.OriginalFingerprint = row.State.Fingerprint
		row.Phase = "ready"
		row.State.State = "running"
		row.Generation++
		row.Polls = 0
		if err := row.save(tx); err != nil {
			return err
		}
		return row.enqueue(ctx, tx)
	})
}

// Executor executes trial submission and recovery against saved trial state.
type Executor interface {
	Run(context.Context, application.TrialInput) (application.TrialProgress, error)
}

// Handler executes the saved trial steps with the configured executor.
func (s *Sessions) Handler(executor Executor) taskruntime.Handler {
	return func(ctx context.Context, j taskruntime.Job) (time.Time, error) {
		var ref trialRef
		if err := json.Unmarshal(j.Reference, &ref); err != nil {
			return time.Time{}, err
		}
		var input application.TrialInput
		var run bool
		err := taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
			row, err := read(tx, ref.ID, true)
			if err != nil {
				return err
			}
			if row.Generation != ref.Generation || row.Phase == "done" {
				return nil
			}
			if row.Phase == "dispatching" {
				row.Phase = "done"
				row.State.State = "unknown"
				turn := &row.State.Turns[len(row.State.Turns)-1]
				turn.State = "unknown"
				turn.ErrorCode = "trial_dispatch_unknown"
				return row.save(tx)
			}
			input = row.Input
			if input.Action == "" {
				input.Action = "submit"
			}
			if input.Action == "submit" {
				row.Phase = "dispatching"
				if err := row.save(tx); err != nil {
					return err
				}
			}
			run = true
			return nil
		})
		if err != nil || !run {
			return time.Time{}, err
		}
		result, err := executor.Run(ctx, input)
		if ctx.Err() != nil {
			return time.Time{}, ctx.Err()
		}
		if err != nil {
			result = application.TrialProgress{State: "unknown", ErrorCode: "trial_dispatch_unknown", TaskID: input.TaskID, CredentialID: input.CredentialID, ConfigVersionID: input.ConfigVersionID}
		}
		var next time.Time
		err = taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
			row, err := read(tx, ref.ID, true)
			if err != nil {
				return err
			}
			if row.Generation != ref.Generation {
				return nil
			}
			turn := &row.State.Turns[len(row.State.Turns)-1]
			if row.Phase == "done" {
				if row.State.State == "unknown" && result.TaskID != "" && turn.TaskID == "" {
					turn.TaskID = result.TaskID
					turn.ConfigVersionID = result.ConfigVersionID
					turn.CredentialID = result.CredentialID
					return row.save(tx)
				}
				return nil
			}
			row.Phase = "done"
			if result.State == "pending" && result.TaskID != "" {
				row.Polls++
				if row.Polls > 240 {
					result.State = "unknown"
					result.ErrorCode = "trial_query_timeout"
				} else {
					row.Input.Action = "query"
					row.Input.TaskID = result.TaskID
					row.Input.ConfigVersionID = result.ConfigVersionID
					row.Input.CredentialID = result.CredentialID
					row.Phase = "poll"
					next = time.Now().Add(5 * time.Second)
				}
			}
			turn.TrialProgress = result
			row.State.State = result.State
			return row.save(tx)
		})
		return next, err
	}
}
