package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// The single accepted profile freezes code, model and output semantics.
const (
	DepthProfileID        = "vda-small-relative-v1"
	DepthVDACommit        = "4f5ae23172ba60fd7bc11ef671cca678842c7072"
	DepthModelSHA256      = "13379300b739e659f076a59d52e9801bd8d38c541a7e71f73bbca4dcfb013609"
	DepthSourceTreeSHA256 = "275d52984b0f60536f53e778c8d38b082a8ff945b174f49d449e6860d1ee0342"
)

// ErrInvalidDepthJob rejects corrupted frozen facts or execution transitions.
var ErrInvalidDepthJob = errors.New("invalid depth job")

// DepthStatus includes private output review and explicit cancellation.
type DepthStatus string

// Closed job statuses describe native processing and reviewed publication.
const (
	DepthQueued          DepthStatus = "queued"
	DepthRunning         DepthStatus = "running"
	DepthReviewRequired  DepthStatus = "review_required"
	DepthSucceeded       DepthStatus = "succeeded"
	DepthFailed          DepthStatus = "failed"
	DepthCancelRequested DepthStatus = "cancel_requested"
	DepthCancelled       DepthStatus = "cancelled"
)

// DepthProcessState distinguishes actual termination from a missing activity owner.
type DepthProcessState string

// Physical process states never use lease expiry as cessation evidence.
const (
	DepthProcessNone    DepthProcessState = "none"
	DepthProcessStarted DepthProcessState = "started"
	DepthProcessEnded   DepthProcessState = "ended"
	DepthProcessUnknown DepthProcessState = "unknown"
)

// DepthJob is the safe public projection; no output receipt or private key is serialized.
type DepthJob struct {
	ID                      uuid.UUID   `json:"id"`
	ProjectID               uuid.UUID   `json:"project_id"`
	Source                  Source      `json:"source"`
	SourceAssetID           uuid.UUID   `json:"source_asset_id"`
	SourceAssetRevision     int64       `json:"source_asset_revision"`
	SourceSHA256            string      `json:"source_sha256"`
	ProfileID               string      `json:"profile_id"`
	Status                  DepthStatus `json:"status"`
	Stage                   string      `json:"stage"`
	Attempt                 int         `json:"attempt"`
	Revision                int64       `json:"revision"`
	AssetID                 *uuid.UUID  `json:"asset_id" extensions:"x-nullable"`
	SHA256                  *string     `json:"sha256" extensions:"x-nullable"`
	FailureCode             *string     `json:"failure_code" extensions:"x-nullable"`
	Retryable               bool        `json:"retryable"`
	NeedsReconciliation     bool        `json:"needs_reconciliation"`
	ExecutionUnconfirmed    bool        `json:"execution_unconfirmed"`
	CancellationRequested   bool        `json:"cancellation_requested"`
	ReconciliationRequested bool        `json:"reconciliation_requested"`
	CreatedAt               time.Time   `json:"created_at"`
	UpdatedAt               time.Time   `json:"updated_at"`
}

// FrozenDepth contains only server-selected formal source facts and one fixed profile.
type FrozenDepth struct {
	ProfileID string       `json:"profile_id"`
	Input     FrozenSource `json:"input"`
}

// ValidDepthSHA accepts canonical full content hashes.
func ValidDepthSHA(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Validate refuses oversized or incomplete sources instead of truncating them.
func (f FrozenDepth) Validate() error {
	i := f.Input
	if f.ProfileID != DepthProfileID || i.AssetID == uuid.Nil || i.Revision < 1 || i.Kind != "video" || i.ObjectKey == "" || !strings.HasPrefix(i.MIMEType, "video/") || !ValidDepthSHA(i.SHA256) || i.ByteSize < 1 || i.ByteSize > 500<<20 || i.DurationMS == nil || *i.DurationMS < 1 || *i.DurationMS > 15100 || i.Width == nil || i.Height == nil || *i.Width < 1 || *i.Height < 1 {
		return ErrInvalidDepthJob
	}
	return nil
}

// DepthExecution is a private worker fence around the safe job projection.
type DepthExecution struct {
	Job          DepthJob
	WorkerID     uuid.UUID
	ProcessState DepthProcessState
}

// Claim never adopts an old process or restarts an unknown inference.
func (s *DepthExecution) Claim(worker uuid.UUID, reconcile bool) error {
	if worker == uuid.Nil || s.WorkerID != uuid.Nil || s.Job.ExecutionUnconfirmed {
		return ErrInvalidDepthJob
	}
	if reconcile {
		if s.Job.Status != DepthFailed || !s.Job.NeedsReconciliation || s.ProcessState != DepthProcessEnded {
			return ErrInvalidDepthJob
		}
		s.Job.Status = DepthRunning
		s.Job.Stage = "reconciling"
	} else {
		if (s.Job.Status != DepthQueued && s.Job.Status != DepthCancelRequested) || s.Job.NeedsReconciliation {
			return ErrInvalidDepthJob
		}
		if s.ProcessState != "" && s.ProcessState != DepthProcessNone && s.ProcessState != DepthProcessEnded {
			return ErrInvalidDepthJob
		}
		if s.Job.CancellationRequested {
			s.Job.Status = DepthCancelRequested
			s.Job.Stage = "cleanup"
		} else {
			s.Job.Status = DepthRunning
			s.Job.Stage = "downloading"
		}
	}
	s.WorkerID = worker
	s.Job.Revision++
	return nil
}

// StartProcess persists dispatch before invoking the native processor.
func (s *DepthExecution) StartProcess(worker uuid.UUID) error {
	if worker != s.WorkerID || worker == uuid.Nil || s.Job.Status != DepthRunning || s.Job.CancellationRequested || (s.ProcessState != "" && s.ProcessState != DepthProcessNone) {
		return ErrInvalidDepthJob
	}
	s.ProcessState = DepthProcessStarted
	s.Job.Revision++
	return nil
}

// EndProcess acknowledges the original synchronous call's actual exit only.
func (s *DepthExecution) EndProcess(worker uuid.UUID) error {
	if worker != s.WorkerID || worker == uuid.Nil {
		return ErrInvalidDepthJob
	}
	s.ProcessState = DepthProcessEnded
	s.Job.ExecutionUnconfirmed = false
	s.Job.Revision++
	return nil
}

// Interrupt keeps the original owner until physical execution proves its exit.
func (s *DepthExecution) Interrupt(worker uuid.UUID) error {
	if worker == uuid.Nil || worker != s.WorkerID {
		return ErrInvalidDepthJob
	}
	if s.ProcessState != DepthProcessEnded {
		s.ProcessState = DepthProcessUnknown
	}
	s.Job.Status = DepthFailed
	s.Job.Stage = "awaiting_reconciliation"
	s.Job.NeedsReconciliation = true
	s.Job.ExecutionUnconfirmed = s.ProcessState != DepthProcessEnded
	s.Job.Retryable = false
	s.Job.Revision++
	return nil
}

// RequestCancel records intent without manufacturing native cessation.
func (s *DepthExecution) RequestCancel() error {
	switch s.Job.Status {
	case DepthQueued, DepthRunning, DepthReviewRequired, DepthFailed, DepthCancelRequested:
	default:
		return ErrInvalidDepthJob
	}
	s.Job.CancellationRequested = true
	if !s.Job.NeedsReconciliation {
		s.Job.Status = DepthCancelRequested
		s.Job.Stage = "cancelling"
	}
	s.Job.Revision++
	return nil
}

// FinishCancelled requires a caller that already verified all output cleanup.
func (s *DepthExecution) FinishCancelled(worker uuid.UUID) error {
	if worker == uuid.Nil || worker != s.WorkerID || !s.Job.CancellationRequested || s.Job.ExecutionUnconfirmed || (s.ProcessState != DepthProcessNone && s.ProcessState != "" && s.ProcessState != DepthProcessEnded) {
		return ErrInvalidDepthJob
	}
	s.WorkerID = uuid.Nil
	s.Job.Status = DepthCancelled
	s.Job.Stage = "cancelled"
	s.Job.NeedsReconciliation = false
	s.Job.Retryable = true
	s.Job.FailureCode = nil
	s.Job.Revision++
	return nil
}

// Retry creates a new fenced attempt only after all prior work is known stopped.
func (s *DepthExecution) Retry() error {
	if (s.Job.Status != DepthFailed && s.Job.Status != DepthCancelled) || !s.Job.Retryable || s.Job.NeedsReconciliation || s.Job.ExecutionUnconfirmed || s.WorkerID != uuid.Nil || s.Job.Attempt >= 100 {
		return ErrInvalidDepthJob
	}
	s.Job.Status = DepthQueued
	s.Job.Stage = "queued"
	s.Job.Attempt++
	s.Job.Revision++
	s.ProcessState = DepthProcessNone
	s.Job.AssetID = nil
	s.Job.SHA256 = nil
	s.Job.FailureCode = nil
	s.Job.CancellationRequested = false
	s.Job.Retryable = false
	return nil
}
