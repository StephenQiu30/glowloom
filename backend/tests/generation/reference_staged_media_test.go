package generation_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

func referenceStagedFixture(t *testing.T) (domain.ReferenceCallReceipt, []byte, domain.ReferenceObjectStoreRef) {
	t.Helper()
	_, input := referenceReceiptFixture(t)
	var contents bytes.Buffer
	if err := png.Encode(&contents, image.NewRGBA(image.Rect(0, 0, input.Output.Width, input.Output.Height))); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents.Bytes())
	input.Output.SHA256, input.Output.Bytes = hex.EncodeToString(digest[:]), int64(contents.Len())
	receipt, err := domain.BuildReferenceCallReceipt(input)
	if err != nil {
		t.Fatal(err)
	}
	return receipt, contents.Bytes(), domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse", ObjectKey: input.Output.StagingObjectKey}
}

func TestReferenceStagedMediaValidatesBytesAndImmutableIdentity(t *testing.T) {
	receipt, contents, location := referenceStagedFixture(t)
	before, err := domain.NewReferenceStagedMedia(receipt, location)
	if err != nil || before.ID != receipt.SubmissionToken || before.State != "quarantined" || before.Revision != 1 || before.RightsObservation != "not_assessed" {
		t.Fatalf("new staged media: %+v %v", before, err)
	}
	for _, mode := range []string{"ready", "size", "digest", "trailing", "dimensions", "invalid_png"} {
		t.Run(mode, func(t *testing.T) {
			input, data := receipt.ReferenceCallReceiptInput, bytes.Clone(contents)
			output := *input.Output
			input.Output = &output
			switch mode {
			case "size":
				data = data[:len(data)-1]
			case "digest":
				data[len(data)/2] ^= 1
			case "trailing":
				data = append(data, []byte("<script>payload</script>")...)
			case "dimensions":
				var wrong bytes.Buffer
				if err := png.Encode(&wrong, image.NewRGBA(image.Rect(0, 0, output.Width+1, output.Height))); err != nil {
					t.Fatal(err)
				}
				data = wrong.Bytes()
			case "invalid_png":
				data = bytes.Repeat([]byte("x"), len(data))
			}
			if mode == "trailing" || mode == "invalid_png" || mode == "dimensions" {
				hash := sha256.Sum256(data)
				input.Output.SHA256, input.Output.Bytes = hex.EncodeToString(hash[:]), int64(len(data))
			}
			observed, err := domain.BuildReferenceCallReceipt(input)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := domain.NewReferenceStagedMedia(observed, location)
			if err != nil {
				t.Fatal(err)
			}
			after, err := domain.CompleteReferenceStagedMedia(pending, data, receipt.ObservedAt.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			expected := "rejected"
			if mode == "ready" {
				expected = "ready_for_review"
			}
			if after.State != expected || after.Revision != 2 || after.ValidatedAt == nil {
				t.Fatalf("validation: %+v", after)
			}
			if err := domain.ValidateReferenceStagedMediaTransition(pending, after); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(after)
			decoded, err := domain.DecodeReferenceStagedMedia(raw)
			if err != nil || !reflect.DeepEqual(after, decoded) {
				t.Fatalf("roundtrip: %v", err)
			}
			if _, err := domain.CompleteReferenceStagedMedia(after, contents, receipt.ObservedAt.Add(2*time.Second)); err == nil {
				t.Fatal("completed media was revalidated in place")
			}
		})
	}
	for _, mutate := range []func(*domain.ReferenceStagedMedia){
		func(v *domain.ReferenceStagedMedia) { v.SHA256 = strings.Repeat("f", 64) },
		func(v *domain.ReferenceStagedMedia) { v.RightsObservation = "cleared" },
		func(v *domain.ReferenceStagedMedia) { v.ObjectStoreRef.ObjectKey += "/foreign" },
		func(v *domain.ReferenceStagedMedia) { v.ReceiptRef.ContentHash = strings.Repeat("f", 64) },
		func(v *domain.ReferenceStagedMedia) { v.State = "ready_for_review" },
	} {
		changed := before
		mutate(&changed)
		raw, _ := json.Marshal(changed)
		if _, err := domain.DecodeReferenceStagedMedia(raw); err == nil {
			t.Fatal("tampered staged identity accepted")
		}
	}
	location.ObjectKey = "https://provider.example/image.png"
	if _, err := domain.NewReferenceStagedMedia(receipt, location); err == nil {
		t.Fatal("external URL accepted")
	}
	raw, _ := json.Marshal(before)
	for _, field := range []string{`,"unexpected":true}`, `,"state":"quarantined"}`} {
		if _, err := domain.DecodeReferenceStagedMedia(append(bytes.Clone(raw[:len(raw)-1]), []byte(field)...)); err == nil {
			t.Fatal("extra or duplicate field accepted")
		}
	}
}
