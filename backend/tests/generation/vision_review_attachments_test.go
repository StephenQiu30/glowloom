package generation_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

func visionAttachmentSubject(t *testing.T, facts domain.ReferenceBundleFacts) contract.VisionReviewSubject {
	t.Helper()
	collection, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil {
		t.Fatal(err)
	}
	subject := contract.VisionReviewSubject{
		WorkspaceID: facts.WorkspaceID, ProjectID: facts.ProjectID, TargetKind: facts.TargetKind,
		GenerationRound: facts.GenerationRound, TargetRef: facts.TargetRef, ExecutionRef: facts.Calls[0].ExecutionRef,
		BundleInputRef:  domain.GenerationActionRef{ID: collection.Bundles[0].Input.ID, ContentHash: collection.Bundles[0].Input.ContentHash},
		BriefRevisionID: uuid.NewString(), BriefRevisionHash: strings.Repeat("a", 64),
		StageReleaseHash: strings.Repeat("b", 64), InputHash: strings.Repeat("c", 64),
	}
	for _, media := range facts.Media[:3] {
		subject.Slots = append(subject.Slots, contract.VisionReviewSlot{
			SlotKey: media.Call.SlotKey, ViewRole: media.Call.SlotKey, SHA256: media.SHA256,
			MediaRef: domain.GenerationRevisionRef{ID: media.ID, Revision: media.Revision, ContentHash: media.ContentHash},
		})
	}
	return subject
}

func TestVisionReviewAttachmentsCompileOnlyExactReadyMedia(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	subject := visionAttachmentSubject(t, facts)
	media := slices.Clone(facts.Media[:3])
	attachments, err := contract.BuildVisionReviewAttachments(subject, media)
	if err != nil || len(attachments) != 3 {
		t.Fatalf("compile attachments: %v", err)
	}
	for i, attachment := range attachments {
		if attachment.Slot != subject.Slots[i] || attachment.ProviderReceiptRef != media[i].ReceiptRef || attachment.RightsObservation != "not_assessed" || attachment.PageCount != 1 || attachment.FrameCount != 1 || attachment.ByteLength != media[i].ByteSize {
			t.Fatalf("attachment lost its exact provenance: %+v", attachment)
		}
	}
	slices.Reverse(media)
	repeated, err := contract.BuildVisionReviewAttachments(subject, media)
	if err != nil || !reflect.DeepEqual(repeated, attachments) {
		t.Fatalf("owner read order changed the manifest: %v", err)
	}
	raw, err := json.Marshal(attachments)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"object_key", "object_store_ref", "staging/reference/", "bucket", "path", "selected", "publication_ready"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private location or permission leaked: %s", private)
		}
	}
}

func TestVisionReviewAttachmentsRejectOwnerAndSubjectDrift(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	for _, fault := range []string{"missing", "extra", "mixed_bundle", "duplicate", "pending", "rights", "metadata", "media_hash", "scope", "execution", "bundle", "digest", "revision", "slot"} {
		t.Run(fault, func(t *testing.T) {
			subject := visionAttachmentSubject(t, facts)
			media := slices.Clone(facts.Media[:3])
			switch fault {
			case "missing":
				media = media[:2]
			case "extra":
				media = append(media, facts.Media[3])
			case "mixed_bundle":
				media[0] = facts.Media[3]
			case "duplicate":
				media[1] = media[0]
			case "pending":
				var err error
				media[0], err = domain.InitialReferenceStagedMedia(media[0])
				if err != nil {
					t.Fatal(err)
				}
			case "rights":
				media[0].RightsObservation = "approved"
			case "metadata":
				media[0].Width++
			case "media_hash":
				subject.Slots[0].MediaRef.ContentHash = strings.Repeat("f", 64)
			case "scope":
				subject.ProjectID = uuid.NewString()
			case "execution":
				subject.ExecutionRef.Revision++
			case "bundle":
				subject.CandidateBundleIndex++
			case "digest":
				subject.Slots[0].SHA256 = strings.Repeat("f", 64)
			case "revision":
				subject.Slots[0].MediaRef.Revision++
			case "slot":
				subject.Slots[0].SlotKey = "front"
			}
			if _, err := contract.BuildVisionReviewAttachments(subject, media); err == nil {
				t.Fatal("accepted inconsistent media")
			}
		})
	}
}
