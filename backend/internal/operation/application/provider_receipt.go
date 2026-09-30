package application

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxProviderReceiptBytes = 4 * 1024
	maxProviderImageBytes   = 32 * 1024 * 1024
)

// ProviderDispatchIdentity binds a paid submit to its durable call and frozen
// model and price. It does not grant permission to send the request again.
type ProviderDispatchIdentity struct {
	ProjectID             uuid.UUID `json:"project_id"`
	OperationID           uuid.UUID `json:"operation_id"`
	Action                string    `json:"action"`
	Attempt               int32     `json:"attempt"`
	RequestKey            string    `json:"request_key"`
	ModelProfileVersionID uuid.UUID `json:"model_profile_version_id"`
	PriceRuleVersionID    uuid.UUID `json:"price_rule_version_id"`
}

// Validate requires the complete identity of one synchronous submit attempt.
func (i ProviderDispatchIdentity) Validate() error {
	if i.ProjectID == uuid.Nil || i.OperationID == uuid.Nil ||
		i.ModelProfileVersionID == uuid.Nil || i.PriceRuleVersionID == uuid.Nil ||
		i.Action != "submit" || i.Attempt < 1 ||
		i.RequestKey == "" || len(i.RequestKey) > 256 ||
		strings.TrimSpace(i.RequestKey) != i.RequestKey {
		return ErrInvalidProviderCall
	}
	return nil
}

// ProviderReceiptOutput describes the sole staged image without exposing bytes
// or an address. Trusted media code derives its location from the call identity.
type ProviderReceiptOutput struct {
	Sequence  int32  `json:"sequence"`
	SizeBytes int64  `json:"size_bytes"`
	MIMEType  string `json:"mime_type"`
	SHA256    string `json:"sha256"`
}

// ProviderReceipt records complete staged output evidence independently of cost.
// Possessing a receipt does not establish usage, moderation, or a settled charge.
type ProviderReceipt struct {
	Version        int                      `json:"version"`
	Identity       ProviderDispatchIdentity `json:"identity"`
	ManifestSHA256 string                   `json:"manifest_sha256"`
	Outputs        []ProviderReceiptOutput  `json:"outputs"`
}

// Validate enforces the bounded, single-image receipt contract.
func (r ProviderReceipt) Validate() error {
	if r.Version != 1 || r.Identity.Validate() != nil ||
		!validReceiptSHA256(r.ManifestSHA256) || len(r.Outputs) != 1 {
		return ErrInvalidProviderCall
	}
	output := r.Outputs[0]
	if output.Sequence != 1 || output.SizeBytes < 1 || output.SizeBytes > maxProviderImageBytes ||
		!validReceiptSHA256(output.SHA256) {
		return ErrInvalidProviderCall
	}
	switch output.MIMEType {
	case "image/png", "image/jpeg", "image/webp":
		return nil
	default:
		return ErrInvalidProviderCall
	}
}

// UnmarshalJSON rejects open or ambiguous receipts before they can become
// durable evidence. Errors never include untrusted field names or values.
func (r *ProviderReceipt) UnmarshalJSON(raw []byte) error {
	if len(raw) > maxProviderReceiptBytes {
		return ErrInvalidProviderCall
	}
	type wireReceipt ProviderReceipt
	var wire wireReceipt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return ErrInvalidProviderCall
	}
	fields, err := receiptObjectFields(raw, "version", "identity", "manifest_sha256", "outputs")
	if err != nil {
		return ErrInvalidProviderCall
	}
	if _, err := receiptObjectFields(fields["identity"], "project_id", "operation_id", "action",
		"attempt", "request_key", "model_profile_version_id", "price_rule_version_id"); err != nil {
		return ErrInvalidProviderCall
	}
	var outputs []json.RawMessage
	if err := json.Unmarshal(fields["outputs"], &outputs); err != nil || len(outputs) != 1 {
		return ErrInvalidProviderCall
	}
	if _, err := receiptObjectFields(outputs[0], "sequence", "size_bytes", "mime_type", "sha256"); err != nil {
		return ErrInvalidProviderCall
	}
	receipt := ProviderReceipt(wire)
	if err := receipt.Validate(); err != nil {
		return err
	}
	*r = receipt
	return nil
}

// ProviderDispatchResult distinguishes the one sending owner from a replay.
// A previous send without a receipt is uncertain, never permission to resend.
type ProviderDispatchResult struct {
	Claimed           bool             `json:"claimed"`
	DispatchStartedAt *time.Time       `json:"dispatch_started_at,omitempty"`
	Receipt           *ProviderReceipt `json:"receipt,omitempty"`
}

func validReceiptSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func receiptObjectFields(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrInvalidProviderCall
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidProviderCall
		}
		name, ok := token.(string)
		if !ok || !slices.Contains(allowed, name) {
			return nil, ErrInvalidProviderCall
		}
		if _, exists := fields[name]; exists {
			return nil, ErrInvalidProviderCall
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidProviderCall
		}
		fields[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, ErrInvalidProviderCall
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidProviderCall
	}
	return fields, nil
}
