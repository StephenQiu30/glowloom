package generation_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

func referenceBundleFacts(t *testing.T, duplicate bool) domain.ReferenceBundleFacts {
	t.Helper()
	_, template := referenceReceiptFixture(t)
	slots := referenceOutputSlots([]string{"back", "front", "profile"})
	for i := range slots {
		slots[i].MinWidth, slots[i].MinHeight, slots[i].AspectRatio = 32, 32, "1:1"
	}
	output, err := domain.BuildReferenceOutputContract("character_identity_anchor", 2, slots)
	if err != nil {
		t.Fatal(err)
	}
	var inputs []domain.ReferenceProviderCallInput
	for bundle := range 2 {
		for _, slot := range slots {
			inputs = append(inputs, domain.ReferenceProviderCallInput{BundleIndex: bundle, SlotKey: slot.SlotKey, CompiledRequestHash: strings.Repeat("b", 64)})
		}
	}
	job, calls, err := domain.BuildReferenceProviderJob(template.Call.ExecutionRef, inputs)
	if err != nil {
		t.Fatal(err)
	}
	facts := domain.ReferenceBundleFacts{WorkspaceID: template.WorkspaceID, ProjectID: template.ProjectID, TargetRef: domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("c", 64)}, GenerationRound: 1, TargetKind: "character_identity_anchor", DependencyRootHash: strings.Repeat("d", 64), Output: output, Job: job, Calls: calls}
	for i, call := range calls {
		pending, _ := domain.NewReferenceCallState(call.CallKey)
		dispatch := domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: uuid.NewString(), MembershipTokenVersion: 1, DispatchedAt: template.ObservedAt.Add(-time.Second), DeadlineAt: template.ObservedAt.Add(time.Minute)}
		claimed, _, err := domain.ClaimReferenceCall(pending, dispatch)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 32, 32))
		if !duplicate {
			img.SetRGBA(0, 0, color.RGBA{R: uint8(i + 1), A: 255})
		}
		var contents bytes.Buffer
		if err := png.Encode(&contents, img); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(contents.Bytes())
		input := template
		input.Call, input.SubmissionToken, input.Slot = call, dispatch.SubmissionToken, slots[i%3]
		input.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: "staging/reference/" + facts.WorkspaceID + "/" + facts.ProjectID + "/" + call.ExecutionRef.ID + "/" + call.CallKey + "/" + dispatch.SubmissionToken + "/image.png", SHA256: hex.EncodeToString(digest[:]), Bytes: int64(contents.Len()), MediaType: "image/png", Width: 32, Height: 32}
		receipt, err := domain.BuildReferenceCallReceipt(input)
		if err != nil {
			t.Fatal(err)
		}
		state, _, err := domain.RecordReferenceCallReceipt(claimed, receipt)
		if err != nil {
			t.Fatal(err)
		}
		media, err := domain.NewReferenceStagedMedia(receipt, domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse", ObjectKey: input.Output.StagingObjectKey})
		if err != nil {
			t.Fatal(err)
		}
		media, err = domain.CompleteReferenceStagedMedia(media, contents.Bytes(), template.ObservedAt.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		facts.States = append(facts.States, state)
		facts.Media = append(facts.Media, media)
	}
	return facts
}

func TestReferenceBundleInputsFreezeWholeGroupsWithoutRightsApproval(t *testing.T) {
	facts := referenceBundleFacts(t, false)
	result, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil || len(result.Bundles) != 2 {
		t.Fatalf("bundle inputs: %+v %v", result, err)
	}
	for i, bundle := range result.Bundles {
		if bundle.Input.CandidateBundleIndex != i || len(bundle.Input.Slots) != 3 || bundle.Input.BundleCompleteness != "complete" || bundle.BundleQC.Status != "blocked" || bundle.BundleQC.InputRoot != bundle.Input.SlotSetRoot {
			t.Fatalf("unsafe bundle result: %+v", bundle)
		}
		if bundle.Input.BundleQCRef.ContentHash != bundle.BundleQC.ContentHash || len(bundle.SlotQC) != 3 {
			t.Fatal("QC references are not closed")
		}
	}
	slices.Reverse(facts.Calls)
	slices.Reverse(facts.States)
	slices.Reverse(facts.Media)
	repeated, err := domain.BuildReferenceBundleInputs(facts)
	if err != nil || !reflect.DeepEqual(repeated, result) {
		t.Fatal("input order changed content identity")
	}
	raw, _ := json.Marshal(result)
	for _, private := range []string{"staging/reference/", "submission_token", "b64_json", "object_key"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private material leaked")
		}
	}
	decoded, err := domain.DecodeReferenceBundleInputs(raw)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("roundtrip: %v", err)
	}
}

func TestReferenceBundleAdmissionSeparatesInternalReviewFromFormalUse(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		facts := referenceBundleFacts(t, duplicate)
		result, err := domain.BuildReferenceBundleInputs(facts)
		if err != nil {
			t.Fatal(err)
		}
		for _, bundle := range result.Bundles {
			a := bundle.Admission
			if !a.PolicyRef.Valid() || a.InternalReviewReady == duplicate || a.SelectionReady || a.PublicationReady || !slices.Contains(a.FormalUseBlockers, "rights_not_assessed") || !slices.Contains(a.FormalUseBlockers, "vision_review_required") {
				t.Fatalf("unsafe purpose admission: %+v", a)
			}
			if !duplicate && (len(a.InternalReviewBlockers) != 0 || bundle.BundleQC.Status != "blocked") {
				t.Fatal("internal review rewrote rights QC")
			}
			if duplicate && !slices.Contains(a.InternalReviewBlockers, "duplicate_image") {
				t.Fatal("technical failure admitted")
			}
		}
		for _, media := range facts.Media {
			if media.RightsObservation != "not_assessed" {
				t.Fatal("rights observation changed")
			}
		}
		raw, _ := json.Marshal(result)
		if _, err := domain.DecodeReferenceBundleInputs(raw); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReferenceBundleAdmissionRejectsForgedReadinessWithRecomputedHash(t *testing.T) {
	for _, fault := range []string{"selection", "publication", "review_failed_media", "formal_blockers", "review_blockers", "policy"} {
		t.Run(fault, func(t *testing.T) {
			value, err := domain.BuildReferenceBundleInputs(referenceBundleFacts(t, true))
			if err != nil {
				t.Fatal(err)
			}
			a := &value.Bundles[0].Admission
			switch fault {
			case "selection":
				a.SelectionReady = true
			case "publication":
				a.PublicationReady = true
			case "review_failed_media":
				a.InternalReviewReady = true
			case "formal_blockers":
				a.FormalUseBlockers = []string{}
			case "review_blockers":
				a.InternalReviewBlockers = []string{}
			case "policy":
				a.PolicyRef.ContentHash = strings.Repeat("f", 64)
			}
			value.ContentHash = ""
			raw, _ := json.Marshal(value)
			value.ContentHash, err = canonical.Hash(raw)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ = json.Marshal(value)
			if _, err := domain.DecodeReferenceBundleInputs(raw); err == nil {
				t.Fatal("accepted forged readiness with valid collection hash")
			}
		})
	}
}

func TestReferenceBundleAdmissionRequiresExplicitReadinessFields(t *testing.T) {
	value, err := domain.BuildReferenceBundleInputs(referenceBundleFacts(t, true))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"internal_review_ready", "selection_ready", "publication_ready"} {
		for _, mode := range []string{"missing", "null"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				old, replacement := `"`+field+`":false`, `"`+field+`":null`
				if mode == "missing" {
					old += ","
					replacement = ""
				}
				changed := bytes.Replace(raw, []byte(old), []byte(replacement), 1)
				if bytes.Equal(changed, raw) {
					t.Fatal("fault was not injected")
				}
				if _, err := domain.DecodeReferenceBundleInputs(changed); err == nil {
					t.Fatal("accepted omitted or null readiness with original hash")
				}
			})
		}
	}
}

func TestReferenceBundleInputsRejectMissingMixedOrUnresolvedFacts(t *testing.T) {
	for _, mode := range []string{"call_missing", "call_duplicate", "media_missing", "media_duplicate", "unknown", "pending", "scope", "target", "media_hash", "job_hash", "output"} {
		t.Run(mode, func(t *testing.T) {
			facts := referenceBundleFacts(t, false)
			switch mode {
			case "call_missing":
				facts.Calls = facts.Calls[1:]
			case "call_duplicate":
				facts.Calls[1] = facts.Calls[0]
			case "media_missing":
				facts.Media = facts.Media[1:]
			case "media_duplicate":
				facts.Media[1] = facts.Media[0]
			case "unknown":
				pending, _ := domain.NewReferenceCallState(facts.Calls[0].CallKey)
				claimed, _, _ := domain.ClaimReferenceCall(pending, *facts.States[0].Dispatch)
				facts.States[0], _, _ = domain.ExpireReferenceCall(claimed, claimed.Dispatch.DeadlineAt)
			case "pending":
				facts.States[0], _ = domain.NewReferenceCallState(facts.Calls[0].CallKey)
			case "scope":
				facts.ProjectID = uuid.NewString()
			case "target":
				facts.TargetRef.ContentHash = "invalid"
			case "media_hash":
				facts.Media[0].SHA256 = strings.Repeat("f", 64)
			case "job_hash":
				facts.Job.ContentHash = strings.Repeat("f", 64)
			case "output":
				facts.Output.CandidateBundleCount = 1
			}
			if got, err := domain.BuildReferenceBundleInputs(facts); err == nil || !reflect.DeepEqual(got, domain.ReferenceBundleInputCollection{}) {
				t.Fatal("unsafe bundle facts accepted")
			}
		})
	}
	result, err := domain.BuildReferenceBundleInputs(referenceBundleFacts(t, true))
	if err != nil {
		t.Fatal(err)
	}
	for _, bundle := range result.Bundles {
		if bundle.BundleQC.Status != "failed" || !slices.Contains(bundle.BundleQC.Issues, "duplicate_image") {
			t.Fatal("duplicate images passed bundle QC")
		}
	}
}

func TestReferenceBundleInputsRetainExplicitFailureAndRejectContentDrift(t *testing.T) {
	for _, mode := range []string{"explicit_failure", "media_rejected"} {
		t.Run(mode, func(t *testing.T) {
			facts := referenceBundleFacts(t, false)
			if mode == "explicit_failure" {
				state := facts.States[1]
				input := state.Receipt.ReferenceCallReceiptInput
				input.Output = nil
				input.Usage = domain.ProviderUsageObservation{}
				input.Disposition = "output_rejected"
				input.ReasonCode = "invalid_png_contents"
				receipt, err := domain.BuildReferenceCallReceipt(input)
				if err != nil {
					t.Fatal(err)
				}
				pending, _ := domain.NewReferenceCallState(state.CallKey)
				claimed, _, _ := domain.ClaimReferenceCall(pending, *state.Dispatch)
				facts.States[1], _, err = domain.RecordReferenceCallReceipt(claimed, receipt)
				if err != nil {
					t.Fatal(err)
				}
				facts.Media = append(facts.Media[:1], facts.Media[2:]...)
			} else {
				initial, err := domain.InitialReferenceStagedMedia(facts.Media[1])
				if err != nil {
					t.Fatal(err)
				}
				facts.Media[1], err = domain.RejectReferenceStagedMedia(initial, "checksum_mismatch", initial.CreatedAt.Add(time.Second))
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := domain.BuildReferenceBundleInputs(facts)
			if err != nil {
				t.Fatal(err)
			}
			if result.Bundles[0].BundleQC.Status != "failed" || result.Bundles[1].BundleQC.Status != "blocked" {
				t.Fatal("one failed group corrupted independent group outcome")
			}
			if result.Bundles[0].Admission.InternalReviewReady || !result.Bundles[1].Admission.InternalReviewReady || result.Bundles[1].Admission.SelectionReady || result.Bundles[1].Admission.PublicationReady {
				t.Fatal("purpose admission ignored technical failure or approved formal use")
			}
			if mode == "explicit_failure" && result.Bundles[0].Input.BundleCompleteness != "partial_explicit_failure" {
				t.Fatal("explicit failure became complete")
			}
			raw, _ := json.Marshal(result)
			if _, err := domain.DecodeReferenceBundleInputs(raw); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{
				strings.TrimSuffix(string(raw), "}") + `,"vision_candidate_ref":{}}`,
				strings.Replace(string(raw), `"status":"blocked"`, `"status":"passed"`, 1),
				strings.Replace(string(raw), `"candidate_bundle_index":0`, `"candidate_bundle_index":1`, 1),
				strings.Replace(string(raw), `"slot_set_root":"`, `"slot_set_root":"f`, 1),
			} {
				if _, err := domain.DecodeReferenceBundleInputs(json.RawMessage(bad)); err == nil {
					t.Fatal("tampered/cyclic Bundle input accepted")
				}
			}
		})
	}
}
