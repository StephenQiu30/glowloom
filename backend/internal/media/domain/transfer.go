package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrTransferConflict preserves a stale revision or occupied worker fence.
	ErrTransferConflict = errors.New("media transfer conflict")
	// ErrTransferCancelled tells physical work to stop before any further write.
	ErrTransferCancelled = errors.New("media transfer cancelled")
)

// TransferItemResult reveals only safe identities and actual per-item outcomes.
type TransferItemResult struct {
	Index         int        `json:"index"`
	SourceItemID  uuid.UUID  `json:"source_item_id"`
	TargetItemID  uuid.UUID  `json:"target_item_id"`
	TargetAssetID *uuid.UUID `json:"target_asset_id" extensions:"x-nullable"`
	Status        string     `json:"status"`
	FailureCode   *string    `json:"failure_code" extensions:"x-nullable"`
}

// TransferJob is a current authorized view of a durable media ownership transfer.
// It carries no private object key, source notes or model execution history.
type TransferJob struct {
	ID                    uuid.UUID            `json:"id"`
	CurrentActorID        uuid.UUID            `json:"current_actor_id"`
	CurrentOrgID          uuid.UUID            `json:"current_org_id"`
	Source                LibraryScope         `json:"source"`
	Target                LibraryScope         `json:"target"`
	TargetFolderID        *uuid.UUID           `json:"target_folder_id" extensions:"x-nullable"`
	Status                string               `json:"status"`
	Stage                 string               `json:"stage"`
	Attempt               int                  `json:"attempt"`
	Revision              int64                `json:"revision"`
	NeedsReconciliation   bool                 `json:"needs_reconciliation"`
	CancellationRequested bool                 `json:"cancellation_requested"`
	ExecutionUnconfirmed  bool                 `json:"execution_unconfirmed"`
	Items                 []TransferItemResult `json:"items"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
}
