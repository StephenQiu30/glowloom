package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

// VisionReviewMediaFactsReader revalidates current authorization and recompiles
// the exact input in a standalone transaction, returning only its frozen group.
type VisionReviewMediaFactsReader interface {
	ReadBaseVisionReviewMediaFacts(context.Context, Actor, contract.VisionReviewInput) ([]domain.ReferenceStagedMedia, error)
}

type VisionReviewImage struct {
	Attachment contract.VisionReviewMediaAttachment `json:"attachment"`
	Contents   []byte                               `json:"-"`
}

// VisionReviewMedia owns short-lived buffers, never object locations. The
// consumer must defer Clear; neither this value nor a successful read authorizes
// model dispatch, asset selection, publication, or persistence of these bytes.
type VisionReviewMedia []VisionReviewImage

func (media VisionReviewMedia) Clear() {
	for i := range media {
		clear(media[i].Contents)
		media[i].Contents = nil
	}
}

type VisionReviewMediaReader struct {
	facts    VisionReviewMediaFactsReader
	objects  ReferenceStagedObjectReader
	location domain.ReferenceObjectStoreRef
}

func NewVisionReviewMediaReader(facts VisionReviewMediaFactsReader, objects ReferenceStagedObjectReader, location domain.ReferenceObjectStoreRef) (*VisionReviewMediaReader, error) {
	if facts == nil || objects == nil || !location.Valid() || location.ObjectKey != "" {
		return nil, errors.New("Vision Review requires bound private object and facts readers")
	}
	return &VisionReviewMediaReader{facts: facts, objects: objects, location: location}, nil
}

func (reader *VisionReviewMediaReader) Load(ctx context.Context, actor Actor, input contract.VisionReviewInput) (VisionReviewMedia, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Validate without resealing and detach nested caller-owned input values.
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	input, err = contract.DecodeVisionReviewInput(raw)
	if err != nil {
		return nil, err
	}
	before, err := reader.readFacts(ctx, actor, input)
	if err != nil {
		return nil, err
	}
	media := make(VisionReviewMedia, 0, len(before))
	complete := false
	defer func() {
		if !complete {
			media.Clear()
		}
	}()
	for i, item := range before {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The facts transaction has completed before any object IO. ReadVerified
		// transfers ownership of its bounded buffer, including on an error.
		contents, readErr := reader.objects.ReadVerified(ctx, item.ObjectStoreRef.ObjectKey, item.ByteSize, item.SHA256, item.ByteSize)
		media = append(media, VisionReviewImage{Attachment: input.Attachments[i], Contents: contents})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if readErr != nil {
			return nil, &visionReviewObjectReadError{cause: readErr}
		}
		// Reuse the original complete PNG validation without mutating its Owner
		// or inventing another media policy. An Adapter success is not evidence.
		initial, err := domain.InitialReferenceStagedMedia(item)
		if err != nil {
			return nil, err
		}
		validated, err := domain.CompleteReferenceStagedMedia(initial, contents, *item.ValidatedAt)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(validated, item) {
			return nil, errors.New("Vision Review object differs from validated media")
		}
	}
	after, err := reader.readFacts(ctx, actor, input)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(before, after) {
		return nil, errors.New("Vision Review media changed during object reading")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	complete = true
	return media, nil
}

func (reader *VisionReviewMediaReader) readFacts(ctx context.Context, actor Actor, input contract.VisionReviewInput) ([]domain.ReferenceStagedMedia, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts, err := reader.facts.ReadBaseVisionReviewMediaFacts(ctx, actor, input)
	if err != nil {
		return nil, fmt.Errorf("read current Vision Review facts: %w", err)
	}
	attachments, err := contract.BuildVisionReviewAttachments(input.Subject, facts)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(attachments, input.Attachments) {
		return nil, errors.New("Vision Review media differs from frozen attachments")
	}
	bySlot := make(map[string]domain.ReferenceStagedMedia, len(facts))
	for _, item := range facts {
		if item.ObjectStoreRef.Profile != reader.location.Profile || item.ObjectStoreRef.Bucket != reader.location.Bucket {
			return nil, errors.New("Vision Review media does not belong to the bound private store")
		}
		bySlot[item.Call.SlotKey] = item
	}
	ordered := make([]domain.ReferenceStagedMedia, 0, len(facts))
	for _, slot := range input.Subject.Slots {
		ordered = append(ordered, bySlot[slot.SlotKey])
	}
	// Owner timestamps contain pointers; freeze the entire observation before IO.
	raw, err := json.Marshal(ordered)
	if err != nil {
		return nil, err
	}
	var detached []domain.ReferenceStagedMedia
	if err := json.Unmarshal(raw, &detached); err != nil {
		return nil, err
	}
	return detached, nil
}

// Keep the diagnostic chain available without exposing object keys or SDK URLs
// in the service error text. This error must not be serialized with its cause.
type visionReviewObjectReadError struct{ cause error }

func (err *visionReviewObjectReadError) Error() string {
	return "Vision Review private object is unavailable"
}
func (err *visionReviewObjectReadError) Unwrap() error { return err.cause }
