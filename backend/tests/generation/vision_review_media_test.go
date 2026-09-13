package generation_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type visionMediaFactsStub struct {
	expected   contract.VisionReviewInput
	media      []domain.ReferenceStagedMedia
	reads      int
	beforeRead func(int) error
}

func (f *visionMediaFactsStub) ReadBaseVisionReviewMediaFacts(ctx context.Context, actor app.Actor, input contract.VisionReviewInput) ([]domain.ReferenceStagedMedia, error) {
	f.reads++
	if !reflect.DeepEqual(input, f.expected) {
		return nil, errors.New("input differs from Owner facts")
	}
	if f.beforeRead != nil {
		if err := f.beforeRead(f.reads); err != nil {
			return nil, err
		}
	}
	return slices.Clone(f.media), ctx.Err()
}

type visionMediaObjectsStub struct {
	objects    map[string][]byte
	reads      []string
	returned   [][]byte
	beforeRead func(int) error
	corrupt    bool
}

func (o *visionMediaObjectsStub) ReadVerified(ctx context.Context, key string, size int64, hash string, max int64) ([]byte, error) {
	o.reads = append(o.reads, key)
	data := bytes.Clone(o.objects[key])
	if size != int64(len(data)) || max != size || len(hash) != 64 {
		return nil, errors.New("incorrect bounded read")
	}
	if o.corrupt {
		data[len(data)-1] ^= 1
	}
	o.returned = append(o.returned, data)
	if o.beforeRead != nil {
		if err := o.beforeRead(len(o.reads)); err != nil {
			return data, err
		}
	}
	return data, ctx.Err()
}

func visionMediaFixture(t *testing.T) (contract.VisionReviewInput, *visionMediaFactsStub, *visionMediaObjectsStub) {
	t.Helper()
	raw, err := os.ReadFile("../agent/testdata/vision_review_input.json")
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.DecodeVisionReviewInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	var callInputs []domain.ReferenceProviderCallInput
	for _, slot := range input.Subject.Slots {
		callInputs = append(callInputs, domain.ReferenceProviderCallInput{SlotKey: slot.SlotKey, CompiledRequestHash: strings.Repeat("a", 64)})
	}
	_, calls, err := domain.BuildReferenceProviderJob(input.Subject.ExecutionRef, callInputs)
	if err != nil {
		t.Fatal(err)
	}
	facts, objects := &visionMediaFactsStub{}, &visionMediaObjectsStub{objects: map[string][]byte{}}
	input.Subject.Slots = nil
	for index, call := range calls {
		_, observed := referenceReceiptFixture(t)
		observed.WorkspaceID, observed.ProjectID, observed.Call = input.Subject.WorkspaceID, input.Subject.ProjectID, call
		observed.SubmissionToken = uuid.NewString()
		observed.Slot = referenceOutputSlots([]string{call.SlotKey})[0]
		pixels := image.NewRGBA(image.Rect(0, 0, observed.Slot.MinWidth, observed.Slot.MinHeight))
		pixels.SetRGBA(0, 0, color.RGBA{R: uint8(index + 1), A: 255})
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, pixels); err != nil {
			t.Fatal(err)
		}
		data := buffer.Bytes()
		hash := sha256.Sum256(data)
		key := "staging/reference/" + observed.WorkspaceID + "/" + observed.ProjectID + "/" + call.ExecutionRef.ID + "/" + call.CallKey + "/" + observed.SubmissionToken + "/image.png"
		observed.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: key, SHA256: hex.EncodeToString(hash[:]), MediaType: "image/png", Bytes: int64(len(data)), Width: observed.Slot.MinWidth, Height: observed.Slot.MinHeight}
		receipt, err := domain.BuildReferenceCallReceipt(observed)
		if err != nil {
			t.Fatal(err)
		}
		pending, err := domain.NewReferenceStagedMedia(receipt, domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse", ObjectKey: key})
		if err != nil {
			t.Fatal(err)
		}
		media, err := domain.CompleteReferenceStagedMedia(pending, data, receipt.ObservedAt.Add(time.Second))
		if err != nil || media.State != "ready_for_review" {
			t.Fatalf("ready media: %v", err)
		}
		input.Subject.Slots = append(input.Subject.Slots, contract.VisionReviewSlot{SlotKey: call.SlotKey, ViewRole: call.SlotKey, MediaRef: domain.GenerationRevisionRef{ID: media.ID, Revision: media.Revision, ContentHash: media.ContentHash}, SHA256: media.SHA256})
		facts.media = append(facts.media, media)
		objects.objects[key] = bytes.Clone(data)
	}
	input.Attachments, err = contract.BuildVisionReviewAttachments(input.Subject, facts.media)
	if err != nil {
		t.Fatal(err)
	}
	input, err = contract.BuildVisionReviewInput(input)
	if err != nil {
		t.Fatal(err)
	}
	facts.expected = input
	return input, facts, objects
}

func TestVisionReviewMediaReadsExactWholeGroupAndClearsBuffers(t *testing.T) {
	input, facts, objects := visionMediaFixture(t)
	slices.Reverse(facts.media)
	reader, err := app.NewVisionReviewMediaReader(facts, objects, domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse"})
	if err != nil {
		t.Fatal(err)
	}
	media, err := reader.Load(context.Background(), app.Actor{}, input)
	if err != nil || len(media) != len(input.Attachments) || facts.reads != 2 {
		t.Fatalf("read whole group: %v", err)
	}
	for i, item := range media {
		if item.Attachment != input.Attachments[i] || !bytes.Equal(item.Contents, objects.objects[objects.reads[i]]) {
			t.Fatal("media lost frozen order or contents")
		}
	}
	raw, err := json.Marshal(media)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"Contents", "contents", "object_key", "staging/reference/", "bucket", "iVBOR"} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatalf("JSON exposed private media: %s", private)
		}
	}
	media.Clear()
	media.Clear()
	for _, data := range objects.returned {
		if !bytes.Equal(data, make([]byte, len(data))) {
			t.Fatal("caller release did not clear returned buffers")
		}
	}
	for _, item := range media {
		if item.Contents != nil {
			t.Fatal("cleared group retained buffers")
		}
	}
}

func TestVisionReviewMediaFailsClosedWithoutPartialBytes(t *testing.T) {
	for _, fault := range []string{"input", "before_authorization", "after_authorization", "facts_missing", "facts_drift", "wrong_profile", "wrong_bucket", "object_failure", "checksum", "cancel_before", "cancel_during", "post_read_drift"} {
		t.Run(fault, func(t *testing.T) {
			input, facts, objects := visionMediaFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			location := domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse"}
			denied := errors.New("permission revoked")
			switch fault {
			case "input":
				input.Subject.InputHash = strings.Repeat("f", 64)
			case "before_authorization":
				facts.beforeRead = func(int) error { return denied }
			case "after_authorization":
				facts.beforeRead = func(n int) error {
					if n == 2 {
						return denied
					}
					return nil
				}
			case "facts_missing":
				facts.media = facts.media[:2]
			case "facts_drift":
				facts.media[0].Width++
			case "wrong_profile":
				location.Profile = "other"
			case "wrong_bucket":
				location.Bucket = "other-bucket"
			case "object_failure":
				objects.beforeRead = func(n int) error {
					if n == 2 {
						return errors.New("private location must not leak")
					}
					return nil
				}
			case "checksum":
				objects.corrupt = true
			case "cancel_before":
				cancel()
			case "cancel_during":
				objects.beforeRead = func(n int) error {
					if n == 2 {
						cancel()
					}
					return nil
				}
			case "post_read_drift":
				facts.beforeRead = func(n int) error {
					if n == 2 {
						facts.media[0].ContentHash = strings.Repeat("f", 64)
					}
					return nil
				}
			}
			reader, err := app.NewVisionReviewMediaReader(facts, objects, location)
			if err != nil {
				t.Fatal(err)
			}
			result, err := reader.Load(ctx, app.Actor{}, input)
			if err == nil || result != nil {
				t.Fatalf("accepted %s or returned partial bytes", fault)
			}
			if strings.HasPrefix(fault, "cancel_") && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation error")
			}
			if strings.HasSuffix(fault, "authorization") && !errors.Is(err, denied) {
				t.Fatal("lost authorization error chain")
			}
			if fault == "object_failure" && strings.Contains(err.Error(), "private location") {
				t.Fatal("object error leaked private details")
			}
			if slices.Contains([]string{"input", "before_authorization", "facts_missing", "facts_drift", "wrong_profile", "wrong_bucket", "cancel_before"}, fault) && len(objects.reads) != 0 {
				t.Fatal("performed IO before preflight passed")
			}
			for _, data := range objects.returned {
				if !bytes.Equal(data, make([]byte, len(data))) {
					t.Fatal("failure retained private byte buffers")
				}
			}
		})
	}
}

func TestVisionReviewMediaRequiresExplicitBoundDependencies(t *testing.T) {
	_, facts, objects := visionMediaFixture(t)
	for _, location := range []domain.ReferenceObjectStoreRef{{}, {Profile: "private", Bucket: "lanverse", ObjectKey: "arbitrary"}} {
		if _, err := app.NewVisionReviewMediaReader(facts, objects, location); err == nil {
			t.Fatal("accepted unbound or client-selected location")
		}
	}
	location := domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse"}
	if _, err := app.NewVisionReviewMediaReader(nil, objects, location); err == nil {
		t.Fatal("accepted missing facts reader")
	}
	if _, err := app.NewVisionReviewMediaReader(facts, nil, location); err == nil {
		t.Fatal("accepted missing objects reader")
	}
}

func TestVisionReviewMediaRechecksPNGDespiteReadyMetadataAndMatchingDigest(t *testing.T) {
	for _, fault := range []string{"trailing", "dimensions", "invalid_png"} {
		t.Run(fault, func(t *testing.T) {
			input, facts, objects := visionMediaFixture(t)
			item := &facts.media[0]
			data := bytes.Clone(objects.objects[item.ObjectStoreRef.ObjectKey])
			switch fault {
			case "trailing":
				data = append(data, []byte("unexpected payload")...)
			case "dimensions":
				var buffer bytes.Buffer
				if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, item.Width+1, item.Height))); err != nil {
					t.Fatal(err)
				}
				data = buffer.Bytes()
			case "invalid_png":
				data = bytes.Repeat([]byte("x"), len(data))
			}
			// A validly hashed Owner record is still not evidence of actual pixels.
			hash := sha256.Sum256(data)
			item.SHA256, item.ByteSize, item.ContentHash = hex.EncodeToString(hash[:]), int64(len(data)), ""
			raw, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			item.ContentHash, err = contract.ProductionCanonicalHash(raw)
			if err != nil {
				t.Fatal(err)
			}
			input.Subject.Slots[0].SHA256, input.Subject.Slots[0].MediaRef.ContentHash = item.SHA256, item.ContentHash
			input.Attachments, err = contract.BuildVisionReviewAttachments(input.Subject, facts.media)
			if err != nil {
				t.Fatal(err)
			}
			input, err = contract.BuildVisionReviewInput(input)
			if err != nil {
				t.Fatal(err)
			}
			facts.expected = input
			objects.objects[item.ObjectStoreRef.ObjectKey] = data
			reader, err := app.NewVisionReviewMediaReader(facts, objects, domain.ReferenceObjectStoreRef{Profile: "private", Bucket: "lanverse"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := reader.Load(context.Background(), app.Actor{}, input)
			if err == nil || result != nil || len(objects.reads) != 1 {
				t.Fatal("ready metadata bypassed full PNG validation")
			}
			for _, data := range objects.returned {
				if !bytes.Equal(data, make([]byte, len(data))) {
					t.Fatal("invalid PNG buffer was not cleared")
				}
			}
		})
	}
}
