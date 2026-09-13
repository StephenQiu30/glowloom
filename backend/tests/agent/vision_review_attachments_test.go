package agent_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func visionReviewAttachmentFixture(t *testing.T) (contract.VisionReviewSubject, json.RawMessage) {
	t.Helper()
	raw, err := os.ReadFile("testdata/vision_review_candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate, _, err := contract.DecodeVisionReviewCandidate(raw)
	if err != nil {
		t.Fatal(err)
	}
	attachments, err := os.ReadFile("testdata/vision_review_attachments.json")
	if err != nil {
		t.Fatal(err)
	}
	return candidate.Subject, attachments
}

func TestVisionReviewAttachmentsSharedWire(t *testing.T) {
	subject, raw := visionReviewAttachmentFixture(t)
	value, err := contract.DecodeVisionReviewAttachments(raw, subject)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range []json.RawMessage{raw, typed} {
		hash, err := contract.ProductionCanonicalHash(wire)
		if err != nil || hash != "bfefa99207bf28ebcb6dd1df22cbdbb7cf8761d0c9df138a7caab5fe4035da7d" {
			t.Fatalf("attachment canonical identity drifted: %s %v", hash, err)
		}
	}
	repeated, err := contract.DecodeVisionReviewAttachments(typed, subject)
	if err != nil || !reflect.DeepEqual(repeated, value) {
		t.Fatalf("roundtrip: %v", err)
	}
}

func TestVisionReviewAttachmentsRejectWireDrift(t *testing.T) {
	for _, fault := range []string{"missing", "extra", "duplicate", "order", "slot", "receipt", "receipt_hash", "digest", "revision", "media_hash", "media_type", "empty", "oversize", "pixels", "edge", "pages", "frames", "rights", "private_path", "missing_field", "null", "bool_integer", "float_integer"} {
		t.Run(fault, func(t *testing.T) {
			subject, raw := visionReviewAttachmentFixture(t)
			var value []map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			first := value[0]
			slot := first["slot"].(map[string]any)
			switch fault {
			case "missing":
				value = value[:2]
			case "extra":
				value = append(value, first)
			case "duplicate":
				value[1] = first
			case "order":
				slices.Reverse(value)
			case "slot":
				slot["view_role"] = "other"
			case "receipt":
				first["provider_receipt_ref"].(map[string]any)["id"] = "00000000-0000-4000-8000-000000000099"
			case "receipt_hash":
				first["provider_receipt_ref"].(map[string]any)["content_hash"] = "invalid"
			case "digest":
				slot["sha256"] = strings.Repeat("f", 64)
			case "revision":
				slot["staged_media_ref"].(map[string]any)["revision"] = 1
			case "media_hash":
				slot["staged_media_ref"].(map[string]any)["content_hash"] = strings.Repeat("f", 64)
			case "media_type":
				first["media_type"] = "image/jpeg"
			case "empty":
				first["byte_length"] = 0
			case "oversize":
				first["byte_length"] = (10 << 20) + 1
			case "pixels":
				first["pixel_width"], first["pixel_height"] = 8192, 8192
			case "edge":
				first["pixel_width"] = 8193
			case "pages":
				first["page_count"] = 2
			case "frames":
				first["frame_count"] = 0
			case "rights":
				first["rights_observation"] = "approved"
			case "private_path":
				first["path"] = "/private/image.png"
			case "missing_field":
				delete(first, "frame_count")
			case "null":
				first["frame_count"] = nil
			case "bool_integer":
				first["frame_count"] = true
			case "float_integer":
				first["frame_count"] = json.Number("1.0")
			}
			wire, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := contract.DecodeVisionReviewAttachments(wire, subject); err == nil {
				t.Fatal("accepted invalid attachment wire")
			}
		})
	}
}

func TestVisionReviewAttachmentsAllPurposesAndGroupBudget(t *testing.T) {
	subject, raw := visionReviewAttachmentFixture(t)
	attachments, err := contract.DecodeVisionReviewAttachments(raw, subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"character_identity_anchor", "character_appearance", "location_board", "prop_sheet", "interaction_composition", "scene_composition"} {
		t.Run(kind, func(t *testing.T) {
			subject.TargetKind, subject.Slots = kind, nil
			var group []contract.VisionReviewMediaAttachment
			for i, role := range contract.ReferenceBriefRequiredViewRoles(kind) {
				item := attachments[0]
				item.Slot.SlotKey, item.Slot.ViewRole = role, role
				item.Slot.MediaRef.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", 21+i)
				item.ProviderReceiptRef.ID = item.Slot.MediaRef.ID
				item.Slot.SHA256 = strings.Repeat(string("abcd"[i]), 64)
				item.ByteLength = 8 << 20
				subject.Slots = append(subject.Slots, item.Slot)
				group = append(group, item)
			}
			if err := contract.ValidateVisionReviewAttachments(subject, group); err != nil {
				t.Fatal(err)
			}
			if kind == "prop_sheet" {
				group[0].ByteLength++
				if err := contract.ValidateVisionReviewAttachments(subject, group); err == nil {
					t.Fatal("accepted oversized group")
				}
			}
		})
	}
}
