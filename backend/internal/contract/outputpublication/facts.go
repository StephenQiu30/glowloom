// Package outputpublication defines closed immutable facts for private owner delivery.
// It contains no supplier client, object credentials or owner business services.
package outputpublication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// Publication errors retain unknown responsibility separately from final rejection.
var (
	ErrInvalidOutputPublication  = errors.New("invalid operation output publication")
	ErrOutputPublicationConflict = errors.New("operation output publication conflicts with durable evidence")
	ErrOutputPublicationUnknown  = errors.New("operation output publication is unresolved")
	ErrOutputPublicationOwner    = errors.New("operation output publication requires Media owner")
)

// OutputProviderEvidence retains the original paid call. It never authorizes resubmission.
type OutputProviderEvidence struct {
	RequestKey          string `json:"request_key"`
	Attempt             int32  `json:"attempt"`
	ManifestSHA256      string `json:"manifest_sha256"`
	UsageSHA256         string `json:"usage_sha256"`
	SupplierUsageSHA256 string `json:"supplier_usage_sha256"`
	ActualCostMicros    int64  `json:"actual_cost_micros"`
}

// OutputWorkerEvidence is independent from account and project policy epochs.
type OutputWorkerEvidence struct {
	Kind   string    `json:"kind"`
	TaskID uuid.UUID `json:"task_id"`
	Owner  string    `json:"owner"`
	Epoch  int64     `json:"epoch"`
}

// OperationOutputIdentity carries immutable facts, never a bucket, key or URL.
type OperationOutputIdentity struct {
	OperationID uuid.UUID              `json:"operation_id"`
	OutputID    uuid.UUID              `json:"output_id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	ScopeID     uuid.UUID              `json:"scope_id"`
	Sequence    int                    `json:"sequence"`
	Sequences   []int                  `json:"sequences"`
	ByteSize    int64                  `json:"byte_size"`
	SHA256      string                 `json:"sha256"`
	MIMEType    string                 `json:"mime_type"`
	Kind        string                 `json:"kind"`
	ProviderKey string                 `json:"provider_key"`
	ModelKey    string                 `json:"model_key"`
	Region      string                 `json:"region"`
	AspectRatio string                 `json:"aspect_ratio"`
	Provider    OutputProviderEvidence `json:"provider"`
	Worker      OutputWorkerEvidence   `json:"worker"`
}

// Validate checks wire shape only. Format, manifest cardinality, geography and
// moderation decisions are independently validated by the owning application.
func (o OperationOutputIdentity) Validate() error {
	if o.OperationID == uuid.Nil || o.OutputID == uuid.Nil || o.ProjectID == uuid.Nil || o.ScopeID == uuid.Nil || o.Sequence < 0 || o.Sequence > 2147483647 || o.ByteSize < 1 || o.ByteSize > 1<<40 || !digestValid(o.SHA256) || !digestValid(o.Provider.ManifestSHA256) || o.Provider.Attempt < 1 || o.Provider.ActualCostMicros < 0 || o.Worker.TaskID == uuid.Nil || o.Worker.Epoch < 1 {
		return ErrInvalidOutputPublication
	}
	if len(o.Sequences) < 1 || len(o.Sequences) > 128 {
		return ErrInvalidOutputPublication
	}
	found := false
	for i, sequence := range o.Sequences {
		if sequence < 0 || sequence > 2147483647 || (i > 0 && sequence <= o.Sequences[i-1]) {
			return ErrInvalidOutputPublication
		}
		found = found || sequence == o.Sequence
	}
	if !found {
		return ErrInvalidOutputPublication
	}
	for _, value := range []string{o.ProviderKey, o.ModelKey, o.Provider.RequestKey, o.Worker.Owner, o.Worker.Kind} {
		if len(value) == 0 || len(value) > 256 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return ErrInvalidOutputPublication
		}
	}
	if !digestValid(o.Provider.UsageSHA256) || !digestValid(o.Provider.SupplierUsageSHA256) {
		return ErrInvalidOutputPublication
	}
	for _, value := range []string{o.Kind, o.MIMEType, o.Region, o.AspectRatio} {
		if len(value) == 0 || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return ErrInvalidOutputPublication
		}
	}
	return nil
}

// Digest binds the wire identity without converting exact usage numbers to floats.
func (o OperationOutputIdentity) Digest() string {
	raw, err := json.Marshal(o)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// OperationOutputReceipt is the Media-owned durable final answer to one command.
// A rejected result retains the same paid provider facts and physical responsibility.
type OperationOutputReceipt struct {
	CommandID        uuid.UUID               `json:"command_id"`
	RequestSHA256    string                  `json:"request_sha256"`
	PayloadSHA256    string                  `json:"payload_sha256"`
	Output           OperationOutputIdentity `json:"output"`
	AssetID          uuid.UUID               `json:"asset_id"`
	AssetRevision    int64                   `json:"asset_revision"`
	ModerationStatus string                  `json:"moderation_status"`
	ModerationDetail json.RawMessage         `json:"moderation_detail"`
	State            string                  `json:"state"`
}

// Validate checks only the closed receipt wire shape and digest binding.
func (r OperationOutputReceipt) Validate() error {
	if r.CommandID == uuid.Nil || !digestValid(r.RequestSHA256) || r.Output.Validate() != nil || r.PayloadSHA256 != r.Output.Digest() || r.AssetID == uuid.Nil || r.AssetRevision < 1 || len(r.ModerationDetail) == 0 || len(r.ModerationDetail) > 16384 || !json.Valid(r.ModerationDetail) {
		return ErrOutputPublicationConflict
	}
	if (r.State != "published" && r.State != "rejected") || (r.ModerationStatus != "passed" && r.ModerationStatus != "skipped" && r.ModerationStatus != "rejected") {
		return ErrOutputPublicationConflict
	}
	return nil
}
