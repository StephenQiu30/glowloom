package domain

import (
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidProjectCopy rejects incomplete frozen identities or receipts.
	ErrInvalidProjectCopy = errors.New("invalid project copy")
	// ErrProjectCopyStateConflict rejects unverified publication and unsafe recovery.
	ErrProjectCopyStateConflict = errors.New("project copy state conflict")
	// ErrProjectCopyWorkerConflict fences stale workers from changing a copy.
	ErrProjectCopyWorkerConflict = errors.New("project copy worker conflict")
)

// ProjectCopyManifest records owning-module snapshot identities, not their private contents.
type ProjectCopyManifest struct {
	WorkspaceSHA256  string    `json:"workspace_sha256"`
	CanvasSnapshotID uuid.UUID `json:"canvas_snapshot_id"`
	CanvasSHA256     string    `json:"canvas_sha256"`
	Documents        int       `json:"documents"`
	MediaSnapshotID  uuid.UUID `json:"media_snapshot_id"`
	MediaSHA256      string    `json:"media_sha256"`
	Assets           int       `json:"assets"`
	Renditions       int       `json:"renditions"`
}

// ProjectCopyReceipt binds exact counts and the resulting content to one frozen module.
type ProjectCopyReceipt struct {
	ManifestSHA256 string `json:"manifest_sha256"`
	ContentSHA256  string `json:"content_sha256"`
	PrimaryCount   int    `json:"primary_count"`
	SecondaryCount int    `json:"secondary_count"`
}

// ProjectCopyJob describes one project copy; it contains no provider or financial history.
type ProjectCopyJob struct {
	RequestID               string
	ID                      uuid.UUID
	OrgID                   uuid.UUID
	ActorID                 uuid.UUID
	SourceProjectID         uuid.UUID
	SourceRevision          int64
	TargetProjectID         uuid.UUID
	TargetName              string
	Status                  string
	Stage                   string
	Revision                int64
	Attempt                 int64
	WorkerID                uuid.UUID
	StartedAt               *time.Time
	Manifest                ProjectCopyManifest
	MediaReceipt            *ProjectCopyReceipt
	CanvasReceipt           *ProjectCopyReceipt
	FailureCode             string
	Retryable               bool
	NeedsReconciliation     bool
	CancellationRequested   bool
	ReconciliationRequested bool
	ExecutionUnconfirmed    bool
}

func copyDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

// Validate checks identities, immutable snapshot evidence and lifecycle counters.
func (j ProjectCopyJob) Validate() error {
	m := j.Manifest
	request, requestErr := uuid.Parse(j.RequestID)
	if requestErr != nil || request == uuid.Nil || request.String() != j.RequestID {
		return ErrInvalidProjectCopy
	}
	if j.ID == uuid.Nil || j.OrgID == uuid.Nil || j.ActorID == uuid.Nil || j.SourceProjectID == uuid.Nil || j.TargetProjectID == uuid.Nil || j.SourceProjectID == j.TargetProjectID || j.SourceRevision < 1 || j.SourceRevision > math.MaxInt32 || j.Revision < 1 || j.Revision > math.MaxInt32 || j.Attempt < 0 || j.Attempt > math.MaxInt32 || !utf8.ValidString(j.TargetName) || strings.TrimSpace(j.TargetName) == "" || utf8.RuneCountInString(j.TargetName) > 50 || strings.ContainsRune(j.TargetName, '\x00') || !copyDigest(m.WorkspaceSHA256) || !copyDigest(m.CanvasSHA256) || !copyDigest(m.MediaSHA256) || m.CanvasSnapshotID == uuid.Nil || m.MediaSnapshotID == uuid.Nil || m.Documents < 0 || m.Assets < 0 || m.Renditions < 0 {
		return ErrInvalidProjectCopy
	}
	switch j.Status {
	case "queued", "running", "failed", "cancel_requested", "cancelled", "succeeded":
	default:
		return ErrInvalidProjectCopy
	}
	switch j.Stage {
	case "media", "canvases", "finalizing", "cleanup", "complete":
	default:
		return ErrInvalidProjectCopy
	}
	if j.MediaReceipt != nil && !validCopyReceipt(*j.MediaReceipt, m.MediaSHA256, m.Assets, m.Renditions) {
		return ErrInvalidProjectCopy
	}
	if j.CanvasReceipt != nil && (!validCopyReceipt(*j.CanvasReceipt, m.CanvasSHA256, m.Documents, j.CanvasReceipt.SecondaryCount) || j.MediaReceipt == nil) {
		return ErrInvalidProjectCopy
	}
	if (j.Stage == "canvases" && j.MediaReceipt == nil) || (j.Stage == "finalizing" && (j.MediaReceipt == nil || j.CanvasReceipt == nil)) || (j.Stage == "media" && (j.MediaReceipt != nil || j.CanvasReceipt != nil)) {
		return ErrInvalidProjectCopy
	}
	if j.NeedsReconciliation && (j.Status != "failed" || j.Retryable) || j.ReconciliationRequested && (!j.NeedsReconciliation || j.ExecutionUnconfirmed) || j.ExecutionUnconfirmed && (!j.NeedsReconciliation || j.WorkerID == uuid.Nil) {
		return ErrInvalidProjectCopy
	}
	switch j.Status {
	case "queued":
		if j.WorkerID != uuid.Nil || j.CancellationRequested || j.Stage == "cleanup" || j.Stage == "complete" {
			return ErrInvalidProjectCopy
		}
	case "running":
		if j.WorkerID == uuid.Nil || j.Attempt == 0 || j.StartedAt == nil || j.StartedAt.IsZero() || j.CancellationRequested || j.Stage == "cleanup" || j.Stage == "complete" {
			return ErrInvalidProjectCopy
		}
	case "failed":
		if j.WorkerID != uuid.Nil && !j.ExecutionUnconfirmed || !validCopyFailure(j.FailureCode) || j.Stage == "complete" {
			return ErrInvalidProjectCopy
		}
	case "cancel_requested":
		if !j.CancellationRequested || j.Stage != "cleanup" {
			return ErrInvalidProjectCopy
		}
	case "cancelled":
		if !j.CancellationRequested || j.Stage != "complete" || j.WorkerID != uuid.Nil {
			return ErrInvalidProjectCopy
		}
	case "succeeded":
		if j.Stage != "complete" || j.WorkerID != uuid.Nil || j.CancellationRequested || j.MediaReceipt == nil || j.CanvasReceipt == nil {
			return ErrInvalidProjectCopy
		}
	}
	if j.Status != "failed" && (j.Retryable || j.FailureCode != "") {
		return ErrInvalidProjectCopy
	}
	return nil
}

func validCopyReceipt(r ProjectCopyReceipt, manifest string, primary, secondary int) bool {
	return r.ManifestSHA256 == manifest && copyDigest(r.ContentSHA256) && r.PrimaryCount == primary && r.SecondaryCount == secondary && r.PrimaryCount >= 0 && r.SecondaryCount >= 0
}

func validCopyFailure(code string) bool {
	return strings.TrimSpace(code) != "" && utf8.ValidString(code) && utf8.RuneCountInString(code) <= 128 && !strings.ContainsRune(code, 0)
}

func (j *ProjectCopyJob) advance() error {
	if err := j.Validate(); err != nil || j.Revision == math.MaxInt32 {
		return ErrInvalidProjectCopy
	}
	j.Revision++
	return nil
}

func (j ProjectCopyJob) worker(worker uuid.UUID) error {
	if worker == uuid.Nil || j.WorkerID != worker {
		return ErrProjectCopyWorkerConflict
	}
	return nil
}

// Start claims one waiting job with a durable attempt identity.
func (j *ProjectCopyJob) Start(worker uuid.UUID, now time.Time) error {
	if worker == uuid.Nil || now.IsZero() || j.Attempt == math.MaxInt32 {
		return ErrInvalidProjectCopy
	}
	if (j.Status != "queued" && j.Status != "cancel_requested") || j.WorkerID != uuid.Nil || j.NeedsReconciliation {
		return ErrProjectCopyStateConflict
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.WorkerID, j.Attempt = worker, j.Attempt+1
	started := now.UTC()
	j.StartedAt = &started
	if j.CancellationRequested {
		j.Status, j.Stage = "cancel_requested", "cleanup"
	} else {
		j.Status = "running"
	}
	return nil
}

// AcceptMediaReceipt advances only after every frozen asset and rendition is verified.
func (j *ProjectCopyJob) AcceptMediaReceipt(worker uuid.UUID, receipt ProjectCopyReceipt) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" || j.Stage != "media" || j.CancellationRequested {
		return ErrProjectCopyStateConflict
	}
	if receipt.ManifestSHA256 != j.Manifest.MediaSHA256 || !copyDigest(receipt.ContentSHA256) || receipt.PrimaryCount != j.Manifest.Assets || receipt.SecondaryCount != j.Manifest.Renditions {
		return ErrInvalidProjectCopy
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.MediaReceipt, j.Stage = &receipt, "canvases"
	return nil
}

// AcceptCanvasReceipt advances only after the exact frozen document set is copied.
func (j *ProjectCopyJob) AcceptCanvasReceipt(worker uuid.UUID, receipt ProjectCopyReceipt) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" || j.Stage != "canvases" || j.MediaReceipt == nil || j.CancellationRequested {
		return ErrProjectCopyStateConflict
	}
	if receipt.ManifestSHA256 != j.Manifest.CanvasSHA256 || !copyDigest(receipt.ContentSHA256) || receipt.PrimaryCount != j.Manifest.Documents || receipt.SecondaryCount < 0 {
		return ErrInvalidProjectCopy
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.CanvasReceipt, j.Stage = &receipt, "finalizing"
	return nil
}

// Publish is called in the same transaction that activates the complete target.
func (j *ProjectCopyJob) Publish(worker uuid.UUID) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" || j.Stage != "finalizing" || j.MediaReceipt == nil || j.CanvasReceipt == nil || j.CancellationRequested || j.NeedsReconciliation {
		return ErrProjectCopyStateConflict
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.Status, j.Stage, j.WorkerID = "succeeded", "complete", uuid.Nil
	return nil
}

// RequestCancel records intent without claiming that transfers or cleanup ended.
func (j *ProjectCopyJob) RequestCancel() error {
	if j.Status == "succeeded" || j.Status == "cancelled" {
		return ErrProjectCopyStateConflict
	}
	if j.CancellationRequested {
		return nil
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.CancellationRequested, j.Stage = true, "cleanup"
	j.FailureCode, j.Retryable = "", false
	if j.NeedsReconciliation {
		j.FailureCode = "cancel_requires_reconciliation"
	}
	if !j.NeedsReconciliation {
		j.Status = "cancel_requested"
	}
	return nil
}

// Fail retains cancellation and uncertainty when the actual transfer result is unknown.
func (j *ProjectCopyJob) Fail(worker uuid.UUID, code string, retryable, unknown bool) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" && j.Status != "cancel_requested" && (j.Status != "failed" || !j.ExecutionUnconfirmed) {
		return ErrProjectCopyStateConflict
	}
	if !validCopyFailure(code) || retryable && unknown {
		return ErrInvalidProjectCopy
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.Status, j.FailureCode, j.Retryable, j.NeedsReconciliation, j.WorkerID = "failed", code, retryable, unknown, uuid.Nil
	j.ExecutionUnconfirmed, j.ReconciliationRequested = false, false
	return nil
}

// Retry queues the original frozen task only when the prior attempt has a known outcome.
func (j *ProjectCopyJob) Retry() error {
	if j.Status != "failed" || !j.Retryable || j.NeedsReconciliation || j.CancellationRequested || j.WorkerID != uuid.Nil {
		return ErrProjectCopyStateConflict
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.Status, j.FailureCode, j.Retryable = "queued", "", false
	return nil
}

// ResumeReconciliation claims recovery of durable receipts, never a fresh transfer identity.
func (j *ProjectCopyJob) ResumeReconciliation(worker uuid.UUID) error {
	if worker == uuid.Nil {
		return ErrInvalidProjectCopy
	}
	if j.Status != "failed" || !j.NeedsReconciliation || !j.ReconciliationRequested || j.ExecutionUnconfirmed || j.WorkerID != uuid.Nil {
		return ErrProjectCopyStateConflict
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.WorkerID, j.NeedsReconciliation, j.FailureCode = worker, false, ""
	j.ReconciliationRequested = false
	if j.CancellationRequested {
		j.Status, j.Stage = "cancel_requested", "cleanup"
	} else {
		j.Status = "running"
	}
	return nil
}

// ConfirmCancelled requires the owner to verify all target cleanup and pin-release receipts.
func (j *ProjectCopyJob) ConfirmCancelled(worker uuid.UUID, cleanupVerified bool) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "cancel_requested" || !j.CancellationRequested || j.NeedsReconciliation || !cleanupVerified {
		return ErrProjectCopyStateConflict
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.Status, j.Stage, j.WorkerID = "cancelled", "complete", uuid.Nil
	return nil
}

// RequestReconciliation records explicit recovery of the same stopped attempt.
func (j *ProjectCopyJob) RequestReconciliation() error {
	if j.Status != "failed" || !j.NeedsReconciliation || j.ExecutionUnconfirmed || j.WorkerID != uuid.Nil {
		return ErrProjectCopyStateConflict
	}
	if j.ReconciliationRequested {
		return nil
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.ReconciliationRequested = true
	return nil
}

// Interrupt preserves the original worker until its synchronous execution acknowledges exit.
func (j *ProjectCopyJob) Interrupt(worker uuid.UUID) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" && j.Status != "cancel_requested" {
		return ErrProjectCopyStateConflict
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.Status, j.FailureCode, j.Retryable, j.NeedsReconciliation = "failed", "execution_unconfirmed", false, true
	j.ExecutionUnconfirmed = true
	return nil
}
