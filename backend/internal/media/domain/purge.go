package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrPurgeConflict preserves a stale catalog or an occupied physical fence.
var ErrPurgeConflict = errors.New("media purge conflict")

// ErrPurgeCancelled prevents starting a new irreversible deletion after cancel.
var ErrPurgeCancelled = errors.New("media purge cancelled")

// PurgeStatus is the persisted lifecycle of a purge job.
type PurgeStatus string

// PurgeItemStatus is the persisted lifecycle of one item in a purge job.
type PurgeItemStatus string

// PurgeStage is the persisted worker stage of a purge job.
type PurgeStage string

// Purge job states distinguish queued, executing, uncertain and terminal outcomes.
const (
	PurgeStatusQueued              PurgeStatus = "queued"
	PurgeStatusRunning             PurgeStatus = "running"
	PurgeStatusNeedsReconciliation PurgeStatus = "needs_reconciliation"
	PurgeStatusCancelRequested     PurgeStatus = "cancel_requested"
	PurgeStatusSucceeded           PurgeStatus = "succeeded"
	PurgeStatusPartialFailed       PurgeStatus = "partial_failed"
	PurgeStatusFailed              PurgeStatus = "failed"
	PurgeStatusCancelled           PurgeStatus = "cancelled"

	PurgeItemStatusQueued              PurgeItemStatus = "queued"
	PurgeItemStatusRunning             PurgeItemStatus = "running"
	PurgeItemStatusNeedsReconciliation PurgeItemStatus = "needs_reconciliation"
	PurgeItemStatusSucceeded           PurgeItemStatus = "succeeded"
	PurgeItemStatusBlocked             PurgeItemStatus = "blocked"
	PurgeItemStatusCancelled           PurgeItemStatus = "cancelled"

	PurgeStageFrozen      PurgeStage = "frozen"
	PurgeStageVerifying   PurgeStage = "verifying"
	PurgeStageRemoving    PurgeStage = "removing"
	PurgeStageCompleted   PurgeStage = "completed"
	PurgeFailureCancelled            = "cancelled"
)

// PurgeItemResult exposes an actual item outcome without names or object keys.
type PurgeItemResult struct {
	Index       int             `json:"index"`
	ItemID      uuid.UUID       `json:"item_id"`
	AssetID     *uuid.UUID      `json:"asset_id" extensions:"x-nullable"`
	Status      PurgeItemStatus `json:"status"`
	FailureCode *string         `json:"failure_code" extensions:"x-nullable"`
}

// PurgeJob is a durable deletion view. Hidden catalog rows do not make this job
// successful: every private object must have an owning absence receipt.
type PurgeJob struct {
	ID                    uuid.UUID         `json:"id"`
	CurrentActorID        uuid.UUID         `json:"current_actor_id"`
	CurrentOrgID          uuid.UUID         `json:"current_org_id"`
	Scope                 LibraryScope      `json:"scope"`
	Status                PurgeStatus       `json:"status"`
	Stage                 PurgeStage        `json:"stage"`
	Revision              int64             `json:"revision"`
	Attempt               int               `json:"attempt"`
	CancellationRequested bool              `json:"cancellation_requested"`
	NeedsReconciliation   bool              `json:"needs_reconciliation"`
	ExecutionUnconfirmed  bool              `json:"execution_unconfirmed"`
	Items                 []PurgeItemResult `json:"items"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}
