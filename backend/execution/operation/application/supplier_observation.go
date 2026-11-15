package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// SupplierObservation is small immutable evidence captured immediately after the
// original supplier response, before download, FFmpeg, artifact hashing or Media IO.
// Raw usage bytes remain inside Execution and are not converted through float64.
// No supplier key, URL, image/audio bytes or physical route is part of this record.
type SupplierObservation struct {
	Identity             ProviderDispatchIdentity   `json:"identity"`
	CallAction           string                     `json:"call_action"`
	CallAttempt          int32                      `json:"call_attempt"`
	ReceivedAt           time.Time                  `json:"received_at"`
	Outcome              ProviderOutcome            `json:"outcome"`
	State                ProviderTaskState          `json:"state"`
	TaskID               string                     `json:"task_id,omitempty"`
	HTTPStatus           int                        `json:"http_status,omitempty"`
	ProviderLogID        string                     `json:"provider_log_id,omitempty"`
	ProviderTerminalCode int                        `json:"provider_terminal_code,omitempty"`
	SupplierUsage        []byte                     `json:"supplier_usage,omitempty"`
	ImageCount           int                        `json:"image_count"`
	AudioCount           int                        `json:"audio_count"`
	URLCount             int                        `json:"url_count"`
	TextPresent          bool                       `json:"text_present"`
	Worker               *SupplierObservationWorker `json:"worker,omitempty"`
}

// SupplierObservationWorker retains the actual original claim, independently of
// credential, account/session head and Creation resource epochs.
type SupplierObservationWorker struct {
	Kind       string `json:"kind"`
	BusinessID string `json:"business_id"`
	Owner      string `json:"owner"`
	Epoch      int64  `json:"epoch"`
}

// Validate checks the bounded original call envelope, not current spending rights.
func (o SupplierObservation) Validate() error {
	if o.Identity.Validate() != nil || (o.CallAction != "submit" && o.CallAction != "query") || o.CallAttempt < 1 || o.ReceivedAt.IsZero() || len(o.SupplierUsage) > 64<<10 || len(o.TaskID) > 256 || len(o.ProviderLogID) > 256 || strings.ContainsAny(o.ProviderLogID, "\x00\r\n") || o.ImageCount < 0 || o.AudioCount < 0 || o.URLCount < 0 || o.ImageCount > 2147483647 || o.AudioCount > 2147483647 || o.URLCount > 2147483647 {
		return ErrInvalidProviderCall
	}
	switch o.Outcome {
	case ProviderOutcomeAccepted, ProviderOutcomeCompleted, ProviderOutcomeRejected, ProviderOutcomeUnknown, ProviderOutcomeCancelled, ProviderOutcomeNotCancelled:
	default:
		return ErrInvalidProviderCall
	}
	if o.Worker != nil && (o.Worker.Kind == "" || o.Worker.BusinessID == "" || o.Worker.Owner == "" || o.Worker.Epoch < 1) {
		return ErrInvalidProviderCall
	}
	return nil
}

// SupplierObservations is the credential-owning worker's mandatory early receipt port.
// The store preserves raw evidence even if frozen pricing cannot yet determine cost.
type SupplierObservations interface {
	RecordSupplierObservation(context.Context, SupplierObservation) error
}

// SupplierObservationReader recovers original task evidence without supplier IO.
type SupplierObservationReader interface {
	ReadSupplierObservation(context.Context, ProviderDispatchIdentity) (*SupplierObservation, error)
}

// ObservedUsage extracts only already known normalized quantities. Missing or
// inconsistent facts remain unknown; it does not estimate media duration or run IO.
func (o SupplierObservation) ObservedUsage(capability string) (json.RawMessage, error) {
	if o.Validate() != nil || o.Outcome != ProviderOutcomeCompleted || o.State != ProviderTaskSucceeded {
		return nil, ErrProviderCostUnknown
	}
	fields := map[string]json.RawMessage{}
	if len(o.SupplierUsage) > 0 && json.Unmarshal(o.SupplierUsage, &fields) != nil {
		return nil, ErrProviderCostUnknown
	}
	usage := map[string]json.RawMessage{}
	for _, key := range []string{"reference_image_count", "billed_duration_ms", "original_duration_seconds", "character_count", "cached_input_tokens", "input_tokens", "output_tokens"} {
		if value, ok := fields[key]; ok {
			usage[key] = value
		}
	}
	for _, pair := range [][2]string{{"prompt_tokens", "input_tokens"}, {"completion_tokens", "output_tokens"}} {
		if alias, ok := fields[pair[0]]; ok {
			var value *int64
			if json.Unmarshal(alias, &value) != nil || value == nil || *value < 0 {
				return nil, ErrProviderCostUnknown
			}
			if previous, ok := usage[pair[1]]; ok {
				var n *int64
				if json.Unmarshal(previous, &n) != nil || n == nil || *n != *value {
					return nil, ErrProviderCostUnknown
				}
			}
			usage[pair[1]] = alias
		}
	}
	count := 0
	switch capability {
	case "image.generate":
		count = o.ImageCount
	case "audio.generate", "audio.tts":
		count = o.AudioCount
		if count == 0 {
			count = o.URLCount
		}
	case "video.generate":
		count = o.URLCount
	}
	if count > 0 {
		raw, _ := json.Marshal(count)
		usage["output_count"] = raw
	}
	if _, present := usage["billable_requests"]; !present {
		usage["billable_requests"] = json.RawMessage("1")
	}
	raw, err := json.Marshal(usage)
	if err != nil {
		return nil, err
	}
	if _, err := ParseBillableUsage(raw); err != nil {
		return nil, ErrProviderCostUnknown
	}
	return raw, nil
}
