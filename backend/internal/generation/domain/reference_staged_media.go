package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"reflect"
	"regexp"
	"slices"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type ReferenceObjectStoreRef struct {
	Profile   string `json:"profile"`
	Bucket    string `json:"bucket"`
	ObjectKey string `json:"object_key"`
}

func (ref ReferenceObjectStoreRef) Valid() bool {
	return targetNamePattern.MatchString(ref.Profile) && referenceBucketPattern.MatchString(ref.Bucket)
}

var referenceBucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

type ReferenceStagedMedia struct {
	ContractID          string                  `json:"contract_id"`
	ID                  string                  `json:"staged_media_id"`
	WorkspaceID         string                  `json:"workspace_id"`
	ProjectID           string                  `json:"project_id"`
	Call                ReferenceProviderCall   `json:"provider_call_ref"`
	ReceiptRef          GenerationActionRef     `json:"provider_receipt_ref"`
	OutputIdentity      string                  `json:"output_identity"`
	ObjectStoreRef      ReferenceObjectStoreRef `json:"object_store_ref"`
	SHA256              string                  `json:"sha256"`
	ByteSize            int64                   `json:"byte_size"`
	MediaType           string                  `json:"media_type"`
	Width               int                     `json:"width"`
	Height              int                     `json:"height"`
	DecodingContractRef GenerationContractRef   `json:"decoding_contract_ref"`
	RightsObservation   string                  `json:"rights_provenance_observation"`
	State               string                  `json:"state"`
	Revision            int64                   `json:"revision"`
	FailureCode         string                  `json:"failure_code"`
	CreatedAt           time.Time               `json:"created_at"`
	ValidatedAt         *time.Time              `json:"validated_at,omitempty"`
	ContentHash         string                  `json:"content_hash"`
}

func referencePNGDecodingContract() GenerationContractRef {
	const policy = `{"format":"png","max_bytes":10485760,"max_edge":8192,"max_pixels":16777216,"reject_trailing_bytes":true,"verify_sha256":true}`
	digest := sha256.Sum256([]byte(policy))
	return GenerationContractRef{ContractID: "reference-png-decoding", ContentHash: hex.EncodeToString(digest[:])}
}

func NewReferenceStagedMedia(receipt ReferenceCallReceipt, location ReferenceObjectStoreRef) (ReferenceStagedMedia, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return ReferenceStagedMedia{}, err
	}
	if _, err = DecodeReferenceCallReceipt(raw); err != nil || receipt.Disposition != "staged" || receipt.Output == nil {
		return ReferenceStagedMedia{}, errors.New("staged media requires a successful exact receipt")
	}
	o := receipt.Output
	if location.ObjectKey != o.StagingObjectKey {
		return ReferenceStagedMedia{}, errors.New("staged media object differs from receipt")
	}
	return buildReferenceStagedMedia(ReferenceStagedMedia{
		ContractID: "reference-staged-media", ID: receipt.SubmissionToken, WorkspaceID: receipt.WorkspaceID, ProjectID: receipt.ProjectID,
		Call: receipt.Call, ReceiptRef: GenerationActionRef{ID: receipt.SubmissionToken, ContentHash: receipt.ContentHash},
		OutputIdentity: o.OutputKey, ObjectStoreRef: location, SHA256: o.SHA256, ByteSize: o.Bytes, MediaType: o.MediaType, Width: o.Width, Height: o.Height,
		DecodingContractRef: referencePNGDecodingContract(), RightsObservation: "not_assessed", State: "quarantined", Revision: 1, CreatedAt: receipt.ObservedAt,
	})
}

func buildReferenceStagedMedia(v ReferenceStagedMedia) (ReferenceStagedMedia, error) {
	for _, id := range []string{v.ID, v.WorkspaceID, v.ProjectID} {
		if !(GenerationRevisionRef{ID: id, Revision: 1, ContentHash: v.SHA256}).Valid() {
			return ReferenceStagedMedia{}, errors.New("invalid staged media identity")
		}
	}
	raw, err := json.Marshal(v.Call)
	if err != nil {
		return ReferenceStagedMedia{}, err
	}
	if _, err = DecodeReferenceProviderCall(raw); err != nil {
		return ReferenceStagedMedia{}, err
	}
	expectedKey := "staging/reference/" + v.WorkspaceID + "/" + v.ProjectID + "/" + v.Call.ExecutionRef.ID + "/" + v.Call.CallKey + "/" + v.ID + "/image.png"
	if v.ContractID != "reference-staged-media" || !v.ReceiptRef.Valid() || v.ReceiptRef.ID != v.ID || v.OutputIdentity != "image" || !v.ObjectStoreRef.Valid() || v.ObjectStoreRef.ObjectKey != expectedKey || v.MediaType != "image/png" || v.ByteSize < 1 || v.ByteSize > 10<<20 || v.Width < 1 || v.Height < 1 || v.Width > 8192 || v.Height > 8192 || v.DecodingContractRef != referencePNGDecodingContract() || v.RightsObservation != "not_assessed" || v.CreatedAt.IsZero() {
		return ReferenceStagedMedia{}, errors.New("invalid staged media provenance or policy")
	}
	v.CreatedAt = v.CreatedAt.UTC().Truncate(time.Microsecond)
	if v.State == "quarantined" {
		if v.Revision != 1 || v.ValidatedAt != nil || v.FailureCode != "" {
			return ReferenceStagedMedia{}, errors.New("invalid pending staged media")
		}
	} else {
		if v.Revision != 2 || v.ValidatedAt == nil || v.ValidatedAt.Before(v.CreatedAt) {
			return ReferenceStagedMedia{}, errors.New("invalid completed staged media")
		}
		at := v.ValidatedAt.UTC().Truncate(time.Microsecond)
		v.ValidatedAt = &at
		switch v.State {
		case "ready_for_review":
			if v.FailureCode != "" {
				return ReferenceStagedMedia{}, errors.New("ready media has a failure")
			}
		case "rejected":
			if !slices.Contains([]string{"size_mismatch", "checksum_mismatch", "image_dimensions_mismatch", "invalid_png_contents"}, v.FailureCode) {
				return ReferenceStagedMedia{}, errors.New("invalid media rejection")
			}
		default:
			return ReferenceStagedMedia{}, errors.New("invalid staged media state")
		}
	}
	v.ContentHash = ""
	raw, err = json.Marshal(v)
	if err != nil {
		return ReferenceStagedMedia{}, err
	}
	v.ContentHash, err = canonical.Hash(raw)
	return v, err
}

func DecodeReferenceStagedMedia(raw json.RawMessage) (ReferenceStagedMedia, error) {
	var v ReferenceStagedMedia
	if err := canonical.Decode(raw, &v); err != nil {
		return ReferenceStagedMedia{}, err
	}
	expected, err := buildReferenceStagedMedia(v)
	if err != nil || !reflect.DeepEqual(expected, v) {
		return ReferenceStagedMedia{}, errors.New("staged media content has drifted")
	}
	return v, nil
}

// InitialReferenceStagedMedia re-derives the immutable identity for receipt checks.
func InitialReferenceStagedMedia(v ReferenceStagedMedia) (ReferenceStagedMedia, error) {
	expected, err := buildReferenceStagedMedia(v)
	if err != nil || !reflect.DeepEqual(expected, v) {
		return ReferenceStagedMedia{}, errors.New("invalid staged media snapshot")
	}
	v.State, v.Revision, v.FailureCode, v.ValidatedAt = "quarantined", 1, "", nil
	return buildReferenceStagedMedia(v)
}

func CompleteReferenceStagedMedia(before ReferenceStagedMedia, contents []byte, at time.Time) (ReferenceStagedMedia, error) {
	expected, err := InitialReferenceStagedMedia(before)
	if err != nil || !reflect.DeepEqual(expected, before) || at.Before(before.CreatedAt) {
		return ReferenceStagedMedia{}, errors.New("staged media is not awaiting validation")
	}
	after := before
	at = at.UTC().Truncate(time.Microsecond)
	after.State, after.Revision, after.ValidatedAt = "ready_for_review", 2, &at
	switch {
	case int64(len(contents)) != before.ByteSize:
		after.FailureCode = "size_mismatch"
	default:
		hash := sha256.Sum256(contents)
		if hex.EncodeToString(hash[:]) != before.SHA256 {
			after.FailureCode = "checksum_mismatch"
		} else {
			size, err := png.DecodeConfig(bytes.NewReader(contents))
			if err != nil || size.Width != before.Width || size.Height != before.Height || int64(size.Width)*int64(size.Height) > 16777216 {
				after.FailureCode = "image_dimensions_mismatch"
			} else {
				reader := bytes.NewReader(contents)
				if _, err = png.Decode(reader); err != nil || reader.Len() != 0 {
					after.FailureCode = "invalid_png_contents"
				}
			}
		}
	}
	if after.FailureCode != "" {
		after.State = "rejected"
	}
	return buildReferenceStagedMedia(after)
}

func ValidateReferenceStagedMediaTransition(before, after ReferenceStagedMedia) error {
	initial, err := InitialReferenceStagedMedia(after)
	if err != nil || before.State != "quarantined" || after.Revision != 2 || !reflect.DeepEqual(initial, before) {
		return errors.New("invalid staged media transition")
	}
	return nil
}

func RejectReferenceStagedMedia(before ReferenceStagedMedia, reason string, at time.Time) (ReferenceStagedMedia, error) {
	initial, err := InitialReferenceStagedMedia(before)
	if err != nil || !reflect.DeepEqual(initial, before) || at.Before(before.CreatedAt) {
		return ReferenceStagedMedia{}, errors.New("staged media is not awaiting validation")
	}
	at = at.UTC().Truncate(time.Microsecond)
	before.State, before.Revision, before.FailureCode, before.ValidatedAt = "rejected", 2, reason, &at
	return buildReferenceStagedMedia(before)
}
