package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	generation "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

// VisionReviewMediaAttachment describes validated bytes without exposing their
// private location. It is neither a complete stage input nor a use permission.
type VisionReviewMediaAttachment struct {
	Slot               VisionReviewSlot               `json:"slot"`
	ProviderReceiptRef generation.GenerationActionRef `json:"provider_receipt_ref"`
	MediaType          string                         `json:"media_type"`
	ByteLength         int64                          `json:"byte_length"`
	PixelWidth         int                            `json:"pixel_width"`
	PixelHeight        int                            `json:"pixel_height"`
	PageCount          int                            `json:"page_count"`
	FrameCount         int                            `json:"frame_count"`
	RightsObservation  string                         `json:"rights_observation"`
}

// BuildVisionReviewAttachments compiles exactly one bundle from immutable Owner
// snapshots. The caller must separately verify current authorization, admission
// and the complete input before resolving and sending any private media bytes.
func BuildVisionReviewAttachments(subject VisionReviewSubject, media []generation.ReferenceStagedMedia) ([]VisionReviewMediaAttachment, error) {
	if err := subject.Validate(); err != nil {
		return nil, err
	}
	if len(media) != len(subject.Slots) {
		return nil, errors.New("Vision Review requires exactly the frozen bundle media")
	}
	bySlot := make(map[string]generation.ReferenceStagedMedia, len(media))
	for _, item := range media {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("encode Vision Review media: %w", err)
		}
		if _, err := generation.DecodeReferenceStagedMedia(raw); err != nil {
			return nil, fmt.Errorf("validate Vision Review media: %w", err)
		}
		if item.State != "ready_for_review" || item.WorkspaceID != subject.WorkspaceID || item.ProjectID != subject.ProjectID ||
			item.Call.ExecutionRef != subject.ExecutionRef || item.Call.BundleIndex != subject.CandidateBundleIndex {
			return nil, errors.New("Vision Review media is not ready in the frozen bundle scope")
		}
		if _, exists := bySlot[item.Call.SlotKey]; exists {
			return nil, errors.New("Vision Review media has duplicate slots")
		}
		bySlot[item.Call.SlotKey] = item
	}
	attachments := make([]VisionReviewMediaAttachment, 0, len(subject.Slots))
	for _, slot := range subject.Slots {
		item, exists := bySlot[slot.SlotKey]
		if !exists || slot.MediaRef.ID != item.ID || slot.MediaRef.Revision != item.Revision || slot.MediaRef.ContentHash != item.ContentHash || slot.SHA256 != item.SHA256 {
			return nil, errors.New("Vision Review media differs from the frozen slot")
		}
		attachments = append(attachments, VisionReviewMediaAttachment{
			Slot: slot, ProviderReceiptRef: item.ReceiptRef, MediaType: item.MediaType, ByteLength: item.ByteSize,
			PixelWidth: item.Width, PixelHeight: item.Height, PageCount: 1, FrameCount: 1, RightsObservation: item.RightsObservation,
		})
	}
	if err := ValidateVisionReviewAttachments(subject, attachments); err != nil {
		return nil, err
	}
	return attachments, nil
}

func ValidateVisionReviewAttachments(subject VisionReviewSubject, attachments []VisionReviewMediaAttachment) error {
	if err := subject.Validate(); err != nil {
		return err
	}
	if len(attachments) != len(subject.Slots) {
		return errors.New("Vision Review attachment set is incomplete")
	}
	var total int64
	for i, attachment := range attachments {
		if attachment.Slot != subject.Slots[i] || !attachment.ProviderReceiptRef.Valid() || attachment.ProviderReceiptRef.ID != attachment.Slot.MediaRef.ID ||
			attachment.MediaType != "image/png" || attachment.ByteLength < 1 || attachment.ByteLength > 10<<20 ||
			attachment.PixelWidth < 1 || attachment.PixelWidth > 8192 || attachment.PixelHeight < 1 || attachment.PixelHeight > 8192 ||
			int64(attachment.PixelWidth)*int64(attachment.PixelHeight) > 16777216 || attachment.PageCount != 1 || attachment.FrameCount != 1 || attachment.RightsObservation != "not_assessed" {
			return errors.New("Vision Review attachment identity or media policy is invalid")
		}
		total += attachment.ByteLength
	}
	if total > 32<<20 {
		return errors.New("Vision Review attachments exceed the bundle byte budget")
	}
	return nil
}

func DecodeVisionReviewAttachments(raw json.RawMessage, subject VisionReviewSubject) ([]VisionReviewMediaAttachment, error) {
	var attachments []VisionReviewMediaAttachment
	if err := canonical.Decode(raw, &attachments); err != nil {
		return nil, fmt.Errorf("decode Vision Review attachments: %w", err)
	}
	if err := ValidateVisionReviewAttachments(subject, attachments); err != nil {
		return nil, err
	}
	wire, err := canonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	typed, err := json.Marshal(attachments)
	if err != nil {
		return nil, err
	}
	typedWire, err := canonical.JSON(typed)
	if err != nil || !bytes.Equal(wire, typedWire) {
		return nil, errors.New("Vision Review attachment wire shape is incomplete")
	}
	return attachments, nil
}
